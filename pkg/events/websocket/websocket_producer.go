package websocket_producer

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/gomessguii/logger"
	"github.com/gorilla/websocket"
)

const websocketWriteTimeout = 10 * time.Second

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		logger.LogInfo("Verificando origem da conexão WebSocket")
		return true
	},
}

// wsConn owns the only data writer for a physical connection, including when
// the connection is registered in both instance and broadcast collections.
type wsConn struct {
	conn         *websocket.Conn
	writeMu      sync.Mutex
	closed       atomic.Bool
	writeTimeout time.Duration
}

func (w *wsConn) writeJSON(v any) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	if w.closed.Load() {
		return net.ErrClosed
	}
	if err := w.conn.SetWriteDeadline(time.Now().Add(w.writeTimeout)); err != nil {
		w.close()
		return err
	}
	if err := w.conn.WriteJSON(v); err != nil {
		// A timed-out Gorilla connection cannot be reused. Close immediately so
		// pending writers stop and the reader can finish.
		w.close()
		return err
	}
	return nil
}

func (w *wsConn) close() {
	if w.closed.CompareAndSwap(false, true) {
		// Gorilla explicitly permits Close concurrently with reads and writes.
		if err := w.conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			logger.LogWarn("Erro ao fechar conexão websocket: %v", err)
		}
	}
}

type websocketProducer struct {
	clients       map[string]*wsConn
	broadcast     []*wsConn
	connections   map[*websocket.Conn]*wsConn
	clientsMux    sync.RWMutex
	loggerWrapper *logger_wrapper.LoggerManager
	writeTimeout  time.Duration
}

// NewWebsocketProducer creates a producer that owns its registered connections.
func NewWebsocketProducer(loggerWrapper *logger_wrapper.LoggerManager) *websocketProducer {
	return &websocketProducer{
		clients:       make(map[string]*wsConn),
		connections:   make(map[*websocket.Conn]*wsConn),
		loggerWrapper: loggerWrapper,
		writeTimeout:  websocketWriteTimeout,
	}
}

// ServeWs keeps the reader in the HTTP handler until the socket closes. Cleanup
// belongs to this connection, so an older handler cannot remove a replacement.
func ServeWs(w http.ResponseWriter, r *http.Request, instanceId string, producer *websocketProducer) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.LogError("Erro ao fazer upgrade da conexão websocket: %v", err)
		return
	}
	var client *wsConn
	if instanceId == "" {
		client = producer.addBroadcastClient(conn)
	} else {
		client = producer.addClient(instanceId, conn)
	}
	defer producer.removeConnection(client)
	for {
		// Process control frames and discard incoming data without buffering an
		// entire client message; this endpoint only sends events.
		if _, _, err := conn.NextReader(); err != nil {
			return
		}
	}
}

// connectionLocked returns the canonical writer. Call with clientsMux held.
func (p *websocketProducer) connectionLocked(conn *websocket.Conn) *wsConn {
	if conn == nil {
		return nil
	}
	if client, ok := p.connections[conn]; ok {
		return client
	}
	client := &wsConn{conn: conn, writeTimeout: p.writeTimeout}
	p.connections[conn] = client
	return client
}

// AddBroadcastClient registers a socket once to receive events from all instances.
func (p *websocketProducer) AddBroadcastClient(conn *websocket.Conn) {
	p.addBroadcastClient(conn)
}

func (p *websocketProducer) addBroadcastClient(conn *websocket.Conn) *wsConn {
	p.clientsMux.Lock()
	client := p.connectionLocked(conn)
	if client != nil && !slices.Contains(p.broadcast, client) {
		p.broadcast = append(p.broadcast, client)
	}
	p.clientsMux.Unlock()
	return client
}

// RemoveBroadcastClient closes the socket only when no instance still uses it.
func (p *websocketProducer) RemoveBroadcastClient(conn *websocket.Conn) {
	p.clientsMux.Lock()
	client := p.connections[conn]
	p.broadcast = slices.DeleteFunc(p.broadcast, func(registered *wsConn) bool {
		return registered == client
	})
	closeClient := p.releaseConnectionLocked(client)
	p.clientsMux.Unlock()
	if closeClient {
		client.close()
	}
}

// AddClient replaces the instance connection and releases its previous socket.
func (p *websocketProducer) AddClient(instanceID string, conn *websocket.Conn) {
	p.addClient(instanceID, conn)
}

func (p *websocketProducer) addClient(instanceID string, conn *websocket.Conn) *wsConn {
	p.clientsMux.Lock()
	client := p.connectionLocked(conn)
	previous := p.clients[instanceID]
	if client != nil {
		p.clients[instanceID] = client
	}
	closePrevious := client != nil && previous != client && p.releaseConnectionLocked(previous)
	p.clientsMux.Unlock()
	if closePrevious {
		previous.close()
	}
	return client
}

// RemoveClient unregisters and closes the current connection for the instance.
func (p *websocketProducer) RemoveClient(instanceID string) {
	p.clientsMux.Lock()
	client := p.clients[instanceID]
	delete(p.clients, instanceID)
	closeClient := p.releaseConnectionLocked(client)
	p.clientsMux.Unlock()
	if closeClient {
		client.close()
	}
}

// releaseConnectionLocked releases a writer only after its last registration.
// Callers close it outside clientsMux. A closed socket needs a new connection.
func (p *websocketProducer) releaseConnectionLocked(client *wsConn) bool {
	if client == nil || slices.Contains(p.broadcast, client) {
		return false
	}
	for _, registered := range p.clients {
		if registered == client {
			return false
		}
	}
	if p.connections[client.conn] != client {
		return false
	}
	delete(p.connections, client.conn)
	return true
}

func (p *websocketProducer) removeConnection(client *wsConn) {
	if client == nil {
		return
	}
	p.clientsMux.Lock()
	if p.connections[client.conn] == client {
		delete(p.connections, client.conn)
	}
	for id, registered := range p.clients {
		if registered == client {
			delete(p.clients, id)
		}
	}
	p.broadcast = slices.DeleteFunc(p.broadcast, func(registered *wsConn) bool {
		return registered == client
	})
	p.clientsMux.Unlock()
	// Closing the transport is outside the registry lock and unblocks a writer.
	client.close()
}

// Produce attempts every matching recipient. An error may mean partial delivery;
// callers must not assume retrying the event is safe from duplicate delivery.
func (p *websocketProducer) Produce(queueName string, payload []byte, instanceID string, _ string) error {
	message := map[string]interface{}{
		"queue":   strings.ToLower(queueName),
		"payload": string(payload),
	}
	// Copy targets before network I/O. Registration/removal must remain available
	// even while one of these sockets is slow or blocked.
	p.clientsMux.RLock()
	client := p.clients[instanceID]
	targets := make([]*wsConn, 0, 1+len(p.broadcast))
	if client != nil {
		targets = append(targets, client)
	}
	for _, broadcast := range p.broadcast {
		if broadcast != client {
			targets = append(targets, broadcast)
		}
	}
	p.clientsMux.RUnlock()

	var failures []error
	for _, target := range targets {
		if err := target.writeJSON(message); err != nil {
			p.removeConnection(target)
			p.loggerWrapper.GetLogger(instanceID).LogError("Erro ao enviar mensagem websocket: %v", err)
			failures = append(failures, fmt.Errorf("send websocket event: %w", err))
		}
	}
	// Healthy recipients were still attempted. An error can represent partial
	// delivery; events are not retried here.
	return errors.Join(failures...)
}

// CreateGlobalQueues is a no-op because WebSocket delivery has no broker queues.
func (p *websocketProducer) CreateGlobalQueues() error {
	return nil
}
