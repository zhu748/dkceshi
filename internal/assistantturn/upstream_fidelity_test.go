package assistantturn

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestUpstreamArgumentsSurviveSSECollectionAndHiddenReasoning(t *testing.T) {
	value := "before <think>literal</think> [citation:7] <|assistant|>\nCONTENT_FILTERblocked\nCONTENT_FILTER=enabled; after"
	raw := `<tool_calls><invoke name="write_file"><parameter name="content"><![CDATA[` + value + `]]></parameter></invoke></tool_calls>`
	tools := []any{map[string]any{"name": "write_file", "parameters": map[string]any{"type": "object", "properties": map[string]any{"content": map[string]any{"type": "string"}}, "required": []any{"content"}}}}
	policy, err := promptcompat.ParseOpenAIToolPolicy(map[string]any{"tools": tools})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"response/content", "response/thinking_content"} {
		for _, showThinking := range []bool{false, true} {
			for _, size := range []int{1, 7, 127, len(raw)} {
				var wire strings.Builder
				chars := []rune(raw)
				for start := 0; start < len(chars); start += size {
					data, err := json.Marshal(map[string]any{"p": path, "v": string(chars[start:min(start+size, len(chars))])})
					if err != nil {
						t.Fatal(err)
					}
					wire.WriteString("data: " + string(data) + "\n\n")
				}
				wire.WriteString("data: [DONE]\n\n")
				response := &http.Response{Body: io.NopCloser(strings.NewReader(wire.String()))}
				result := sse.CollectStreamWithOptions(response, showThinking, true, sse.CollectOptions{Context: context.Background()})
				turn := BuildTurnFromCollected(result, BuildOptions{ToolsRaw: tools, ToolChoice: policy, StripReferenceMarkers: true})
				if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["content"] != value {
					t.Fatalf("path=%s thinking=%v size=%d: error=%+v calls=%+v", path, showThinking, size, turn.Error, turn.ToolCalls)
				}
				if turn.Usage.OutputTokens == 0 || turn.Text != "" {
					t.Fatalf("lost tool usage or leaked body: %+v", turn)
				}
			}
		}
	}
}

func TestReasoningPlanningDoesNotBecomeAnExecutableCall(t *testing.T) {
	call := `<tool_calls><invoke name="write_file"></invoke></tool_calls>`
	for _, source := range []string{"Do not execute this example: " + call, "I will not call it. " + call, call + " I rejected this action."} {
		for _, compatible := range []bool{false, true} {
			turn := BuildTurnFromCollected(sse.CollectResult{ToolDetectionThinking: source}, BuildOptions{ToolChoice: promptcompat.ToolChoicePolicy{AllowReasoningProse: compatible}})
			if compatible && len(turn.ToolCalls) != 1 || !compatible && len(turn.ToolCalls) != 0 {
				t.Fatalf("compatibility=%v calls=%+v", compatible, turn.ToolCalls)
			}
		}
	}
}

func TestTerminalToolStatesNeverInvokeRepair(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		result := sse.CollectResult{Text: `<|EPSE|call name="write">broken</|EPSE|call>`, ContentFilter: filtered}
		if !filtered {
			result.UpstreamError = "explicit upstream failure"
		}
		count := 0
		turn := BuildTurnFromCollected(result, BuildOptions{ToolCallRepair: func(context.Context, string) (string, error) { count++; return "", nil }})
		if count != 0 || turn.Error == nil || !FinalizeTurn(turn, FinalizeOptions{}).ShouldFail {
			t.Fatalf("terminal failure repaired or discarded: count=%d turn=%+v", count, turn)
		}
	}
}

func TestIdenticalCallsRemainDistinctUnlessUpstreamMarksASnapshot(t *testing.T) {
	call := `<tool_calls><invoke name="tick"></invoke></tool_calls>`
	for _, snapshot := range []bool{false, true} {
		var wire strings.Builder
		for range 2 {
			chunk := map[string]any{"p": "response/content", "v": call}
			if snapshot {
				chunk["o"] = "SET"
			} else {
				chunk["o"] = "APPEND"
			}
			data, err := json.Marshal(chunk)
			if err != nil {
				t.Fatal(err)
			}
			wire.WriteString("data: " + string(data) + "\n\n")
		}
		wire.WriteString("data: [DONE]\n\n")
		result := sse.CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(wire.String()))}, false, true)
		turn := BuildTurnFromCollected(result, BuildOptions{})
		want := 2
		if snapshot {
			want = 1
		}
		if turn.Error != nil || len(turn.ToolCalls) != want {
			t.Fatalf("snapshot=%v calls=%d want=%d error=%+v", snapshot, len(turn.ToolCalls), want, turn.Error)
		}
	}
}
