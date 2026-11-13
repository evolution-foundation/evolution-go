package websocket_producer

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// blockedWSConn lets the handshake finish before blocking data writes. Deadlines
// and Close still interrupt the write, as they do on a real network connection.
type blockedWSConn struct {
	net.Conn
	armed      atomic.Bool
	started    chan struct{}
	closed     chan struct{}
	startOnce  sync.Once
	closeOnce  sync.Once
	deadlineMu sync.Mutex
	deadline   time.Time
}

func (c *blockedWSConn) Write(data []byte) (int, error) {
	if !c.armed.Load() {
		return c.Conn.Write(data)
	}
	c.startOnce.Do(func() { close(c.started) })
	c.deadlineMu.Lock()
	deadline := c.deadline
	c.deadlineMu.Unlock()
	var timeout <-chan time.Time
	if !deadline.IsZero() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	case <-timeout:
		return 0, os.ErrDeadlineExceeded
	}
}

func (c *blockedWSConn) SetWriteDeadline(deadline time.Time) error {
	c.deadlineMu.Lock()
	c.deadline = deadline
	c.deadlineMu.Unlock()
	return c.Conn.SetWriteDeadline(deadline)
}

func (c *blockedWSConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func dialBlockedWS(t *testing.T) (*testWSConnection, *blockedWSConn) {
	t.Helper()
	var blocked *blockedWSConn
	dialer := &websocket.Dialer{NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		blocked = &blockedWSConn{Conn: conn, started: make(chan struct{}), closed: make(chan struct{})}
		return blocked, nil
	}}
	pair := dialTestWS(t, dialer)
	blocked.armed.Store(true)
	return pair, blocked
}

func startBlockedProduce(t *testing.T, producer *websocketProducer, pair *testWSConnection, blocked *blockedWSConn) <-chan error {
	t.Helper()
	producer.AddClient(testInstanceID, pair.client)
	result := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		result <- producer.Produce("EVENT", []byte("slow"), testInstanceID, "")
	}()
	t.Cleanup(func() {
		closeTestWS(t, pair.client)
		waitTestDone(t, done)
	})
	select {
	case <-blocked.started:
	case <-time.After(3 * time.Second):
		t.Fatal("data write did not start")
	}
	return result
}

func TestBlockedWriteDoesNotBlockRegistryOrOtherInstance(t *testing.T) {
	producer := NewWebsocketProducer(newTestLoggerManager(t))
	slow, blocked := dialBlockedWS(t)
	result := startBlockedProduce(t, producer, slow, blocked)
	healthy := dialTestWS(t, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		producer.AddClient("other-instance", healthy.server)
		if err := producer.Produce("EVENT", []byte("healthy"), "other-instance", ""); err != nil {
			t.Error(err)
		}
		producer.RemoveClient(testInstanceID)
	}()
	t.Cleanup(func() {
		closeTestWS(t, slow.client)
		waitTestDone(t, done)
	})
	if event := readTestEvent(t, healthy); event.Payload != "healthy" {
		t.Fatal(event)
	}
	waitTestDone(t, done)
	select {
	case err := <-result:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("removal did not interrupt blocked writer: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("removed connection retained a blocked writer")
	}
}

func TestWriteDeadlineClosesAndPrunesConnection(t *testing.T) {
	producer := NewWebsocketProducer(newTestLoggerManager(t))
	producer.writeTimeout = 50 * time.Millisecond
	pair, blocked := dialBlockedWS(t)
	result := startBlockedProduce(t, producer, pair, blocked)
	select {
	case err := <-result:
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatalf("expected write timeout, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("write deadline did not interrupt slow connection")
	}
	producer.clientsMux.RLock()
	remaining := len(producer.connections) + len(producer.clients)
	producer.clientsMux.RUnlock()
	if remaining != 0 {
		t.Fatal("timed-out connection was retained")
	}
	select {
	case <-blocked.closed:
	default:
		t.Fatal("timed-out transport was not closed")
	}
}

func TestServeWsReconnectAndDisconnectCleanup(t *testing.T) {
	producer := NewWebsocketProducer(newTestLoggerManager(t))
	returned := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(w, r, testInstanceID, producer)
		returned <- struct{}{}
	}))
	t.Cleanup(server.Close)
	dial := func() (*testWSConnection, <-chan struct{}) {
		client, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if err != nil {
			if response != nil {
				response.Body.Close()
			}
			t.Fatal(err)
		}
		pair := &testWSConnection{client: client, events: make(chan testWSEvent, 2), pongs: make(chan struct{}, 2)}
		client.SetPongHandler(func(string) error { pair.pongs <- struct{}{}; return nil })
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				var event testWSEvent
				if err := client.ReadJSON(&event); err != nil {
					return
				}
				pair.events <- event
			}
		}()
		t.Cleanup(func() { closeTestWS(t, client); waitTestDone(t, done) })
		flushTestWS(t, pair) // Registration completed before ServeWs processed the ping.
		return pair, done
	}
	_, oldDone := dial()
	replacement, newDone := dial()
	waitTestDone(t, oldDone)
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("superseded handler did not finish")
	}
	if err := producer.Produce("EVENT", []byte("replacement"), testInstanceID, ""); err != nil {
		t.Fatal(err)
	}
	if event := readTestEvent(t, replacement); event.Payload != "replacement" {
		t.Fatal(event)
	}
	closeTestWS(t, replacement.client)
	waitTestDone(t, newDone)
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnected handler did not finish")
	}
	producer.clientsMux.RLock()
	remaining := len(producer.connections) + len(producer.clients)
	producer.clientsMux.RUnlock()
	if remaining != 0 {
		t.Fatal("ServeWs retained a disconnected client")
	}
}
