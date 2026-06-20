package toolcall

import (
	"testing"
)

// TestParseClaudeCodeToolInputFormat 验证 Claude Code 客户端训练偏好的
// "Tool: X\n<tool_input>{...}</tool_input>" 格式能被解析。
// 这是 Claude Code 用户实际遇到的问题：模型输出这种格式但解析器不识别，
// 导致工具调用被当成普通文本卡住。
func TestParseClaudeCodeToolInputFormat(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []ParsedToolCall
	}{
		{
			name: "Tool_input_with_newline",
			text: `我来帮你查看这个项目的用途。先检查一下项目根目录下有哪些关键文件。
Tool: Glob
<tool_input>
{"pattern": "*"}
</tool_input>`,
			want: []ParsedToolCall{
				{Name: "Glob", Input: map[string]any{"pattern": "*"}},
			},
		},
		{
			name: "Tool_input_compact",
			text: `Tool: Bash
<tool_input>{"command": "ls -la"}</tool_input>`,
			want: []ParsedToolCall{
				{Name: "Bash", Input: map[string]any{"command": "ls -la"}},
			},
		},
		{
			name: "Calling_double_star_variant",
			text: `**Calling:** Read
{"file_path": "README.md"}`,
			want: []ParsedToolCall{
				{Name: "Read", Input: map[string]any{"file_path": "README.md"}},
			},
		},
		{
			name: "Multi_param",
			text: `Tool: Write
<tool_input>{"file_path": "notes.txt", "content": "hello world"}</tool_input>`,
			want: []ParsedToolCall{
				{Name: "Write", Input: map[string]any{"file_path": "notes.txt", "content": "hello world"}},
			},
		},
		{
			name: "Number_and_bool_params",
			text: `Tool: read_file
<tool_input>{"path": "src/main.go", "offset": 100, "limit": 50, "verbose": true}</tool_input>`,
			want: []ParsedToolCall{
				{Name: "read_file", Input: map[string]any{
					"path":    "src/main.go",
					"offset":  float64(100),
					"limit":   float64(50),
					"verbose": true,
				}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ParseStandaloneToolCallsDetailed(tc.text, nil)
			if !result.SawToolCallSyntax {
				t.Fatalf("expected SawToolCallSyntax=true, got false for text: %s", tc.text)
			}
			if len(result.Calls) != len(tc.want) {
				t.Fatalf("expected %d calls, got %d (result: %+v)", len(tc.want), len(result.Calls), result.Calls)
			}
			for i, want := range tc.want {
				got := result.Calls[i]
				if got.Name != want.Name {
					t.Errorf("call[%d].Name: got %q, want %q", i, got.Name, want.Name)
				}
				if len(got.Input) != len(want.Input) {
					t.Errorf("call[%d].Input size: got %d, want %d", i, len(got.Input), len(want.Input))
					continue
				}
				for k, wantV := range want.Input {
					gotV, ok := got.Input[k]
					if !ok {
						t.Errorf("call[%d].Input[%q]: missing", i, k)
						continue
					}
					if !equalArgValue(gotV, wantV) {
						t.Errorf("call[%d].Input[%q]: got %v (%T), want %v (%T)", i, k, gotV, gotV, wantV, wantV)
					}
				}
			}
		})
	}
}

// TestParseClaudeCodeFallbackDoesNotAffectNormalText 验证普通文本和原生 DSML
// 不受 Claude Code fallback 的影响。
func TestParseClaudeCodeFallbackDoesNotAffectNormalText(t *testing.T) {
	// 普通文本
	result := ParseStandaloneToolCallsDetailed("hello world, this is a normal response", nil)
	if result.SawToolCallSyntax {
		t.Fatalf("normal text should not trigger SawToolCallSyntax, got: %+v", result)
	}
	if len(result.Calls) != 0 {
		t.Fatalf("normal text should produce 0 calls, got %d", len(result.Calls))
	}

	// 原生 DSML 仍正常
	dsml := `<|DSML|tool_calls>
  <|DSML|invoke name="Glob">
    <|DSML|parameter name="pattern"><![CDATA[*]]></|DSML|parameter>
  </|DSML|invoke>
</|DSML|tool_calls>`
	result2 := ParseStandaloneToolCallsDetailed(dsml, nil)
	if !result2.SawToolCallSyntax {
		t.Fatalf("DSML should trigger SawToolCallSyntax")
	}
	if len(result2.Calls) != 1 || result2.Calls[0].Name != "Glob" {
		t.Fatalf("DSML parse failed: %+v", result2.Calls)
	}
}

// equalArgValue 比较参数值（JSON 数字会被解析成 float64）。
func equalArgValue(got, want any) bool {
	switch w := want.(type) {
	case int:
		g, ok := got.(float64)
		return ok && g == float64(w)
	case float64:
		g, ok := got.(float64)
		return ok && g == w
	case string:
		g, ok := got.(string)
		return ok && g == w
	case bool:
		g, ok := got.(bool)
		return ok && g == w
	default:
		return got == want
	}
}
