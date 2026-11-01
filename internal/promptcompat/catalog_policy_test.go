package promptcompat

import "testing"

type strictCatalogSettings bool

func (strictCatalogSettings) ModelAliases() map[string]string { return nil }
func (s strictCatalogSettings) SettingBool(section, field string) bool {
	return bool(s) && section == "tool_safety" && field == "require_tool_catalog"
}

func TestCatalogPresenceAndStrictMode(t *testing.T) {
	for _, strict := range []bool{false, true} {
		for _, explicitEmpty := range []bool{false, true} {
			req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "continue"}}, "require_tool_catalog": false}
			if explicitEmpty {
				req["tools"] = []any{}
			}
			std, err := NormalizeOpenAIChatRequest(strictCatalogSettings(strict), req, "")
			if err != nil {
				t.Fatal(err)
			}
			if std.ToolChoice.Allows("write") == (strict || explicitEmpty) {
				t.Fatalf("strict=%t empty=%t policy=%+v", strict, explicitEmpty, std.ToolChoice)
			}
			if (strict || explicitEmpty) && !std.ToolChoice.IsNone() {
				t.Fatal("empty strict catalog injected tools")
			}
		}
	}
}
