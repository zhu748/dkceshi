package prompt

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTextContextPreservesKnownBodiesAndScrubsBinary(t *testing.T) {
	for _, body := range []string{"  line\n\tline\r\n ", strings.Repeat("A", 6000), "data:literal code, not a binary URL"} {
		block := map[string]any{"type": "document", "source": map[string]any{"type": "text", "media_type": "text/plain", "data": body}}
		out, ok := PreserveTextContextBlock(block)
		if !ok || !reflect.DeepEqual(out, block) {
			t.Fatal("known text was trimmed, truncated or treated as binary")
		}
		encoded, _ := json.Marshal(body)
		if !strings.Contains(NormalizeContent([]any{block}), string(encoded)) {
			t.Fatal("document text missing in final serialization")
		}
	}
	block := map[string]any{"type": "search_result", "content": []any{
		map[string]any{"type": "text", "text": strings.Repeat("word", 2000)},
		map[string]any{"type": "image", "source": map[string]any{"type": "base64", "data": "SECRET_IMAGE_BYTES"}},
	}}
	out, ok := PreserveTextContextBlock(block)
	encoded, _ := json.Marshal(out)
	if !ok || strings.Contains(string(encoded), "SECRET_IMAGE_BYTES") || !strings.Contains(string(encoded), strings.Repeat("word", 2000)) {
		t.Fatal("known text preservation changed binary handling")
	}
}
