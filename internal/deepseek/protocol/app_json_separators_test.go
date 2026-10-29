package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAppJSONPreservesUnicodeAndLiteralEscapeText(t *testing.T) {
	for _, prompt := range []string{"A\u2028B\u2029C", `A\u2028B\u2029C`, "\\\u2028", "<>&\n\"\\"} {
		body, err := MarshalAppJSON(DeepSeekCompletionURL, map[string]any{"prompt": prompt})
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("invalid JSON: %s: %v", body, err)
		}
		if decoded["prompt"] != prompt {
			t.Fatalf("changed prompt %q to %q", prompt, decoded["prompt"])
		}
		if strings.ContainsRune(prompt, '\u2028') && !strings.ContainsRune(string(body), '\u2028') {
			t.Fatal("escaped separator differs from APK/Node")
		}
	}
}
