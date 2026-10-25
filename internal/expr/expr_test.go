package expr

import "testing"

func TestEval(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"5 + 11 - 2", 14, true},
		{"7 + 8", 15, true},
		{"12 - 5", 7, true},
		{"6 * 7", 42, true},
		{"18 / 3", 6, true},
		{"2 + 3 * 4", 14, true},  // precedence: 3*4 first
		{"10 - 2 - 3", 5, true},  // left-to-right
		{"9 * 3", 27, true},
		{"24 / 6", 4, true},
		{"15 - 7 + 4", 12, true},
		{"10 / 3", 0, false}, // non-integer
		{"5 / 0", 0, false},  // div by zero
		{"abc", 0, false},
		{"", 0, false},
		{"7", 0, false}, // bare number
	}
	for _, c := range cases {
		got, err := Eval(c.in)
		if c.ok && err != nil {
			t.Errorf("Eval(%q) unexpected err: %v", c.in, err)
			continue
		}
		if !c.ok && err == nil {
			t.Errorf("Eval(%q) expected error, got %d", c.in, got)
			continue
		}
		if c.ok && got != c.want {
			t.Errorf("Eval(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestIsExpression(t *testing.T) {
	yes := []string{"5 + 11 - 2", "6 * 7", "12 - 5"}
	no := []string{"7", "", "hello world", "5 +", "+ 5", "1 2 3"}
	for _, s := range yes {
		if !IsExpression(s) {
			t.Errorf("IsExpression(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if IsExpression(s) {
			t.Errorf("IsExpression(%q) = true, want false", s)
		}
	}
}
