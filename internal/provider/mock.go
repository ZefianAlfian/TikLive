package provider

import (
	"math/rand"
	"strconv"
	"sync"
	"time"

	"tiklive/internal/expr"
)

// Mock emits synthetic events (comments answering simple arithmetic, gifts,
// likes, joins, viewer counts) so the whole pipeline — engine, websockets,
// overlay — can be developed and demoed without a real TikTok LIVE.
type Mock struct {
	cfg    Config
	events chan RawEvent
	stopCh chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
	mu     sync.Mutex
	conn   bool
	errMsg string
}

// RawEvent is defined in provider.go (canonical, shared by all adapters).

func NewMock(cfg Config) *Mock {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}
	return &Mock{
		cfg:    cfg,
		events: make(chan RawEvent, cfg.QueueSize),
		stopCh: make(chan struct{}),
	}
}

func (m *Mock) Name() string            { return "mock" }
func (m *Mock) Events() <-chan RawEvent { return m.events }

func (m *Mock) Start() error {
	m.mu.Lock()
	if m.conn {
		m.mu.Unlock()
		return nil
	}
	m.conn = true
	m.errMsg = ""
	m.mu.Unlock()
	m.wg.Add(3)
	go m.commentLoop()
	go m.likeJoinLoop()
	go m.viewerLoop()
	return nil
}

func (m *Mock) Stop() {
	m.once.Do(func() {
		close(m.stopCh)
		m.wg.Wait()
		m.mu.Lock()
		m.conn = false
		m.mu.Unlock()
		close(m.events)
	})
}

func (m *Mock) Status() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]any{
		"type":      "mock",
		"connected": m.conn,
		"error":     m.errMsg,
		"username":  m.cfg.Username,
		"room_id":   "mock-room",
	}
}

var mockUsers = []string{"Andi", "Budi", "Citra", "Dewi", "Eko", "Fitri", "Gani", "Hana"}
var mockQuestions = []string{"7 + 8", "12 - 5", "6 * 7", "18 / 3", "5 + 11 - 2", "9 * 3", "24 / 6", "15 - 7 + 4"}

func (m *Mock) emit(e RawEvent) { m.events <- e }

func (m *Mock) commentLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			q := mockQuestions[i%len(mockQuestions)]
			i++
			user := mockUsers[rand.Intn(len(mockUsers))]
			var ans string
			if rand.Float64() < 0.7 { // most answers correct
				if v, err := expr.Eval(q); err == nil {
					ans = strconv.Itoa(v)
				} else {
					ans = "?"
				}
			} else { // occasional wrong / chatter
				ans = strconv.Itoa(rand.Intn(40) - 10)
			}
			m.emit(RawEvent{Kind: "comment", Username: user, Text: ans})
		}
	}
}

func (m *Mock) likeJoinLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			user := mockUsers[rand.Intn(len(mockUsers))]
			if i%3 == 0 {
				m.emit(RawEvent{Kind: "join", Username: user})
			} else {
				m.emit(RawEvent{Kind: "like", Username: user, LikeCount: 1 + rand.Intn(9)})
			}
			if i%5 == 0 {
				gifts := []struct {
					n string
					v int
				}{{"Rose", 1}, {"TikTok", 1}, {"Panda", 5}, {"Heart", 1}, {"Diamond", 10}}
				g := gifts[rand.Intn(len(gifts))]
				m.emit(RawEvent{Kind: "gift", Username: user, GiftName: g.n, GiftValue: g.v})
			}
			i++
		}
	}
}

func (m *Mock) viewerLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	viewers := 120
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			viewers += rand.Intn(9) - 4
			if viewers < 40 {
				viewers = 40
			}
			m.emit(RawEvent{Kind: "viewers", Viewers: viewers})
		}
	}
}
