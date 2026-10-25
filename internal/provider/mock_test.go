package provider

import (
	"testing"
	"time"
)

func drainWithTimeout(t *testing.T, evs <-chan RawEvent, n int, d time.Duration) []RawEvent {
	t.Helper()
	out := make([]RawEvent, 0, n)
	deadline := time.After(d)
	for len(out) < n {
		select {
		case ev, ok := <-evs:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
	return out
}

func TestMockStartEmitsComments(t *testing.T) {
	m := NewMock(Config{QueueSize: 500})
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	evs := drainWithTimeout(t, m.Events(), 3, 6*time.Second)
	if len(evs) < 3 {
		t.Fatalf("mock tidak memproduksi cukup event, dapat %d", len(evs))
	}
	kinds := map[string]int{}
	for _, e := range evs {
		kinds[e.Kind]++
	}
	if kinds["comment"] == 0 {
		t.Fatalf("mock tidak mengirim komentar: %v", kinds)
	}
	st := m.Status()
	if st["connected"] != true {
		t.Fatalf("mock harus connected setelah Start: %v", st)
	}
}

func TestMockStartIdempotent(t *testing.T) {
	m := NewMock(Config{QueueSize: 100})
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal("Start kedua harus no-op, bukan error:", err)
	}
	m.Stop()
	m.Stop() // idempotent
}

func TestMockEmitsGift(t *testing.T) {
	m := NewMock(Config{QueueSize: 500})
	m.Start()
	defer m.Stop()
	evs := drainWithTimeout(t, m.Events(), 40, 10*time.Second)
	found := false
	for _, e := range evs {
		if e.Kind == "gift" && e.GiftName != "" && e.GiftValue > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("mock tidak mengirim gift lengkap")
	}
}
