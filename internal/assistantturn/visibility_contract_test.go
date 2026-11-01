package assistantturn

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestSharedVisibilityContractFinalValidation(t *testing.T) {
	data, err := os.ReadFile("../../tests/compat/fixtures/visibility/stream.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name, Marker, Raw, Visible string
		Invalid                    bool
		Calls                      []toolcall.ParsedToolCall
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			turn := BuildTurnFromCollected(sse.CollectResult{Text: f.Raw}, BuildOptions{ToolMarker: f.Marker, ToolNames: []string{"read"}})
			invalid := turn.Error != nil && turn.Error.Code == "invalid_tool_arguments"
			if invalid != f.Invalid {
				t.Fatalf("invalid=%v want=%v error=%+v", invalid, f.Invalid, turn.Error)
			}
			if !f.Invalid {
				if len(turn.ToolCalls) != len(f.Calls) {
					t.Fatalf("calls=%#v want=%#v", turn.ToolCalls, f.Calls)
				}
				for i, call := range turn.ToolCalls {
					if call.Name != f.Calls[i].Name || !reflect.DeepEqual(call.Input, f.Calls[i].Input) {
						t.Fatalf("call=%#v want=%#v", call, f.Calls[i])
					}
				}
			}
			if turn.Text != f.Visible {
				t.Fatalf("final text=%q want=%q", turn.Text, f.Visible)
			}
		})
	}
}
