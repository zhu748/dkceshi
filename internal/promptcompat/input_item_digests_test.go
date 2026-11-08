package promptcompat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSourceFieldPathsRemainAvailableAfterAdapterRewrites(t *testing.T) {
	request := map[string]any{"messages": []any{map[string]any{"role": "user", "custom/key~": "PRIVATE_VALUE", "attachments": []any{map[string]any{"file_id": "file-a"}}, "content": []any{
		map[string]any{"type": "text", "text": "ORIGINAL_TEXT", "annotations": []any{"PRIVATE_ANNOTATION"}},
	}}}}
	CaptureInputFieldDigests(request)
	before := InputFieldDigests(request)
	request["messages"] = []any{map[string]any{"role": "user", "content": "rewritten"}}
	if !reflect.DeepEqual(before, InputFieldDigests(request)) {
		t.Fatal("source fields were replaced with normalized data")
	}
	paths := map[string]ContextFieldDigest{}
	for _, field := range before {
		paths[field.Path] = field
	}
	for _, path := range []string{"/messages/0/attachments", "/messages/0/content/0/text", "/messages/0/content/0/annotations", "/messages/0/custom~1key~0"} {
		if field := paths[path]; field.Disposition != "adapter_source_field" || len(field.SHA256) != 64 {
			t.Fatalf("missing source evidence %s", path)
		}
	}
	data, _ := json.Marshal(before)
	if strings.Contains(string(data), "PRIVATE_VALUE") || strings.Contains(string(data), "PRIVATE_ANNOTATION") || strings.Contains(string(data), "ORIGINAL_TEXT") {
		t.Fatal("source trace embedded field values")
	}
}

func TestSourceFieldTraceIsBoundedWithoutChangingConversation(t *testing.T) {
	parts := make([]any, maxInputSourceFields)
	for i := range parts {
		parts[i] = map[string]any{"type": "text", "text": "x"}
	}
	request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": parts}}}
	fields := InputFieldDigests(request)
	if len(fields) != maxInputSourceFields+2 || fields[len(fields)-1].Disposition != "adapter_source_trace_limit" {
		t.Fatal("source trace did not declare its bounded evidence")
	}
	standard, err := NormalizeOpenAIChatRequest(nil, request, "")
	if err != nil || !strings.Contains(standard.FinalPrompt, strings.Repeat("x", len(parts))) {
		t.Fatal("tracing budget changed the actual conversation")
	}
}

func TestSourceFieldTraceDoesNotRetainUnboundedPaths(t *testing.T) {
	key := strings.Repeat("long-key", 1024)
	fields := InputFieldDigests(map[string]any{"input": []any{map[string]any{key: "value"}}})
	if len(fields) != 2 || fields[1].Disposition != "adapter_source_trace_limit" || len(fields[1].Path) > maxInputSourcePathLength {
		t.Fatal("source trace retained an unbounded path")
	}
}

func TestSourceTraceBoundsRepeatedHashingOfLargeContainers(t *testing.T) {
	body := strings.Repeat("x", maxInputSourceEncodedBytes+1)
	request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": body}}}
	fields := InputFieldDigests(request)
	if len(fields) != 2 || fields[1].Disposition != "adapter_source_trace_limit" || fields[0].Bytes < len(body) {
		t.Fatal("large source trace lost its aggregate digest or exceeded its work budget")
	}
	standard, err := NormalizeOpenAIChatRequest(nil, request, "")
	if err != nil || !strings.Contains(standard.FinalPrompt, body) {
		t.Fatal("source trace budget truncated actual chat text")
	}
}
