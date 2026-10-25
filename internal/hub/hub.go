// Package hub bridges the game engine, the provider adapters, and the
// WebSocket clients (overlay + dashboard). It owns the single event bus
// goroutine that serializes engine access, caps the in-flight queue, and
// broadcasts normalized events/state to every connected client.
package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"tiklive/internal/engine"
	"tiklive/internal/provider"
)

// WSMessage is a JSON frame sent over a client websocket.
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

// Client is one connected websocket.
type Client struct {
	Conn *websocket.Conn
	Send chan []byte // buffered outbound
	Role string      // "overlay" | "dashboard"
}

// Hub is the central coordinator.
type Hub struct {
	eng *engine.Engine
	bus chan provider.RawEvent // bounded event bus

	mu      sync.Mutex
	clients map[*Client]bool

	// provider management
	adapter   provider.Adapter
	adapterMu sync.Mutex

	// event log (ring) for dashboard monitoring
	logMu  sync.Mutex
	events []WSMessage
	maxLog int

	// status snapshot
	stateMu      sync.Mutex
	viewers      int
	connected    bool
	providerName string
	liveTitle    string
	roomID       string
}

// New creates a hub wired to a fresh engine.
func New(maxQueue int) *Hub {
	h := &Hub{
		bus:     make(chan provider.RawEvent, maxQueue),
		clients: make(map[*Client]bool),
		maxLog:  500,
	}
	h.eng = engine.New(engine.Callbacks{
		Emit: func(ev engine.Event) { h.broadcastEngineEvent(ev) },
		OnWinner: func(winner *engine.Player, scores []engine.LeaderRow) {
			h.broadcast(WSMessage{Type: "winner", Data: map[string]interface{}{
				"winner": winner,
				"scores": scores,
			}})
		},
	})
	return h
}

func (h *Hub) Engine() *engine.Engine { return h.eng }

// ---------- client registration ----------

func (h *Hub) AddClient(c *Client) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	log.Printf("[hub] client connected (%s), total=%d", c.Role, len(h.clients))
}

func (h *Hub) RemoveClient(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.Send)
	}
	h.mu.Unlock()
	log.Printf("[hub] client disconnected (%s), total=%d", c.Role, len(h.clients))
}

// BroadcastState pushes the current snapshot + a fresh leaderboard to all.
func (h *Hub) BroadcastState() {
	st := h.StateSnapshot()
	h.broadcast(WSMessage{Type: "state", Data: st})
	h.broadcast(WSMessage{Type: "leaderboard", Data: h.eng.Leaderboard()})
}

// BroadcastStatus pushes provider/connection status.
func (h *Hub) BroadcastStatus() {
	h.adapterMu.Lock()
	adapter := h.adapter
	h.adapterMu.Unlock()
	st := map[string]interface{}{
		"provider":  "mock",
		"connected": false,
		"error":     "",
	}
	if adapter != nil {
		st = adapter.Status()
	}
	h.broadcast(WSMessage{Type: "status", Data: st})
}

// StateSnapshot builds the current authoritative state for clients.
func (h *Hub) StateSnapshot() map[string]interface{} {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	q := h.eng.Question()
	return map[string]interface{}{
		"game":       h.eng.Phase(),
		"mode":       "math",
		"question":   q,
		"time_left":  h.engTimeLeft(),
		"round":      h.eng.Round(),
		"round_max":  h.eng.RoundMax(),
		"viewers":    h.viewers,
		"connected":  h.connected,
		"provider":   h.providerName,
		"live_title": h.liveTitle,
		"room_id":    h.roomID,
		"scores":     h.eng.Leaderboard(),
	}
}

// ---------- event bus ----------

// Bus returns the ingestion channel for provider events.
func (h *Hub) Bus() chan<- provider.RawEvent { return h.bus }

// Run consumes the event bus and dispatches to the engine until stop.
func (h *Hub) Run(stop <-chan struct{}) {
	defer h.StopProvider() // clean teardown when the bus consumer exits
	for {
		select {
		case <-stop:
			return
		case ev, ok := <-h.bus:
			if !ok {
				return
			}
			h.handleEvent(ev)
		}
	}
}

func (h *Hub) handleEvent(ev provider.RawEvent) {
	// Forward to engine for game-relevant events.
	switch ev.Kind {
	case "comment", "gift":
		h.eng.HandleEvent(engine.Input{
			Kind:      ev.Kind,
			Username:  ev.Username,
			Text:      ev.Text,
			GiftValue: ev.GiftValue,
			Viewers:   ev.Viewers,
		})
	}
	// Track viewer count / connection for state broadcast.
	h.stateMu.Lock()
	switch ev.Kind {
	case "viewers":
		h.viewers = ev.Viewers
	case "connect":
		h.connected = true
		h.roomID = ev.Username
		h.providerName = "tiktok"
	case "disconnect":
		h.connected = false
	}
	h.stateMu.Unlock()

	// Log and broadcast a client-facing event envelope.
	msg := WSMessage{Type: "event", Data: ev}
	h.logEvent(msg)
	h.broadcast(msg)
	if ev.Kind == "viewers" || ev.Kind == "connect" || ev.Kind == "disconnect" {
		h.broadcast(WSMessage{Type: "state", Data: h.StateSnapshot()})
	}
}

// broadcastEngineEvent fans out an engine-generated event to clients.
func (h *Hub) broadcastEngineEvent(ev engine.Event) {
	// Expose engine events as typed websocket messages.
	msg := WSMessage{Type: ev.Type, Data: ev.Data}
	h.broadcast(msg)
}

func (h *Hub) logEvent(msg WSMessage) {
	h.logMu.Lock()
	h.events = append(h.events, msg)
	if len(h.events) > h.maxLog {
		h.events = h.events[len(h.events)-h.maxLog:]
	}
	h.logMu.Unlock()
}

// RecentEvents returns the event log tail for the dashboard.
func (h *Hub) RecentEvents() []WSMessage {
	h.logMu.Lock()
	defer h.logMu.Unlock()
	out := make([]WSMessage, len(h.events))
	copy(out, h.events)
	return out
}

func (h *Hub) engTimeLeft() int { return h.eng.TimeLeft() }

// ---------- broadcast ----------

func (h *Hub) broadcast(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.Lock()
	for c := range h.clients {
		select {
		case c.Send <- data:
		default:
			// Slow client: drop it rather than block the hub.
			log.Printf("[hub] dropping slow client %s", c.Role)
			delete(h.clients, c)
			close(c.Send)
		}
	}
	h.mu.Unlock()
}

// StopProvider tears down the active adapter.
func (h *Hub) StopProvider() {
	h.adapterMu.Lock()
	a := h.adapter
	h.adapter = nil
	h.adapterMu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// SetProvider swaps the active adapter and wires a forwarder from its event
// channel into the hub bus. Any previously running adapter is stopped first.
func (h *Hub) SetProvider(a provider.Adapter) {
	h.adapterMu.Lock()
	old := h.adapter
	h.adapter = a
	if a != nil {
		h.stateMu.Lock()
		h.providerName = a.Name()
		h.stateMu.Unlock()
	}
	h.adapterMu.Unlock()
	if old != nil && old != a {
		old.Stop()
	}
	if a != nil {
		go h.forward(a)
	}
}

// forward copies the adapter's events into the hub bus until the adapter's
// channel closes or the hub stops, with bounded non-blocking sends so a full
// bus never stalls the adapter goroutine.
func (h *Hub) forward(a provider.Adapter) {
	for ev := range a.Events() {
		select {
		case h.bus <- ev:
		default:
			log.Printf("[hub] event bus full; dropping event %q from %s", ev.Kind, a.Name())
		}
	}
}

// ---------- helpers ----------

// WritePump / ReadPump for a client connection are defined in server.go to
// keep net dependencies out of this package's core. Provide a small pump here
// for reuse.
func (c *Client) WriteLoop() {
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case msg, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(msg)
			w.Close()
		case <-ping.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
