package toolcall

import "strings"

func toolCallsContainPlaceholderLiteral(calls []ParsedToolCall, token cdataSlotToken) bool {
	prefix := token.literalPrefix()
	for _, c := range calls {
		for _, v := range c.Input {
			if valueContainsSubstring(v, prefix) {
				return true
			}
		}
	}
	return false
}

func valueContainsSubstring(value any, needle string) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, needle)
	case []any:
		for _, item := range v {
			if valueContainsSubstring(item, needle) {
				return true
			}
		}
	case map[string]any:
		for _, item := range v {
			if valueContainsSubstring(item, needle) {
				return true
			}
		}
	}
	return false
}

// pickBetterRepairResult chooses between the two interpretations' parse results:
// more tool calls wins, then more total populated parameters. Ties go to the
// primary so the default (greedy) interpretation stays authoritative.
func pickBetterRepairResult(primary []ParsedToolCall, primaryName string, alternate []ParsedToolCall, alternateName string) ([]ParsedToolCall, string) {
	if len(alternate) == 0 {
		return primary, primaryName
	}
	if len(primary) == 0 {
		return alternate, alternateName
	}
	if len(alternate) > len(primary) {
		return alternate, alternateName
	}
	if len(alternate) == len(primary) && countToolCallInputs(alternate) > countToolCallInputs(primary) {
		return alternate, alternateName
	}
	return primary, primaryName
}

func countToolCallInputs(calls []ParsedToolCall) int {
	total := 0
	for _, c := range calls {
		total += len(c.Input)
	}
	return total
}

func alternateInterpretationName(plan toolCallRepairPlan) string {
	if plan.interpretation == "bounded" {
		return "greedy"
	}
	return "bounded"
}
