// Package expr is a tiny integer arithmetic evaluator supporting +, -, *, /
// and mixed expressions, e.g. "5 + 11 - 2". Used by both the game engine
// (to build questions and validate answers) and the mock provider (to
// generate plausible answers). Division only counts when exact.
package expr

import (
	"fmt"
	"regexp"
	"strings"
)

var tokenRe = regexp.MustCompile(`\d+|[+\-*/]`)

// Tokens splits an expression into numbers and operators (ignoring spaces).
func Tokens(s string) []string { return tokenRe.FindAllString(s, -1) }

// IsExpression reports whether s looks like a simple arithmetic expression
// (alternating number/op, no junk) AND is not a bare number.
func IsExpression(s string) bool {
	if s == "" {
		return false
	}
	var prevIsNum bool
	for i, t := range Tokens(s) {
		isNum := t[0] >= '0' && t[0] <= '9'
		if i == 0 && !isNum {
			return false
		}
		if i > 0 && isNum == prevIsNum {
			return false // two numbers or two ops in a row
		}
		if !isNum && !strings.ContainsRune("+-*/", rune(t[0])) {
			return false
		}
		prevIsNum = isNum
	}
	return len(Tokens(s)) >= 3 // must have at least one op
}

// Eval computes the result of an expression, respecting operator precedence
// (* and / before + and -) and left-to-right within a precedence level.
// Division is exact-only: a non-integer result is an error.
func Eval(s string) (int, error) {
	nums, ops, err := tokenize(s)
	if err != nil {
		return 0, err
	}
	// Fold multiply/divide first.
	folded := []int{nums[0]}
	i := 0
	for _, op := range ops {
		if op == '*' || op == '/' {
			left := folded[len(folded)-1]
			folded = folded[:len(folded)-1]
			right := nums[i+1]
			var v int
			switch op {
			case '*':
				v = left * right
			case '/':
				if right == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				if left%right != 0 {
					return 0, fmt.Errorf("non-integer division: %d / %d", left, right)
				}
				v = left / right
			}
			folded = append(folded, v)
		} else {
			folded = append(folded, nums[i+1])
		}
		i++
	}
	// Now +/- left-to-right.
	acc := folded[0]
	opIdx := 0
	for _, op := range ops {
		if op == '+' || op == '-' {
			v := folded[opIdx+1]
			if op == '+' {
				acc += v
			} else {
				acc -= v
			}
		}
		opIdx++
	}
	return acc, nil
}

func tokenize(s string) ([]int, []byte, error) {
	toks := Tokens(s)
	if len(toks) < 3 || len(toks)%2 == 0 {
		return nil, nil, fmt.Errorf("not an arithmetic expression: %q", s)
	}
	nums := make([]int, 0, len(toks)/2+1)
	ops := make([]byte, 0, len(toks)/2)
	for _, t := range toks {
		if t[0] >= '0' && t[0] <= '9' {
			var v int
			if _, err := fmt.Sscanf(t, "%d", &v); err != nil {
				return nil, nil, fmt.Errorf("bad number %q: %w", t, err)
			}
			nums = append(nums, v)
			continue
		}
		if len(t) != 1 || !strings.ContainsRune("+-*/", rune(t[0])) {
			return nil, nil, fmt.Errorf("bad token %q", t)
		}
		ops = append(ops, t[0])
	}
	return nums, ops, nil
}
