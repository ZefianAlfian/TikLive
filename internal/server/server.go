// Package server exposes the HTTP + REST + WebSocket front door for the hub.
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"tiklive/internal/config"
	"tiklive/internal/engine"
	"tiklive/internal/hub"
	"tiklive/internal/provider"
)

// Server ties together HTTP + WebSockets and the hub.
type Server struct {
	cfg config.Config
	hub *hub.Hub
	up  *websocket.Upgrader
	dir string // static web root (built frontend)
	mu  sync.Mutex
}

// New builds the HTTP server; staticDir is the web build directory served at /.
func New(cfg config.Config, h *hub.Hub, staticDir string) *Server {
	return &Server{
		cfg: cfg,
		hub: h,
		up: &websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		dir: staticDir,
	}
}

// Routes registers every HTTP route; bind the returned mux as the handler.
func (s *Server) Routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("/ws", s.handleWS)
	m.HandleFunc("/api/control", s.requireAdmin(s.handleControl))
	m.HandleFunc("/api/config", s.requireAdmin(s.handleConfig))
	m.HandleFunc("/api/state", s.handleState)
	m.HandleFunc("/api/events", s.handleEvents)
	m.HandleFunc("/api/simulate", s.requireAdmin(s.handleSimulate))
	fs := http.FileServer(http.Dir(s.dir))
	m.Handle("/", fs)
	return m
}

// admin protection — local binding defaults; token optional.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := s.cfg.AdminToken
		if token != "" && r.Header.Get("X-Admin-Token") != token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// ---------- websocket ----------

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.up.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	role := r.URL.Query().Get("role")
	if role != "dashboard" {
		role = "overlay"
	}
	cl := &hub.Client{Conn: conn, Send: make(chan []byte, 256), Role: role}
	go cl.WriteLoop()
	s.hub.AddClient(cl)
	defer s.hub.RemoveClient(cl)
	// Detect dead connections (OBS suspend/resume, half-open TCP): require a
	// pong every 60s or drop the client. Browsers answer protocol pings
	// automatically, so healthy clients are never affected.
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// ---------- control ----------

type controlReq struct {
	Action string          `json:"action"`
	Config json.RawMessage `json:"config,omitempty"`
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req controlReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	eng := s.hub.Engine()
	switch req.Action {
	case "start":
		s.hub.Submit(func() { eng.Start() })
	case "pause":
		s.hub.Submit(func() { eng.Pause() })
	case "resume":
		s.hub.Submit(func() { eng.Resume() })
	case "end":
		s.hub.Submit(func() { eng.End() })
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	s.hub.BroadcastState()
	writeJSON(w, map[string]string{"ok": "ok"})
}

// handleConfig GET returns current engine config; POST applies a new one.
// Both access the engine only through the hub bus goroutine (race-free).
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	eng := s.hub.Engine()
	switch r.Method {
	case http.MethodGet:
		var cfg engine.Config
		s.hub.Submit(func() { cfg = eng.Config() })
		writeJSON(w, cfg)
	case http.MethodPost:
		var cfg engine.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if cfg.TimerSec <= 0 {
			cfg.TimerSec = 15
		}
		if cfg.BasePoints <= 0 {
			cfg.BasePoints = 8
		}
		if cfg.RoundTotal <= 0 {
			cfg.RoundTotal = 10
		}
		s.hub.Submit(func() { eng.SetConfig(cfg) })
		s.hub.BroadcastState()
		writeJSON(w, map[string]string{"ok": "config applied"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------- read endpoints ----------

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.hub.StateSnapshot())
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.hub.RecentEvents())
}

// ---------- simulation ----------

type simulateReq struct {
	Kind     string `json:"kind"`
	Username string `json:"username"`
	Text     string `json:"text,omitempty"`
	Value    int    `json:"value,omitempty"`
}

func (s *Server) handleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req simulateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Kind == "" {
		req.Kind = "comment"
	}
	if req.Username == "" {
		req.Username = "guest"
	}
	ev := provider.RawEvent{Kind: req.Kind, Username: req.Username, Text: req.Text}
	switch req.Kind {
	case "gift":
		ev.GiftName = req.Text
		ev.GiftValue = req.Value
	case "like":
		ev.LikeCount = req.Value
	case "viewers":
		ev.Viewers = req.Value
	}
	select {
	case s.hub.Bus() <- ev:
		writeJSON(w, map[string]string{"ok": "simulated"})
	default:
		http.Error(w, "event bus full", http.StatusServiceUnavailable)
	}
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// keep imported packages used
var _ = log.Printf
var _ = filepath.Join
var _ = strings.TrimSpace
