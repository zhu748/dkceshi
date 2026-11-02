package assistantturn

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/jsonvalue"
	"ds2api/internal/sse"
)

func questionArrayTools() []any {
	return []any{map[string]any{"type": "function", "function": map[string]any{
		"name": "ask_user_question", "parameters": map[string]any{
			"type": "object", "required": []any{"questions"}, "properties": map[string]any{
				"questions": map[string]any{"type": "array", "items": map[string]any{
					"type": "object", "required": []any{"id", "question"}, "properties": map[string]any{
						"id": map[string]any{"type": "string"}, "question": map[string]any{"type": "string"},
						"multi_select": map[string]any{"type": "boolean"},
						"options": map[string]any{"type": "array", "items": map[string]any{
							"type": "object", "required": []any{"label"}, "properties": map[string]any{
								"label": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
							},
						}},
					},
				}},
			},
		},
	}}}
}

func buildQuestionArrayTurns(t *testing.T, value string) []Turn {
	t.Helper()
	const marker = "ZBBZUX"
	raw := `<|ZBBZUX|tool_calls><|ZBBZUX|invoke name="ask_user_question"><|ZBBZUX|parameter name="questions"><![CDATA[` + value + `]]></|ZBBZUX|parameter></|ZBBZUX|invoke></|ZBBZUX|tool_calls>`
	opts := BuildOptions{ToolMarker: marker, ToolNames: []string{"ask_user_question"}, ToolsRaw: questionArrayTools(),
		ToolCallRepair: func(context.Context, string) (string, error) {
			t.Fatal("array value recovery must not require an LLM repair")
			return "", nil
		},
	}
	return []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw, VisibleText: raw}, opts),
	}
}

func TestQuestionArrayMissingClosingBracket(t *testing.T) {
	// Real failure shape: the final array ] was omitted before the CDATA ]]>,
	// while both question objects and their options were complete.
	const missing = `[{"id":"install-target","question":"安装到哪里？","multi_select":true,"options":[{"label":"Codex","description":"C:\\Users\\Tester\\.codex\\skills &amp; [x]"}]},{"id":"front-door","question":"是否启用？","options":[{"label":"先不写"},{"label":"启用"}]}`
	var want any
	if err := jsonvalue.Unmarshal([]byte(missing+`]`), &want); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{missing, missing + `]`, " \n" + missing + " \n"} {
		for i, turn := range buildQuestionArrayTurns(t, value) {
			if turn.Error != nil || len(turn.ToolCalls) != 1 {
				t.Fatalf("path=%d: error=%+v calls=%+v", i, turn.Error, turn.ToolCalls)
			}
			if !reflect.DeepEqual(turn.ToolCalls[0].Input["questions"], want) {
				t.Fatalf("path=%d: changed question values: %#v", i, turn.ToolCalls[0].Input)
			}
			if strings.Contains(turn.Text, "tool_calls") || strings.Contains(turn.Text, "CDATA") {
				t.Fatalf("path=%d: tool protocol leaked: %q", i, turn.Text)
			}
		}
	}
}

func TestQuestionArrayRecoveryRejectsIncompleteOrInvalidItems(t *testing.T) {
	for _, value := range []string{
		`[{"id":"q","question":"unfinished}`,
		`[{"id":"q","question":"x"`,
		`[{"id":"q","question":"x"},`,
		`[{"id":"q","question":"x","options":[{"label":"x"}`,
		`[{"id":"q"}`,
		`[{"id":"q","question":"x","multi_select":"true"}`,
		`[{"id":"q","question":"x","options":[{}]}`,
		`[{"id":"q","question":"x"} {} `,
	} {
		for i, turn := range buildQuestionArrayTurns(t, value) {
			if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" {
				t.Fatalf("path=%d: invalid questions accepted: %q, error=%+v", i, value, turn.Error)
			}
		}
	}
}
