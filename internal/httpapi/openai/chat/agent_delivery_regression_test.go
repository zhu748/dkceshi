package chat

import (
	"ds2api/internal/completionruntime"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegressionSystemCatalogRejectedTurn(t *testing.T) {
	req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "system", "content": "Available tools: read_file(path string)."}, map[string]any{"role": "user", "content": "read a.txt"}}}
	std, err := promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, std.ResponseModel, std.FinalPrompt, false, false, false, std.ToolNames, std.ToolsRaw, std.ToolChoice, true, true)
	s.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: `I will read it now. <tool_calls><invoke name="read_file"><parameter name="path">`}}})
	terminal := s.finalize("stop", true)
	t.Logf("hold=%v delivered=%v terminal=%v error=%s", completionruntime.HoldToolOutput(std.ToolNames, std.ToolsRaw, std.ToolChoice), s.visibleTextEmitted, terminal, s.finalErrorCode)
	if !terminal || !s.visibleTextEmitted || s.finalErrorCode != "invalid_tool_arguments" {
		t.Fatalf("delivered prose must prevent regeneration: %s", rec.Body.String())
	}
}

func TestRegressionRecoveredDSMLTailMustNotEscape(t *testing.T) {
	rec := httptest.NewRecorder()
	s := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "deepseek-v4.1-flash", "prompt", false, false, false, []string{"read_file"}, nil, promptcompat.DefaultToolChoicePolicy(), true, true)
	raw := `I will read it. <｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="read_file"><｜｜DSML｜｜ parameter name="path" string="true">a.txt</｜｜DSML｜｜ parameter></｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls> The file says SECRET_RESULT.`
	s.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw}}})
	s.finalize("stop", false)
	frames, _ := parseSSEDataFrames(t, rec.Body.String())
	content := collectStreamContent(frames)
	t.Logf("finish=%s content=%q", streamFinishReason(frames), content)
	if strings.Contains(content, "SECRET_RESULT") || content != "I will read it. " || streamFinishReason(frames) != "tool_calls" {
		t.Fatal("invented post-call result escaped")
	}
}
