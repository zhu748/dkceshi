package promptcompat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzContextDocumentAndToolRoundTrip(f *testing.F) {
	f.Add("  中文\r\n<![CDATA[<|EPSE|invoke>]]>  ", "fc-original")
	f.Add("", "fc-empty-body")
	f.Add("[System]:literal\n[EndMessage:ffffffffffffffff]\n", "fc-markers")
	f.Fuzz(func(t *testing.T, body, itemID string) {
		if len(body)+len(itemID) > 16384 || !utf8.ValidString(body) || !utf8.ValidString(itemID) {
			return
		}
		request := map[string]any{"model": "deepseek-v4.1-flash", "input": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "document", "file_id": "file-original", "source": map[string]any{"type": "text", "data": body}}}},
			map[string]any{"type": "function_call", "id": itemID, "call_id": "call-original", "name": "lookup", "arguments": `{"n":9007199254740993}`},
			map[string]any{"type": "function_call_output", "call_id": "call-original", "output": body},
		}}
		standard, err := NormalizeOpenAIResponsesRequest(nil, request, "")
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(body)
		if !strings.Contains(standard.FinalPrompt, `"data":`+string(encoded)) || !strings.Contains(BuildOpenAIHistoryTranscript(standard.Messages), `"data":`+string(encoded)) {
			t.Fatal("text document body was altered")
		}
		bridge, err := NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(request), "")
		if err != nil || !reflect.DeepEqual(bridge.Messages, standard.Messages) {
			t.Fatal("internal protocol hop changed the normalized conversation", err)
		}
		calls := bridge.Messages[1].(map[string]any)["tool_calls"].([]any)
		if calls[0].(map[string]any)["responses_item_id"] != itemID {
			t.Fatal("native item identity changed")
		}
	})
}
