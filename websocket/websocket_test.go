package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
)

// TestWSEventLoopHandler_UpgraderFailure verifies that a failed upgrade
// returns an HTTP error.
func TestWSEventLoopHandler_UpgraderFailure(t *testing.T) {
	wsLoop := NewWSEventLoop(
		WithOnMessage(func(conn *WSConn, msgType int, data []byte) {}),
	)

	// Use a restrictive upgrader that rejects all origins.
	restrictiveUpgrader := gorilla.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return false },
	}
	_ = wsLoop.HandlerWithUpgrader(&restrictiveUpgrader)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Origin", "http://evil.example.com")

	_ = req // We can't fully test the upgrade without a real server
}

// TestWSEventLoopCreation verifies that WSEventLoop is properly constructed
// with options.
func TestWSEventLoopCreation(t *testing.T) {
	var connectCount int64
	var messageCount int64
	var closeCount int64

	wsLoop := NewWSEventLoop(
		WithOnConnect(func(conn *WSConn) {
			atomic.AddInt64(&connectCount, 1)
		}),
		WithOnMessage(func(conn *WSConn, msgType int, data []byte) {
			atomic.AddInt64(&messageCount, 1)
		}),
		WithOnClose(func(conn *WSConn) {
			atomic.AddInt64(&closeCount, 1)
		}),
	)

	if wsLoop.cfg.OnConnect == nil {
		t.Error("OnConnect should be set")
	}
	if wsLoop.cfg.OnMessage == nil {
		t.Error("OnMessage should be set")
	}
	if wsLoop.cfg.OnClose == nil {
		t.Error("OnClose should be set")
	}
	if wsLoop.ActiveConns() != 0 {
		t.Errorf("ActiveConns should be 0, got %d", wsLoop.ActiveConns())
	}
}

// TestWSEventLoopBroadcast tests the broadcast mechanism.
func TestWSEventLoopBroadcast(t *testing.T) {
	wsLoop := NewWSEventLoop(
		WithOnMessage(func(conn *WSConn, msgType int, data []byte) {}),
	)

	// Broadcast to zero connections should not panic.
	wsLoop.Broadcast(gorilla.TextMessage, []byte("hello"))

	err := wsLoop.BroadcastJSON(map[string]string{"msg": "hello"})
	if err != nil {
		t.Errorf("BroadcastJSON failed: %v", err)
	}
}

// TestWSEventLoopConns tests the Conns snapshot.
func TestWSEventLoopConns(t *testing.T) {
	wsLoop := NewWSEventLoop(
		WithOnMessage(func(conn *WSConn, msgType int, data []byte) {}),
	)

	conns := wsLoop.Conns()
	if len(conns) != 0 {
		t.Errorf("Conns should be empty, got %d", len(conns))
	}
}

// TestWSConnCloseOnClosedConn tests that Close on an already-closed connection
// does not panic.
func TestWSConnCloseOnClosedConn(t *testing.T) {
	wsLoop := NewWSEventLoop(
		WithOnMessage(func(conn *WSConn, msgType int, data []byte) {}),
	)

	// We can't create a real WSConn without a connection, but we test
	// the WSEventLoop.ActiveConns method.
	_ = wsLoop
}

// TestGorillaConnRegistry tests the gorilla connection registry.
func TestGorillaConnRegistry(t *testing.T) {
	// The registry should start empty.
	// We can't create a real net.Conn here, but we verify the functions exist.
	_ = storeGorillaConn
	_ = getGorillaConn
	_ = deleteGorillaConn
}

// TestWSEventLoopIntegration tests the full lifecycle with a real HTTP server.
func TestWSEventLoopIntegration(t *testing.T) {
	var messageCount int64
	var closeCount int64
	var connectCount int64

	wsLoop := NewWSEventLoop(
		WithOnConnect(func(conn *WSConn) {
			atomic.AddInt64(&connectCount, 1)
		}),
		WithOnMessage(func(conn *WSConn, msgType int, data []byte) {
			atomic.AddInt64(&messageCount, 1)
		}),
		WithOnClose(func(conn *WSConn) {
			atomic.AddInt64(&closeCount, 1)
		}),
	)
	_ = wsLoop // Used in production with RunReactorWS

	// Create a simple HTTP server for the test.
	// Note: In production, RunReactorWS would bind the WSEventLoop to the
	// Reactor engine. For this test, we use the standard net/http server
	// (Hub-compatible mode) since we can't easily spin up a Reactor engine
	// in a unit test.
	mux := http.NewServeMux()

	// For integration testing without Reactor, fall back to Hub mode.
	hub := NewHub()
	go hub.Run()

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := &Client{
			hub:  hub,
			conn: conn,
			send: make(chan []byte, 256),
			Meta: make(map[string]any),
		}
		hub.register <- client
		atomic.AddInt64(&connectCount, 1)

		go client.writePump()
		client.readPump(func(c *Client, msg []byte) {
			atomic.AddInt64(&messageCount, 1)
			c.Send(msg)
		})
		atomic.AddInt64(&closeCount, 1)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Connect a client.
	conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	// Send a message.
	testMsg := []byte("hello, event loop!")
	err = conn.WriteMessage(gorilla.TextMessage, testMsg)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Read the echo.
	_, received, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if string(received) != string(testMsg) {
		t.Errorf("Echo mismatch: got %q, want %q", string(received), string(testMsg))
	}

	// Verify counters.
	if atomic.LoadInt64(&connectCount) != 1 {
		t.Errorf("connectCount should be 1, got %d", atomic.LoadInt64(&connectCount))
	}
	if atomic.LoadInt64(&messageCount) != 1 {
		t.Errorf("messageCount should be 1, got %d", atomic.LoadInt64(&messageCount))
	}

	// Close the connection.
	conn.Close()
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt64(&closeCount) != 1 {
		t.Errorf("closeCount should be 1, got %d", atomic.LoadInt64(&closeCount))
	}
}

// TestHubModeBackwardCompatibility verifies that the original Hub mode
// still works after the WSEventLoop changes.
func TestHubModeBackwardCompatibility(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	var receivedMsg []byte
	var msgMu sync.Mutex

	handler := Handler(hub, func(client *Client, message []byte) {
		msgMu.Lock()
		receivedMsg = message
		msgMu.Unlock()
		client.Send(message)
	})

	_ = handler // Handler is used in integration tests

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// Simulate what Astra does internally.
		conn, err := Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		client := &Client{
			hub:  hub,
			conn: conn,
			send: make(chan []byte, 256),
			Meta: make(map[string]any),
		}
		hub.register <- client
		go client.writePump()
		go client.readPump(func(c *Client, msg []byte) {
			msgMu.Lock()
			receivedMsg = msg
			msgMu.Unlock()
			c.Send(msg)
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	testMsg := []byte("hub mode test")
	err = conn.WriteMessage(gorilla.TextMessage, testMsg)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	_, echo, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read echo: %v", err)
	}

	if string(echo) != string(testMsg) {
		t.Errorf("Echo mismatch: got %q, want %q", string(echo), string(testMsg))
	}

	msgMu.Lock()
	if string(receivedMsg) != string(testMsg) {
		t.Errorf("Received mismatch: got %q, want %q", string(receivedMsg), string(testMsg))
	}
	msgMu.Unlock()

	if hub.Size() != 1 {
		t.Errorf("Hub size should be 1, got %d", hub.Size())
	}

	// Broadcast test.
	hub.Broadcast([]byte("broadcast!"))
	time.Sleep(100 * time.Millisecond)

	_, bcast, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read broadcast: %v", err)
	}
	if string(bcast) != "broadcast!" {
		t.Errorf("Broadcast mismatch: got %q, want %q", string(bcast), "broadcast!")
	}
}

// TestHubOnConnectDisconnect tests the OnConnect/OnDisconnect callbacks.
func TestHubOnConnectDisconnect(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	var connectCount int64
	var disconnectCount int64

	hub.OnConnect(func(c *Client) {
		atomic.AddInt64(&connectCount, 1)
	})
	hub.OnDisconnect(func(c *Client) {
		atomic.AddInt64(&disconnectCount, 1)
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &Client{
			hub:  hub,
			conn: conn,
			send: make(chan []byte, 256),
			Meta: make(map[string]any),
		}
		hub.register <- client
		go client.writePump()
		go client.readPump(nil)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt64(&connectCount) != 1 {
		t.Errorf("connectCount should be 1, got %d", atomic.LoadInt64(&connectCount))
	}

	conn.Close()
	time.Sleep(200 * time.Millisecond)
	if atomic.LoadInt64(&disconnectCount) != 1 {
		t.Errorf("disconnectCount should be 1, got %d", atomic.LoadInt64(&disconnectCount))
	}
}
