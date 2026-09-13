package rabbitmq_producer

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	producer_interfaces "github.com/evolution-foundation/evolution-go/pkg/events/interfaces"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/gomessguii/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type rabbitMQProducer struct {
	conn               *amqp.Connection
	amqpGlobalEnabled  bool
	amqpGlobalEvents   []string
	amqpSpecificEvents []string
	connStr            string
	maxRetries         int
	loggerWrapper      *logger_wrapper.LoggerManager
}

func NewRabbitMQProducer(
	conn *amqp.Connection,
	amqpGlobalEnabled bool,
	amqpGlobalEvents []string,
	amqpSpecificEvents []string,
	connStr string,
	loggerWrapper *logger_wrapper.LoggerManager,
) producer_interfaces.Producer {
	producer := &rabbitMQProducer{
		conn:               conn,
		amqpGlobalEnabled:  amqpGlobalEnabled,
		amqpGlobalEvents:   amqpGlobalEvents,
		amqpSpecificEvents: amqpSpecificEvents,
		connStr:            connStr,
		maxRetries:         3,
		loggerWrapper:      loggerWrapper,
	}

	return producer
}

// maskConnectionString masks sensitive information in the connection string for logging
func (p *rabbitMQProducer) maskConnectionString(connStr string) string {
	if connStr == "" {
		return "empty"
	}

	parsedURL, err := url.Parse(connStr)
	if err != nil {
		return "invalid-url"
	}

	// Mask password if present
	if parsedURL.User != nil {
		if _, hasPassword := parsedURL.User.Password(); hasPassword {
			parsedURL.User = url.UserPassword(parsedURL.User.Username(), "***")
		}
	}

	return parsedURL.String()
}

// handleConnectionClose monitors connection close events and logs them
func (p *rabbitMQProducer) handleConnectionClose() {
	if p.conn == nil {
		return
	}

	closeChan := make(chan *amqp.Error)
	p.conn.NotifyClose(closeChan)

	closeErr := <-closeChan
	if closeErr != nil {
		logger.LogWarn("RabbitMQ connection closed unexpectedly: %v", closeErr)
		logger.LogInfo("Connection will be re-established on next message send")
	} else {
		logger.LogInfo("RabbitMQ connection closed gracefully")
	}
}

func (p *rabbitMQProducer) reconnect() error {
	if p.connStr == "" {
		return fmt.Errorf("connection string is empty - RabbitMQ URL not configured")
	}

	logger.LogInfo("Starting RabbitMQ reconnection process with URL: %s", p.maskConnectionString(p.connStr))

	var err error
	for i := 0; i < 3; i++ {
		logger.LogInfo("Tentando reconectar ao RabbitMQ (tentativa %d/3)", i+1)

		// Create connection with heartbeat to prevent timeouts
		config := amqp.Config{
			Heartbeat: 30 * time.Second, // Send heartbeat every 30 seconds
			Locale:    "en_US",
		}

		p.conn, err = amqp.DialConfig(p.connStr, config)
		if err == nil {
			logger.LogInfo("Reconectado com sucesso ao RabbitMQ com heartbeat de 30s")

			// Set up connection close notification
			go p.handleConnectionClose()
			return nil
		}

		logger.LogError("Falha na tentativa %d/3 de reconexão: %v", i+1, err)
		if i < 2 { // Don't sleep on the last attempt
			time.Sleep(time.Second * 2)
		}
	}
	return fmt.Errorf("falha ao reconectar após 3 tentativas: %v", err)
}

func (p *rabbitMQProducer) ensureConnection() error {
	if p.conn == nil || p.conn.IsClosed() {
		return p.reconnect()
	}
	return nil
}

func (p *rabbitMQProducer) publishWithRetry(
	channel *amqp.Channel,
	exchange string,
	routingKey string,
	payload []byte,
	userID string,
) error {
	var err error
	for i := 0; i < p.maxRetries; i++ {
		err = channel.Publish(
			exchange,   // exchange
			routingKey, // routing key
			false,      // mandatory
			false,      // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         payload,
				DeliveryMode: amqp.Persistent, // Garante persistência da mensagem
			})

		if err == nil {
			return nil
		}

		logger.LogWarn("[%s] Falha ao publicar mensagem (tentativa %d/%d): %v",
			userID, i+1, p.maxRetries, err)

		// Se o erro for de conexão, tenta reconectar
		if err.Error() == "Exception (504) Reason: \"channel/connection is not open\"" {
			if err := p.ensureConnection(); err != nil {
				continue
			}

			// Cria novo canal após reconexão
			channel, err = p.conn.Channel()
			if err != nil {
				continue
			}
		}

		time.Sleep(time.Second * time.Duration(i+1))
	}
	return err
}

func (p *rabbitMQProducer) Produce(
	queueName string,
	payload []byte,
	rabbitmqEnable string,
	userID string,
) error {
	p.loggerWrapper.GetLogger(userID).LogInfo("[%s] RabbitMQ Producer - Starting produce for queue: %s", userID, queueName)

	if p.connStr == "" {
		return fmt.Errorf("RabbitMQ connection string is empty - check AMQP_URL configuration")
	}

	if err := p.ensureConnection(); err != nil {
		p.loggerWrapper.GetLogger(userID).LogError("[%s] Failed to ensure RabbitMQ connection: %v", userID, err)
		return fmt.Errorf("falha ao garantir conexão: %v", err)
	}

	channel, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("falha ao abrir canal: %v", err)
	}
	defer channel.Close()

	// Configura confirmação de publicação
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("falha ao configurar confirms do canal: %v", err)
	}

	args := amqp.Table{
		"x-queue-type": "quorum",
		"x-ha-policy":  "all", // Alta disponibilidade
	}

	if rabbitmqEnable == "global" || rabbitmqEnable == "enabled" {
		_, err = channel.QueueDeclare(
			queueName, // name
			true,      // durable
			false,     // delete when unused
			false,     // exclusive
			false,     // no-wait
			args,      // arguments
		)
		if err != nil {
			return fmt.Errorf("falha ao declarar fila %s: %v", queueName, err)
		}

		err = p.publishWithRetry(channel, "", queueName, payload, userID)
		if err != nil {
			return fmt.Errorf("falha ao publicar mensagem após todas as tentativas: %v", err)
		}

		p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Mensagem publicada com sucesso na fila: %s", userID, queueName)
	}

	return nil
}

// ProduceToExchange publishes to a named topic exchange with a custom
// routing key, instead of the default-exchange/queue-name publish that
// Produce performs.
func (p *rabbitMQProducer) ProduceToExchange(
	exchange string,
	routingKey string,
	payload []byte,
	userID string,
) error {
	p.loggerWrapper.GetLogger(userID).LogInfo("[%s] RabbitMQ Producer - Starting produce to exchange: %s (routing key: %s)", userID, exchange, routingKey)

	if p.connStr == "" {
		return fmt.Errorf("RabbitMQ connection string is empty - check AMQP_URL configuration")
	}

	if err := p.ensureConnection(); err != nil {
		p.loggerWrapper.GetLogger(userID).LogError("[%s] Failed to ensure RabbitMQ connection: %v", userID, err)
		return fmt.Errorf("falha ao garantir conexão: %v", err)
	}

	channel, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("falha ao abrir canal: %v", err)
	}
	defer channel.Close()

	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("falha ao configurar confirms do canal: %v", err)
	}

	if err := channel.ExchangeDeclare(
		exchange, // name
		"topic",  // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	); err != nil {
		return fmt.Errorf("falha ao declarar exchange %s: %v", exchange, err)
	}

	if err := p.publishWithRetry(channel, exchange, routingKey, payload, userID); err != nil {
		return fmt.Errorf("falha ao publicar mensagem na exchange %s após todas as tentativas: %v", exchange, err)
	}

	p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Mensagem publicada com sucesso na exchange %s (routing key: %s)", userID, exchange, routingKey)
	return nil
}

// CreateGlobalQueues cria todas as filas globais no startup da aplicação
func (p *rabbitMQProducer) CreateGlobalQueues() error {
	if !p.amqpGlobalEnabled {
		return nil
	}

	p.loggerWrapper.GetLogger("system").LogInfo("Creating global queues for enabled events")

	if err := p.ensureConnection(); err != nil {
		return fmt.Errorf("failed to ensure connection: %v", err)
	}

	channel, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %v", err)
	}
	defer channel.Close()

	args := amqp.Table{
		"x-queue-type": "quorum",
		"x-ha-policy":  "all", // Alta disponibilidade
	}

	createdQueues := 0

	// AMQP_SPECIFIC_EVENTS tem prioridade sobre AMQP_GLOBAL_EVENTS
	if len(p.amqpSpecificEvents) > 0 {
		p.loggerWrapper.GetLogger("system").LogInfo("Using AMQP_SPECIFIC_EVENTS (priority over AMQP_GLOBAL_EVENTS)")

		// Cria filas diretas para eventos específicos
		for _, eventName := range p.amqpSpecificEvents {
			queueName := strings.ToLower(eventName)

			_, err = channel.QueueDeclare(
				queueName, // name
				true,      // durable
				false,     // delete when unused
				false,     // exclusive
				false,     // no-wait
				args,      // arguments
			)
			if err != nil {
				p.loggerWrapper.GetLogger("system").LogError("Failed to create specific queue %s: %v", queueName, err)
				return fmt.Errorf("failed to create specific queue %s: %v", queueName, err)
			}
			p.loggerWrapper.GetLogger("system").LogInfo("Specific queue created: %s", queueName)
			createdQueues++
		}
	} else {
		p.loggerWrapper.GetLogger("system").LogInfo("Using AMQP_GLOBAL_EVENTS (fallback mode)")

		// Mapeia eventos globais para os eventos originais que precisam de filas (modo antigo)
		eventMap := map[string][]string{
			"MESSAGE":       {"message"},
			"SEND_MESSAGE":  {"sendmessage"},
			"READ_RECEIPT":  {"receipt"},
			"PRESENCE":      {"presence"},
			"HISTORY_SYNC":  {"historysync"},
			"CHAT_PRESENCE": {"chatpresence", "archive"},
			"CALL":          {"calloffer", "callaccept", "callterminate", "calloffernotice", "callrelaylatency"},
			"CONNECTION":    {"connected", "pairsuccess", "temporaryban", "loggedout", "connectfailure", "disconnected"},
			"LABEL":         {"labeledit", "labelassociationchat", "labelassociationmessage"},
			"CONTACT":       {"contact", "pushname"},
			"GROUP":         {"groupinfo", "joinedgroup"},
			"NEWSLETTER":    {"newsletterjoin", "newsletterleave"},
			"QRCODE":        {"qrcode", "qrtimeout", "qrsuccess"},
		}

		for _, globalEvent := range p.amqpGlobalEvents {
			if queueNames, exists := eventMap[globalEvent]; exists {
				for _, queueName := range queueNames {
					_, err = channel.QueueDeclare(
						queueName, // name
						true,      // durable
						false,     // delete when unused
						false,     // exclusive
						false,     // no-wait
						args,      // arguments
					)
					if err != nil {
						p.loggerWrapper.GetLogger("system").LogError("Failed to create global queue %s: %v", queueName, err)
						return fmt.Errorf("failed to create global queue %s: %v", queueName, err)
					}
					p.loggerWrapper.GetLogger("system").LogInfo("Global queue created: %s", queueName)
					createdQueues++
				}
			}
		}
	}

	p.loggerWrapper.GetLogger("system").LogInfo("Successfully created %d global queues", createdQueues)
	return nil
}
