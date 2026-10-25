package hub

import (
	"testing"
	"time"

	"tiklive/internal/provider"
)

// fakeAdapter drives a RawEvent channel on demand, letting us test the hub's
// ingestion + broadcast without any network.
type fakeAdapter struct {
	evs chan provider.RawEvent
}

func (f *fakeAdapter) Name() string                     { return "fake" }
func (f *fakeAdapter) Start() error                     { return nil }
func (f *fakeAdapter) Stop()                            { close(f.evs) }
func (f *fakeAdapter) Events() <-chan provider.RawEvent { return f.evs }
func (f *fakeAdapter) Status() map[string]any {
	return map[string]any{"type": "fake", "connected": true}
}

// TestHubIngestForwardsToEngine verifies a comment reaches the engine (leaderboard
// gains a player) through the bus. This is the core event pipeline check.
func TestHubIngestToEngine(t *testing.T) {
	h := New(1000)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { defer close(done); h.Run(stop) }()
	defer func() { close(stop); <-done }()

	f := &fakeAdapter{evs: make(chan provider.RawEvent, 10)}
	h.SetProvider(f)

	// Start a game so comments are scored.
	h.Engine().Start()

	// Push a comment into the bus via the fake adapter channel (as the
	// forwarder would).
	f.evs <- provider.RawEvent{Kind: "comment", Username: "Rina", Text: "0"}
	// Let the bus consumer process and the engine advance.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rows := h.Engine().Leaderboard()
		if len(rows) > 0 {
			return // processed
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("komentar tidak sampai ke engine melalui hub")
}

// TestHubChannelDropDoesNotBlock redundant with forward() default semantics.
// (kept as documentation that the forwarder never blocks the adapter)
