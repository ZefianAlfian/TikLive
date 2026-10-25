package engine

import (
	"strings"
	"testing"

	"tiklive/internal/expr"
)

// exprEval is a thin alias used by tests to compute expected answers.
func exprEval(s string) (int, error) { return expr.Eval(s) }

// collect builds an engine whose events are gathered into a slice.
func collect() (*Engine, *[]Event) {
	var evs []Event
	e := New(Callbacks{Emit: func(ev Event) { evs = append(evs, ev) }})
	return e, &evs
}

func TestGenerateValidates(t *testing.T) {
	for _, diff := range []string{"easy", "medium", "hard"} {
		for i := 0; i < 200; i++ {
			text, ans := Generate(diff, nil)
			val, err := exprEval(text)
			if err != nil || val != ans || ans < 0 {
				t.Fatalf("[%s] %q => %d (eval %d, %v)", diff, text, ans, val, err)
			}
		}
	}
}

func TestStartEmitsQuestion(t *testing.T) {
	e, evs := collect()
	e.SetConfig(NewConfig())
	e.Start()
	if len(*evs) == 0 {
		t.Fatal("no events after Start")
	}
	var q *Question
	for _, ev := range *evs {
		if ev.Type == "question" {
			q = ev.Data.(*Question)
		}
	}
	if q == nil {
		t.Fatal("no question event")
	}
	if !strings.Contains(q.Text, " ") {
		t.Errorf("question text looks odd: %q", q.Text)
	}
}

func TestCorrectAnswerScores(t *testing.T) {
	e, _ := collect()
	cfg := NewConfig()
	cfg.RoundTotal = 50
	cfg.TimerSec = 60
	e.SetConfig(cfg)
	e.Start()

	// Determine the correct answer from the public question text.
	q := e.Question()
	if q == nil {
		t.Fatal("no question after Start")
	}
	val, err := exprEval(q.Text)
	if err != nil {
		t.Fatalf("question %q not evaluable: %v", q.Text, err)
	}

	e.HandleEvent(Input{Kind: "comment", Username: "Andi", Text: itoa(val)})
	rows := e.Leaderboard()
	if len(rows) == 0 || rows[0].Correct != 1 || rows[0].Points == 0 {
		t.Fatalf("correct answer not scored: %+v", rows)
	}
}

func TestTimeoutAdvancesRound(t *testing.T) {
	e, evs := collect()
	cfg := NewConfig()
	cfg.TimerSec = 1
	cfg.RoundTotal = 3
	e.SetConfig(cfg)
	e.Start()

	// Drive the countdown manually via Tick() — the hub calls this each second.
	for i := 0; i < 2; i++ {
		e.Tick()
	}
	if e.Question() == nil || e.Question().ID <= 1 {
		t.Fatalf("question did not advance after timeout: round=%d", e.Round())
	}
	var timeoutSeen bool
	for _, ev := range *evs {
		if ev.Type == "timeout" {
			timeoutSeen = true
		}
	}
	if !timeoutSeen {
		t.Error("no timeout event emitted")
	}
}

func TestPauseResume(t *testing.T) {
	e, _ := collect()
	cfg := NewConfig()
	cfg.TimerSec = 10
	e.SetConfig(cfg)
	e.Start()
	if e.Phase() != PhaseRunning {
		t.Fatalf("want running, got %s", e.Phase())
	}
	e.Pause()
	if e.Phase() != PhasePaused {
		t.Fatalf("want paused, got %s", e.Phase())
	}
	e.Resume()
	if e.Phase() != PhaseRunning {
		t.Fatalf("want running, got %s", e.Phase())
	}
	e.End()
	if e.Phase() != PhaseEnded {
		t.Fatalf("want ended, got %s", e.Phase())
	}
}
