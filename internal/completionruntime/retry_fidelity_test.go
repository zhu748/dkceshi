package completionruntime

import (
	"strings"
	"testing"

	"ds2api/internal/assistantturn"
)

func TestEveryRegenerationPreservesOriginalPromptBytes(t *testing.T) {
	for _, original := range []string{"  prelude\r\n[Assistant]:code\n\t  ", "\t \r\n", ""} {
		payload := map[string]any{"prompt": original, "parent_message_id": 4}
		for _, failure := range []*assistantturn.OutputError{nil, {Code: "upstream_prompt_echo"}, {Code: "invalid_tool_arguments", Message: "invalid"}, {Code: "tool_choice_violation", Message: "forbidden"}} {
			out := ClonePayloadForOutputRetry(payload, 9, failure)
			prompt := out["prompt"].(string)
			if original != "" && !strings.HasPrefix(prompt, original+"\n\n") || out["parent_message_id"] != 9 || payload["prompt"] != original {
				t.Fatalf("retry changed source bytes: original=%q retry=%q", original, prompt)
			}
		}
	}
}
