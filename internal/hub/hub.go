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

	// cmdCh serializes all engine commands (start/pause/resume/end/config)
	// onto the bus consumer goroutine. The engine is NEVER touched from
	// other goroutines — this prevents data races that previously stalled
	// the round timer.
	cmdCh chan func()

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
		cmdCh:   make(chan func(), 64),
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

	// Immediately sync the new (or reconnecting) client with the current
	// state. Without this, an overlay that reconnects after an OBS
	// throttle/network blip would sit on stale data until the next event.
	var st map[string]interface{}
	var lb []engine.LeaderRow
	h.Submit(func() {
		st = h.stateSnapshotOnBus()
		lb = h.eng.Leaderboard()
	})
	if st != nil {
		h.sendTo(c, WSMessage{Type: "state", Data: st})
	}
	if lb != nil {
		h.sendTo(c, WSMessage{Type: "leaderboard", Data: lb})
	}
}

// sendTo delivers one message to a single client without ever blocking the
// hub; a slow/full client is dropped instead.
func (h *Hub) sendTo(c *Client, msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case c.Send <- data:
	default:
		log.Printf("[hub] dropping slow client %s", c.Role)
		h.mu.Lock()
		if _, ok := h.clients[c]; ok {
			delete(h.clients, c)
			close(c.Send)
		}
		h.mu.Unlock()
	}
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
	// Read the engine state on the bus goroutine to stay race-free.
	var st map[string]interface{}
	var lb []engine.LeaderRow
	h.Submit(func() {
		st = h.stateSnapshotOnBus()
		lb = h.eng.Leaderboard()
	})
	if st != nil {
		h.broadcast(WSMessage{Type: "state", Data: st})
	}
	if lb != nil {
		h.broadcast(WSMessage{Type: "leaderboard", Data: lb})
	}
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
// Safe to call from any goroutine: engine reads are funneled through Submit.
func (h *Hub) StateSnapshot() map[string]interface{} {
	var st map[string]interface{}
	h.Submit(func() { st = h.stateSnapshotOnBus() })
	return st
}

// stateSnapshotOnBus reads the engine state; MUST be called on the bus
// consumer goroutine (inside Submit).
func (h *Hub) stateSnapshotOnBus() map[string]interface{} {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	return map[string]interface{}{
		"game":       h.eng.Phase(),
		"mode":       "math",
		"question":   h.eng.Question(),
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

// Run consumes the event bus and drives the engine's 1-second countdown from
// the SAME goroutine, so the engine is never accessed concurrently. All
// engine commands (start/pause/end/config) are funneled through cmdCh here.
func (h *Hub) Run(stop <-chan struct{}) {
	defer h.StopProvider() // clean teardown when the bus consumer exits
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	stateTick := time.NewTicker(5 * time.Second)
	defer stateTick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			h.eng.Tick()
		case <-stateTick.C:
			// Self-healing heartbeat: periodically rebroadcast the full
			// state so any client that missed frames (OBS throttling,
			// brief disconnect, dropped frames) resyncs automatically
			// without requiring a manual refresh.
			h.broadcast(WSMessage{Type: "state", Data: h.stateSnapshotOnBus()})
			h.broadcast(WSMessage{Type: "leaderboard", Data: h.eng.Leaderboard()})
		case ev, ok := <-h.bus:
			if !ok {
				return
			}
			h.handleEvent(ev)
		case cmd := <-h.cmdCh:
			cmd()
		}
	}
}

// Submit runs fn on the bus consumer goroutine (serialized with all other
// engine access) and waits for it to finish. Returns false if the hub is
// shutting down.
func (h *Hub) Submit(fn func()) bool {
	done := make(chan struct{})
	select {
	case h.cmdCh <- func() { fn(); close(done) }:
		select {
		case <-done:
			return true
		case <-time.After(3 * time.Second):
			log.Printf("[hub] Submit TIMEOUT menunggu bus consumer (cmdCh busy)")
			return false
		}
	default:
		log.Printf("[hub] Submit DROP — cmdCh penuh (len=%d)", len(h.cmdCh))
		return false
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
		// NOTE: we are ON the bus goroutine here — must NOT call
		// StateSnapshot() (which Submits & waits → self-deadlock). Use the
		// direct on-bus variant.
		h.broadcast(WSMessage{Type: "state", Data: h.stateSnapshotOnBus()})
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
