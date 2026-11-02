package completionruntime

import (
	"testing"

	"ds2api/internal/promptcompat"
)

func TestLeadingTextPreservesSuccessfulWhitespaceAndDropsFailedAttempt(t *testing.T) {
	var gate LeadingText
	if got := gate.Write("\n\t　"); got != "" {
		t.Fatalf("empty attempt committed: %q", got)
	}
	gate.Reset()
	if got := gate.Write("\n"); got != "" {
		t.Fatal(got)
	}
	if got := gate.Write("answer"); got != "\nanswer" {
		t.Fatalf("wrong answer prefix: %q", got)
	}
	if got := gate.Write("\n"); got != "\n" {
		t.Fatal("committed whitespace was withheld")
	}
}

func TestOnlyRequiredToolPolicyHoldsProse(t *testing.T) {
	for _, mode := range []promptcompat.ToolChoiceMode{"", promptcompat.ToolChoiceAuto, promptcompat.ToolChoiceNone, promptcompat.ToolChoiceRequired, promptcompat.ToolChoiceForced} {
		choice := promptcompat.ToolChoicePolicy{Mode: mode, RestrictAllowed: true, Allowed: map[string]struct{}{"read": {}}, DisableParallel: true}
		for _, catalog := range []bool{false, true} {
			var names []string
			var tools any
			if catalog {
				names, tools = []string{"read"}, []any{map[string]any{"name": "read"}}
			}
			if got := HoldToolOutput(names, tools, choice); got != choice.IsRequired() {
				t.Fatalf("mode=%s catalog=%v hold=%v", mode, catalog, got)
			}
		}
	}
}
