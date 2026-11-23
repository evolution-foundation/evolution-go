package websocket_producer

import (
	"net/http"
	"strings"
	"sync"

	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/gomessguii/logger"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// client wraps a connection together with its own write mutex.
//
// gorilla/websocket permits only one concurrent writer per connection, and
// events are dispatched from independent goroutines (see the `go CallWebhook`
// calls in the whatsmeow event handler), so two events arriving at once used to
// race on the same socket. Serialising per connection removes that.
type client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *client) writeJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

type websocketProducer struct {
	// clients holds every connection subscribed to a given instance. This is a
	// slice, not a single connection: previously a second subscriber replaced the
	// first, so opening the UI in two tabs silently cut delivery to the older one,
	// and either tab disconnecting removed the whole instance entry.
	clients       map[string][]*client
	broadcast     []*client
	clientsMux    sync.RWMutex
	loggerWrapper *logger_wrapper.LoggerManager
}

func NewWebsocketProducer(loggerWrapper *logger_wrapper.LoggerManager) *websocketProducer {
	return &websocketProducer{
		clients:       make(map[string][]*client),
		broadcast:     make([]*client, 0),
		loggerWrapper: loggerWrapper,
	}
}

// ServeWs upgrades an HTTP request and registers it for the lifetime of the
// connection. An empty instanceId subscribes to every instance's events.
func ServeWs(w http.ResponseWriter, r *http.Request, instanceId string, producer *websocketProducer) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.LogError("Erro ao fazer upgrade da conexão websocket: %v", err)
		return
	}

	c := &client{conn: conn}
	if instanceId == "" {
		producer.addBroadcastClient(c)
	} else {
		producer.addClient(instanceId, c)
	}

	// The read loop exists purely to notice a closed socket; inbound frames are
	// ignored. Deregistration is keyed on this exact connection so sibling
	// subscribers of the same instance keep receiving events.
	go func() {
		defer func() {
			if instanceId == "" {
				producer.removeBroadcastClient(c)
			} else {
				producer.removeClient(instanceId, c)
			}
			_ = conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (p *websocketProducer) addBroadcastClient(c *client) {
	p.clientsMux.Lock()
	p.broadcast = append(p.broadcast, c)
	n := len(p.broadcast)
	p.clientsMux.Unlock()
	logger.LogInfo("Cliente broadcast websocket adicionado (total: %d)", n)
}

func (p *websocketProducer) removeBroadcastClient(c *client) {
	p.clientsMux.Lock()
	p.broadcast = drop(p.broadcast, c)
	n := len(p.broadcast)
	p.clientsMux.Unlock()
	logger.LogInfo("Cliente broadcast websocket removido (total: %d)", n)
}

func (p *websocketProducer) addClient(instanceID string, c *client) {
	p.clientsMux.Lock()
	p.clients[instanceID] = append(p.clients[instanceID], c)
	n := len(p.clients[instanceID])
	p.clientsMux.Unlock()
	p.loggerWrapper.GetLogger(instanceID).LogInfo(
		"Cliente websocket adicionado para instância: %s (total: %d)", instanceID, n)
}

func (p *websocketProducer) removeClient(instanceID string, c *client) {
	p.clientsMux.Lock()
	remaining := drop(p.clients[instanceID], c)
	if len(remaining) == 0 {
		delete(p.clients, instanceID)
	} else {
		p.clients[instanceID] = remaining
	}
	p.clientsMux.Unlock()
	p.loggerWrapper.GetLogger(instanceID).LogInfo(
		"Cliente websocket removido para instância: %s (restantes: %d)", instanceID, len(remaining))
}

// drop returns a new slice without target. It allocates rather than filtering in
// place, so a snapshot taken by a concurrent Produce can never observe a mutated
// backing array.
func drop(list []*client, target *client) []*client {
	out := make([]*client, 0, len(list))
	for _, c := range list {
		if c != target {
			out = append(out, c)
		}
	}
	return out
}

func (p *websocketProducer) Produce(queueName string, payload []byte, instanceID string, _ string) error {
	message := map[string]interface{}{
		"queue":   strings.ToLower(queueName),
		"payload": string(payload),
	}

	// Snapshot under the lock, then write outside it: a slow or half-open socket
	// must not block other instances' deliveries.
	p.clientsMux.RLock()
	targets := append([]*client(nil), p.clients[instanceID]...)
	targets = append(targets, p.broadcast...)
	p.clientsMux.RUnlock()

	var failed []*client
	for _, c := range targets {
		if err := c.writeJSON(message); err != nil {
			p.loggerWrapper.GetLogger(instanceID).LogError(
				"Erro ao enviar mensagem websocket para %s: %v", instanceID, err)
			failed = append(failed, c)
		}
	}

	if len(failed) > 0 {
		p.clientsMux.Lock()
		for _, c := range failed {
			if remaining := drop(p.clients[instanceID], c); len(remaining) == 0 {
				delete(p.clients, instanceID)
			} else {
				p.clients[instanceID] = remaining
			}
			p.broadcast = drop(p.broadcast, c)
		}
		p.clientsMux.Unlock()
	}

	if n := len(targets) - len(failed); n > 0 {
		p.loggerWrapper.GetLogger(instanceID).LogInfo(
			"Mensagem websocket enviada para %d cliente(s) da instância %s na fila %s", n, instanceID, queueName)
	}

	// Deliberately nil even when a write failed. sendToQueueOrWebhook aborts the
	// remaining producers on error, so reporting a dead browser tab here would
	// silently suppress RabbitMQ/NATS/webhook delivery for the same event.
	return nil
}

// CreateGlobalQueues não faz nada para websocket producer
func (p *websocketProducer) CreateGlobalQueues() error {
	return nil
}
