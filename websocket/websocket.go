// Package websocket provides WebSocket support for Astra.
//
// It offers two modes:
//
//  1. Hub mode (gorilla/websocket compatible): simple Hub/Client pattern with
//     two goroutines per connection (readPump + writePump). Suitable for <10K
//     concurrent connections. Use NewHub() + Handler().
//
//  2. EventLoop mode (Reactor-integrated): WebSocket connections are parked in
//     the Reactor engine's epoll/kqueue event loops, consuming zero goroutines
//     when idle. Supports 100K+ concurrent connections with a fixed worker pool.
//     Use WSEventLoop with app.RunReactorWS().
//
// Quick start — Hub mode:
//
//	hub := websocket.NewHub()
//	go hub.Run()
//	app.GET("/ws", websocket.Handler(hub, func(client *websocket.Client, msg []byte) {
//	    hub.Broadcast(msg)
//	}))
//
// Quick start — EventLoop mode:
//
//	wsLoop := websocket.NewWSEventLoop(
//	    websocket.WithOnMessage(func(conn *websocket.WSConn, msgType int, data []byte) {
//	        conn.WriteMessage(msgType, data) // echo
//	    }),
//	    websocket.WithOnClose(func(conn *websocket.WSConn) {
//	        slog.Info("client disconnected", "meta", conn.Meta)
//	    }),
//	)
//	app.GET("/ws", wsLoop.Handler())
//	app.RunReactorWS(":8080", wsLoop)
package websocket

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/astra-go/astra"
	netengine "github.com/astra-go/astra/netengine"
	gorilla "github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024 // 512KB
)

// Upgrader is the default WebSocket upgrader.
// SECURITY: By default, CheckOrigin requires same-origin policy to prevent
// Cross-Site WebSocket Hijacking (CSWSH). To allow all origins (NOT recommended
// for production), explicitly set websocket.Upgrader.CheckOrigin = func(r *http.Request) bool { return true }
var Upgrader = gorilla.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Default: same-origin policy to prevent CSWSH
		// Compare Origin header with Host header
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Non-browser clients (curl, CLI tools) don't send Origin.
			// Reject by default to prevent CSWSH. For non-browser clients,
			// use a custom Upgrader with a permissive CheckOrigin or add
			// a pre-shared token to the upgrade request.
			return false
		}
		// Parse origin and compare with host
		originURL, err := url.Parse(origin)
		if err != nil {
			return false
		}
		host := r.Host
		// Allow same-origin requests
		return originURL.Host == host
	},
}

// Message is a structured WebSocket message with an optional event type.
type Message struct {
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data"`
	from  *Client
}

func init() {
	// Register the gorilla/websocket reader/writer with netengine so
	// WSEventLoop can read/write WebSocket frames without importing gorilla.
	netengine.RegisterWSReader(gorillaReadMessage)
	netengine.RegisterWSWriter(gorillaWriteMessage)
}

// gorillaReadMessage reads one WebSocket message using gorilla/websocket.
func gorillaReadMessage(nc net.Conn) (msgType int, data []byte, err error) {
	// The net.Conn passed here is the raw connection that was upgraded.
	// We need the gorilla.Conn to read WebSocket frames. However, after
	// upgrade the gorilla.Conn wraps the net.Conn, and we don't have a
	// way to get it back from the net.Conn alone.
	//
	// Instead, we use a sync.Map to store gorilla.Conn by net.Conn pointer.
	conn := getGorillaConn(nc)
	if conn == nil {
		return 0, nil, errors.New("websocket: gorilla connection not found")
	}
	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	msgType, data, err = conn.ReadMessage()
	conn.SetReadDeadline(time.Time{}) // clear deadline after read
	return
}

// gorillaWriteMessage writes one WebSocket message using gorilla/websocket.
func gorillaWriteMessage(nc net.Conn, msgType int, data []byte) error {
	conn := getGorillaConn(nc)
	if conn == nil {
		return errors.New("websocket: gorilla connection not found")
	}
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	err := conn.WriteMessage(msgType, data)
	conn.SetWriteDeadline(time.Time{})
	return err
}

// ─── gorilla.Conn registry ──────────────────────────────────────────────────
//
// After upgrading an HTTP connection to WebSocket, the gorilla.Conn wraps the
// underlying net.Conn.  The Reactor engine only knows about the net.Conn (via
// its file descriptor), so we need a way to look up the gorilla.Conn when the
// poller fires a readable event.

var gorillaConnRegistry sync.Map // *net.Conn (pointer) → *gorilla.Conn

func storeGorillaConn(nc net.Conn, gc *gorilla.Conn) {
	gorillaConnRegistry.Store(nc, gc)
}

func getGorillaConn(nc net.Conn) *gorilla.Conn {
	val, ok := gorillaConnRegistry.Load(nc)
	if !ok {
		return nil
	}
	return val.(*gorilla.Conn)
}

func deleteGorillaConn(nc net.Conn) {
	gorillaConnRegistry.Delete(nc)
}

// ─── Hub mode (original, goroutine-per-connection) ──────────────────────────

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
// SECURITY: MaxClients limits the number of concurrent connections.
// When MaxClients > 0 and limit is reached, new connections are rejected
// with HTTP 503 Service Unavailable.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex

	MaxClients   int           // Maximum concurrent clients (0 = unlimited)
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

// WithMaxClients sets the maximum number of concurrent connections.
// When the limit is reached, new connections receive HTTP 503.
// Recommended for production: set to a reasonable limit based on your server capacity.
func (h *Hub) WithMaxClients(max int) *Hub {
	h.MaxClients = max
	return h
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
			// Check MaxClients limit
			if h.MaxClients > 0 && len(h.clients) >= h.MaxClients {
				h.mu.Unlock()
				// Reject connection - client will be unregistered
				client.mu.Lock()
				client.closed = true
				client.mu.Unlock()
				client.conn.WriteMessage(gorilla.CloseMessage,
					gorilla.FormatCloseMessage(gorilla.CloseTryAgainLater, "server at capacity"))
				client.conn.Close()
				slog.Warn("websocket: rejected connection, max clients reached",
					slog.Int("max", h.MaxClients))
				continue
			}
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

// ─── Astra Handler (Hub mode) ────────────────────────────────────────────────

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

// ─── EventLoop mode (Reactor-integrated) ────────────────────────────────────
//
// WSEventLoop provides a Reactor-engine-integrated WebSocket event loop that
// eliminates the goroutine-per-connection bottleneck. Instead of spawning two
// goroutines (readPump + writePump) per WebSocket client, connections are
// parked in epoll/kqueue event loops and only consume a worker goroutine when
// data arrives.
//
// Scaling comparison:
//
//	Mode         Goroutines per conn   100K conns total goroutines
//	Hub mode     2                     200,000+
//	EventLoop    0 (idle)              ~32 (worker pool)
//
// Usage:
//
//	wsLoop := websocket.NewWSEventLoop(
//	    websocket.WithOnMessage(func(conn *websocket.WSConn, msgType int, data []byte) {
//	        // Echo back
//	        conn.WriteMessage(msgType, data)
//	    }),
//	    websocket.WithOnClose(func(conn *websocket.WSConn) {
//	        slog.Info("client disconnected")
//	    }),
//	)
//	app.GET("/ws", wsLoop.Handler())
//	app.RunReactorWS(":8080", wsLoop)

// WSEventLoopConfig holds configuration for the WebSocket event loop.
type WSEventLoopConfig struct {
	// Upgrader is the gorilla/websocket upgrader used for HTTP→WS upgrade.
	// Defaults to the package-level Upgrader if nil.
	Upgrader *gorilla.Upgrader

	// OnMessage is called when a WebSocket message arrives.
	// Runs in a worker pool goroutine — must not block.
	OnMessage func(conn *WSConn, msgType int, data []byte)

	// OnError is called when a read error occurs.
	// After this call the connection is automatically unregistered.
	OnError func(conn *WSConn, err error)

	// OnClose is called after a connection is fully cleaned up.
	OnClose func(conn *WSConn)

	// OnConnect is called after a new connection is successfully registered
	// with the event loop.
	OnConnect func(conn *WSConn)
}

// WSEventLoopOption is a functional option for WSEventLoopConfig.
type WSEventLoopOption func(*WSEventLoopConfig)

// WithUpgrader sets a custom WebSocket upgrader.
func WithUpgrader(u *gorilla.Upgrader) WSEventLoopOption {
	return func(c *WSEventLoopConfig) { c.Upgrader = u }
}

// WithOnMessage sets the message callback.
func WithOnMessage(fn func(conn *WSConn, msgType int, data []byte)) WSEventLoopOption {
	return func(c *WSEventLoopConfig) { c.OnMessage = fn }
}

// WithOnError sets the error callback.
func WithOnError(fn func(conn *WSConn, err error)) WSEventLoopOption {
	return func(c *WSEventLoopConfig) { c.OnError = fn }
}

// WithOnClose sets the close callback.
func WithOnClose(fn func(conn *WSConn)) WSEventLoopOption {
	return func(c *WSEventLoopConfig) { c.OnClose = fn }
}

// WithOnConnect sets the connect callback.
func WithOnConnect(fn func(conn *WSConn)) WSEventLoopOption {
	return func(c *WSEventLoopConfig) { c.OnConnect = fn }
}

// WSConn represents a WebSocket connection managed by the Reactor engine's
// event loop. It wraps netengine.WSConn and provides WebSocket-specific
// methods like WriteJSON.
type WSConn struct {
	*netengine.WSConn
	gorillaConn *gorilla.Conn
	netConn     net.Conn
	wsLoop      *WSEventLoop
	writeMu     sync.Mutex
}

// WriteJSON marshals v to JSON and writes it as a text message.
// Safe to call from any goroutine.
func (c *WSConn) WriteJSON(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.IsClosed() {
		return errors.New("websocket: connection closed")
	}
	c.gorillaConn.SetWriteDeadline(time.Now().Add(writeWait))
	err := c.gorillaConn.WriteJSON(v)
	c.gorillaConn.SetWriteDeadline(time.Time{})
	return err
}

// WriteMessage writes a WebSocket message with the given type.
// Safe to call from any goroutine.
func (c *WSConn) WriteMessage(msgType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.IsClosed() {
		return errors.New("websocket: connection closed")
	}
	c.gorillaConn.SetWriteDeadline(time.Now().Add(writeWait))
	err := c.gorillaConn.WriteMessage(msgType, data)
	c.gorillaConn.SetWriteDeadline(time.Time{})
	return err
}

// Close sends a close frame and unregisters the connection.
func (c *WSConn) Close() {
	// Send a clean close frame before unregistering.
	c.writeMu.Lock()
	if !c.IsClosed() {
		c.gorillaConn.SetWriteDeadline(time.Now().Add(writeWait))
		c.gorillaConn.WriteMessage(gorilla.CloseMessage,
			gorilla.FormatCloseMessage(gorilla.CloseNormalClosure, "")) //nolint:errcheck
		c.gorillaConn.SetWriteDeadline(time.Time{})
	}
	c.writeMu.Unlock()
	c.WSConn.Close()
}

// WSEventLoop manages WebSocket connections integrated with the Reactor engine.
type WSEventLoop struct {
	cfg      WSEventLoopConfig
	engineWS *netengine.WSEventLoop

	// connMap stores WSConn by netengine.WSConn pointer for callback dispatch.
	connMap sync.Map // *netengine.WSConn → *WSConn

	// activeConns tracks the number of active connections for metrics.
	activeConns int64

	// hub for broadcasting (optional).
	hubMu    sync.RWMutex
	hubConns map[*WSConn]bool
}

// NewWSEventLoop creates a new WebSocket event loop with the given options.
func NewWSEventLoop(opts ...WSEventLoopOption) *WSEventLoop {
	cfg := WSEventLoopConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &WSEventLoop{
		cfg:      cfg,
		hubConns: make(map[*WSConn]bool),
	}
}

// RegisterEngine binds this WSEventLoop to a netengine.Engine.
// Called internally by app.RunReactorWS().
func (w *WSEventLoop) RegisterEngine(e *netengine.Engine) {
	w.engineWS = e.WS()
	if w.engineWS == nil {
		e.EnableWS(
			w.onNetengineMessage,
			w.onNetengineError,
			w.onNetengineClose,
		)
		w.engineWS = e.WS()
	}
}

// onNetengineMessage is the callback invoked by netengine when a WebSocket
// message arrives.  It looks up the associated WSConn and dispatches to the
// application's OnMessage callback.
func (w *WSEventLoop) onNetengineMessage(conn *netengine.WSConn, msgType int, data []byte) {
	val, ok := w.connMap.Load(conn)
	if !ok {
		return
	}
	wsConn := val.(*WSConn)

	if w.cfg.OnMessage != nil {
		w.cfg.OnMessage(wsConn, msgType, data)
	}
}

func (w *WSEventLoop) onNetengineError(conn *netengine.WSConn, err error) {
	val, ok := w.connMap.Load(conn)
	if !ok {
		return
	}
	wsConn := val.(*WSConn)

	if w.cfg.OnError != nil {
		w.cfg.OnError(wsConn, err)
	}
}

func (w *WSEventLoop) onNetengineClose(conn *netengine.WSConn) {
	val, ok := w.connMap.Load(conn)
	if !ok {
		return
	}
	wsConn := val.(*WSConn)

	// Clean up gorilla conn registry.
	deleteGorillaConn(wsConn.netConn)

	// Remove from broadcast hub.
	w.hubMu.Lock()
	delete(w.hubConns, wsConn)
	w.hubMu.Unlock()

	atomic.AddInt64(&w.activeConns, -1)

	if w.cfg.OnClose != nil {
		w.cfg.OnClose(wsConn)
	}
}

// Handler returns an Astra HandlerFunc that upgrades HTTP connections to WebSocket
// and registers them with the WSEventLoop.  Must be used with app.RunReactorWS().
func (w *WSEventLoop) Handler() astra.HandlerFunc {
	return w.HandlerWithUpgrader(nil)
}

// HandlerWithUpgrader returns a handler using a custom upgrader.
func (w *WSEventLoop) HandlerWithUpgrader(upgrader *gorilla.Upgrader) astra.HandlerFunc {
	if upgrader == nil {
		u := Upgrader // use package-level default
		upgrader = &u
	}

	return func(c *astra.Ctx) error {
		// We need to hijack the connection to get the raw net.Conn.
		// This only works with net/http server, not the Reactor engine's
		// bufResponseWriter. For Reactor mode, we use a different approach:
		// the upgrader.Upgrade() call gets the underlying connection via
		// http.Hijacker.

		conn, err := upgrader.Upgrade(c.Writer(), c.Request(), nil)
		if err != nil {
			return astra.NewHTTPError(http.StatusBadRequest, "websocket upgrade failed: "+err.Error())
		}

		// Get the underlying net.Conn from gorilla.
		nc := conn.UnderlyingConn()

		// Register the gorilla.Conn so the Reactor reader can find it.
		storeGorillaConn(nc, conn)

		// Register with the netengine WSEventLoop.
		neConn, err := w.engineWS.Register(nc)
		if err != nil {
			conn.Close()
			return astra.NewHTTPError(http.StatusInternalServerError, "websocket event loop registration failed: "+err.Error())
		}

		// Create our WSConn wrapper.
		wsConn := &WSConn{
			WSConn:      neConn,
			gorillaConn: conn,
			netConn:     nc,
			wsLoop:      w,
		}

		// Store in our lookup map.
		w.connMap.Store(neConn, wsConn)

		// Add to broadcast hub.
		w.hubMu.Lock()
		w.hubConns[wsConn] = true
		w.hubMu.Unlock()

		atomic.AddInt64(&w.activeConns, 1)

		if w.cfg.OnConnect != nil {
			w.cfg.OnConnect(wsConn)
		}

		return nil
	}
}

// Broadcast sends a message to all active WebSocket connections.
func (w *WSEventLoop) Broadcast(msgType int, data []byte) {
	w.hubMu.RLock()
	defer w.hubMu.RUnlock()
	for conn := range w.hubConns {
		conn.WriteMessage(msgType, data) //nolint:errcheck
	}
}

// BroadcastJSON marshals v to JSON and broadcasts it as a text message.
func (w *WSEventLoop) BroadcastJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.Broadcast(gorilla.TextMessage, b)
	return nil
}

// ActiveConns returns the number of active WebSocket connections.
func (w *WSEventLoop) ActiveConns() int64 {
	return atomic.LoadInt64(&w.activeConns)
}

// Conns returns a snapshot of all active connections.
func (w *WSEventLoop) Conns() []*WSConn {
	w.hubMu.RLock()
	defer w.hubMu.RUnlock()
	conns := make([]*WSConn, 0, len(w.hubConns))
	for conn := range w.hubConns {
		conns = append(conns, conn)
	}
	return conns
}
