package files

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestStructuredToolJSONNeverBecomesAnAttachment(t *testing.T) {
	for _, data := range []string{"plain tool result", "QUJDRA=="} {
		for _, key := range []string{"name", "filename", "file_name"} {
			content := map[string]any{key: "row", "data": data, "status": "ok", "items": []any{map[string]any{"type": "input_file", "file_id": "business-value"}}}
			for _, rawContent := range []any{content, []any{map[string]any{"type": "text", "text": "output", "data": content}}} {
				req := map[string]any{"messages": []any{map[string]any{"role": "tool", "tool_call_id": "c1", "content": rawContent}}}
				uploader := &nativeFileUploader{}
				if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err != nil {
					t.Fatal(err)
				}
				message := req["messages"].([]any)[0].(map[string]any)
				if len(uploader.uploads) != 0 || !reflect.DeepEqual(message["content"], rawContent) || len(promptcompat.CollectOpenAIRefFileIDs(req)) != 0 {
					t.Fatalf("tool JSON was interpreted as a file: %+v", req)
				}
			}
		}
	}
}

func TestNativeGeminiToolMediaUsesSharedUpload(t *testing.T) {
	var req map[string]any
	if err := json.Unmarshal([]byte(`{"contents":[{"role":"user","parts":[{"functionResponse":{"id":"c1","name":"get_image","response":{"image":{"$ref":"photo.png"},"data":"plain result"},"parts":[{"inlineData":{"mimeType":"image/png","data":"QUJDRA==","displayName":"photo.png"}}]}}]}]}`), &req); err != nil {
		t.Fatal(err)
	}
	uploader := &nativeFileUploader{}
	if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err != nil {
		t.Fatal(err)
	}
	if len(uploader.uploads) != 1 || string(uploader.uploads[0].Data) != "ABCD" {
		t.Fatal("tool media was not uploaded exactly once")
	}
	response := req["contents"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	part := response["parts"].([]any)[0].(map[string]any)
	if part["file_id"] != "file-test" || part["display_name"] != "photo.png" || response["response"].(map[string]any)["data"] != "plain result" {
		t.Fatal("tool data or media reference binding changed")
	}
	if ids := promptcompat.CollectOpenAIRefFileIDs(req); !reflect.DeepEqual(ids, []string{"file-test"}) {
		t.Fatalf("media reference missing: %v", ids)
	}
}
