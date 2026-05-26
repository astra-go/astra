// WebSocket example: real-time chat with named rooms.
//
// Connect:  GET /ws?room=<name>   (WebSocket upgrade)
// Rooms:    GET /api/rooms         list rooms and connection counts
//
// Clients send plain text or {"text":"hello"}; messages are broadcast
// to all other clients in the same room.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	"github.com/astra-go/astra/websocket"
)

// ─── Room manager ─────────────────────────────────────────────────────────────

// RoomManager maps room names to their dedicated Hub.
type RoomManager struct {
	mu    sync.RWMutex
	rooms map[string]*websocket.Hub
}

func NewRoomManager() *RoomManager {
	return &RoomManager{rooms: make(map[string]*websocket.Hub)}
}

// GetOrCreate returns the existing hub for a room or creates a new one.
func (rm *RoomManager) GetOrCreate(name string) *websocket.Hub {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	h, ok := rm.rooms[name]
	if !ok {
		h = websocket.NewHub()
		h.OnConnect(func(cl *websocket.Client) {
			cl.Meta["room"] = name
			h.BroadcastJSON(astra.Map{"event": "join", "room": name, "size": h.Size()})
		})
		h.OnDisconnect(func(_ *websocket.Client) {
			h.BroadcastJSON(astra.Map{"event": "leave", "room": name, "size": h.Size()})
		})
		go h.Run()
		rm.rooms[name] = h
	}
	return h
}

func (rm *RoomManager) Snapshot() map[string]int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	out := make(map[string]int, len(rm.rooms))
	for name, h := range rm.rooms {
		out[name] = h.Size()
	}
	return out
}

// ─── Chat message ──────────────────────────────────────────────────────────────

type ChatMsg struct {
	Event string `json:"event"`
	Room  string `json:"room"`
	Text  string `json:"text"`
	At    string `json:"at"`
}

func roomHandler(hub *websocket.Hub, room string) websocket.MessageHandler {
	return func(_ *websocket.Client, raw []byte) {
		var payload struct {
			Text string `json:"text"`
		}
		text := string(raw)
		if err := json.Unmarshal(raw, &payload); err == nil && payload.Text != "" {
			text = payload.Text
		}
		hub.BroadcastJSON(ChatMsg{
			Event: "message",
			Room:  room,
			Text:  text,
			At:    time.Now().Format(time.RFC3339),
		})
	}
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	app := astra.New(astra.WithShutdownTimeout(10))
	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS(),
	)

	rooms := NewRoomManager()

	// Upgrade HTTP → WebSocket and join the named room.
	app.GET("/ws", func(c *astra.Ctx) error {
		room := c.DefaultQuery("room", "general")
		hub := rooms.GetOrCreate(room)
		return websocket.Handler(hub, roomHandler(hub, room))(c)
	})

	// List active rooms and their client counts.
	app.GET("/api/rooms", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"rooms": rooms.Snapshot()})
	})

	fmt.Println("WebSocket chat server :8080")
	fmt.Println("  ws://localhost:8080/ws?room=general")
	fmt.Println("  GET http://localhost:8080/api/rooms")
	if err := app.Run(":8080"); err != nil {
		panic(err)
	}
}
