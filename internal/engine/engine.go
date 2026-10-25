// Package engine implements the "Matematika Hitung Cepat" game: it holds the
// authoritative session state, generates questions, validates answers, scores
// players, and drives the server-side countdown timer. It is fully decoupled
// from providers — the server calls HandleEvent from a single goroutine (the
// event bus consumer), so the engine needs no internal locking.
package engine

import (
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"tiklive/internal/expr"
)

// ---------- configuration ----------

type Config struct {
	Difficulty string `json:"difficulty"` // easy | medium | hard
	TimerSec   int    `json:"timer_sec"`
	BasePoints int    `json:"base_points"`
	SpeedBonus bool   `json:"speed_bonus"`
	ComboBonus bool   `json:"combo_bonus"`
	RoundTotal int    `json:"round_total"`
	GiftTarget int    `json:"gift_target"`
}

func NewConfig() Config {
	return Config{
		Difficulty: "easy",
		TimerSec:   15,
		BasePoints: 8,
		SpeedBonus: true,
		ComboBonus: true,
		RoundTotal: 10,
		GiftTarget: 0,
	}
}

// ---------- public types ----------

type Phase string

const (
	PhaseIdle    Phase = "idle"
	PhaseRunning Phase = "running"
	PhasePaused  Phase = "paused"
	PhaseEnded   Phase = "ended"
)

type Player struct {
	Username  string
	Points    int
	Correct   int
	Wrong     int
	Streak    int
	MaxStreak int
}

type LeaderRow struct {
	Username string `json:"username"`
	Points   int    `json:"points"`
	Correct  int    `json:"correct"`
	Wrong    int    `json:"wrong"`
	Streak   int    `json:"streak"`
	Rank     int    `json:"rank"`
}

// Question delivered to the overlay. The answer stays server-side only.
type Question struct {
	ID          int64  `json:"id"`
	Text        string `json:"text"`
	DurationSec int    `json:"duration_sec"`
	StartedUnix int64  `json:"started_at_unix"`
}

// Result describes an answer outcome broadcast to clients.
type Result struct {
	Username   string `json:"username"`
	Correct    bool   `json:"correct"`
	Answer     string `json:"answer"`
	Points     int    `json:"points"`
	Bonus      int    `json:"bonus"`
	Streak     int    `json:"streak"`
	QuestionID int64  `json:"question_id"`
	Now        int64  `json:"now"`
}

// GiftRaised tracks progress toward the optional gift target.
type GiftRaised struct {
	Raised int `json:"raised"`
	Target int `json:"target"`
}

// Event is emitted by the engine for the hub to broadcast.
type Event struct {
	Type string
	Data interface{}
}

// Callbacks wires the engine to the outside world.
type Callbacks struct {
	// Emit is called for every Event; must not block (hub broadcast).
	Emit func(Event)
	// OnWinner is called when a session ends with a winner.
	OnWinner func(winner *Player, scores []LeaderRow)
}

// Engine is the authoritative game state for one session.
// The Engine is NOT goroutine-safe and MUST be driven from one goroutine
// (the hub's event bus consumer). Timing is driven by Hub.Tick(), never by
// an internal goroutine, so there is no concurrent access.
type Engine struct {
	cfg  Config
	cb   Callbacks
	rand *rand.Rand

	phase         Phase
	question      *Question
	ansAnswer     int
	timeLeft      int
	questionStart time.Time

	players map[string]*Player
	gifts   int

	nextQID int64
	round   int
}

// New creates an idle engine with default config.
func New(cb Callbacks) *Engine {
	e := &Engine{
		cb:      cb,
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
		players: make(map[string]*Player),
	}
	e.SetConfig(NewConfig())
	e.phase = PhaseIdle
	return e
}

func (e *Engine) SetConfig(c Config)  { e.cfg = c }
func (e *Engine) Config() Config      { return e.cfg }
func (e *Engine) Phase() Phase        { return e.phase }
func (e *Engine) TimeLeft() int       { return e.timeLeft }
func (e *Engine) Round() int          { return e.round }
func (e *Engine) RoundMax() int       { return e.cfg.RoundTotal }
func (e *Engine) Question() *Question { return e.question }

// Start begins a fresh session.
func (e *Engine) Start() {
	e.players = make(map[string]*Player)
	e.gifts = 0
	e.round = 0
	e.nextQID = 0
	e.phase = PhaseRunning
	e.timeLeft = e.cfg.TimerSec
	e.nextQuestion()
}

// Pause halts the countdown but keeps state.
func (e *Engine) Pause() {
	if e.phase != PhaseRunning {
		return
	}
	e.phase = PhasePaused
}

// Resume continues a paused session.
func (e *Engine) Resume() {
	if e.phase != PhasePaused {
		return
	}
	e.phase = PhaseRunning
}

// Tick advances the server countdown by one second. Called once per second
// by the hub bus consumer. On timeout it emits timeout and advances the round.
func (e *Engine) Tick() {
	if e.phase != PhaseRunning {
		return
	}
	e.timeLeft--
	e.emit(Event{"tick", e.timeLeft})
	if e.timeLeft <= 0 {
		e.emit(Event{"timeout", e.question})
		e.nextQuestion()
	}
}

// End stops the session and announces the winner.
func (e *Engine) End() {
	if e.phase == PhaseIdle || e.phase == PhaseEnded {
		return
	}
	e.phase = PhaseEnded
	if e.cb.OnWinner != nil {
		rows := e.Leaderboard()
		var winner *Player
		if len(rows) > 0 && rows[0].Points > 0 {
			winner = e.players[strings.ToLower(rows[0].Username)]
		}
		e.cb.OnWinner(winner, rows)
	}
}

// Leaderboard returns players sorted by points desc, then correct, then name.
func (e *Engine) Leaderboard() []LeaderRow {
	rows := make([]LeaderRow, 0, len(e.players))
	for _, p := range e.players {
		rows = append(rows, LeaderRow{Username: p.Username, Points: p.Points,
			Correct: p.Correct, Wrong: p.Wrong, Streak: p.Streak})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Points != rows[j].Points {
			return rows[i].Points > rows[j].Points
		}
		if rows[i].Correct != rows[j].Correct {
			return rows[i].Correct > rows[j].Correct
		}
		return rows[i].Username < rows[j].Username
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows
}

func (e *Engine) QID() int64 { return e.nextQID }

// ---------- event ingestion ----------

// Input is the minimal normalized event the engine consumes. The hub adapts
// provider.RawEvent into this shape before calling HandleEvent.
type Input struct {
	Kind      string
	Username  string
	Text      string
	GiftValue int
	Viewers   int
}

// HandleEvent is the single ingestion point for normalized events. MUST be
// called from one goroutine (the event bus consumer).
func (e *Engine) HandleEvent(ev Input) {
	switch ev.Kind {
	case "comment":
		e.handleComment(ev.Username, ev.Text)
	case "gift":
		e.gifts += ev.GiftValue
		e.emit(Event{"gift.raised", GiftRaised{Raised: e.gifts, Target: e.cfg.GiftTarget}})
	case "viewers":
		// forwarded by hub as a broadcast; engine has no use for it
	}
}

func (e *Engine) handleComment(username, text string) {
	if e.phase != PhaseRunning || e.question == nil {
		return
	}
	if username == "" || strings.TrimSpace(text) == "" {
		return
	}
	ans := strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
	if ans == "" {
		return
	}
	val, err := strconv.ParseFloat(ans, 64)
	if err != nil {
		e.record(username, false, 0)
		return
	}
	if int(val) != e.ansAnswer || val != float64(e.ansAnswer) {
		e.record(username, false, 0)
		return
	}

	// Correct answer.
	p := e.player(username)
	points, bonus := e.pointsFor(p)
	p.Points += points
	p.Correct++
	p.Streak++
	if p.Streak > p.MaxStreak {
		p.MaxStreak = p.Streak
	}
	e.emit(Event{"answer", Result{
		Username:   username,
		Correct:    true,
		Answer:     ans,
		Points:     points,
		Bonus:      bonus,
		Streak:     p.Streak,
		QuestionID: e.question.ID,
		Now:        time.Now().Unix(),
	}})
	e.emit(Event{"leaderboard", e.Leaderboard()})
	e.nextQuestion()
}

func (e *Engine) record(username string, correct bool, pts int) {
	p := e.player(username)
	if correct {
		p.Points += pts
		p.Correct++
		p.Streak++
		if p.Streak > p.MaxStreak {
			p.MaxStreak = p.Streak
		}
	} else {
		p.Wrong++
		p.Streak = 0
	}
}

// pointsFor computes base + speed bonus + combo bonus for a correct answer.
func (e *Engine) pointsFor(p *Player) (points, bonus int) {
	points = e.cfg.BasePoints
	if e.cfg.SpeedBonus {
		elapsed := time.Since(e.questionStart).Seconds()
		frac := 1.0 - elapsed/float64(e.cfg.TimerSec)
		if frac < 0 {
			frac = 0
		}
		bonus += int(float64(points) * 0.5 * frac)
	}
	if e.cfg.ComboBonus && p.Streak >= 2 {
		bonus += (p.Streak - 1) * 2
	}
	points += bonus
	return points, bonus
}

func (e *Engine) player(username string) *Player {
	key := strings.ToLower(username)
	p, ok := e.players[key]
	if !ok {
		p = &Player{Username: username}
		e.players[key] = p
	}
	return p
}

// nextQuestion generates a fresh question and restarts the countdown.
func (e *Engine) nextQuestion() {
	if e.phase != PhaseRunning {
		return
	}
	if e.round >= e.cfg.RoundTotal {
		e.End()
		return
	}
	e.round++
	text, answer := Generate(e.cfg.Difficulty, e.rand)
	e.nextQID++
	e.question = &Question{
		ID:          e.nextQID,
		Text:        text,
		DurationSec: e.cfg.TimerSec,
		StartedUnix: time.Now().Unix(),
	}
	e.ansAnswer = answer
	e.questionStart = time.Now()
	e.timeLeft = e.cfg.TimerSec
	e.emit(Event{"question", e.question})
}

// emit forwards an event to the callback (hub broadcast).
func (e *Engine) emit(ev Event) {
	if e.cb.Emit != nil {
		e.cb.Emit(ev)
	}
}

// ---------- question generation ----------

// Generate produces a question expression and its integer answer for a
// difficulty. Expressions always evaluate to a non-negative integer.
func Generate(difficulty string, r *rand.Rand) (string, int) {
	r = ensureRand(r)
	for {
		var text string
		var answer int
		switch difficulty {
		case "hard":
			text, answer = genHard(r)
		case "medium":
			text, answer = genMedium(r)
		default:
			text, answer = genEasy(r)
		}
		if a, err := expr.Eval(text); err == nil && a == answer && answer >= 0 {
			return text, answer
		}
	}
}

func ensureRand(r *rand.Rand) *rand.Rand {
	if r == nil {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return r
}

// genEasy: a op b with small numbers, single operator.
func genEasy(r *rand.Rand) (string, int) {
	op := pickOp(r, "+-")
	a, b := r.Intn(9)+2, r.Intn(9)+2
	switch op {
	case "-":
		if a < b {
			a, b = b, a
		}
		return itoa(a) + " - " + itoa(b), a - b
	case "+":
		return itoa(a) + " + " + itoa(b), a + b
	}
	return itoa(a) + " + " + itoa(b), a + b
}

// genMedium: two operators, larger numbers.
func genMedium(r *rand.Rand) (string, int) {
	op := pickOp(r, "+-*")
	a, b := r.Intn(15)+3, r.Intn(15)+3
	switch op {
	case "+":
		return itoa(a) + " + " + itoa(b), a + b
	case "-":
		if a < b {
			a, b = b, a
		}
		return itoa(a) + " - " + itoa(b), a - b
	case "*":
		return itoa(a) + " * " + itoa(b), a * b
	}
	return itoa(a) + " + " + itoa(b), a + b
}

// genHard: mixed expression with three terms (e.g. "5 + 11 - 2").
func genHard(r *rand.Rand) (string, int) {
	for {
		a, b, c := r.Intn(20)+2, r.Intn(20)+2, r.Intn(20)+2
		op1 := pickOp(r, "+-")
		op2 := pickOp(r, "+-*")
		t := itoa(a) + " " + op1 + " " + itoa(b) + " " + op2 + " " + itoa(c)
		val, err := expr.Eval(t)
		if err == nil && val >= 0 {
			return t, val
		}
	}
}

func pickOp(r *rand.Rand, ops string) string {
	return string(ops[r.Intn(len(ops))])
}

func itoa(i int) string { return strconv.Itoa(i) }
