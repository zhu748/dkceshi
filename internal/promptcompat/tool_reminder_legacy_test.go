package promptcompat

import (
	"strings"
	"testing"
)

func TestSavedBuiltinToolReminderUpgradesWithoutOverwritingCustomText(t *testing.T) {
	upstream := strings.Replace(legacyBuiltinToolReminder,
		"Use exactly the tag prefix shown above; do not substitute another tool format.",
		"The only valid label is EPSE; the use of any label prefixed with DSML is **strictly prohibited**.", 1)
	for _, saved := range []string{legacyBuiltinToolReminder, upstream, "For client-provided tools only:\n" + upstream, strings.ReplaceAll(legacyBuiltinToolReminder, "\n", "\r\n")} {
		got := (ToolReminderConfig{Enabled: true, Prompt: saved}).ReminderText()
		if got != DefaultBottomFormatInjectionPrompt || !strings.Contains(got, "Every invoke must include all required parameters") {
			t.Fatalf("saved default did not upgrade: %q", got)
		}
		if got := (ToolReminderConfig{Enabled: false, Prompt: saved}).ReminderText(); got != "" {
			t.Fatalf("disabled saved default was reenabled: %q", got)
		}
	}
	custom := legacyBuiltinToolReminder + "\nMy additional rule."
	if got := (ToolReminderConfig{Enabled: true, Prompt: custom}).ReminderText(); got != custom {
		t.Fatalf("custom text was overwritten: %q", got)
	}
}

func TestDefaultReminderScopeAndPlacementKeepCompletedAndPrefilledTurnsDistinct(t *testing.T) {
	const marker = "Q7ZK3M"
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "read_file", "parameters": map[string]any{"type": "object", "required": []any{"file_path"}, "properties": map[string]any{"file_path": map[string]any{"type": "string"}}}}}}
	for _, prefix := range []bool{false, true} {
		messages := []any{map[string]any{"role": "system", "content": "Task instructions."}, map[string]any{"role": "user", "content": "Inspect a file."}, map[string]any{"role": "assistant", "content": "PAST_OR_PREFIX", "prefix": prefix}}
		final, _ := BuildOpenAIPrompt(messages, tools, "", DefaultToolChoicePolicy(), false, DefaultToolReminderConfig(), marker)
		reminder := strings.Index(final, "For client-provided tools only:")
		history := strings.Index(final, "PAST_OR_PREFIX")
		if reminder < 0 || (prefix && reminder > history) || (!prefix && reminder < history) {
			t.Fatalf("prefix=%v wrong reminder placement: %q", prefix, final)
		}
		if !strings.Contains(final, "DSML-prefixed tags are strictly prohibited") || strings.Contains(final, "<|EPSE|") || strings.Contains(final, `parameter name="path"`) {
			t.Fatalf("marker/scope/schema contract changed: %q", final)
		}
		if prefix && !strings.HasSuffix(final, "PAST_OR_PREFIX") {
			t.Fatalf("prefill no longer continues: %q", final)
		}
		if !prefix && !strings.HasSuffix(final, "[Assistant]:") {
			t.Fatalf("completed assistant turn was reused as prefill: %q", final)
		}
	}
}
