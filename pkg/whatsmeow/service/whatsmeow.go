package whatsmeow_service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/webp"
	"google.golang.org/protobuf/proto"

	_ "github.com/lib/pq"
	"github.com/patrickmn/go-cache"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	producer_interfaces "github.com/evolution-foundation/evolution-go/pkg/events/interfaces"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	"github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
	label_model "github.com/evolution-foundation/evolution-go/pkg/label/model"
	label_repository "github.com/evolution-foundation/evolution-go/pkg/label/repository"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
	"github.com/evolution-foundation/evolution-go/pkg/passkey/ceremony"
	poll_service "github.com/evolution-foundation/evolution-go/pkg/poll/service"
	storage_interfaces "github.com/evolution-foundation/evolution-go/pkg/storage/interfaces"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

type WhatsmeowService interface {
	StartClient(clientData *ClientData)
	ConnectOnStartup(clientName string)
	StartInstance(instanceId string) error
	ReconnectClient(instanceId string) error
	ClearInstanceCache(instanceId string, token string) error
	CallWebhook(instance *instance_model.Instance, queueName string, jsonData []byte)
	SendToGlobalQueues(event string, jsonData []byte, userId string)
	ForceUpdateJid(instanceId string, number string) error
	UpdateInstanceSettings(instanceId string) error
	UpdateInstanceAdvancedSettings(instanceId string) error
	GetPollService() poll_service.PollService // NOVO: Acesso ao serviço de polls

	// Passkey (WebAuthn) pairing bridge — read by the public ceremony endpoint,
	// written by the whatsmeow event goroutine.
	PasskeyCeremonyStore() *ceremony.Store
	SubmitPasskeyResponse(instanceId string, resp *types.WebAuthnResponse) error
	ConfirmPasskey(instanceId string) error
}

type clientVersion struct {
	Major int
	Minor int
	Patch int
}

type whatsmeowService struct {
	instanceRepository instance_repository.InstanceRepository
	authDB             *sql.DB
	messageRepository  message_repository.MessageRepository
	labelRepository    label_repository.LabelRepository
	pollService        poll_service.PollService // NOVO: Serviço de enquetes
	config             *config.Config
	killChannel        map[string](chan bool)
	userInfoCache      *cache.Cache
	clientPointer      map[string]*whatsmeow.Client
	myClientPointer    map[string]*MyClient
	rabbitmqProducer   producer_interfaces.Producer
	webhookProducer    producer_interfaces.Producer
	websocketProducer  producer_interfaces.Producer
	sqliteDB           *sql.DB
	exPath             string
	mediaStorage       storage_interfaces.MediaStorage
	processedMessages  *cache.Cache
	natsProducer       producer_interfaces.Producer
	loggerWrapper      *logger_wrapper.LoggerManager
	passkeyCeremony    *ceremony.Store
}

// sqlstore.New abre um *sql.DB novo a cada chamada, e o pool resultante mantém
// conexões ociosas vivas indefinidamente. Como o container era criado a cada
// (re)conexão e nunca fechado, cada reconexão vazava conexões: numa instância de
// produção o pool do Postgres saiu de 2 para 99 conexões ociosas ao longo de
// alguns dias, esgotou max_connections e o cliente passou a falhar já na
// inicialização com "pq: sorry, too many clients already" — deixando de
// reconectar por completo.
//
// O DSN é fixo durante a vida do processo, então um único container por DSN
// basta e é o que whatsmeow espera: ele é seguro para uso concorrente.
var (
	storeContainersMu sync.Mutex
	storeContainers   = map[string]*sqlstore.Container{}
)

// sharedStoreContainer devolve o container já aberto para este DSN, criando-o
// apenas na primeira chamada.
func sharedStoreContainer(dialect, address string, log waLog.Logger) (*sqlstore.Container, error) {
	key := dialect + "|" + address
	storeContainersMu.Lock()
	defer storeContainersMu.Unlock()
	if c, ok := storeContainers[key]; ok {
		return c, nil
	}
	c, err := sqlstore.New(context.Background(), dialect, address, log)
	if err != nil {
		return nil, err
	}
	storeContainers[key] = c
	return c, nil
}

type MyClient struct {
	service            WhatsmeowService
	WAClient           *whatsmeow.Client
	eventHandlerID     uint32
	userID             string
	Instance           *instance_model.Instance
	token              string
	subscriptions      []string
	webhookUrl         string
	rabbitmqEnable     string
	natsEnable         string
	websocketEnable    string
	instanceRepository instance_repository.InstanceRepository
	messageRepository  message_repository.MessageRepository
	labelRepository    label_repository.LabelRepository
	pollService        poll_service.PollService // NOVO: Serviço de enquetes
	clientPointer      map[string]*whatsmeow.Client
	myClientPointer    map[string]*MyClient
	killChannel        map[string](chan bool)
	userInfoCache      *cache.Cache
	config             *config.Config
	historySyncID      int32
	rabbitmqProducer   producer_interfaces.Producer
	webhookProducer    producer_interfaces.Producer
	websocketProducer  producer_interfaces.Producer
	mediaStorage       storage_interfaces.MediaStorage
	processedMessages  *cache.Cache
	natsProducer       producer_interfaces.Producer
	loggerWrapper      *logger_wrapper.LoggerManager
	qrcodeCount        int
	passkeyCeremony    *ceremony.Store
}

func (mycli *MyClient) persistMessageAsync(message message_model.Message) {
	if mycli == nil || mycli.messageRepository == nil {
		return
	}

	go func() {
		if err := mycli.messageRepository.InsertMessage(message); err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to persist message %s: %v", mycli.userID, message.MessageID, err)
		}
	}()
}

type ClientData struct {
	Instance      *instance_model.Instance
	Subscriptions []string
	Phone         string
	IsProxy       bool
}

type Values struct {
	m map[string]string
}

func (v Values) Get(key string) string {
	return v.m[key]
}

type UserCollection struct {
	Users map[types.JID]types.UserInfo
}

type ProxyConfig struct {
	Protocol string `json:"protocol,omitempty"`
	Host     string `json:"host"`
	Password string `json:"password"`
	Port     string `json:"port"`
	Username string `json:"username"`
}

func (w whatsmeowService) ReconnectClient(instanceId string) error {
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Starting reconnection process - simulating restart", instanceId)

	// Passo 1: Limpar conexão existente se houver
	if client, exists := w.clientPointer[instanceId]; exists {
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Disconnecting existing client", instanceId)

		// Desconectar o cliente WebSocket
		if client.IsConnected() {
			client.Disconnect()
			w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] WebSocket disconnected", instanceId)
		}

		// Remover event handler se existir
		if mycli, ok := w.myClientPointer[instanceId]; ok {
			if mycli.eventHandlerID != 0 {
				client.RemoveEventHandler(mycli.eventHandlerID)
				w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Event handler removed", instanceId)
			}
		}
	}

	// Passo 2: Limpar todos os recursos da instância
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Cleaning up resources", instanceId)

	// Enviar sinal de kill se o canal existir
	if killChan, exists := w.killChannel[instanceId]; exists {
		select {
		case killChan <- true:
			w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Kill signal sent", instanceId)
		default:
			// Canal pode estar bloqueado, continua
		}
	}

	// Remover das estruturas
	delete(w.clientPointer, instanceId)
	delete(w.myClientPointer, instanceId)
	delete(w.killChannel, instanceId)

	// Limpar cache de userInfo para esta instância
	if instance, err := w.instanceRepository.GetInstanceByID(instanceId); err == nil {
		w.userInfoCache.Delete(instance.Token)
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] UserInfo cache cleared for token: %s", instanceId, instance.Token)
	}

	// Passo 3: Atualizar status no banco
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return fmt.Errorf("failed to get instance: %v", err)
	}

	instance.Connected = false
	instance.DisconnectReason = "Reconnecting"
	err = w.instanceRepository.UpdateConnected(instanceId, false, "Reconnecting")
	if err != nil {
		w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Failed to update disconnect status: %v", instanceId, err)
	}

	// Passo 4: Aguardar um pouco para garantir limpeza completa
	time.Sleep(2 * time.Second)

	// Passo 5: Iniciar nova instância como se fosse a primeira vez
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Starting fresh instance", instanceId)
	return w.StartInstance(instanceId)
}

func (w whatsmeowService) ForceUpdateJid(instanceId string, number string) error {
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting instance: %v", instanceId, err)
		return err
	}

	if instance.Jid == "" && number != "" {
		sqlDeviceSearch := fmt.Sprintf("SELECT jid FROM whatsmeow_device WHERE jid LIKE '%%%s%%'", number)
		rows, err := w.authDB.Query(sqlDeviceSearch)
		if err != nil {
			w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting device: %v", instanceId, err)
			return err
		}

		defer rows.Close()

		var latestJid string
		var latestSession int

		for rows.Next() {
			type deviceStruct struct {
				Jid string `json:"jid"`
			}
			var device deviceStruct
			err := rows.Scan(&device.Jid)
			if err != nil {
				w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting device: %v", instanceId, err)
				return err
			}

			// Extrair o número da sessão do JID
			parts := strings.Split(device.Jid, ":")
			if len(parts) == 2 {
				sessionPart := strings.Split(parts[1], "@")[0]
				session, err := strconv.Atoi(sessionPart)
				if err != nil {
					w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error parsing session number: %v", instanceId, err)
					return err
				}

				// Atualizar se for a sessão mais recente
				if session > latestSession {
					latestSession = session
					latestJid = device.Jid
				}
			}
		}

		// Atualizar a instância com o JID mais recente
		if latestJid != "" {
			instance.Jid = latestJid
			err = w.instanceRepository.UpdateJid(instanceId, latestJid)
			if err != nil {
				w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error updating instance: %v", instanceId, err)
			}
			w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Updated instance with latest JID: %s (session: %d)", instanceId, latestJid, latestSession)
		}
	}

	return nil
}

func (w whatsmeowService) StartClient(cd *ClientData) {

	w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("Starting websocket connection to Whatsapp for user '%s'", cd.Instance.Id)

	var deviceStore *store.Device
	var err error

	if w.clientPointer[cd.Instance.Id] != nil {
		if w.clientPointer[cd.Instance.Id].IsConnected() {
			return
		}
	}

	var container *sqlstore.Container

	var dbLog waLog.Logger
	if w.config.WaDebug != "" {
		dbLog = waLog.Stdout("Database", w.config.WaDebug, true)
	}
	if w.config.PostgresAuthDB != "" {
		container, err = sharedStoreContainer("postgres", w.config.PostgresAuthDB, dbLog)
	} else {
		dsn := fmt.Sprintf("file:%s/dbdata/main.db?_pragma=foreign_keys(1)&_busy_timeout=5000&cache=shared&mode=rwc&_journal_mode=WAL", w.exPath)
		container, err = sharedStoreContainer("sqlite", dsn, dbLog)
	}

	if err != nil {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to create container: %v", cd.Instance.Id, err)
		return
	}

	if cd.Instance.Jid != "" {
		jid, _ := utils.ParseJID(cd.Instance.Jid)
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Jid found. Getting device store for jid: %s", cd.Instance.Id, jid)
		deviceStore, err = container.GetDevice(context.Background(), jid)
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Erro ao obter device store: %v", cd.Instance.Id, err)
			return
		}
	} else {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] No jid found. Creating new device", cd.Instance.Id)
		deviceStore = container.NewDevice()
	}

	if deviceStore == nil {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] No store found. Creating new one", cd.Instance.Id)
		deviceStore = container.NewDevice()

		cd.Instance.Connected = false
		err := w.instanceRepository.UpdateConnected(cd.Instance.Id, cd.Instance.Connected, cd.Instance.DisconnectReason)
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Error updating instance: %s", cd.Instance.Id, err)
		}
	}

	var version clientVersion

	platformID, ok := waCompanionReg.DeviceProps_PlatformType_value[strings.ToUpper("chrome")]
	if ok {
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_PlatformType(platformID).Enum()
	}
	if cd.Instance.OsName == "" {
		cd.Instance.OsName = utils.WhatsAppGetUserOS()
	}

	store.DeviceProps.Os = &cd.Instance.OsName
	store.DeviceProps.RequireFullSync = proto.Bool(true)

	if w.config.WhatsappVersionMajor != 0 && w.config.WhatsappVersionMinor != 0 && w.config.WhatsappVersionPatch != 0 {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Setting whatsapp version to %d.%d.%d", cd.Instance.Id, w.config.WhatsappVersionMajor, w.config.WhatsappVersionMinor, w.config.WhatsappVersionPatch)
		version.Major = w.config.WhatsappVersionMajor
		if err == nil {
			store.DeviceProps.Version.Primary = proto.Uint32(uint32(version.Major))
		}
		version.Minor = w.config.WhatsappVersionMinor
		if err == nil {
			store.DeviceProps.Version.Secondary = proto.Uint32(uint32(version.Minor))
		}
		version.Patch = w.config.WhatsappVersionPatch
		if err == nil {
			store.DeviceProps.Version.Tertiary = proto.Uint32(uint32(version.Patch))
		}
	} else {
		// Try to fetch version from WhatsApp Web
		webVersion, err := fetchWhatsAppWebVersion()
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to fetch WhatsApp Web version: %v", cd.Instance.Id, err)
		} else {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Setting whatsapp version from web to %d.%d.%d", cd.Instance.Id, webVersion.Major, webVersion.Minor, webVersion.Patch)
			version = *webVersion
			store.DeviceProps.Version.Primary = proto.Uint32(uint32(version.Major))
			store.DeviceProps.Version.Secondary = proto.Uint32(uint32(version.Minor))
			store.DeviceProps.Version.Tertiary = proto.Uint32(uint32(version.Patch))
		}
	}

	// 🔒 FIX: Sempre criar logger, mesmo que WaDebug esteja vazio
	// Usar "INFO" como nível mínimo para garantir que logs importantes apareçam
	minLevel := w.config.WaDebug
	if minLevel == "" {
		minLevel = "INFO" // Nível mínimo para garantir que logs INFO apareçam
	}
	clientLog := waLog.Stdout("Client", minLevel, true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	w.clientPointer[cd.Instance.Id] = client

	if cd.IsProxy {
		var proxyConfig ProxyConfig
		err := json.Unmarshal([]byte(cd.Instance.Proxy), &proxyConfig)
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] error unmarshalling proxy config", cd.Instance.Id)
			return
		}

		proxyProtocol := proxyConfig.Protocol
		proxyHost := proxyConfig.Host
		proxyPort := proxyConfig.Port
		proxyUsername := proxyConfig.Username
		proxyPassword := proxyConfig.Password

		if proxyConfig.Host == "" {
			proxyHost = w.config.ProxyHost
		}

		if proxyConfig.Port == "" {
			proxyPort = w.config.ProxyPort
		}

		if proxyConfig.Protocol == "" {
			proxyProtocol = w.config.ProxyProtocol
		}

		if proxyConfig.Username == "" {
			proxyUsername = w.config.ProxyUsername
		}

		if proxyConfig.Password == "" {
			proxyPassword = w.config.ProxyPassword
		}

		proxyAddress, err := utils.BuildProxyAddress(proxyProtocol, proxyHost, proxyPort, proxyUsername, proxyPassword)
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] Proxy error, continuing without proxy: %v", cd.Instance.Id, err)
		} else {
			err = client.SetProxyAddress(proxyAddress)
			if err != nil {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] Proxy error, continuing without proxy: %v", cd.Instance.Id, err)
			} else {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Proxy enabled (%s)", cd.Instance.Id, utils.NormalizeProxyProtocol(proxyProtocol, proxyPort))
			}
		}
	}

	client.EnableAutoReconnect = false
	client.AutoTrustIdentity = true

	mycli := &MyClient{
		service:            &w,
		Instance:           cd.Instance,
		WAClient:           client,
		eventHandlerID:     1,
		userID:             cd.Instance.Id,
		token:              cd.Instance.Token,
		subscriptions:      cd.Subscriptions,
		webhookUrl:         cd.Instance.Webhook,
		rabbitmqEnable:     cd.Instance.RabbitmqEnable,
		natsEnable:         cd.Instance.NatsEnable,
		websocketEnable:    cd.Instance.WebSocketEnable,
		instanceRepository: w.instanceRepository,
		messageRepository:  w.messageRepository,
		labelRepository:    w.labelRepository,
		pollService:        w.pollService, // NOVO: Serviço de enquetes
		userInfoCache:      w.userInfoCache,
		clientPointer:      w.clientPointer,
		myClientPointer:    w.myClientPointer,
		killChannel:        w.killChannel,
		config:             w.config,
		historySyncID:      0,
		rabbitmqProducer:   w.rabbitmqProducer,
		webhookProducer:    w.webhookProducer,
		websocketProducer:  w.websocketProducer,
		mediaStorage:       w.mediaStorage,
		processedMessages:  w.processedMessages,
		natsProducer:       w.natsProducer,
		loggerWrapper:      w.loggerWrapper,
		qrcodeCount:        0,
		passkeyCeremony:    w.passkeyCeremony,
	}

	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)

	// Armazena o MyClient no map para permitir atualizações posteriores
	w.myClientPointer[cd.Instance.Id] = mycli

	if client.Store.ID != nil {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Already logged in with JID: %s", cd.Instance.Id, client.Store.ID.String())
		err = client.Connect()
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Erro de conexão WebSocket (EOF). Tentando reconectar em 5 segundos...", cd.Instance.Id)
				time.Sleep(5 * time.Second)
				err = client.Connect()
				if err != nil {
					w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Falha na segunda tentativa de conexão: %v", cd.Instance.Id, err)
					return
				}
			} else if strings.Contains(err.Error(), "username/password authentication failed") {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] Proxy authentication failed, attempting to connect without proxy", cd.Instance.Id)

				// Desabilita o proxy
				client.SetProxy(nil)

				// Tenta conectar sem proxy
				err = client.Connect()
				if err != nil {
					w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to connect even without proxy: %v", cd.Instance.Id, err)
					return
				}
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Successfully connected without proxy", cd.Instance.Id)
			} else {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to connect: %v", cd.Instance.Id, err)
				return
			}
		}
	} else {
		// New-device pairing. We intentionally do NOT use client.GetQRChannel:
		// in the installed whatsmeow its qrChannel handler auto-confirms a
		// passkey ceremony (SkipHandoffUX) and Disconnects the socket when the
		// QR codes run out, both of which break passkey pairing (DOC2 §4.3/§4.4).
		// Instead we Connect() directly and consume *events.QR in myEventHandler
		// (see handleQRCodes), which pair.go dispatches to every handler anyway.
		err = client.Connect()
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Erro de conexão WebSocket (EOF). Tentando reconectar em 5 segundos...", cd.Instance.Id)
				time.Sleep(5 * time.Second)
				err = client.Connect()
				if err != nil {
					w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Falha na segunda tentativa de conexão: %v", cd.Instance.Id, err)
					return
				}
			} else if strings.Contains(err.Error(), "username/password authentication failed") {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] Proxy authentication failed during QR connection, attempting without proxy", cd.Instance.Id)

				// Desabilita o proxy
				client.SetProxy(nil)

				// Tenta conectar sem proxy
				err = client.Connect()
				if err != nil {
					w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to connect even without proxy: %v", cd.Instance.Id, err)
					return
				}
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Successfully connected without proxy", cd.Instance.Id)
			} else {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to connect: %v", cd.Instance.Id, err)
				return
			}
		}

	}

	// Removed auto-reconnect logic to prevent infinite loops

	for {
		select {
		case <-w.killChannel[cd.Instance.Id]:
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("Received kill signal for user '%s'", cd.Instance.Id)
			client.Disconnect()

			delete(w.clientPointer, cd.Instance.Id)
			delete(w.myClientPointer, cd.Instance.Id)

			// Limpar cache de userInfo para esta instância
			w.userInfoCache.Delete(cd.Instance.Token)
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] UserInfo cache cleared for token: %s", cd.Instance.Id, cd.Instance.Token)

			cd.Instance.Connected = false

			err := w.instanceRepository.UpdateConnected(cd.Instance.Id, cd.Instance.Connected, cd.Instance.DisconnectReason)
			if err != nil {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Error updating instance: %s", cd.Instance.Id, err)
			}

			postMap := make(map[string]interface{})

			postMap["event"] = "LoggedOut"

			dataMap := make(map[string]interface{})

			dataMap["reason"] = "Logged out"

			postMap["data"] = dataMap

			postMap["instanceToken"] = mycli.token
			postMap["instanceId"] = mycli.userID
			postMap["instanceName"] = cd.Instance.Name

			var queueName string

			if _, ok := postMap["event"]; ok {
				queueName = strings.ToLower(fmt.Sprintf("%s.%s", cd.Instance.Id, postMap["event"]))
			}

			values, err := json.Marshal(postMap)
			if err != nil {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to marshal JSON for queue", cd.Instance.Id)
				return
			}

			go w.CallWebhook(cd.Instance, queueName, values)

			if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
				go mycli.service.SendToGlobalQueues(postMap["event"].(string), values, mycli.userID)
			}

			// restart client
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Restarting client", cd.Instance.Id)
			w.StartClient(cd)
			return
		default:
			time.Sleep(1000 * time.Millisecond)
		}
	}
}

func schedulePresenceUpdates(mycli *MyClient) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Verificar se a instância ainda existe
			_, err := mycli.instanceRepository.GetInstanceByID(mycli.userID)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Instance no longer exists, stopping presence updates", mycli.userID)
				return // Encerra a goroutine se a instância não existir mais
			}

			processPresenceUpdates(mycli)

			ticker.Stop()
			randomInterval := time.Duration(1+rand.Intn(3)) * time.Hour
			ticker = time.NewTicker(randomInterval)

		case <-mycli.killChannel[mycli.userID]:
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Received kill signal, stopping presence updates", mycli.userID)
			return // Encerra a goroutine quando receber sinal de kill
		}
	}
}

func processPresenceUpdates(mycli *MyClient) {
	now := time.Now()
	location, _ := time.LoadLocation("America/Sao_Paulo")
	nowSp := now.In(location)

	if nowSp.Hour() >= 1 && nowSp.Hour() < 24 {
		err := mycli.WAClient.SendPresence(context.Background(), types.PresenceUnavailable)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to set presence as unavailable %v", mycli.userID, err)
		} else {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Marked self as unavailable", mycli.userID)
		}

		time.Sleep(time.Duration(1+rand.Intn(5)) * time.Second)

		err = mycli.WAClient.SendPresence(context.Background(), types.PresenceAvailable)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to set presence as available %v", mycli.userID, err)
		} else {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Marked self as available", mycli.userID)
		}
	}
}

// handleQRCodes forwards a batch of QR codes (events.QR.Codes) to the manager,
// rotating them with whatsmeow's native timing (first code ~60s, the rest ~20s)
// WITHOUT using GetQRChannel. GetQRChannel is deliberately avoided for new-device
// pairing because, in the installed whatsmeow, its qrChannel handler both
// auto-confirms PairPasskeyConfirmation when SkipHandoffUX is set (racing our
// own confirm flow) and Disconnects the socket when codes run out — either of
// which breaks an in-flight passkey ceremony (DOC2 §4.3/§4.4). Consuming
// events.QR here keeps the socket alive for as long as pairing (QR or passkey)
// needs, since events.QR is dispatched to every handler by pair.go regardless.
//
// This preserves the original GetQRChannel-loop behavior byte-for-byte for the
// per-code work (max-count enforcement, PNG encode, DB persist, webhook/queue
// fan-out) and the timeout teardown; only the trigger (batch vs. per-code) and
// the rotation/self-timer are new. Runs in its own goroutine so it never blocks
// the whatsmeow event dispatch.
func (mycli *MyClient) handleQRCodes(codes []string) {
	go func() {
		instanceID := mycli.userID
		for i, code := range codes {
			// A successful pair (Store.ID set) or an in-flight passkey ceremony
			// supersedes QR — stop rotating WITHOUT tearing down. Store.ID stays
			// nil throughout a passkey ceremony (it is only set at PairSuccess),
			// so we must also consult the ceremony store, otherwise a ceremony
			// that outlasts QR rotation would have its socket/client torn down.
			if mycli.WAClient == nil || mycli.WAClient.Store.ID != nil {
				return
			}
			if mycli.passkeyCeremony != nil && mycli.passkeyCeremony.HasActiveByInstance(instanceID) {
				mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] Passkey ceremony in progress — pausing QR rotation, keeping socket alive", instanceID)
				return
			}

			mycli.qrcodeCount++

			if mycli.config.QrcodeMaxCount > 0 {
				mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR code generated #%d (max: %d)", instanceID, mycli.qrcodeCount, mycli.config.QrcodeMaxCount)
			} else {
				mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR code generated #%d (limit disabled)", instanceID, mycli.qrcodeCount)
			}

			// Max-count reached: force logout + teardown + QRTimeout (0 = disabled).
			// But never tear down while a passkey ceremony is in flight.
			if mycli.config.QrcodeMaxCount > 0 && mycli.qrcodeCount >= mycli.config.QrcodeMaxCount {
				if mycli.passkeyCeremony != nil && mycli.passkeyCeremony.HasActiveByInstance(instanceID) {
					mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR max-count reached but passkey ceremony active — not tearing down", instanceID)
					return
				}
				mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] Maximum QR code count reached (%d), forcing logout and QRTimeout", instanceID, mycli.config.QrcodeMaxCount)

				if mycli.WAClient.IsConnected() {
					if err := mycli.WAClient.Logout(context.Background()); err != nil {
						mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] Error during forced logout: %v", instanceID, err)
					}
				}
				mycli.teardownQR(fmt.Sprintf("Maximum QR code count (%d) reached", mycli.config.QrcodeMaxCount), true)
				return
			}

			if mycli.config.LogType != "json" {
				fmt.Println("QR code:\n", code)
			}

			image, _ := qrcode.Encode(code, qrcode.Medium, 256)
			base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
			base64WithCode := base64qrcode + "|" + code

			if err := mycli.instanceRepository.UpdateQrcode(instanceID, base64WithCode); err != nil {
				mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Error updating instance: %s", instanceID, err)
			}

			postMap := map[string]interface{}{
				"event": "QRCode",
				"data": map[string]interface{}{
					"qrcode":   base64qrcode,
					"code":     code,
					"count":    mycli.qrcodeCount,
					"maxCount": mycli.config.QrcodeMaxCount,
				},
				"instanceToken": mycli.token,
				"instanceId":    instanceID,
				"instanceName":  mycli.Instance.Name,
			}
			queueName := strings.ToLower(fmt.Sprintf("%s.%s", instanceID, "QRCode"))
			if values, err := json.Marshal(postMap); err == nil {
				go mycli.service.CallWebhook(mycli.Instance, queueName, values)
				if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
					go mycli.service.SendToGlobalQueues("QRCode", values, instanceID)
				}
			} else {
				mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Failed to marshal JSON for queue", instanceID)
			}

			// Rotation timing: first code lives ~60s, subsequent ~20s (whatsmeow native).
			timeout := 20 * time.Second
			if i == 0 {
				timeout = 60 * time.Second
			}
			time.Sleep(timeout)
		}

		// Ran out of codes without a PairSuccess. Treat as QR timeout (mirrors
		// GetQRChannel's "timeout") — UNLESS a passkey ceremony is in flight, in
		// which case the socket must stay alive for the ceremony to complete.
		if mycli.WAClient != nil && mycli.WAClient.Store.ID == nil {
			if mycli.passkeyCeremony != nil && mycli.passkeyCeremony.HasActiveByInstance(instanceID) {
				mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR codes exhausted but passkey ceremony active — keeping socket alive", instanceID)
				return
			}
			mycli.teardownQR("", false)
		}
	}()
}

// teardownQR clears the QR state and emits a QRTimeout event, then signals the
// kill channel so StartClient's select loop performs the actual disconnect and
// map cleanup. IMPORTANT: this method must NOT delete from the shared
// clientPointer/myClientPointer/killChannel maps itself — those are unsynchronized
// service-wide maps and this runs in the handleQRCodes goroutine; doing the
// delete()s here (concurrent with other instances' goroutines and the whatsmeow
// dispatch) risks a `fatal error: concurrent map writes`. The kill-channel send
// is blocking (like the original GetQRChannel timeout branch) so the signal is
// never dropped and the socket can't be orphaned. Cleanup happens in the
// StartClient goroutine, the single writer of those maps for this instance.
// If reason is non-empty it is included in the QRTimeout payload (max-count path).
func (mycli *MyClient) teardownQR(reason string, forceLogout bool) {
	instanceID := mycli.userID

	if err := mycli.instanceRepository.UpdateQrcode(instanceID, ""); err != nil {
		mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Error updating instance: %s", instanceID, err)
	}

	if reason != "" {
		if err := mycli.instanceRepository.UpdateConnected(instanceID, false, reason); err != nil {
			mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Error updating instance status: %v", instanceID, err)
		}
	}

	data := map[string]interface{}{}
	if reason != "" {
		data["reason"] = reason
		data["qrcount"] = mycli.qrcodeCount
		data["maxCount"] = mycli.config.QrcodeMaxCount
		data["forceLogout"] = forceLogout
	}
	postMap := map[string]interface{}{
		"event":         "QRTimeout",
		"data":          data,
		"instanceToken": mycli.token,
		"instanceId":    instanceID,
		"instanceName":  mycli.Instance.Name,
	}
	queueName := strings.ToLower(fmt.Sprintf("%s.%s", instanceID, "QRTimeout"))
	if values, err := json.Marshal(postMap); err == nil {
		go mycli.service.CallWebhook(mycli.Instance, queueName, values)
		if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
			go mycli.service.SendToGlobalQueues("QRTimeout", values, instanceID)
		}
	}

	// Signal StartClient's select loop to disconnect and clean up the shared
	// maps (it is the single writer for this instance). Blocking send mirrors
	// the original timeout branch so the signal is never dropped.
	mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] QR timeout — signaling kill channel", instanceID)
	if killChan, exists := mycli.killChannel[instanceID]; exists {
		killChan <- true
	}
}

func (mycli *MyClient) myEventHandler(rawEvt interface{}) {
	userID := mycli.userID
	postMap := make(map[string]interface{})
	postMap["data"] = rawEvt
	doWebhook := false

	switch evt := rawEvt.(type) {
	case *events.QR:
		// New-device pairing emits QR codes here (we connect without GetQRChannel
		// so the socket survives a passkey ceremony). Forward + rotate them.
		mycli.handleQRCodes(evt.Codes)
		return
	case *events.AppStateSyncComplete:
		if len(mycli.WAClient.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
			err := mycli.WAClient.SendPresence(context.Background(), types.PresenceUnavailable)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to send unavailable presence %v", mycli.userID, err)
			} else {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Marked self as unavailable", mycli.userID)
			}
		}
	case *events.Connected, *events.PushNameSetting:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] events.Connected to Whatsapp for user '%s'", mycli.userID, mycli.WAClient.Store.PushName)
		if len(mycli.WAClient.Store.PushName) > 0 {
			doWebhook = true
			postMap["event"] = "Connected"

			if postMap["data"] != nil {
				jsonBytes, err := json.Marshal(postMap["data"])
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
					return
				}

				var dataMap map[string]interface{}
				err = json.Unmarshal(jsonBytes, &dataMap)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
					return
				}

				postMap["data"] = dataMap
			} else {
				postMap["data"] = make(map[string]interface{})
			}

			dataMap := postMap["data"].(map[string]interface{})

			dataMap["status"] = "open"
			dataMap["jid"] = mycli.WAClient.Store.ID.String()
			dataMap["pushName"] = mycli.WAClient.Store.PushName

			// jid, ok := utils.ParseJID(mycli.WAClient.Store.ID.ToNonAD().User)
			// if ok {
			// 	profilePicUrl, err := mycli.clientPointer[mycli.userID].GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{
			// 		Preview: false,
			// 	})
			// 	if err != nil {
			// 		w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to get profile picture info: %v", mycli.userID, err)
			// 	} else {
			// 		dataMap["profilePicUrl"] = profilePicUrl.URL
			// 	}
			// }

			postMap["data"] = dataMap

			// Respect the alwaysOnline instance flag. Previously the device was marked
			// online unconditionally on every connect (and the periodic presence job was
			// started), which kept the linked device permanently "available". WhatsApp then
			// delivers messages to that active session and suppresses push notifications on
			// the user's phone. When alwaysOnline is false we now send Unavailable instead.
			var err error
			if mycli.Instance.AlwaysOnline {
				go schedulePresenceUpdates(mycli)

				err = mycli.WAClient.SendPresence(context.Background(), types.PresenceAvailable)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to send available presence %v", mycli.userID, err)
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Marked self as available", mycli.userID)
				}
			} else {
				err = mycli.WAClient.SendPresence(context.Background(), types.PresenceUnavailable)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to send unavailable presence %v", mycli.userID, err)
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Marked self as unavailable (alwaysOnline=false)", mycli.userID)
				}
			}

			mycli.Instance.Connected = true
			mycli.Instance.DisconnectReason = ""
			err = mycli.instanceRepository.UpdateConnected(mycli.Instance.Id, mycli.Instance.Connected, mycli.Instance.DisconnectReason)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
			}

			err = mycli.instanceRepository.UpdateQrcode(mycli.Instance.Id, "")
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
			}
		}
	case *events.PairSuccess:
		doWebhook = true
		postMap["event"] = "PairSuccess"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("QR Pair Success for user '%s' with JID '%s' - '%s'", mycli.userID, evt.ID.String(), mycli.WAClient.Store.ID.String())

		instance, err := mycli.instanceRepository.GetInstanceByID(mycli.userID)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error getting instance: %s", mycli.userID, err)
		}

		instance.Qrcode = ""
		instance.Connected = true
		instance.DisconnectReason = ""
		instance.Jid = mycli.WAClient.Store.ID.String()

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Updating JID: %s in Instance: %s", mycli.userID, mycli.WAClient.Store.ID.String(), instance.Jid)

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Attempting to update instance in DB: %+v", mycli.userID, instance)
		err = mycli.instanceRepository.Update(instance)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.userID, err)
		} else {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Instance successfully updated", mycli.userID)
		}

		myUserInfo, found := mycli.userInfoCache.Get(mycli.token)

		if !found {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] No user info cached on pairing?", mycli.userID)
		} else {
			txtid := myUserInfo.(Values).Get("Id")
			token := myUserInfo.(Values).Get("Token")

			updatedUserInfo := utils.UpdateUserInfo(myUserInfo, "Jid", evt.ID.String())

			mycli.userInfoCache.Set(token, updatedUserInfo, cache.NoExpiration)
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User information set for user '%s'", mycli.userID, txtid)
		}

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = dataMap
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap := postMap["data"].(map[string]interface{})

		dataMap["status"] = "open"
		dataMap["jid"] = mycli.WAClient.Store.ID.String()

		if mycli.WAClient.Store.PushName != "" {
			dataMap["pushName"] = mycli.WAClient.Store.PushName
		}

		postMap["data"] = dataMap

		// Pairing succeeded — tear down any pending passkey ceremony for this instance.
		mycli.passkeyCeremony.Clear(mycli.userID)
	case *events.PairPasskeyRequest:
		// The server demands a WebAuthn passkey to finish linking. We CANNOT
		// produce the assertion here (it needs the user's authenticator on the
		// web.whatsapp.com origin) — we only forward the challenge. The browser
		// extension (tools/passkey-helper) runs navigator.credentials.get() and
		// POSTs the assertion back to /passkey-ceremony/{token}/response, which
		// is where SendPasskeyResponse is actually called.
		doWebhook = true
		postMap["event"] = "PasskeyRequest"

		pkJSON, err := json.Marshal(evt.PublicKey)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal passkey publicKey: %v", mycli.userID, err)
			mycli.passkeyCeremony.SetError(mycli.userID, "failed to encode passkey challenge")
			return
		}

		token := mycli.passkeyCeremony.Start(mycli.userID, pkJSON)

		// Build the #wapk payload the extension consumes: base64url({t,b}).
		// `b` must be the PUBLICLY reachable API base the browser can hit
		// (a tunnel / LAN IP in dev) — set PASSKEY_PUBLIC_URL to that base.
		publicBase := os.Getenv("PASSKEY_PUBLIC_URL")
		if publicBase == "" {
			publicBase = "<SET_PASSKEY_PUBLIC_URL>"
		}
		payload := fmt.Sprintf(`{"t":%q,"b":%q}`, token, publicBase)
		wapk := base64.RawURLEncoding.EncodeToString([]byte(payload))
		openURL := "https://web.whatsapp.com/#wapk=" + wapk

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo(
			"[%s] Passkey required. Open this URL in a browser with the Evolution Passkey Helper extension:\n%s\n(ceremony token=%s, base=%s)",
			mycli.userID, openURL, token, publicBase,
		)

		// Surface the ceremony info to webhooks/queues so the manager UI can
		// render the "Abrir WhatsApp Web" button.
		postMap["data"] = map[string]interface{}{
			"ceremonyToken": token,
			"openUrl":       openURL,
			"stage":         "challenge",
		}
	case *events.PairPasskeyConfirmation:
		// The server returned a confirmation code. Per DOC2 §4.2 we NEVER
		// auto-confirm on SkipHandoffUX — we always force skipHandoffUX=false so
		// the extension shows the manual "Confirmar" button, and the actual
		// SendPasskeyConfirmation happens from /passkey-ceremony/{token}/confirm.
		doWebhook = true
		postMap["event"] = "PasskeyConfirmation"
		mycli.passkeyCeremony.SetConfirmation(mycli.userID, evt.Code, false)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo(
			"[%s] Passkey confirmation code=%s (skipHandoffUX from server=%v, forced to manual)",
			mycli.userID, evt.Code, evt.SkipHandoffUX,
		)
		postMap["data"] = map[string]interface{}{
			"code":  evt.Code,
			"stage": "confirmation",
		}
	case *events.PairPasskeyError:
		doWebhook = true
		postMap["event"] = "PasskeyError"
		msg := "unknown passkey error"
		if evt.Error != nil {
			msg = evt.Error.Error()
		}
		mycli.passkeyCeremony.SetError(mycli.userID, msg)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError(
			"[%s] Passkey pairing error (continuation=%v): %s", mycli.userID, evt.Continuation, msg,
		)
		postMap["data"] = map[string]interface{}{
			"error": msg,
			"stage": "error",
		}
	case *events.StreamReplaced:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Received StreamReplaced event", mycli.userID)
		return
	case *events.TemporaryBan:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User received temporary ban for %s", mycli.userID, evt.Code.String())
		doWebhook = true
		postMap["event"] = "TemporaryBan"

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = dataMap
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap := postMap["data"].(map[string]interface{})

		dataMap["reason"] = evt.Code.String()
		dataMap["expire"] = evt.Expire

		postMap["data"] = dataMap
	case *events.Message:
		doWebhook = true
		postMap["event"] = "Message"
		// Message received

		// Log message arrival with detailed info
		messageSize := "unknown"
		if evt.Message.GetDocumentMessage() != nil && evt.Message.GetDocumentMessage().FileLength != nil {
			messageSize = fmt.Sprintf("%d bytes", *evt.Message.GetDocumentMessage().FileLength)
		} else if evt.Message.GetVideoMessage() != nil && evt.Message.GetVideoMessage().FileLength != nil {
			messageSize = fmt.Sprintf("%d bytes", *evt.Message.GetVideoMessage().FileLength)
		} else if evt.Message.GetImageMessage() != nil && evt.Message.GetImageMessage().FileLength != nil {
			messageSize = fmt.Sprintf("%d bytes", *evt.Message.GetImageMessage().FileLength)
		} else if evt.Message.GetAudioMessage() != nil && evt.Message.GetAudioMessage().FileLength != nil {
			messageSize = fmt.Sprintf("%d bytes", *evt.Message.GetAudioMessage().FileLength)
		}

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== MESSAGE RECEIVED ===== ID: %s, From: %s, Type: %s, Size: %s", mycli.userID, evt.Info.ID, evt.Info.Chat.String(), evt.Info.Type, messageSize)

		// se readMessages for true ele marca como lida
		if mycli.Instance.ReadMessages {
			messageIDs := []string{evt.Info.ID}
			err := mycli.WAClient.MarkRead(context.Background(), messageIDs, time.Now(), evt.Info.Sender, evt.Info.Sender)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to auto-mark message as read: %v", mycli.userID, err)
			} else {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Auto-marked message as read from %s", mycli.userID, evt.Info.Chat.String())
			}
		}

		// se ignoreStatus for true e o chat for broadcast ou o id for broadcast retorna
		if mycli.Instance.IgnoreStatus && (strings.Contains(evt.Info.Chat.String(), "@broadcast") || strings.Contains(evt.Info.ID, "@broadcast")) {
			return
		}

		// se ignoreGroup for true e o chat for grupo retorna
		if mycli.Instance.IgnoreGroups && strings.Contains(evt.Info.Chat.String(), "@g.us") {
			return
		}

		// Verifica advanced settings para ignorar grupos
		if (mycli.config.EventIgnoreGroup || mycli.Instance.IgnoreGroups) && strings.Contains(evt.Info.Chat.String(), "@g.us") {
			return
		}

		// Verifica advanced settings para ignorar status/broadcast
		if (mycli.config.EventIgnoreStatus || mycli.Instance.IgnoreStatus) && (strings.Contains(evt.Info.Chat.String(), "@broadcast") || strings.Contains(evt.Info.ID, "@broadcast")) {
			return
		}

		// Trata o caso especial onde Sender é @lid e SenderAlt é @s.whatsapp.net
		// Neste caso, devemos inverter: Sender e Chat devem ser @s.whatsapp.net, SenderAlt deve ser @lid
		senderStr := evt.Info.Sender.String()
		senderAltStr := evt.Info.SenderAlt.String()
		chatStr := evt.Info.Chat.String()

		if strings.Contains(senderStr, "@lid") && strings.Contains(senderAltStr, "@s.whatsapp.net") {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Detected LID/WhatsApp JID swap case - Sender: %s, SenderAlt: %s", mycli.userID, senderStr, senderAltStr)

			// Limpa os IDs antes de fazer a troca
			cleanSenderAlt := cleanSenderID(senderAltStr)
			cleanSender := cleanSenderID(senderStr)

			// Inverte: Sender e Chat recebem o @s.whatsapp.net, SenderAlt recebe o @lid
			if cleanedWhatsAppJID, err := types.ParseJID(cleanSenderAlt); err == nil {
				evt.Info.Sender = cleanedWhatsAppJID
				// Se Chat também é @lid, atualiza para @s.whatsapp.net
				if strings.Contains(chatStr, "@lid") {
					evt.Info.Chat = cleanedWhatsAppJID
				}
			}

			if cleanedLID, err := types.ParseJID(cleanSender); err == nil {
				evt.Info.SenderAlt = cleanedLID
			}

			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] JID swap completed - New Sender: %s, New SenderAlt: %s, New Chat: %s",
				mycli.userID, evt.Info.Sender.String(), evt.Info.SenderAlt.String(), evt.Info.Chat.String())
		} else {
			// Comportamento normal: apenas limpa os IDs
			cleanSender := cleanSenderID(senderStr)
			if cleanedJID, err := types.ParseJID(cleanSender); err == nil {
				evt.Info.Sender = cleanedJID
			}

			cleanSenderAlt := cleanSenderID(senderAltStr)
			if cleanedLID, err := types.ParseJID(cleanSenderAlt); err == nil {
				evt.Info.SenderAlt = cleanedLID
			}
		}

		// Auto-marca mensagens como lidas se configurado
		if mycli.Instance.ReadMessages && !evt.Info.IsFromMe {
			go func() {
				time.Sleep(1 * time.Second) // Pequeno delay para parecer mais natural
				err := mycli.WAClient.MarkRead(context.Background(), []types.MessageID{evt.Info.ID}, evt.Info.Timestamp, evt.Info.Chat, evt.Info.Sender)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to auto-mark message as read: %v", mycli.userID, err)
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Auto-marked message as read from %s", mycli.userID, evt.Info.Chat.String())
				}
			}()
		}

		parsedMessageType := utils.GetMessageType(evt.Message)
		if parsedMessageType == "ignore" || strings.HasPrefix(parsedMessageType, "unknown_protocol_") {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message ignored because it's a unknown protocol message", mycli.userID)
			return
		}

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = dataMap
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap, ok := postMap["data"].(map[string]interface{})
		if !ok {
			dataMap = make(map[string]interface{})
		}

		referral := extractReferralFromMessage(evt.Message)

		if evt.Message.GetPollUpdateMessage() != nil {
			fmt.Printf("[POLL DEBUG] 🎯 PollUpdateMessage detected!\n")
			fmt.Printf("[POLL DEBUG] � BEFORE accessing evt.Info - Sender: %s, Server: %s\n", evt.Info.Sender.String(), evt.Info.Sender.Server)
			fmt.Printf("[POLL DEBUG] 📍 BEFORE accessing evt.Info - SenderAlt: %s\n", evt.Info.SenderAlt.String())
			fmt.Printf("[POLL DEBUG] �� mycli.WAClient is nil: %v\n", mycli.WAClient == nil)
			if mycli.WAClient != nil {
				fmt.Printf("[POLL DEBUG] ✅ mycli.WAClient is initialized: %s\n", mycli.WAClient.Store.ID)
			}

			decrypted, err := mycli.clientPointer[mycli.userID].DecryptPollVote(context.Background(), evt)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to decrypt vote: %v", mycli.userID, err)
			} else {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Selected options in decrypted vote:", mycli.userID)
				for _, option := range decrypted.SelectedOptions {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("- %X", option)

				}

				// NOVO: Salvar voto no banco de dados de forma NÃO-INVASIVA
				if mycli.pollService != nil {
					go func() {
						defer func() {
							if r := recover(); r != nil {
								mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Panic ao salvar voto: %v", mycli.userID, r)
							}
						}()

						pollKey := evt.Message.GetPollUpdateMessage().GetPollCreationMessageKey()
						if pollKey == nil {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] PollCreationMessageKey not found", mycli.userID)
							return
						}

						pollInfo := &types.MessageInfo{
							ID: pollKey.GetID(),
							MessageSource: types.MessageSource{
								Chat: evt.Info.Chat, // Usar o chat do evento atual
							},
						}

						// Construir modelo de voto usando helper seguro
						// evt.Info já passou pelo JID swap, então Sender = número real
						pollVote := poll_service.BuildPollVoteFromEvent(
							pollInfo,
							&evt.Info,
							decrypted,
							"", // CompanyID não disponível no MyClient, será vazio
							mycli.Instance.Id,
						)

						// Salvar no banco com timeout de segurança
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()

						if err := mycli.pollService.SavePollVote(ctx, pollVote); err != nil {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to save poll vote to database: %v", mycli.userID, err)
						} else {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Poll vote saved to database successfully", mycli.userID)
						}
					}()
				}
			}
		}

		var quotedMessage *waE2E.Message
		var stanzaID string

		if evt.Message.GetExtendedTextMessage() != nil {
			quotedMessage = evt.Message.GetExtendedTextMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetExtendedTextMessage().GetContextInfo().GetStanzaID()
		} else if evt.Message.GetImageMessage() != nil {
			quotedMessage = evt.Message.GetImageMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetImageMessage().GetContextInfo().GetStanzaID()
		} else if evt.Message.GetAudioMessage() != nil {
			quotedMessage = evt.Message.GetAudioMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetAudioMessage().GetContextInfo().GetStanzaID()
		} else if evt.Message.GetDocumentMessage() != nil {
			quotedMessage = evt.Message.GetDocumentMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetDocumentMessage().GetContextInfo().GetStanzaID()
		} else if evt.Message.GetVideoMessage() != nil {
			quotedMessage = evt.Message.GetVideoMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetVideoMessage().GetContextInfo().GetStanzaID()
		}

		if stanzaID != "" && quotedMessage != nil {
			quotedMap := make(map[string]interface{})

			quotedMap["stanzaID"] = stanzaID
			quotedMap["quotedMessage"] = quotedMessage

			dataMap["quoted"] = quotedMap
			dataMap["isQuoted"] = true
		}

		if len(referral) > 0 {
			dataMap["referral"] = referral
		}

		if mycli.config.WebhookFiles {
			isMedia := false

			img := evt.Message.GetImageMessage()
			audio := evt.Message.GetAudioMessage()
			document := evt.Message.GetDocumentMessage()
			video := evt.Message.GetVideoMessage()
			sticker := evt.Message.GetStickerMessage()

			// Check for associated child messages (like media in replies)
			var associatedImg *waE2E.ImageMessage
			var associatedAudio *waE2E.AudioMessage
			var associatedDocument *waE2E.DocumentMessage
			var associatedVideo *waE2E.VideoMessage
			var associatedSticker *waE2E.StickerMessage

			if evt.Message.GetAssociatedChildMessage() != nil {
				childMsg := evt.Message.GetAssociatedChildMessage().GetMessage()
				if childMsg != nil {
					associatedImg = childMsg.GetImageMessage()
					associatedAudio = childMsg.GetAudioMessage()
					associatedDocument = childMsg.GetDocumentMessage()
					associatedVideo = childMsg.GetVideoMessage()
					associatedSticker = childMsg.GetStickerMessage()
				}
			}

			if img != nil || audio != nil || document != nil || video != nil || sticker != nil ||
				associatedImg != nil || associatedAudio != nil || associatedDocument != nil ||
				associatedVideo != nil || associatedSticker != nil {
				isMedia = true
			}

			if isMedia {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing media message - ID: %s", mycli.userID, evt.Info.ID)

				var data []byte
				var err error
				var extension string
				var mimeType string
				var mediaSize int64

				// Create context with timeout for large files
				downloadCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()

				downloadStart := time.Now()

				// Handle regular media messages
				if img != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading image - ID: %s", mycli.userID, evt.Info.ID)
					data, err = mycli.WAClient.Download(downloadCtx, img)
					extension = ".jpg"
					mimeType = "image/jpeg"
					if img.FileLength != nil {
						mediaSize = int64(*img.FileLength)
					}
				} else if audio != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading audio - ID: %s", mycli.userID, evt.Info.ID)
					data, err = mycli.WAClient.Download(downloadCtx, audio)
					extension = ".ogg"
					mimeType = "audio/ogg"
					if audio.FileLength != nil {
						mediaSize = int64(*audio.FileLength)
					}
				} else if document != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading document - ID: %s, FileName: %s, Size: %d bytes", mycli.userID, evt.Info.ID, document.GetFileName(), document.GetFileLength())
					data, err = mycli.WAClient.Download(downloadCtx, document)
					extension = getExtensionFromMimeType(document.GetMimetype())
					mimeType = document.GetMimetype()
					if document.FileLength != nil {
						mediaSize = int64(*document.FileLength)
					}
				} else if video != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading video - ID: %s, Size: %d bytes", mycli.userID, evt.Info.ID, video.GetFileLength())
					data, err = mycli.WAClient.Download(downloadCtx, video)
					extension = ".mp4"
					mimeType = "video/mp4"
					if video.FileLength != nil {
						mediaSize = int64(*video.FileLength)
					}
				} else if sticker != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading sticker - ID: %s", mycli.userID, evt.Info.ID)
					data, err = mycli.WAClient.Download(downloadCtx, sticker)
					extension = ".png"
					mimeType = "image/png"
					if sticker.FileLength != nil {
						mediaSize = int64(*sticker.FileLength)
					}

					if err == nil {
						webpReader := bytes.NewReader(data)
						img, decErr := webp.Decode(webpReader)
						if decErr != nil {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to decode webp sticker, keeping raw webp: %v", mycli.userID, decErr)
							extension = ".webp"
							mimeType = "image/webp"
						} else {
							var pngBuffer bytes.Buffer
							if encErr := png.Encode(&pngBuffer, img); encErr != nil {
								mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to encode png from sticker, keeping raw webp: %v", mycli.userID, encErr)
								extension = ".webp"
								mimeType = "image/webp"
							} else {
								data = pngBuffer.Bytes()
							}
						}
					}
					// Handle associated child media messages
				} else if associatedImg != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedImg)
					extension = ".jpg"
					mimeType = "image/jpeg"
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child image message", mycli.userID)
				} else if associatedAudio != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedAudio)
					extension = ".ogg"
					mimeType = "audio/ogg"
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child audio message", mycli.userID)
				} else if associatedDocument != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedDocument)
					extension = getExtensionFromMimeType(associatedDocument.GetMimetype())
					mimeType = associatedDocument.GetMimetype()
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child document message", mycli.userID)
				} else if associatedVideo != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedVideo)
					extension = ".mp4"
					mimeType = "video/mp4"
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child video message", mycli.userID)
				} else if associatedSticker != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedSticker)
					extension = ".png"
					mimeType = "image/png"
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child sticker message", mycli.userID)

					if err == nil {
						webpReader := bytes.NewReader(data)
						img, decErr := webp.Decode(webpReader)
						if decErr != nil {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to decode webp sticker, keeping raw webp: %v", mycli.userID, decErr)
							extension = ".webp"
							mimeType = "image/webp"
						} else {
							var pngBuffer bytes.Buffer
							if encErr := png.Encode(&pngBuffer, img); encErr != nil {
								mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to encode png from associated sticker, keeping raw webp: %v", mycli.userID, encErr)
								extension = ".webp"
								mimeType = "image/webp"
							} else {
								data = pngBuffer.Bytes()
							}
						}
					}
				}

				downloadDuration := time.Since(downloadStart)

				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to download media - ID: %s, Size: %d bytes, Duration: %v, Error: %v", mycli.userID, evt.Info.ID, mediaSize, downloadDuration, err)

					// Check if it's a timeout error
					if downloadCtx.Err() == context.DeadlineExceeded {
						mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Download timeout exceeded (5 minutes) for large file - ID: %s, Size: %d bytes", mycli.userID, evt.Info.ID, mediaSize)
					}

					// Don't return here - continue processing the message without media
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Continuing message processing without media download - ID: %s", mycli.userID, evt.Info.ID)
				} else {
					actualSize := len(data)
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Media download successful - ID: %s, Expected: %d bytes, Actual: %d bytes, Duration: %v", mycli.userID, evt.Info.ID, mediaSize, actualSize, downloadDuration)

					// Check for size mismatch
					if mediaSize > 0 && int64(actualSize) != mediaSize {
						mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Size mismatch detected - ID: %s, Expected: %d, Got: %d", mycli.userID, evt.Info.ID, mediaSize, actualSize)
					}

					// Log large file processing
					if actualSize > 13*1024*1024 { // 13MB
						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing large file (>13MB) - ID: %s, Size: %d bytes", mycli.userID, evt.Info.ID, actualSize)
					}
				}

				messageMap, ok := dataMap["Message"].(map[string]interface{})
				if !ok {
					messageMap = make(map[string]interface{})
				}

				// Only process storage if download was successful
				if err == nil && len(data) > 0 {
					if mycli.config.MinioEnabled {
						fileName := evt.Info.ID + extension
						storageStart := time.Now()

						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Uploading to S3/Minio - ID: %s, FileName: %s, Size: %d bytes", mycli.userID, evt.Info.ID, fileName, len(data))

						mediaURL, err := mycli.mediaStorage.Store(context.Background(), data, fileName, mimeType)
						storageDuration := time.Since(storageStart)

						if err != nil {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to store media in S3/Minio - ID: %s, Size: %d bytes, Duration: %v, Error: %v", mycli.userID, evt.Info.ID, len(data), storageDuration, err)

							// Continue processing without storage URL
							mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Continuing message processing without S3 URL - ID: %s", mycli.userID, evt.Info.ID)
						} else {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] S3/Minio upload successful - ID: %s, Size: %d bytes, Duration: %v, URL: %s", mycli.userID, evt.Info.ID, len(data), storageDuration, mediaURL)
							messageMap["mediaUrl"] = mediaURL
							messageMap["mimetype"] = mimeType
						}
					} else {
						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Encoding to base64 - ID: %s, Size: %d bytes", mycli.userID, evt.Info.ID, len(data))
						encodeStart := time.Now()

						encodeData := base64.StdEncoding.EncodeToString(data)
						encodeDuration := time.Since(encodeStart)

						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Base64 encoding completed - ID: %s, Original: %d bytes, Encoded: %d chars, Duration: %v", mycli.userID, evt.Info.ID, len(data), len(encodeData), encodeDuration)
						messageMap["base64"] = encodeData
					}
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Skipping media storage due to download failure - ID: %s", mycli.userID, evt.Info.ID)
				}

				dataMap["Message"] = messageMap
			}
		}

		isGroup := strings.HasSuffix(evt.Info.Chat.String(), "@g.us")
		if isGroup {
			groupData, err := mycli.WAClient.GetGroupInfo(context.Background(), evt.Info.Chat)
			if err == nil {
				dataMap["groupData"] = groupData
			}
		}

		delete(dataMap, "RawMessage")

		if message, ok := dataMap["Message"].(map[string]interface{}); ok {
			if imageMessage, ok := message["imageMessage"].(map[string]interface{}); ok {
				delete(imageMessage, "JPEGThumbnail")
				message["imageMessage"] = imageMessage
				dataMap["Message"] = message
			}

			if videoMessage, ok := message["videoMessage"].(map[string]interface{}); ok {
				delete(videoMessage, "JPEGThumbnail")
				message["videoMessage"] = videoMessage
				dataMap["Message"] = message
			}

			if documentMessage, ok := message["documentMessage"].(map[string]interface{}); ok {
				delete(documentMessage, "JPEGThumbnail")
				message["documentMessage"] = documentMessage
				dataMap["Message"] = message
			}
		}

		postMap["data"] = dataMap

		if mycli.config.DatabaseSaveMessages {
			message := message_model.Message{
				MessageID: evt.Info.ID,
				Timestamp: evt.Info.Timestamp.Format("2006-01-02 15:04:05"),
				Status:    "Received",
				Source:    evt.Info.Chat.ToNonAD().User,
				Referral:  referral,
			}

			mycli.persistMessageAsync(message)
		}

		// ===== BUTTON CLICK EVENT DETECTION =====
		// Detecta cliques em botões e emite evento separado "ButtonClick"
		// Suporta 3 formatos: ButtonsResponseMessage, InteractiveResponseMessage (NativeFlow), TemplateButtonReplyMessage
		var buttonClickData map[string]interface{}

		if resp := evt.Message.GetButtonsResponseMessage(); resp != nil {
			// Legacy buttons response
			buttonClickData = map[string]interface{}{
				"buttonId":   resp.GetSelectedButtonID(),
				"buttonText": resp.GetSelectedDisplayText(),
				"type":       "buttons_response",
			}
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Button click detected (legacy): buttonId=%s, buttonText=%s", mycli.userID, resp.GetSelectedButtonID(), resp.GetSelectedDisplayText())
		} else if resp := evt.Message.GetInteractiveResponseMessage(); resp != nil {
			// NativeFlow interactive response (quick_reply, cta_url, cta_call, cta_copy)
			if nf := resp.GetNativeFlowResponseMessage(); nf != nil {
				buttonId := ""
				buttonText := ""
				// Parse paramsJSON to extract id and display_text
				if nf.GetParamsJSON() != "" {
					var params map[string]interface{}
					if err := json.Unmarshal([]byte(nf.GetParamsJSON()), &params); err == nil {
						if id, ok := params["id"].(string); ok {
							buttonId = id
						}
						if dt, ok := params["display_text"].(string); ok {
							buttonText = dt
						}
					}
				}
				buttonClickData = map[string]interface{}{
					"buttonId":   buttonId,
					"buttonText": buttonText,
					"type":       "native_flow_response",
					"name":       nf.GetName(),
					"paramsJSON": nf.GetParamsJSON(),
				}
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Button click detected (native_flow): name=%s, buttonId=%s, buttonText=%s", mycli.userID, nf.GetName(), buttonId, buttonText)
			}
		} else if resp := evt.Message.GetTemplateButtonReplyMessage(); resp != nil {
			// Template button reply
			buttonClickData = map[string]interface{}{
				"buttonId":   resp.GetSelectedID(),
				"buttonText": resp.GetSelectedDisplayText(),
				"type":       "template_button_reply",
			}
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Button click detected (template): buttonId=%s, buttonText=%s", mycli.userID, resp.GetSelectedID(), resp.GetSelectedDisplayText())
		} else if resp := evt.Message.GetListResponseMessage(); resp != nil {
			// List response (single select)
			buttonClickData = map[string]interface{}{
				"buttonId":    resp.GetSingleSelectReply().GetSelectedRowID(),
				"buttonText":  resp.GetTitle(),
				"type":        "list_response",
				"description": resp.GetDescription(),
			}
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] List selection detected: rowId=%s, title=%s", mycli.userID, resp.GetSingleSelectReply().GetSelectedRowID(), resp.GetTitle())
		}

		// Se detectou clique em botão, emite evento separado "ButtonClick"
		if buttonClickData != nil {
			buttonClickMap := map[string]interface{}{
				"event": "ButtonClick",
				"data": map[string]interface{}{
					"buttonId":   buttonClickData["buttonId"],
					"buttonText": buttonClickData["buttonText"],
					"type":       buttonClickData["type"],
					"phone":      dataMap["Sender"],
					"jid":        dataMap["Sender"],
					"pushName":   dataMap["PushName"],
					"messageId":  dataMap["ID"],
					"chat":       dataMap["Chat"],
					"fromMe":     dataMap["FromMe"],
					"timestamp":  evt.Info.Timestamp.Unix(),
					"extraData":  buttonClickData,
				},
				"instanceToken": mycli.token,
				"instanceId":    mycli.userID,
				"instanceName":  mycli.Instance.Name,
			}

			buttonClickJSON, err := json.Marshal(buttonClickMap)
			if err == nil {
				buttonClickQueue := strings.ToLower(fmt.Sprintf("%s.buttonclick", userID))
				go mycli.service.CallWebhook(mycli.Instance, buttonClickQueue, buttonClickJSON)
				if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
					go mycli.service.SendToGlobalQueues("ButtonClick", buttonClickJSON, mycli.userID)
				}
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== BUTTON CLICK EVENT DISPATCHED ===== Type: %s, ButtonId: %s", mycli.userID, buttonClickData["type"], buttonClickData["buttonId"])
			}
		}

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== MESSAGE PROCESSING COMPLETED ===== ID: %s, From: %s, Type: %s, Webhook: %v", mycli.userID, evt.Info.ID, evt.Info.Chat.String(), evt.Info.Type, doWebhook)
	case *events.Receipt:
		doWebhook = true
		postMap["event"] = "Receipt"

		// se ignoreGroup for true e o chat for grupo retorna
		if mycli.Instance.IgnoreGroups && strings.Contains(evt.Chat.String(), "@g.us") {
			return
		}

		if mycli.config.EventIgnoreGroup && strings.Contains(evt.Chat.String(), "@g.us") {
			return
		}

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Receipt received with ID: %s from %s with type %s", mycli.userID, evt.MessageIDs[0], evt.SourceString(), evt.Type)

		if evt.Type == types.ReceiptTypeRead || evt.Type == types.ReceiptTypeReadSelf {

			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message was read by %s", mycli.userID, evt.SourceString())
			if evt.Type == types.ReceiptTypeRead {
				postMap["state"] = "Read"
				for _, v := range evt.MessageIDs {
					messageKey := fmt.Sprintf("%s_%s_%s", mycli.userID, v, "Read")
					if _, found := mycli.processedMessages.Get(messageKey); found {
						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message duplicated ignored: %s", mycli.userID, v)
						continue
					}

					mycli.processedMessages.Set(messageKey, true, 30*time.Minute)

					var message message_model.Message

					message.MessageID = v
					message.Timestamp = evt.Timestamp.Format("2006-01-02 15:04:05")
					message.Status = "Read"
					message.Source = evt.Chat.ToNonAD().User

					if mycli.config.DatabaseSaveMessages {
						mycli.persistMessageAsync(message)
					}
				}
			} else {
				postMap["state"] = "ReadSelf"
			}
		} else if evt.Type == types.ReceiptTypeDelivered {
			postMap["state"] = "Delivered"

			var message message_model.Message

			message.MessageID = evt.MessageIDs[0]
			message.Timestamp = evt.Timestamp.Format("2006-01-02 15:04:05")
			message.Status = "Delivered"
			message.Source = evt.Chat.ToNonAD().User

			messageKey := fmt.Sprintf("%s_%s_%s", mycli.userID, evt.MessageIDs[0], "Delivered")
			if _, found := mycli.processedMessages.Get(messageKey); found {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message duplicated ignored: %s", mycli.userID, evt.MessageIDs[0])
				return
			}

			mycli.processedMessages.Set(messageKey, true, 30*time.Minute)

			if mycli.config.DatabaseSaveMessages {
				mycli.persistMessageAsync(message)
			}

			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message delivered to %s", mycli.userID, evt.SourceString())
		} else {
			return
		}
	case *events.Presence:
		doWebhook = true
		postMap["event"] = "Presence"

		if evt.Unavailable {
			postMap["state"] = "offline"
			if evt.LastSeen.IsZero() {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User is now offline", mycli.userID)
			} else {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User is now offline since %s", mycli.userID, evt.LastSeen.Format("2006-01-02 15:04:05"))
			}
		} else {
			postMap["state"] = "online"
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User is now online", mycli.userID)
		}
	case *events.Archive:
		doWebhook = true
		postMap["event"] = "Archive"

		dataMap := postMap["data"].(map[string]interface{})
		dataMap["JID"] = evt.JID
		dataMap["Timestamp"] = evt.Timestamp
		dataMap["Action"] = evt.Action
		dataMap["FromFullSync"] = evt.FromFullSync
		postMap["data"] = dataMap

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Chat archived", mycli.userID)
	case *events.HistorySync:
		doWebhook = true
		postMap["event"] = "HistorySync"

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] History sync event received %+v", mycli.userID, evt.Data.SyncType)
	case *events.AppState:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] App state event received %+v", mycli.userID, evt)
	case *events.LoggedOut:
		doWebhook = true
		postMap["event"] = "LoggedOut"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Logged out for reason %s", mycli.userID, evt.Reason.String())

		// Limpar cache de userInfo para esta instância
		mycli.userInfoCache.Delete(mycli.Instance.Token)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] UserInfo cache cleared for token: %s", mycli.userID, mycli.Instance.Token)

		mycli.Instance.DisconnectReason = evt.Reason.String()
		mycli.Instance.Connected = false
		err := mycli.instanceRepository.UpdateConnected(mycli.Instance.Id, mycli.Instance.Connected, mycli.Instance.DisconnectReason)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
		}

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = dataMap
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap := postMap["data"].(map[string]interface{})

		dataMap["reason"] = evt.Reason.String()

		// Enviar evento LoggedOut para webhook/RabbitMQ ANTES de matar o canal
		postMap["instanceToken"] = mycli.Instance.Token
		postMap["instanceId"] = mycli.userID
		postMap["instanceName"] = mycli.Instance.Name

		values, err := json.Marshal(postMap)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal JSON for LoggedOut event", mycli.userID)
		} else {
			var queueName string
			if _, ok := postMap["event"]; ok {
				queueName = strings.ToLower(fmt.Sprintf("%s.%s", mycli.userID, postMap["event"]))
			}

			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== DISPATCHING LOGGEDOUT EVENT ===== Queue: %s", mycli.userID, queueName)

			// Enviar para webhook/RabbitMQ
			go mycli.service.CallWebhook(mycli.Instance, queueName, values)

			if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Sending LoggedOut to global queues - AMQP: %v, NATS: %v", mycli.userID, mycli.config.AmqpGlobalEnabled, mycli.config.NatsGlobalEnabled)
				go mycli.service.SendToGlobalQueues(postMap["event"].(string), values, mycli.userID)
			}
		}

		// Agora mata o canal DEPOIS de enviar o evento
		mycli.killChannel[mycli.userID] <- true
	case *events.ChatPresence:
		doWebhook = true
		postMap["event"] = "ChatPresence"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Chat presence received %+v", mycli.userID, evt)
	case *events.CallOffer:
		doWebhook = true
		postMap["event"] = "CallOffer"

		// Verifica se deve rejeitar chamadas automaticamente
		if mycli.Instance.RejectCall {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Auto-rejecting call from %s", mycli.userID, evt.CallCreator.String())

			// Rejeita a chamada
			mycli.WAClient.RejectCall(context.Background(), evt.CallCreator, evt.CallID)

			// Envia mensagem de rejeição se configurada
			if mycli.Instance.MsgRejectCall != "" {
				msg := &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &mycli.Instance.MsgRejectCall,
					},
				}

				_, err := mycli.WAClient.SendMessage(context.Background(), evt.CallCreator, msg)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to send reject call message: %v", mycli.userID, err)
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Sent reject call message to %s", mycli.userID, evt.CallCreator.String())
				}
			}
			return
		}

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call offer %+v", mycli.userID, evt)
	case *events.CallAccept:
		doWebhook = true
		postMap["event"] = "CallAccept"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call accept %+v", mycli.userID, evt)
	case *events.CallTerminate:
		doWebhook = true
		postMap["event"] = "CallTerminate"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call terminate %+v", mycli.userID, evt)
	case *events.CallOfferNotice:
		doWebhook = true
		postMap["event"] = "CallOfferNotice"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call offer notice %+v", mycli.userID, evt)
	case *events.CallRelayLatency:
		doWebhook = true
		postMap["event"] = "CallRelayLatency"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call relay latency %+v", mycli.userID, evt)
	case *events.OfflineSyncCompleted:
		doWebhook = true
		postMap["event"] = "OfflineSyncCompleted"
	case *events.ConnectFailure:
		doWebhook = true
		postMap["event"] = "ConnectFailure"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Connection failed with reason %s", mycli.userID, evt.Reason.String())

		// Limpar cache de userInfo para esta instância
		mycli.userInfoCache.Delete(mycli.Instance.Token)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] UserInfo cache cleared for token: %s", mycli.userID, mycli.Instance.Token)

		mycli.Instance.DisconnectReason = evt.Reason.String()
		mycli.Instance.Connected = false
		err := mycli.instanceRepository.UpdateConnected(mycli.Instance.Id, mycli.Instance.Connected, mycli.Instance.DisconnectReason)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
		}
	case *events.Disconnected:
		doWebhook = true
		postMap["event"] = "Disconnected"

		// Limpar cache de userInfo para esta instância (mas não para reconexão automática)
		mycli.userInfoCache.Delete(mycli.Instance.Token)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] UserInfo cache cleared for token: %s", mycli.userID, mycli.Instance.Token)

		mycli.Instance.DisconnectReason = "Disconnected emitted because the websocket is closed by the server."
		mycli.Instance.Connected = false
		err := mycli.instanceRepository.UpdateConnected(mycli.Instance.Id, mycli.Instance.Connected, mycli.Instance.DisconnectReason)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
		}

		// Trigger instance restart via websocket-capable service (non-blocking)
		go func(instanceID string) {
			mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] Disconnected detected, restarting instance", instanceID)
			if err := mycli.service.ReconnectClient(instanceID); err != nil {
				mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Failed to restart instance: %v", instanceID, err)
			}
		}(mycli.userID)
	case *events.LabelEdit:
		doWebhook = true
		postMap["event"] = "LabelEdit"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got label edit %+v", mycli.userID, evt.Action)

		label := label_model.Label{
			InstanceID:   mycli.userID,
			LabelID:      evt.LabelID,
			LabelName:    utils.GetStringValue(evt.Action.Name),
			LabelColor:   fmt.Sprintf("%d", evt.Action.Color),
			PredefinedId: fmt.Sprintf("%d", evt.Action.PredefinedID),
		}

		err := mycli.labelRepository.UpsertLabel(label)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to upsert label: %v", mycli.userID, err)
		}
	case *events.LabelAssociationChat:
		doWebhook = true
		postMap["event"] = "LabelAssociationChat"

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Label association chat received %+v", mycli.userID, evt)
	case *events.LabelAssociationMessage:
		doWebhook = true
		postMap["event"] = "LabelAssociationMessage"

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Label association message received %+v", mycli.userID, evt)
	case *events.Contact:
		doWebhook = true
		postMap["event"] = "Contact"
	case *events.PushName:
		doWebhook = true
		postMap["event"] = "PushName"
	case *events.Picture:
		doWebhook = true
		postMap["event"] = "Picture"
	case *events.UserAbout:
		doWebhook = true
		postMap["event"] = "UserAbout"
	case *events.IdentityChange:
		doWebhook = false
	case *events.GroupInfo:
		doWebhook = true
		postMap["event"] = "GroupInfo"
	case *events.JoinedGroup:
		doWebhook = true
		postMap["event"] = "JoinedGroup"
	case *events.NewsletterJoin:
		doWebhook = true
		postMap["event"] = "NewsletterJoin"
	case *events.NewsletterLeave:
		doWebhook = true
		postMap["event"] = "NewsletterLeave"
	case *events.UndecryptableMessage:
		jsonEvt, err := json.Marshal(evt)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Undecryptable message received: %s", mycli.userID, evt.Info.ID)
		}
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Undecryptable message received all: %+v", mycli.userID, string(jsonEvt))

		if evt.UnavailableType == "view_once" {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Undecryptable message received view_once: %s", mycli.userID, evt.Info.ID)

			doWebhook = true
			postMap["event"] = "Message"

			postMap["data"] = evt
		} else if strings.HasPrefix(evt.Info.ID, "66") || strings.HasPrefix(evt.Info.ID, "67") {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] ID 66 or 67 found, reconnecting client", mycli.userID)
			mycli.WAClient.Disconnect()
			err := mycli.WAClient.Connect()
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error reconnecting client: %s", mycli.userID, err)
			}
		} else {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] ID is not 66 or 67 or view_once, skipping", mycli.userID)
		}
	default:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Unhandled event %s: %+v", mycli.userID, fmt.Sprintf("%T", evt), evt)
		return
	}

	if doWebhook {
		postMap["instanceToken"] = mycli.token
		postMap["instanceId"] = mycli.userID
		postMap["instanceName"] = mycli.Instance.Name

		values, err := json.Marshal(postMap)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal JSON for queue", mycli.userID)
			return
		}

		var queueName string
		if _, ok := postMap["event"]; ok {
			queueName = strings.ToLower(fmt.Sprintf("%s.%s", userID, postMap["event"]))
		}

		// Log webhook dispatch
		eventType := "unknown"
		if event, ok := postMap["event"].(string); ok {
			eventType = event
		}

		dataSize := len(values)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== DISPATCHING WEBHOOK ===== Event: %s, Queue: %s, DataSize: %d bytes", mycli.userID, eventType, queueName, dataSize)

		go mycli.service.CallWebhook(mycli.Instance, queueName, values)

		if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Sending to global queues - Event: %s, AMQP: %v, NATS: %v", mycli.userID, eventType, mycli.config.AmqpGlobalEnabled, mycli.config.NatsGlobalEnabled)
			go mycli.service.SendToGlobalQueues(postMap["event"].(string), values, mycli.userID)
		}
	} else {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] ===== WEBHOOK SKIPPED ===== doWebhook=false", mycli.userID)
	}
}

func (w *whatsmeowService) CallWebhook(instance *instance_model.Instance, queueName string, jsonData []byte) {
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return
	}

	eventType, ok := data["event"].(string)
	if !ok {
		return
	}

	eventArray := strings.Split(instance.Events, ",")

	var subscriptions []string

	if len(eventArray) < 1 {
		subscriptions = append(subscriptions, event_types.MESSAGE)
		subscriptions = append(subscriptions, event_types.SEND_MESSAGE)
	} else {
		for _, arg := range eventArray {
			if !event_types.IsEventType(arg) {
				w.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Message type discarded: %s", instance.Id, arg)
				continue
			}
			if !utils.Find(subscriptions, arg) {
				subscriptions = append(subscriptions, arg)
			}

		}
	}

	if contains(subscriptions, "ALL") {
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
		w.sendToQueueOrWebhook(instance, queueName, jsonData)
		return
	}

	w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] subscriptions %s eventType %s", instance.Id, subscriptions, eventType)

	switch eventType {
	case "Message":
		if contains(subscriptions, "MESSAGE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		} else {
			// Forward to GROUP/NEWSLETTER subscribers even without MESSAGE subscription
			if dataMap, ok := data["data"].(map[string]interface{}); ok {
				if infoMap, ok := dataMap["Info"].(map[string]interface{}); ok {
					if chat, ok := infoMap["Chat"].(string); ok {
						if strings.HasSuffix(chat, "@g.us") && contains(subscriptions, "GROUP") {
							w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Group)", instance.Id, eventType)
							w.sendToQueueOrWebhook(instance, queueName, jsonData)
						} else if strings.HasSuffix(chat, "@newsletter") && contains(subscriptions, "NEWSLETTER") {
							w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Newsletter)", instance.Id, eventType)
							w.sendToQueueOrWebhook(instance, queueName, jsonData)
						}
					}
				}
			}
		}
	case "SendMessage":
		if contains(subscriptions, "SEND_MESSAGE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		} else {
			if dataMap, ok := data["data"].(map[string]interface{}); ok {
				if infoMap, ok := dataMap["Info"].(map[string]interface{}); ok {
					if chat, ok := infoMap["Chat"].(string); ok {
						if strings.HasSuffix(chat, "@g.us") && contains(subscriptions, "GROUP") {
							w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Group)", instance.Id, eventType)
							w.sendToQueueOrWebhook(instance, queueName, jsonData)
						} else if strings.HasSuffix(chat, "@newsletter") && contains(subscriptions, "NEWSLETTER") {
							w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Newsletter)", instance.Id, eventType)
							w.sendToQueueOrWebhook(instance, queueName, jsonData)
						}
					}
				}
			}
		}
	case "Receipt":
		if contains(subscriptions, "READ_RECEIPT") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		} else {
			if dataMap, ok := data["data"].(map[string]interface{}); ok {
				if chat, ok := dataMap["Chat"].(string); ok {
					if strings.HasSuffix(chat, "@g.us") && contains(subscriptions, "GROUP") {
						w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Group)", instance.Id, eventType)
						w.sendToQueueOrWebhook(instance, queueName, jsonData)
					} else if strings.HasSuffix(chat, "@newsletter") && contains(subscriptions, "NEWSLETTER") {
						w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Newsletter)", instance.Id, eventType)
						w.sendToQueueOrWebhook(instance, queueName, jsonData)
					}
				}
			}
		}
	case "Presence":
		if contains(subscriptions, "PRESENCE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "HistorySync":
		if contains(subscriptions, "HISTORY_SYNC") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "ChatPresence", "Archive":
		if contains(subscriptions, "CHAT_PRESENCE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency":
		if contains(subscriptions, "CALL") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure", "Disconnected":
		if contains(subscriptions, "CONNECTION") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
		if contains(subscriptions, "LABEL") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "Contact", "PushName":
		if contains(subscriptions, "CONTACT") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "Picture":
		if contains(subscriptions, "PICTURE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "UserAbout":
		if contains(subscriptions, "USER_ABOUT") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "GroupInfo", "JoinedGroup":
		if contains(subscriptions, "GROUP") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "NewsletterJoin", "NewsletterLeave":
		if contains(subscriptions, "NEWSLETTER") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "QRCode", "QRTimeout", "QRSuccess":
		if contains(subscriptions, "QRCODE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "ButtonClick":
		if contains(subscriptions, "BUTTON_CLICK") || contains(subscriptions, "MESSAGE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}

	default:
		return
	}
}

func contains(subscriptions []string, event string) bool {
	for _, sub := range subscriptions {
		if strings.EqualFold(sub, event) {
			return true
		}
	}
	return false
}

func (w *whatsmeowService) sendToQueueOrWebhook(instance *instance_model.Instance, queueName string, jsonData []byte) {
	if instance.RabbitmqEnable == "enabled" || instance.RabbitmqEnable == "true" {
		err := w.rabbitmqProducer.Produce(queueName, jsonData, instance.RabbitmqEnable, instance.Id)
		if err != nil {
			w.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to send message to rabbitmq: %s", instance.Id, err)
			return
		}
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent to rabbitmq successfully", instance.Id)
	}

	if instance.NatsEnable == "enabled" || instance.NatsEnable == "true" {
		err := w.natsProducer.Produce(queueName, jsonData, instance.NatsEnable, instance.Id)
		if err != nil {
			w.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to send message to nats: %s", instance.Id, err)
			return
		}
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent to nats successfully", instance.Id)
	}

	if instance.WebSocketEnable == "enabled" || instance.WebSocketEnable == "true" {
		err := w.websocketProducer.Produce(queueName, jsonData, instance.Id, instance.Token)
		if err != nil {
			w.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to send message to websocket: %s", instance.Id, err)
			return
		}
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent to websocket successfully", instance.Id)
	}

	if instance.Webhook != "" && instance.Webhook != "disabled" {
		err := w.webhookProducer.Produce(queueName, jsonData, instance.Webhook, instance.Id)
		if err != nil {
			w.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to send message to webhook: %s", instance.Id, err)
			return
		}
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent to webhook successfully", instance.Id)
	}
}

func (w whatsmeowService) StartInstance(instanceId string) error {
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return err
	}

	if instance.Proxy == "" && w.config.ProxyHost != "" && w.config.ProxyPort != "" && w.config.ProxyUsername != "" && w.config.ProxyPassword != "" {
		proxyConfig := ProxyConfig{
			Protocol: utils.NormalizeProxyProtocol(w.config.ProxyProtocol, w.config.ProxyPort),
			Host:     w.config.ProxyHost,
			Port:     w.config.ProxyPort,
			Username: w.config.ProxyUsername,
			Password: w.config.ProxyPassword,
		}

		proxyJSON, err := json.Marshal(proxyConfig)
		if err != nil {
			w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to marshal proxy config: %v", instanceId, err)
			return err
		}

		instance.Proxy = string(proxyJSON)

		err = w.instanceRepository.UpdateProxy(instance.Id, instance.Proxy)
		if err != nil {
			w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to update instance: %s", instanceId, err)
			return err
		}
	}

	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Starting client", instance.Id)

	v := Values{map[string]string{
		"Id":     instance.Id,
		"Jid":    instance.Jid,
		"Token":  instance.Token,
		"Events": instance.Events,
		"osName": instance.OsName,
		"Proxy":  instance.Proxy,
	}}

	w.userInfoCache.Set(instance.Token, v, cache.NoExpiration)

	eventArray := strings.Split(instance.Events, ",")

	var subscribedEvents []string

	if len(eventArray) < 1 {
		subscribedEvents = append(subscribedEvents, event_types.MESSAGE)
	} else {
		for _, arg := range eventArray {
			if !event_types.IsEventType(arg) {
				w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Message type discarded: %s", instanceId, arg)
				continue
			}
			if !utils.Find(subscribedEvents, arg) {
				subscribedEvents = append(subscribedEvents, arg)
			}

		}
	}

	w.killChannel[instance.Id] = make(chan bool)

	clientData := &ClientData{
		Instance:      instance,
		Subscriptions: subscribedEvents,
		Phone:         "",
		IsProxy:       false,
	}

	if instance.Proxy != "" {
		var proxyConfig ProxyConfig
		err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig)
		if err != nil {
			w.loggerWrapper.GetLogger(instanceId).LogError("[%s] error unmarshalling proxy config", instanceId)
			return err
		}

		if proxyConfig.Host != "" {
			clientData.IsProxy = true
		}
	}

	go w.StartClient(clientData)

	return nil
}

func (w whatsmeowService) ConnectOnStartup(clientName string) {
	w.loggerWrapper.GetLogger(clientName).LogInfo("Connecting all instances on startup")
	var instances []*instance_model.Instance
	var err error

	if clientName != "" {
		instances, err = w.instanceRepository.GetAllConnectedInstancesByClientName(clientName)
		if err != nil {
			w.loggerWrapper.GetLogger(clientName).LogError("[%s] Error getting all connected instances: %s", clientName, err)
			return
		}
	} else {
		instances, err = w.instanceRepository.GetAllConnectedInstances()
		if err != nil {
			w.loggerWrapper.GetLogger(clientName).LogError("[%s] Error getting all connected instances: %s", clientName, err)
			return
		}
	}

	w.loggerWrapper.GetLogger(clientName).LogInfo("[%s] Found %d connected instances", clientName, len(instances))

	for _, instance := range instances {
		w.loggerWrapper.GetLogger(clientName).LogInfo("[%s] Starting client for user '%s'", clientName, instance.Id)

		err := w.StartInstance(instance.Id)
		if err != nil {
			w.loggerWrapper.GetLogger(clientName).LogError("[%s] Error starting client: %s", clientName, err)
		}
	}
}

func getExtensionFromMimeType(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "application/pdf":
		return ".pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	default:
		// Se não encontrar um tipo conhecido, extrai a extensão do mimetype
		parts := strings.Split(mimeType, "/")
		if len(parts) > 1 {
			return "." + parts[1]
		}
		return ".bin"
	}
}

func (w *whatsmeowService) SendToGlobalQueues(eventType string, payload []byte, userId string) {
	w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Starting sendToGlobalQueues for event: %s", userId, eventType)

	// AMQP: AMQP_SPECIFIC_EVENTS tem prioridade sobre AMQP_GLOBAL_EVENTS
	if w.config.AmqpGlobalEnabled {
		var shouldSendToAmqp bool
		var amqpQueueName string

		// Se AMQP_SPECIFIC_EVENTS estiver configurada, ela tem prioridade
		if len(w.config.AmqpSpecificEvents) > 0 {
			w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Using AMQP_SPECIFIC_EVENTS (priority over AMQP_GLOBAL_EVENTS)", userId)
			// Verifica se o evento específico está na lista
			if utils.Find(w.config.AmqpSpecificEvents, eventType) {
				shouldSendToAmqp = true
				amqpQueueName = strings.ToLower(eventType)
				w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Event %s found in AMQP_SPECIFIC_EVENTS", userId, eventType)
			}
		} else {
			// Fallback para AMQP_GLOBAL_EVENTS (modo antigo com grupos de eventos)
			w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Using AMQP_GLOBAL_EVENTS (fallback mode)", userId)

			// Mapeia o evento do Whatsmeow para o tipo de evento global
			var globalEventType string
			switch eventType {
			case "Message":
				globalEventType = "MESSAGE"
			case "SendMessage":
				globalEventType = "SEND_MESSAGE"
			case "Receipt":
				globalEventType = "READ_RECEIPT"
			case "Presence":
				globalEventType = "PRESENCE"
			case "HistorySync":
				globalEventType = "HISTORY_SYNC"
			case "ChatPresence", "Archive":
				globalEventType = "CHAT_PRESENCE"
			case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency":
				globalEventType = "CALL"
			case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure", "Disconnected":
				globalEventType = "CONNECTION"
			case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
				globalEventType = "LABEL"
			case "Contact", "PushName":
				globalEventType = "CONTACT"
			case "Picture":
				globalEventType = "PICTURE"
			case "UserAbout":
				globalEventType = "USER_ABOUT"
			case "GroupInfo", "JoinedGroup":
				globalEventType = "GROUP"
			case "NewsletterJoin", "NewsletterLeave":
				globalEventType = "NEWSLETTER"
			case "QRCode", "QRTimeout", "QRSuccess":
				globalEventType = "QRCODE"
			default:
				w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Event %s not mapped to global event type", userId, eventType)
				return
			}

			// Verifica se o grupo de eventos está na lista
			if utils.Find(w.config.AmqpGlobalEvents, globalEventType) {
				shouldSendToAmqp = true
				amqpQueueName = strings.ToLower(eventType)
				w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Event group %s found in AMQP_GLOBAL_EVENTS", userId, globalEventType)
			}
		}

		// Envia para RabbitMQ se necessário
		if shouldSendToAmqp {
			w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Sending to AMQP queue: %s", userId, amqpQueueName)
			err := w.rabbitmqProducer.Produce(amqpQueueName, payload, "global", userId)
			if err != nil {
				w.loggerWrapper.GetLogger(userId).LogError("[%s] Failed to send message to RabbitMQ global queue %s: %v", userId, amqpQueueName, err)
			} else {
				w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Successfully sent message to RabbitMQ global queue %s", userId, amqpQueueName)
			}
		} else {
			w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Event %s not configured for AMQP", userId, eventType)
		}
	}

	// NATS: Mantém o comportamento original por enquanto (só NATS_GLOBAL_EVENTS)
	if w.config.NatsGlobalEnabled {
		// Mapeia o evento para grupo (necessário para NATS por enquanto)
		var globalEventType string
		switch eventType {
		case "Message":
			globalEventType = "MESSAGE"
		case "SendMessage":
			globalEventType = "SEND_MESSAGE"
		case "Receipt":
			globalEventType = "READ_RECEIPT"
		case "Presence":
			globalEventType = "PRESENCE"
		case "HistorySync":
			globalEventType = "HISTORY_SYNC"
		case "ChatPresence", "Archive":
			globalEventType = "CHAT_PRESENCE"
		case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency":
			globalEventType = "CALL"
		case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure", "Disconnected":
			globalEventType = "CONNECTION"
		case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
			globalEventType = "LABEL"
		case "Contact", "PushName":
			globalEventType = "CONTACT"
		case "GroupInfo", "JoinedGroup":
			globalEventType = "GROUP"
		case "NewsletterJoin", "NewsletterLeave":
			globalEventType = "NEWSLETTER"
		case "QRCode", "QRTimeout", "QRSuccess":
			globalEventType = "QRCODE"
		default:
			globalEventType = ""
		}

		// Verifica se o evento está na lista de eventos globais NATS
		if globalEventType != "" && utils.Find(w.config.NatsGlobalEvents, globalEventType) {
			queueName := strings.ToLower(eventType)
			w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Sending to NATS subject: %s", userId, queueName)

			err := w.natsProducer.Produce(queueName, payload, "global", userId)
			if err != nil {
				w.loggerWrapper.GetLogger(userId).LogError("[%s] Failed to send message to NATS global subject %s: %v", userId, queueName, err)
			} else {
				w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Successfully sent message to NATS global subject %s", userId, queueName)
			}
		}
	}
}

var (
	cachedWebVersion   *clientVersion
	cachedWebVersionAt time.Time
	cachedWebVersionMu sync.Mutex
	webVersionCacheTTL = 1 * time.Hour
)

func fetchWhatsAppWebVersion() (*clientVersion, error) {
	cachedWebVersionMu.Lock()
	defer cachedWebVersionMu.Unlock()

	if cachedWebVersion != nil && time.Since(cachedWebVersionAt) < webVersionCacheTTL {
		return cachedWebVersion, nil
	}

	resp, err := http.Get("https://web.whatsapp.com/sw.js")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch WhatsApp Web version: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	content := string(body)

	// Múltiplas estratégias para encontrar client_revision
	patterns := []string{
		`"client_revision":\s*(\d+)`,              // Formato direto
		`\\"client_revision\\":\s*(\d+)`,          // Formato escaped
		`client_revision\\?\\"?:[\s]*(\d+)`,       // Formato mais flexível
		`["']client_revision["'][\s]*:[\s]*(\d+)`, // Com aspas variadas
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(content)

		if len(matches) >= 2 {
			clientRevision, err := strconv.Atoi(matches[1])
			if err != nil {
				continue // Tenta próximo padrão
			}

			// Log qual padrão funcionou
			if clientRevision > 0 {
				cachedWebVersion = &clientVersion{
					Major: 2,
					Minor: 3000,
					Patch: clientRevision,
				}
				cachedWebVersionAt = time.Now()
				return cachedWebVersion, nil
			}
		}
	}

	// Se chegou aqui, nenhum padrão funcionou - log do conteúdo para debug
	// Mostra apenas uma parte para não logar muito
	previewLength := 500
	if len(content) > previewLength {
		content = content[:previewLength] + "..."
	}

	return nil, fmt.Errorf("could not find client revision in the fetched content. Content preview: %s", content)
}

func (w whatsmeowService) UpdateInstanceSettings(instanceId string) error {
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Updating instance settings in runtime", instanceId)

	// Busca a instância atualizada do banco
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting instance from DB: %v", instanceId, err)
		return err
	}

	// Verifica se o MyClient existe
	myClient, exists := w.myClientPointer[instanceId]
	if !exists {
		w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] MyClient not found in runtime, instance may not be connected", instanceId)
		return fmt.Errorf("instance %s not found in runtime", instanceId)
	}

	// Atualiza as configurações no MyClient em execução
	myClient.Instance = instance
	myClient.webhookUrl = instance.Webhook
	myClient.rabbitmqEnable = instance.RabbitmqEnable
	myClient.natsEnable = instance.NatsEnable
	myClient.websocketEnable = instance.WebSocketEnable

	// Atualiza as subscriptions se os eventos mudaram
	eventArray := strings.Split(instance.Events, ",")
	var subscribedEvents []string

	if len(eventArray) < 1 {
		subscribedEvents = append(subscribedEvents, event_types.MESSAGE)
	} else {
		for _, arg := range eventArray {
			if !event_types.IsEventType(arg) {
				w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Message type discarded: %s", instanceId, arg)
				continue
			}
			if !utils.Find(subscribedEvents, arg) {
				subscribedEvents = append(subscribedEvents, arg)
			}
		}
	}

	myClient.subscriptions = subscribedEvents

	// Atualiza o cache do userInfo com as novas configurações
	v := Values{map[string]string{
		"Id":     instance.Id,
		"Jid":    instance.Jid,
		"Token":  instance.Token,
		"Events": instance.Events,
		"osName": instance.OsName,
		"Proxy":  instance.Proxy,
	}}
	w.userInfoCache.Set(instance.Token, v, cache.NoExpiration)

	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Instance settings and cache updated in runtime successfully", instanceId)
	return nil
}

func (w whatsmeowService) UpdateInstanceAdvancedSettings(instanceId string) error {
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Updating advanced settings in runtime", instanceId)

	// Busca a instância atualizada do banco
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting instance from DB: %v", instanceId, err)
		return err
	}

	// Verifica se o MyClient existe
	myClient, exists := w.myClientPointer[instanceId]
	if !exists {
		w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] MyClient not found in runtime, instance may not be connected", instanceId)
		return fmt.Errorf("instance %s not found in runtime", instanceId)
	}

	// Atualiza a instância no MyClient com as advanced settings atualizadas
	myClient.Instance = instance

	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Advanced settings updated in runtime successfully", instanceId)
	return nil
}

func (w whatsmeowService) ClearInstanceCache(instanceId string, token string) error {
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Clearing instance cache - Token: %s", instanceId, token)

	// Limpar userInfoCache
	w.userInfoCache.Delete(token)

	// Limpar myClientPointer se existir
	if _, exists := w.myClientPointer[instanceId]; exists {
		delete(w.myClientPointer, instanceId)
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] MyClient pointer cleared", instanceId)
	}

	// Limpar clientPointer se existir
	if _, exists := w.clientPointer[instanceId]; exists {
		delete(w.clientPointer, instanceId)
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Client pointer cleared", instanceId)
	}

	// Limpar killChannel se existir
	if killChan, exists := w.killChannel[instanceId]; exists {
		select {
		case killChan <- true:
			// Canal recebeu o sinal
		default:
			// Canal pode estar bloqueado, apenas fecha
		}
		close(killChan)
		delete(w.killChannel, instanceId)
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Kill channel cleared", instanceId)
	}

	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Instance cache completely cleared", instanceId)
	return nil
}

func NewWhatsmeowService(
	instanceRepository instance_repository.InstanceRepository,
	authDB *sql.DB,
	messageRepository message_repository.MessageRepository,
	labelRepository label_repository.LabelRepository,
	config *config.Config,
	killChannel map[string](chan bool),
	clientPointer map[string]*whatsmeow.Client,
	rabbitmqProducer producer_interfaces.Producer,
	webhookProducer producer_interfaces.Producer,
	websocketProducer producer_interfaces.Producer,
	sqliteDB *sql.DB,
	exPath string,
	mediaStorage storage_interfaces.MediaStorage,
	natsProducer producer_interfaces.Producer,
	loggerWrapper *logger_wrapper.LoggerManager,
) WhatsmeowService {
	// Inicializar PollService de forma segura
	pollSvc := poll_service.NewPollService(authDB, loggerWrapper)

	return &whatsmeowService{
		instanceRepository: instanceRepository,
		authDB:             authDB,
		messageRepository:  messageRepository,
		labelRepository:    labelRepository,
		pollService:        pollSvc, // NOVO: Serviço de enquetes
		config:             config,
		killChannel:        killChannel,
		userInfoCache:      cache.New(5*time.Minute, 10*time.Minute),
		clientPointer:      clientPointer,
		myClientPointer:    make(map[string]*MyClient),
		rabbitmqProducer:   rabbitmqProducer,
		webhookProducer:    webhookProducer,
		websocketProducer:  websocketProducer,
		sqliteDB:           sqliteDB,
		exPath:             exPath,
		mediaStorage:       mediaStorage,
		processedMessages:  cache.New(30*time.Minute, 1*time.Hour),
		natsProducer:       natsProducer,
		loggerWrapper:      loggerWrapper,
		passkeyCeremony:    ceremony.NewStore(),
	}
}

// GetPollService retorna o serviço de polls (evita dupla inicialização)
func (w *whatsmeowService) GetPollService() poll_service.PollService {
	return w.pollService
}

// PasskeyCeremonyStore exposes the shared ceremony store so the public HTTP
// polling endpoint can read the current stage for a given ceremony token.
func (w *whatsmeowService) PasskeyCeremonyStore() *ceremony.Store {
	return w.passkeyCeremony
}

// SubmitPasskeyResponse forwards the browser's WebAuthn assertion to WhatsApp
// for the given instance. Called by POST /passkey-ceremony/{token}/response.
func (w *whatsmeowService) SubmitPasskeyResponse(instanceId string, resp *types.WebAuthnResponse) error {
	client, ok := w.clientPointer[instanceId]
	if !ok || client == nil {
		return fmt.Errorf("no active client for instance %s", instanceId)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := client.SendPasskeyResponse(ctx, resp); err != nil {
		w.passkeyCeremony.SetError(instanceId, err.Error())
		return err
	}
	// Server will asynchronously emit PairPasskeyConfirmation (or Error) into
	// the event handler; move to the waiting stage in the meantime.
	w.passkeyCeremony.SetAwaitingConfirmation(instanceId)
	return nil
}

// ConfirmPasskey finishes the pairing after the user verified the code.
// Called by POST /passkey-ceremony/{token}/confirm.
func (w *whatsmeowService) ConfirmPasskey(instanceId string) error {
	client, ok := w.clientPointer[instanceId]
	if !ok || client == nil {
		return fmt.Errorf("no active client for instance %s", instanceId)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := client.SendPasskeyConfirmation(ctx); err != nil {
		w.passkeyCeremony.SetError(instanceId, err.Error())
		return err
	}
	w.passkeyCeremony.SetConfirmed(instanceId)
	return nil
}

// cleanSenderID remove a parte ":numero" do sender ID para exibir apenas o remoteJid correto
// Exemplo: "557499879409:3@s.whatsapp.net" -> "557499879409@s.whatsapp.net"
func cleanSenderID(senderID string) string {
	// Procura pelo padrão ":numero" antes do @
	if colonIndex := strings.Index(senderID, ":"); colonIndex != -1 {
		if atIndex := strings.Index(senderID, "@"); atIndex != -1 && colonIndex < atIndex {
			// Remove a parte ":numero" mantendo apenas o número principal e o domínio
			return senderID[:colonIndex] + senderID[atIndex:]
		}
	}
	return senderID
}
