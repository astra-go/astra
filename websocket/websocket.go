// Package websocket provides WebSocket support for Astra.
//
// It wraps github.com/gorilla/websocket and provides a Hub/Client
// pattern for managing multiple concurrent connections.
//
// Quick start:
//
//	hub := websocket.NewHub()
//	go hub.Run()
//
//	app.GET("/ws", websocket.Handler(hub, func(client *websocket.Client, msg []byte) {
//	    hub.Broadcast(msg)          // echo to all clients
//	}))
package websocket

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/astra-go/astra"
	gorilla "github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024 // 512KB
)

// Upgrader is the default WebSocket upgrader.
// Override CheckOrigin to restrict origins in production.
var Upgrader = gorilla.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins by default (tighten in production)
	},
}

// Message is a structured WebSocket message with an optional event type.
type Message struct {
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data"`
	from  *Client
}

// Client represents a single WebSocket connection managed by a Hub.
type Client struct {
	hub    *Hub
	conn   *gorilla.Conn
	send   chan []byte
	mu     sync.Mutex
	closed bool
	// Meta holds arbitrary key-value data (e.g. user ID, room).
	Meta map[string]any
}

// Send queues a message to be written to this client.
// Returns false if the client is closed or the buffer is full.
func (c *Client) Send(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		return false
	}
}

// SendJSON marshals v to JSON and queues the message.
func (c *Client) SendJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.Send(b)
	return nil
}

// Close disconnects the client gracefully.
func (c *Client) Close() {
	c.hub.unregister <- c
}

// readPump pumps messages from the WebSocket connection into the hub.
func (c *Client) readPump(handler MessageHandler) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if gorilla.IsUnexpectedCloseError(err,
				gorilla.CloseGoingAway, gorilla.CloseAbnormalClosure) {
				slog.Warn("websocket read error", slog.String("err", err.Error()))
			}
			break
		}
		if handler != nil {
			handler(c, message)
		}
	}
}

// writePump pumps messages from the send channel to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(gorilla.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(gorilla.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Flush pending messages in a single frame
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}
			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(gorilla.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ─── Hub ─────────────────────────────────────────────────────────────────────

// MessageHandler is called for each incoming message from a client.
type MessageHandler func(client *Client, message []byte)

// Hub manages a set of WebSocket clients and routes messages.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex

	onConnect    func(*Client)
	onDisconnect func(*Client)
}

// NewHub creates a new Hub. Call hub.Run() in a goroutine to start it.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
	}
}

// OnConnect registers a callback called when a new client connects.
func (h *Hub) OnConnect(fn func(*Client)) { h.onConnect = fn }

// OnDisconnect registers a callback called when a client disconnects.
func (h *Hub) OnDisconnect(fn func(*Client)) { h.onDisconnect = fn }

// Run processes hub events. Must be called in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			if h.onConnect != nil {
				go h.onConnect(client)
			}

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.mu.Lock()
				if !client.closed {
					client.closed = true
					close(client.send)
				}
				client.mu.Unlock()
			}
			h.mu.Unlock()
			if h.onDisconnect != nil {
				go h.onDisconnect(client)
			}

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				client.Send(message)
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(data []byte) {
	h.broadcast <- data
}

// BroadcastJSON marshals v to JSON and broadcasts it.
func (h *Hub) BroadcastJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.Broadcast(b)
	return nil
}

// Clients returns a snapshot of currently connected clients.
func (h *Hub) Clients() []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	return clients
}

// Size returns the number of currently connected clients.
func (h *Hub) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ─── Astra Handler ────────────────────────────────────────────────────────────

// Handler returns an Astra HandlerFunc that upgrades HTTP connections to WebSocket.
// handler is called for each incoming message; pass nil to ignore messages.
func Handler(hub *Hub, handler MessageHandler) astra.HandlerFunc {
	return HandlerWithUpgrader(hub, handler, Upgrader)
}

// HandlerWithUpgrader returns a handler using a custom upgrader.
func HandlerWithUpgrader(hub *Hub, handler MessageHandler, upgrader gorilla.Upgrader) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		conn, err := upgrader.Upgrade(c.Writer(), c.Request(), nil)
		if err != nil {
			return astra.NewHTTPError(http.StatusBadRequest, "websocket upgrade failed: "+err.Error())
		}

		client := &Client{
			hub:  hub,
			conn: conn,
			send: make(chan []byte, 256),
			Meta: make(map[string]any),
		}

		hub.register <- client

		go client.writePump()
		go client.readPump(handler)

		return nil
	}
}
