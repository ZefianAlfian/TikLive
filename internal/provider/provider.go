// Package provider defines a provider-agnostic adapter interface and ships a
// mock provider (for development without a real LIVE) plus a TikTok adapter
// built on the maintained gotiktoklive fork (steampoweredtaco/gotiktoklive).
//
// The adapter is fully decoupled from game logic: it only emits normalized
// events on an output channel and reports status. Reconnect, de-duplication
// and spike handling live in the server's event pipeline, not the engine.
package provider

// RawEvent is the provider-agnostic event handoff. Adapters produce these;
// the server's normalizer consumes them.
type RawEvent struct {
	Kind      string `json:"kind"` // comment | gift | like | join | viewers
	Username  string `json:"username"`
	Text      string `json:"text,omitempty"`
	GiftName  string `json:"gift_name,omitempty"`
	GiftValue int    `json:"gift_value,omitempty"`
	LikeCount int    `json:"like_count,omitempty"`
	Viewers   int    `json:"viewers,omitempty"`
}

// Adapter is implemented by every event source.
type Adapter interface {
	// Start begins the connection and returns immediately.
	Start() error
	// Stop tears down the connection and closes the Events channel.
	Stop()
	// Name returns "mock" or "tiktok".
	Name() string
	// Status reflects current connection health.
	Status() map[string]any
	// Events returns the normalized event channel.
	Events() <-chan RawEvent
}

// Config is the connection-agnostic adapter configuration.
type Config struct {
	Username     string
	SignerAPIKey string
	// Reconnect attempts before giving up on error (<=0 = infinite).
	MaxReconnect int
	// Output channel capacity (event spike buffer).
	QueueSize int
}
