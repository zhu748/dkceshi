package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestStreamDeliveryUsesVisibilityContract(t *testing.T) {
	data, err := os.ReadFile("../../../../tests/compat/fixtures/visibility/stream.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name, Marker, Raw, Visible string
		Invalid                    bool
		Calls                      []any
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			for _, size := range []int{1, 7, len(f.Raw)} {
				rec := httptest.NewRecorder()
				rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", true, false, false, []string{"read"}, nil, promptcompat.DefaultToolChoicePolicy(), true, true)
				rt.toolSieve.Marker = f.Marker
				for i := 0; i < len(f.Raw); i += size {
					rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: f.Raw[i:min(i+size, len(f.Raw))]}}})
				}
				rt.finalize("stop", false)
				var visible strings.Builder
				calls := 0
				for _, line := range strings.Split(rec.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var frame struct {
						Choices []struct {
							Delta struct {
								Content string `json:"content"`
								Calls   []any  `json:"tool_calls"`
							} `json:"delta"`
						} `json:"choices"`
					}
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame) != nil {
						continue
					}
					for _, choice := range frame.Choices {
						visible.WriteString(choice.Delta.Content)
						calls += len(choice.Delta.Calls)
					}
				}
				want := f.Visible
				if f.Invalid {
					want = "" // Rejected tool turns keep their answer text held.
				}
				if got := visible.String(); got != want {
					t.Fatalf("size=%d got=%q want=%q body=%s", size, got, want, rec.Body.String())
				}
				if f.Invalid {
					if rt.finalErrorCode != "invalid_tool_arguments" || calls != 0 {
						t.Fatalf("invalid batch delivered: calls=%d code=%s", calls, rt.finalErrorCode)
					}
				} else if calls != len(f.Calls) {
					t.Fatalf("calls=%d want=%d", calls, len(f.Calls))
				}
			}
		})
	}
}
