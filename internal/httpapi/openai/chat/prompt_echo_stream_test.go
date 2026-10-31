package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestPromptEchoNeverReachesStreamAndCanRegenerate(t *testing.T) {
	prefix := "<｜System｜>" + promptcompat.DefaultThinkingInjectionPrompt
	input := "\n\n" + prefix + "\n\nSerialized conversation. Each message ends with [EndMessage:0123456789abcdef]"
	for _, size := range []int{1, 2, 7, len(input)} {
		rec := httptest.NewRecorder()
		rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", prefix+"\n\n[User]:continue", true, false, true, nil, nil, promptcompat.DefaultToolChoicePolicy(), false, false)
		rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "thinking", Text: "I will make two edits."}}})
		for i := 0; i < len(input); i += size {
			if rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: input[i:min(i+size, len(input))]}}}).Stop {
				break
			}
		}
		if rt.finalize("stop", true) || rt.finalErrorCode != "upstream_prompt_echo" || strings.Contains(rec.Body.String(), `"content":`) || strings.Contains(rec.Body.String(), `"finish_reason":"stop"`) {
			t.Fatalf("size=%d echo leaked or succeeded: code=%s body=%s", size, rt.finalErrorCode, rec.Body.String())
		}
		rt.resetForRegeneration()
		rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "已完成修改。"}}})
		if !rt.finalize("stop", true) || rt.finalText != "已完成修改。" || rt.finalErrorCode != "" {
			t.Fatalf("size=%d clean retry failed: text=%q code=%s", size, rt.finalText, rt.finalErrorCode)
		}
	}
}

func TestPromptEchoEOFLiteralsAreDeliveredExactlyOnce(t *testing.T) {
	const prefix = "<｜System｜>custom thinking instruction"
	for _, input := range []string{"<｜System", "Serialized conversation.", "`" + prefix + "`"} {
		rec := httptest.NewRecorder()
		rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", prefix, false, false, true, nil, nil, promptcompat.DefaultToolChoicePolicy(), false, false)
		for _, ch := range input {
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: string(ch)}}})
		}
		if !rt.finalize("stop", true) || rt.finalText != input || rt.finalErrorCode != "" {
			t.Fatalf("EOF literal was lost: input=%q text=%q code=%s", input, rt.finalText, rt.finalErrorCode)
		}
	}
}
