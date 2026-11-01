package promptcompat

import "testing"

type reasoningRecoverySettings bool

func (s reasoningRecoverySettings) SettingBool(section, field string) bool {
	return bool(s) && section == "tool_safety" && field == "allow_reasoning_prose"
}
func (reasoningRecoverySettings) ModelAliases() map[string]string { return nil }

func TestServerToolSafetySurvivesRequestNormalization(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "work"}}, "allow_reasoning_prose": !enabled}
		std, err := NormalizeOpenAIChatRequest(reasoningRecoverySettings(enabled), req, "")
		if err != nil {
			t.Fatal(err)
		}
		if std.ToolChoice.AllowReasoningProse != enabled {
			t.Fatal("request overrode server recovery setting")
		}
	}
}
