package provider

import (
	"fmt"
	"log"
	"time"

	gotiktoklive "github.com/steampoweredtaco/gotiktoklive"
)

// tiktokAdapter wraps the maintained gotiktoklive fork with supervised
// lifecycle. The library emits a DisconnectEvent when the connection drops
// (an always-emitted teardown marker), after which the Live must be closed
// and a fresh TrackUser issued. This adapter owns that reconnect loop and
// translates library events into normalized RawEvents.
type tiktokAdapter struct {
	cfg    Config
	events chan RawEvent // adapter outlives connections; never closed

	stopCh chan struct{}
	doneCh chan struct{} // closed when run() finishes

	mu   chan struct{} // held while touching cur/conn/err
	cur  *gotiktoklive.Live
	err  string
	conn bool
}

func NewTikTokAdapter(cfg Config) *tiktokAdapter {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}
	return &tiktokAdapter{
		cfg:    cfg,
		events: make(chan RawEvent, cfg.QueueSize),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
		mu:     make(chan struct{}, 1),
	}
}

func (t *tiktokAdapter) Name() string { return "tiktok" }

func (t *tiktokAdapter) Events() <-chan RawEvent { return t.events }

func (t *tiktokAdapter) Start() error {
	if t.cfg.Username == "" {
		return fmt.Errorf("tiktok username kosong")
	}
	select {
	case t.mu <- struct{}{}:
	default:
		return nil // already starting/started
	}
	defer func() { <-t.mu }()
	select {
	case <-t.stopCh:
		return fmt.Errorf("adapter sudah di-stop")
	default:
	}
	go t.run()
	return nil
}

func (t *tiktokAdapter) Stop() {
	t.closeCurrent()
	select {
	case <-t.stopCh:
	default:
		close(t.stopCh)
	}
	<-t.doneCh
}

func (t *tiktokAdapter) Status() map[string]any {
	return map[string]any{
		"type":      "tiktok",
		"connected": t.isConnected(),
		"error":     t.getErr(),
		"username":  t.cfg.Username,
	}
}

// run is the supervision loop: connect -> drain until DisconnectEvent or
// stop -> close -> backoff -> retry.
func (t *tiktokAdapter) run() {
	defer close(t.doneCh)
	retry := 0
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}
		rtk, err := gotiktoklive.NewTikTok(gotiktoklive.SigningApiKey(t.cfg.SignerAPIKey))
		if err != nil {
			t.setErr(fmt.Errorf("NewTikTok: %w", err))
			if !t.backoff(&retry) {
				return
			}
			continue
		}
		rtk.SetErrorHandler(func(a ...interface{}) {})
		rtk.SetInfoHandler(func(a ...interface{}) {})
		rtk.SetWarnHandler(func(a ...interface{}) {})

		live, err := rtk.TrackUser(t.cfg.Username)
		if err != nil {
			t.setErr(fmt.Errorf("TrackUser: %w", err))
			if !t.backoff(&retry) {
				return
			}
			continue
		}
		retry = 0
		t.setConn(true)
		t.setLive(live)
		t.emit(RawEvent{Kind: "connect", Username: t.cfg.Username})
		t.drain(live)
		t.closeCurrent()
		t.setConn(false)
	}
}

// drain forwards library events until DisconnectEvent or stop requested.
func (t *tiktokAdapter) drain(live *gotiktoklive.Live) {
	for {
		select {
		case <-t.stopCh:
			return
		case ev, ok := <-live.Events:
			if !ok {
				return
			}
			if _, isDisc := ev.(gotiktoklive.DisconnectEvent); isDisc {
				return
			}
			t.translate(live, ev)
		}
	}
}

// translate maps a library event into a normalized RawEvent.
func (t *tiktokAdapter) translate(live *gotiktoklive.Live, ev interface{}) {
	switch e := ev.(type) {
	case gotiktoklive.ChatEvent:
		username := ""
		if e.User != nil {
			username = e.User.Username
		}
		t.emit(RawEvent{Kind: "comment", Username: username, Text: e.Comment})
	case gotiktoklive.GiftEvent:
		username := ""
		if e.User != nil {
			username = e.User.Username
		}
		// Dedupe streakable gifts: only the final streaked message counts.
		if e.Type == 1 && !e.RepeatEnd {
			return
		}
		t.emit(RawEvent{Kind: "gift", Username: username, GiftName: e.Name, GiftValue: e.Diamonds})
	case gotiktoklive.LikeEvent:
		username := ""
		if e.User != nil {
			username = e.User.Username
		}
		t.emit(RawEvent{Kind: "like", Username: username, LikeCount: e.Likes})
	case gotiktoklive.UserEvent:
		username := ""
		if e.User != nil {
			username = e.User.Username
		}
		kind := "join"
		switch e.Event {
		case gotiktoklive.USER_SHARE:
			kind = "share"
		case gotiktoklive.USER_FOLLOW:
			kind = "follow"
		}
		t.emit(RawEvent{Kind: kind, Username: username})
	case gotiktoklive.ViewersEvent:
		t.emit(RawEvent{Kind: "viewers", Viewers: e.Viewers})
	case gotiktoklive.RoomEvent:
		t.emit(RawEvent{Kind: "room", Text: e.Message})
	}
}

func (t *tiktokAdapter) emit(e RawEvent) {
	select {
	case <-t.stopCh:
		return
	case t.events <- e:
	}
}

func (t *tiktokAdapter) closeCurrent() {
	t.mu <- struct{}{}
	if t.cur != nil {
		t.cur.Close()
		t.cur = nil
	}
	<-t.mu
}

func (t *tiktokAdapter) setLive(l *gotiktoklive.Live) {
	t.mu <- struct{}{}
	t.cur = l
	<-t.mu
}

func (t *tiktokAdapter) setConn(v bool) {
	t.mu <- struct{}{}
	t.conn = v
	<-t.mu
}

func (t *tiktokAdapter) isConnected() bool {
	t.mu <- struct{}{}
	defer func() { <-t.mu }()
	return t.conn
}

func (t *tiktokAdapter) getErr() string {
	t.mu <- struct{}{}
	defer func() { <-t.mu }()
	return t.err
}

func (t *tiktokAdapter) setErr(err error) {
	if err != nil {
		t.mu <- struct{}{}
		t.err = err.Error()
		<-t.mu
		log.Printf("[tiktok] %v", err)
	}
}

// backoff sleeps exponentially (2^n seconds, capped), returning false when the
// retry budget is exhausted or a stop is requested.
func (t *tiktokAdapter) backoff(retry *int) bool {
	*retry++
	d := time.Duration(1<<retryCapped(*retry)) * time.Second
	if t.cfg.MaxReconnect > 0 && *retry > t.cfg.MaxReconnect {
		return false
	}
	select {
	case <-t.stopCh:
		return false
	case <-time.After(d):
		return true
	}
}

func retryCapped(n int) int {
	if n < 1 {
		return 1
	}
	if n > 6 {
		return 6
	}
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
