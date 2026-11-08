package promptcompat

import "testing"

func TestAttachmentShapeDiscoveryKeepsBusinessArgumentsAndJSONOpaque(t *testing.T) {
	for _, value := range []any{
		[]any{"plain string", "not a file ID"},
		map[string]any{"role": "tool", "content": map[string]any{"file_id": "business", "description": "not a file"}},
		map[string]any{"type": "tool_use", "input": map[string]any{"type": "input_file", "file_data": "business"}},
		map[string]any{"type": "function_call", "input": map[string]any{"file_data": "business"}},
		map[string]any{"functionCall": map[string]any{"args": map[string]any{"type": "image", "file_id": "business"}}},
		map[string]any{"functionResponse": map[string]any{"response": map[string]any{"parts": []any{map[string]any{"type": "input_file", "file_id": "business"}}}}},
	} {
		if ContentHasAttachments(value) {
			t.Fatalf("business context became a file container: %#v", value)
		}
	}
	for _, value := range []any{
		map[string]any{"type": "search_result", "content": []any{map[string]any{"type": "input_file", "file_id": "file-a"}}},
		map[string]any{"type": "document", "source": map[string]any{"type": "content", "content": []any{map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "QQ=="}}}}},
		map[string]any{"type": "tool_use", "attachments": []any{"file-a"}},
		map[string]any{"type": "text", "text": "context", "files": []any{map[string]any{"file": map[string]any{"id": "file-a"}}}},
	} {
		if !ContentHasAttachments(value) {
			t.Fatalf("nested attachment was missed: %#v", value)
		}
	}
}
