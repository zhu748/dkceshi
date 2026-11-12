package tokenusage

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLatestCaptureMessageCounters(t *testing.T) {
	raw, err := os.ReadFile("../../tests/fixtures/deepseek-latest-message-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name                 string
		SessionID            string `json:"session_id"`
		MessageID            int    `json:"message_id"`
		Input, Output, Total int
		Payload              map[string]any
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			got, err := FromHistory(f.Payload, f.SessionID, f.MessageID)
			if err != nil || *got.InputTokens != f.Input || *got.OutputTokens != f.Output || *got.TotalTokens != f.Total {
				t.Fatalf("got %+v %v", got, err)
			}
			if _, err := FromHistory(f.Payload, "different-session", f.MessageID); err == nil {
				t.Fatal("used another session")
			}
			if _, err := FromHistory(f.Payload, f.SessionID, 999); err == nil {
				t.Fatal("used the latest message instead of exact target")
			}
		})
	}
}

func TestHistoryUsageRejectsUnfinishedMissingOrDecreasingCounters(t *testing.T) {
	for _, raw := range []string{
		`[{"message_id":1,"role":"USER","status":"FINISHED","accumulated_token_usage":58},{"message_id":2,"parent_id":1,"role":"ASSISTANT","status":"WIP","accumulated_token_usage":81}]`,
		`[{"message_id":1,"role":"USER","status":"FINISHED","accumulated_token_usage":58},{"message_id":2,"parent_id":1,"role":"ASSISTANT","status":"FINISHED","accumulated_token_usage":23}]`,
		`[{"message_id":2,"parent_id":1,"role":"ASSISTANT","status":"FINISHED","accumulated_token_usage":81}]`,
		`[{"message_id":1,"role":"USER","status":"FINISHED","accumulated_token_usage":null},{"message_id":2,"parent_id":1,"role":"ASSISTANT","status":"FINISHED","accumulated_token_usage":81}]`,
	} {
		var items []any
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			t.Fatal(err)
		}
		payload := map[string]any{"data": map[string]any{"biz_data": map[string]any{"chat_messages": items}}}
		if _, err := FromHistory(payload, "session", 2); err == nil {
			t.Fatalf("accepted invalid counters: %s", raw)
		}
	}
}

func TestHistoryContinuationUsesThePairedUserBaseline(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"data":{"biz_data":{"chat_messages":[{"message_id":1,"role":"USER","status":"FINISHED","accumulated_token_usage":58},{"message_id":2,"parent_id":1,"role":"ASSISTANT","status":"INCOMPLETE","accumulated_token_usage":70},{"message_id":3,"parent_id":2,"role":"ASSISTANT","status":"FINISHED","accumulated_token_usage":81}]}}}`), &payload); err != nil {
		t.Fatal(err)
	}
	u, err := FromHistory(payload, "session", 3)
	if err != nil || *u.InputTokens != 58 || *u.OutputTokens != 23 || *u.TotalTokens != 81 {
		t.Fatalf("continuation counters were summed twice: %+v %v", u, err)
	}
}
