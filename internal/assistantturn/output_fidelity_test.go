package assistantturn

import (
	"testing"

	"ds2api/internal/outputformat"
	"ds2api/internal/sse"
)

func TestToolTurnPreservesPrefixAndRejectsFabricatedSuffix(t *testing.T) {
	raw := `before <tool_calls><invoke name="read"></invoke></tool_calls> FAKE RESULT`
	for _, turn := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{}),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw, VisibleText: "before FAKE RESULT"}, BuildOptions{}),
	} {
		if turn.Error != nil || turn.Text != "before " || len(turn.ToolCalls) != 1 {
			t.Fatalf("tool turn changed: %+v", turn)
		}
	}
}

func TestOutputFormatAppliesToAnswersAfterToolValidation(t *testing.T) {
	opts := BuildOptions{OutputFormat: outputformat.Format{Type: "json_object"}}
	for _, raw := range []string{`{"result":1}`, `invalid answer`, `<tool_calls><invoke name="read"></invoke></tool_calls>`} {
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts)
		if raw == "invalid answer" {
			if turn.Error == nil || turn.Error.Code != "invalid_response_format" {
				t.Fatalf("invalid JSON escaped validation: %+v", turn)
			}
		} else if turn.Error != nil {
			t.Fatalf("valid answer or intermediate tool turn rejected: %+v", turn)
		}
	}
}
