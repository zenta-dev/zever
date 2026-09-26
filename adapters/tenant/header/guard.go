package header

import (
	"regexp/syntax"
	"strings"

	"github.com/zenta-dev/zever/tenant"
)

// isCatastrophicPattern reports whether s risks catastrophic backtracking.
func isCatastrophicPattern(s string) bool {
	if len(s) > tenant.MaxRegexLength {
		return true
	}

	if strings.Contains(s, "++") || strings.Contains(s, "*+") || strings.Contains(s, "+*") || strings.Contains(s, "**") {
		return true
	}

	for i := 0; i < len(s)-1; i++ {
		if s[i] == ')' && (s[i+1] == '+' || s[i+1] == '*' || s[i+1] == '{') {
			return true
		}
	}

	re, err := syntax.Parse(s, syntax.Perl)
	if err != nil {
		return false
	}

	return hasNestedQuantifier(re)
}

// hasNestedQuantifier reports whether re nests a repeat inside a repeat.
func hasNestedQuantifier(re *syntax.Regexp) bool {
	if isRepeatOp(re.Op) && containsRepeat(re) {
		return true
	}

	for _, sub := range re.Sub {
		if hasNestedQuantifier(sub) {
			return true
		}
	}

	return false
}

// isRepeatOp reports whether op is a repetition operator.
func isRepeatOp(op syntax.Op) bool {
	return op == syntax.OpStar || op == syntax.OpPlus || op == syntax.OpQuest || op == syntax.OpRepeat
}

// containsRepeat reports whether re contains a nested repetition.
func containsRepeat(re *syntax.Regexp) bool {
	for _, sub := range re.Sub {
		if isRepeatOp(sub.Op) {
			return true
		}

		if containsRepeat(sub) {
			return true
		}
	}

	return false
}
