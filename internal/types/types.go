// Package types defines the shared wire protocol between the Go server,
// the dashboard, and the OBS overlay. Messages are JSON over WebSocket.
package types

import (
	"encoding/json"
	"time"
)

// ---------------------------------------------------------------------------
// Game / session state (pushed to overlay + dashboard)
// ---------------------------------------------------------------------------

type Difficulty string

const (
	DiffEasy   Difficulty = "easy"
	DiffMedium Difficulty = "medium"
	DiffHard   Difficulty = "hard"
)

type GameState string

const (
	StateIdle    GameState = "idle"
	StateRunning GameState = "running"
	StatePaused  GameState = "paused"
	StateEnded   GameState = "ended"
)

// Question is the current math problem shown on the overlay.
type Question struct {
	ID            int64   `json:"id"`
	Text          string  `json:"text"` // e.g. "5 + 11 - 2"
	Answer        float64 `json:"answer"`
	DurationSec   int     `json:"duration_sec"`
	StartedAtUnix int64   `json:"started_at_unix"`
}

// PlayerEntry is one row of the real-time leaderboard.
type PlayerEntry struct {
	Username string  `json:"username"`
	Points   float64 `json:"points"`
	Correct  int     `json:"correct"`
	Wrong    int     `json:"wrong"`
	Streak   int     `json:"streak"`
	Rank     int     `json:"rank"`
	Updated  int64   `json:"updated"`
}

// State is the full authoritative snapshot delivered to clients.
type State struct {
	Game      GameState    `json:"game"`
	Mode      string       `json:"mode"`
	Difficulty Difficulty  `json:"difficulty"`
	Question  *Question    `json:"question,omitempty"`
	TimeLeft  int          `json:"time_left"`
	Round     int          `json:"round"`
	RoundTotal int         `json:"round_total"`
	Winner    *PlayerEntry `json:"winner,omitempty"`
	Scores    []PlayerEntry `json:"scores"`

	// Session progress toward optional gift target
	GiftTarget  int `json:"gift_target"`
	GiftRaised  int `json:"gift_raised"`

	// Provider / connection
	Provider    string `json:"provider"`
	Connected   bool   `json:"connected"`
	RoomID      string `json:"room_id"`
	LiveTitle   string `json:"live_title"`
	ViewerCount int    `json:"viewer_count"`
}

// ---------------------------------------------------------------------------
// Inbound frames (client -> server)
// ---------------------------------------------------------------------------

type ClientMsg struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Dashboard control frames
type SetConfigMsg struct {
	Mode       string     `json:"mode"`
	Difficulty Difficulty `json:"difficulty"`
	TimerSec   int        `json:"timer_sec"`
	MinPoints  float64    `json:"min_points"`
	SpeedBonus bool       `json:"speed_bonus"`
	ComboBonus bool       `json:"combo_bonus"`
	RoundTotal int        `json:"round_total"`
	GiftTarget int        `json:"gift_target"`
}

type SimulateMsg struct {
	Kind     string `json:"kind"` // "comment" | "gift" | "like" | "join"
	Username string `json:"username"`
	Text     string `json:"text,omitempty"`
	Value    int    `json:"value,omitempty"` // gift diamonds / like count
}

// ---------------------------------------------------------------------------
// Outbound frames (server -> clients)
// ---------------------------------------------------------------------------

type ServerMsg struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

// EventMessage is a normalized provider event forwarded to the overlay.
type EventMessage struct {
	Kind       string `json:"kind"` // comment | gift | like | join | viewers
	Username   string `json:"username"`
	Text       string `json:"text,omitempty"`
	GiftName   string `json:"gift_name,omitempty"`
	GiftValue  int    `json:"gift_value,omitempty"`
	Value      int    `json:"value,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

// AnswerResult is broadcast when a player answers correctly/incorrectly.
type AnswerResult struct {
	Username  string  `json:"username"`
	Correct   bool    `json:"correct"`
	Answer    string  `json:"answer"`
	Points    float64 `json:"points"`    // awarded (0 if wrong)
	Bonus     float64 `json:"bonus"`     // speed/combo bonus added
	Streak    int     `json:"streak"`
	QuestionID int64  `json:"question_id"`
	Now       int64   `json:"now"`
}

// ProviderStatus describes adapter health.
type ProviderStatus struct {
	Provider  string `json:"provider"`
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
	Username  string `json:"username"`
	RoomID    string `json:"room_id"`
}

// ---------------------------------------------------------------------------
// Normalized provider event (internal bus, also reusable by tests)
// ---------------------------------------------------------------------------

type ProviderEvent struct {
	Kind      string // comment | gift | like | join | viewers | disconnect
	Username  string
	Text      string
	GiftName  string
	GiftValue int // diamonds
	LikeCount int
	Viewers   int
	Timestamp time.Time
}

// RawProviderEvent is the adapter -> normalizer contract.
type RawProviderEvent struct {
	Kind      string      `json:"kind"`
	Username  string      `json:"username"`
	Text      string      `json:"text,omitempty"`
	GiftName  string      `json:"gift_name,omitempty"`
	GiftValue int         `json:"gift_value,omitempty"`
	LikeCount int         `json:"like_count,omitempty"`
	Viewers   int         `json:"viewers,omitempty"`
	Raw       interface{} `json:"-"`
}
