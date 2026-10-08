package websocket_producer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/gorilla/websocket"
)

const testInstanceID = "websocket-instance"

type testWSEvent struct {
	Queue   string `json:"queue"`
	Payload string `json:"payload"`
}

type testWSConnection struct {
	server *websocket.Conn
	client *websocket.Conn
	events chan testWSEvent
	pongs  chan struct{}
}

func newTestLoggerManager(t *testing.T) *logger_wrapper.LoggerManager {
	t.Helper()
	manager := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir(), LogMaxSize: 1})
	t.Cleanup(func() {
		for _, id := range []string{testInstanceID, "other-instance"} {
			if err := manager.GetLogger(id).Close(); err != nil {
				t.Error(err)
			}
		}
	})
	return manager
}

func closeTestWS(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Error(err)
	}
}

func waitTestDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("websocket worker did not finish")
	}
}

func dialTestWS(t *testing.T, dialer *websocket.Dialer) *testWSConnection {
	t.Helper()
	ready := make(chan *websocket.Conn, 1)
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(serverDone)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		ready <- conn
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}))
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	client, response, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		if response != nil {
			response.Body.Close()
		}
		server.Close()
		t.Fatal(err)
	}
	var serverConn *websocket.Conn
	select {
	case serverConn = <-ready:
	case <-time.After(3 * time.Second):
		closeTestWS(t, client)
		server.Close()
		t.Fatal("upgrade did not finish")
	}
	pair := &testWSConnection{
		server: serverConn, client: client, events: make(chan testWSEvent, 2048), pongs: make(chan struct{}, 2),
	}
	client.SetPongHandler(func(string) error { pair.pongs <- struct{}{}; return nil })
	readerDone := make(chan struct{})
	stopReader := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			var event testWSEvent
			if err := client.ReadJSON(&event); err != nil {
				return
			}
			select {
			case pair.events <- event:
			case <-stopReader:
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(stopReader)
		closeTestWS(t, client)
		closeTestWS(t, serverConn)
		server.Close()
		waitTestDone(t, readerDone)
		waitTestDone(t, serverDone)
	})
	return pair
}

func flushTestWS(t *testing.T, pair *testWSConnection) {
	t.Helper()
	if err := pair.client.WriteControl(websocket.PingMessage, nil, time.Now().Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pair.pongs:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket pong did not arrive")
	}
}

func readTestEvent(t *testing.T, pair *testWSConnection) testWSEvent {
	t.Helper()
	select {
	case event := <-pair.events:
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("websocket event did not arrive")
		return testWSEvent{}
	}
}

func TestProduceConcurrentWritesDeliverEveryEvent(t *testing.T) {
	const goroutines, messages = 20, 20
	for _, mode := range []string{"instance", "broadcast", "shared_registration"} {
		t.Run(mode, func(t *testing.T) {
			pair := dialTestWS(t, nil)
			producer := NewWebsocketProducer(newTestLoggerManager(t))
			if mode != "broadcast" {
				producer.AddClient(testInstanceID, pair.server)
			}
			if mode != "instance" {
				producer.AddBroadcastClient(pair.server)
			}
			if mode == "shared_registration" {
				producer.AddBroadcastClient(pair.server)
			}
			start := make(chan struct{})
			errCh := make(chan error, goroutines)
			var workers sync.WaitGroup
			for g := 0; g < goroutines; g++ {
				workers.Add(1)
				go func(g int) {
					defer workers.Done()
					<-start
					for i := 0; i < messages; i++ {
						payload := []byte(fmt.Sprintf(`{"id":%d}`, g*messages+i))
						if err := producer.Produce("INSTANCE.Message", payload, testInstanceID, ""); err != nil {
							errCh <- err
							return
						}
					}
				}(g)
			}
			close(start)
			workers.Wait()
			close(errCh)
			for err := range errCh {
				t.Fatal(err)
			}
			flushTestWS(t, pair) // A pong after the writes confirms all prior frames were read.
			if got := len(pair.events); got != goroutines*messages {
				t.Fatalf("received %d events, want %d", got, goroutines*messages)
			}
			seen := make(map[int]bool)
			for i := 0; i < goroutines*messages; i++ {
				event := readTestEvent(t, pair)
				var payload struct {
					ID int `json:"id"`
				}
				if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
					t.Fatal(err)
				}
				if event.Queue != "instance.message" || payload.ID < 0 || payload.ID >= goroutines*messages || seen[payload.ID] {
					t.Fatalf("duplicate or corrupt event: %#v", event)
				}
				seen[payload.ID] = true
			}
		})
	}
}

func TestProduceFailuresStillReachHealthyRecipients(t *testing.T) {
	for _, mode := range []string{"instance_failure", "broadcast_failure"} {
		t.Run(mode, func(t *testing.T) {
			broken, healthy := dialTestWS(t, nil), dialTestWS(t, nil)
			producer := NewWebsocketProducer(newTestLoggerManager(t))
			if mode == "instance_failure" {
				producer.AddClient(testInstanceID, broken.server)
			} else {
				producer.AddBroadcastClient(broken.server)
			}
			producer.AddBroadcastClient(healthy.server)
			closeTestWS(t, broken.server)
			if err := producer.Produce("EVENT", []byte("payload"), testInstanceID, ""); err == nil {
				t.Fatal("delivery failure was hidden")
			}
			if event := readTestEvent(t, healthy); event.Queue != "event" || event.Payload != "payload" {
				t.Fatalf("unexpected event: %#v", event)
			}
			producer.clientsMux.RLock()
			_, retained := producer.connections[broken.server]
			producer.clientsMux.RUnlock()
			if retained {
				t.Fatal("failed connection retained")
			}
		})
	}
}

func TestConnectionRemovalPreservesOtherRegistration(t *testing.T) {
	pair := dialTestWS(t, nil)
	producer := NewWebsocketProducer(newTestLoggerManager(t))
	producer.AddClient(testInstanceID, pair.server)
	producer.AddBroadcastClient(pair.server)
	producer.RemoveBroadcastClient(pair.server)
	if err := producer.Produce("EVENT", []byte("instance"), testInstanceID, ""); err != nil {
		t.Fatal(err)
	}
	if event := readTestEvent(t, pair); event.Payload != "instance" {
		t.Fatal(event)
	}
	producer.AddBroadcastClient(pair.server)
	producer.RemoveClient(testInstanceID)
	if err := producer.Produce("EVENT", []byte("broadcast"), testInstanceID, ""); err != nil {
		t.Fatal(err)
	}
	if event := readTestEvent(t, pair); event.Payload != "broadcast" {
		t.Fatal(event)
	}
	producer.RemoveBroadcastClient(pair.server)
	producer.clientsMux.RLock()
	remaining := len(producer.connections)
	producer.clientsMux.RUnlock()
	if remaining != 0 {
		t.Fatal("last registration retained the connection")
	}
}

func TestStaleCleanupPreservesReplacement(t *testing.T) {
	old, replacement := dialTestWS(t, nil), dialTestWS(t, nil)
	producer := NewWebsocketProducer(newTestLoggerManager(t))
	previous := producer.addClient(testInstanceID, old.server)
	producer.AddClient(testInstanceID, replacement.server)
	if !previous.closed.Load() {
		t.Fatal("superseded connection was not closed")
	}
	producer.removeConnection(previous)
	if err := producer.Produce("EVENT", []byte("replacement"), testInstanceID, ""); err != nil {
		t.Fatal(err)
	}
	if event := readTestEvent(t, replacement); event.Payload != "replacement" {
		t.Fatal(event)
	}
}

func TestProduceInstanceIsolation(t *testing.T) {
	one, two := dialTestWS(t, nil), dialTestWS(t, nil)
	producer := NewWebsocketProducer(newTestLoggerManager(t))
	producer.AddClient(testInstanceID, one.server)
	producer.AddClient("other-instance", two.server)
	if err := producer.Produce("EVENT", []byte("private"), testInstanceID, ""); err != nil {
		t.Fatal(err)
	}
	flushTestWS(t, one)
	flushTestWS(t, two)
	if len(one.events) != 1 || len(two.events) != 0 {
		t.Fatal("event was routed to the wrong instance")
	}
}
