package assistantturn

import (
	"ds2api/internal/tokenusage"
	"encoding/json"
	"testing"
)

func TestUpstreamUsageResolutionPreservesFieldSources(t *testing.T) {
	base := Usage{InputTokens: 100, OutputTokens: 20, ReasoningTokens: 5, TotalTokens: 120}
	for _, tc := range []struct {
		name                     string
		raw                      string
		input, output, reasoning int
		source                   string
	}{
		{"explicit", `{"usage":{"input_tokens":11,"output_tokens":29,"reasoning_tokens":9,"total_tokens":40}}`, 11, 29, 9, "upstream"},
		{"zero", `{"usage":{"input_tokens":0,"output_tokens":0,"reasoning_tokens":0}}`, 0, 0, 0, "upstream"},
		{"mixed", `{"usage":{"output_tokens":29}}`, 100, 29, 5, "mixed"},
		{"reasoning only", `{"usage":{"reasoning_tokens":9}}`, 100, 24, 9, "mixed"},
		{"raw cumulative", `{"accumulated_token_usage":2551}`, 100, 20, 5, "estimated"},
		{"inconsistent total", `{"usage":{"input_tokens":11,"output_tokens":29,"total_tokens":999}}`, 100, 20, 5, "estimated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observed := usageObservation(t, tc.raw)
			got := ResolveUsage(base, observed, false)
			if got.InputTokens != tc.input || got.OutputTokens != tc.output || got.ReasoningTokens != tc.reasoning || got.TotalTokens != tc.input+tc.output || got.Metadata.Source != tc.source || got.Metadata.Complete {
				t.Fatalf("wrong resolution: %+v metadata=%+v", got, got.Metadata)
			}
			if got.Metadata.Upstream == nil {
				t.Fatal("upstream evidence lost")
			}
		})
	}
}

func TestUnknownReasoningAndGeminiTotals(t *testing.T) {
	turn := Turn{Usage: ResolveUsage(Usage{InputTokens: 100, OutputTokens: 20, ReasoningTokens: 10}, usageObservation(t, `{"usage":{"output_tokens":2}}`), true)}
	if _, exists := OpenAIChatUsage(turn)["completion_tokens_details"]; exists {
		t.Fatal("unknown reasoning reported as a real zero")
	}
	turn.Usage = ResolveUsage(Usage{}, usageObservation(t, `{"usage":{"input_tokens":11,"output_tokens":29,"reasoning_tokens":9}}`), true)
	gemini := GeminiUsage(turn)
	if gemini["candidatesTokenCount"] != 20 || gemini["thoughtsTokenCount"] != 9 || gemini["totalTokenCount"] != 40 {
		t.Fatalf("reasoning double counted: %+v", gemini)
	}
}

func usageObservation(t *testing.T, raw string) tokenusage.Snapshot {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	var s tokenusage.Snapshot
	s.Observe(tokenusage.Parse(m))
	return s
}

func TestResolvedHistorySkipsLocalPromptAndFileEstimates(t *testing.T) {
	var snapshot tokenusage.Snapshot
	snapshot.Observe(tokenusage.Observation{MessageID: 2})
	resolved := usageObservation(t, `{"usage":{"input_tokens":58,"output_tokens":23,"total_tokens":81}}`)
	snapshot.SetRequest(resolved.Messages[0])
	got := buildObservedUsage(BuildOptions{Prompt: "local prompt", RefFileTokens: 86700, PromptTokenCountFn: func(string) (int, bool) { t.Fatal("retokenized a known input count"); return 0, false }}, "", "answer", snapshot, true)
	if got.InputTokens != 58 || got.OutputTokens != 23 || got.TotalTokens != 81 || got.Metadata.Source != "upstream" || got.Metadata.TotalSource != "upstream" {
		t.Fatalf("upstream counts changed: %+v", got)
	}
}
