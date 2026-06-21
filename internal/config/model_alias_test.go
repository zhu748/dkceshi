package config

import "testing"

type mockModelAliasReader map[string]string

func (m mockModelAliasReader) ModelAliases() map[string]string { return m }

func TestResolveModelDirectDeepSeek(t *testing.T) {
	got, ok := ResolveModel(nil, "deepseek-v4-flash")
	if !ok || got != "deepseek-v4-flash" {
		t.Fatalf("expected deepseek-v4-flash, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelDirectDeepSeekNoThinking(t *testing.T) {
	got, ok := ResolveModel(nil, "deepseek-v4-flash-nothinking")
	if !ok || got != "deepseek-v4-flash-nothinking" {
		t.Fatalf("expected deepseek-v4-flash-nothinking, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelAlias(t *testing.T) {
	got, ok := ResolveModel(nil, "gpt-4.1")
	if !ok || got != "deepseek-v4-flash" {
		t.Fatalf("expected alias gpt-4.1 -> deepseek-v4-flash, got ok=%v model=%q", ok, got)
	}
}

func TestResolveLatestOpenAIAlias(t *testing.T) {
	got, ok := ResolveModel(nil, "gpt-5.5")
	if !ok || got != "deepseek-v4-flash" {
		t.Fatalf("expected alias gpt-5.5 -> deepseek-v4-flash, got ok=%v model=%q", ok, got)
	}
}

func TestResolveLatestClaudeAlias(t *testing.T) {
	got, ok := ResolveModel(nil, "claude-sonnet-4-6")
	if !ok || got != "deepseek-v4-flash" {
		t.Fatalf("expected alias claude-sonnet-4-6 -> deepseek-v4-flash, got ok=%v model=%q", ok, got)
	}
}

func TestResolveLatestClaudeAliasNoThinking(t *testing.T) {
	got, ok := ResolveModel(nil, "claude-sonnet-4-6-nothinking")
	if !ok || got != "deepseek-v4-flash-nothinking" {
		t.Fatalf("expected alias claude-sonnet-4-6-nothinking -> deepseek-v4-flash-nothinking, got ok=%v model=%q", ok, got)
	}
}

func TestResolveExpandedHistoricalAliases(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  string
	}{
		{name: "openai old chatgpt", model: "chatgpt-4o", want: "deepseek-v4-flash"},
		{name: "openai codex max", model: "gpt-5.1-codex-max", want: "deepseek-v4-pro"},
		{name: "openai deep research", model: "o3-deep-research", want: "deepseek-v4-pro-search"},
		{name: "openai historical reasoning", model: "o1-preview", want: "deepseek-v4-pro"},
		{name: "claude latest historical", model: "claude-3-5-sonnet-latest", want: "deepseek-v4-flash"},
		{name: "claude historical opus", model: "claude-3-opus-20240229", want: "deepseek-v4-pro"},
		{name: "claude historical haiku", model: "claude-3-haiku-20240307", want: "deepseek-v4-flash"},
		{name: "gemini latest alias", model: "gemini-flash-latest", want: "deepseek-v4-flash"},
		{name: "gemini historical pro", model: "gemini-1.5-pro", want: "deepseek-v4-pro"},
		{name: "gemini vision legacy", model: "gemini-pro-vision", want: "deepseek-v4-vision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ResolveModel(nil, tc.model)
			if !ok || got != tc.want {
				t.Fatalf("expected alias %s -> %s, got ok=%v model=%q", tc.model, tc.want, ok, got)
			}
		})
	}
}

func TestResolveModelUnknown(t *testing.T) {
	_, ok := ResolveModel(nil, "totally-custom-model")
	if ok {
		t.Fatal("expected unknown model to fail resolve")
	}
}

func TestResolveModelUnknownKnownFamilyName(t *testing.T) {
	_, ok := ResolveModel(nil, "gpt-5.5-pro-search")
	if ok {
		t.Fatal("expected unknown known-family model to fail resolve without alias")
	}
}

func TestResolveModelRejectsLegacyDeepSeekIDs(t *testing.T) {
	legacyModels := []string{
		"deepseek-chat",
		"deepseek-reasoner",
		"deepseek-chat-search",
		"deepseek-reasoner-search",
		"deepseek-expert-chat",
		"deepseek-expert-reasoner",
		"deepseek-vision-chat",
	}
	for _, model := range legacyModels {
		if got, ok := ResolveModel(nil, model); ok {
			t.Fatalf("expected legacy model %q to be rejected, got %q", model, got)
		}
	}
}

func TestResolveModelRejectsRetiredHistoricalModels(t *testing.T) {
	retiredModels := []string{
		"claude-2.1",
		"claude-instant-1.2",
		"gpt-3.5-turbo",
	}
	for _, model := range retiredModels {
		if got, ok := ResolveModel(nil, model); ok {
			t.Fatalf("expected retired model %q to be rejected, got %q", model, got)
		}
	}
}

func TestResolveModelDirectDeepSeekExpert(t *testing.T) {
	got, ok := ResolveModel(nil, "deepseek-v4-pro")
	if !ok || got != "deepseek-v4-pro" {
		t.Fatalf("expected deepseek-v4-pro, got ok=%v model=%q", ok, got)
	}
}

// --- -autodelete suffix model tests ---

func TestResolveModelDirectDeepSeekAutoDelete(t *testing.T) {
	got, ok := ResolveModel(nil, "deepseek-v4-flash-autodelete")
	if !ok || got != "deepseek-v4-flash-autodelete" {
		t.Fatalf("expected deepseek-v4-flash-autodelete, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelAliasWithAutoDeleteSuffix(t *testing.T) {
	// gpt-4.1 alias + -autodelete suffix should resolve to deepseek-v4-flash-autodelete
	got, ok := ResolveModel(nil, "gpt-4.1-autodelete")
	if !ok || got != "deepseek-v4-flash-autodelete" {
		t.Fatalf("expected alias gpt-4.1-autodelete -> deepseek-v4-flash-autodelete, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelClaudeAliasWithAutoDeleteSuffix(t *testing.T) {
	got, ok := ResolveModel(nil, "claude-sonnet-4-6-autodelete")
	if !ok || got != "deepseek-v4-flash-autodelete" {
		t.Fatalf("expected alias claude-sonnet-4-6-autodelete -> deepseek-v4-flash-autodelete, got ok=%v model=%q", ok, got)
	}
}

func TestIsAutoDeleteModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4-flash-autodelete", true},
		{"deepseek-v4-pro-search-autodelete", true},
		{"deepseek-v4-flash", false},
		{"deepseek-v4-flash-nothinking", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsAutoDeleteModel(tc.model); got != tc.want {
			t.Fatalf("IsAutoDeleteModel(%q)=%v want=%v", tc.model, got, tc.want)
		}
	}
}

func TestGetModelConfigStripsAutoDeleteSuffix(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-flash-autodelete")
	if !ok || !thinking || search {
		t.Fatalf("expected deepseek-v4-flash-autodelete to behave as flash (thinking=true, search=false), got thinking=%v search=%v ok=%v", thinking, search, ok)
	}
	thinking, search, ok = GetModelConfig("deepseek-v4-pro-search-autodelete")
	if !ok || !thinking || !search {
		t.Fatalf("expected deepseek-v4-pro-search-autodelete to behave as pro-search (thinking=true, search=true), got thinking=%v search=%v ok=%v", thinking, search, ok)
	}
}

func TestGetModelTypeStripsAutoDeleteSuffix(t *testing.T) {
	mt, ok := GetModelType("deepseek-v4-flash-autodelete")
	if !ok || mt != "default" {
		t.Fatalf("expected deepseek-v4-flash-autodelete -> default, got mt=%q ok=%v", mt, ok)
	}
	mt, ok = GetModelType("deepseek-v4-pro-autodelete")
	if !ok || mt != "expert" {
		t.Fatalf("expected deepseek-v4-pro-autodelete -> expert, got mt=%q ok=%v", mt, ok)
	}
}

func TestDeepSeekModelsIncludesAutoDeleteVariants(t *testing.T) {
	// 验证 /v1/models 列表包含 -autodelete 变体
	expected := []string{
		"deepseek-v4-flash-autodelete",
		"deepseek-v4-pro-autodelete",
		"deepseek-v4-flash-search-autodelete",
		"deepseek-v4-pro-search-autodelete",
		"deepseek-v4-vision-autodelete",
	}
	seen := map[string]bool{}
	for _, m := range DeepSeekModels {
		seen[m.ID] = true
	}
	for _, id := range expected {
		if !seen[id] {
			t.Fatalf("expected %q in DeepSeekModels, got: %v", id, seen)
		}
	}
}

func TestResolveModelCustomAliasToExpert(t *testing.T) {
	got, ok := ResolveModel(mockModelAliasReader{
		"my-expert-model": "deepseek-v4-pro-search",
	}, "my-expert-model")
	if !ok || got != "deepseek-v4-pro-search" {
		t.Fatalf("expected alias -> deepseek-v4-pro-search, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelCustomAliasToVision(t *testing.T) {
	got, ok := ResolveModel(mockModelAliasReader{
		"my-vision-model": "deepseek-v4-vision",
	}, "my-vision-model")
	if !ok || got != "deepseek-v4-vision" {
		t.Fatalf("expected alias -> deepseek-v4-vision, got ok=%v model=%q", ok, got)
	}
}

func TestClaudeModelsResponsePaginationFields(t *testing.T) {
	resp := ClaudeModelsResponse()
	if _, ok := resp["first_id"]; !ok {
		t.Fatalf("expected first_id in response: %#v", resp)
	}
	if _, ok := resp["last_id"]; !ok {
		t.Fatalf("expected last_id in response: %#v", resp)
	}
	if _, ok := resp["has_more"]; !ok {
		t.Fatalf("expected has_more in response: %#v", resp)
	}
}

// TestClaudeModelsIncludesDeepSeekDirectVariants 验证 Anthropic /v1/messages 入口
// 直接接受 deepseek-v4-* 系列模型（含 -nothinking / -autodelete 变体），
// 让 Anthropic SDK 用户也能用 deepseek-v4-flash-autodelete 等模型名触发 auto-delete。
func TestClaudeModelsIncludesDeepSeekDirectVariants(t *testing.T) {
	expected := []string{
		"deepseek-v4-flash",
		"deepseek-v4-flash-nothinking",
		"deepseek-v4-flash-autodelete",
		"deepseek-v4-flash-forcehistory",
		"deepseek-v4-pro",
		"deepseek-v4-pro-nothinking",
		"deepseek-v4-pro-autodelete",
		"deepseek-v4-pro-forcehistory",
		"deepseek-v4-flash-search",
		"deepseek-v4-flash-search-nothinking",
		"deepseek-v4-flash-search-autodelete",
		"deepseek-v4-flash-search-forcehistory",
		"deepseek-v4-pro-search",
		"deepseek-v4-pro-search-nothinking",
		"deepseek-v4-pro-search-autodelete",
		"deepseek-v4-pro-search-forcehistory",
		"deepseek-v4-vision",
		"deepseek-v4-vision-nothinking",
		"deepseek-v4-vision-autodelete",
		"deepseek-v4-vision-forcehistory",
	}
	seen := map[string]bool{}
	for _, m := range ClaudeModels {
		seen[m.ID] = true
	}
	for _, id := range expected {
		if !seen[id] {
			t.Fatalf("expected %q in ClaudeModels (so /v1/messages accepts direct DeepSeek model names), got missing", id)
		}
	}
}

// --- -forcehistory suffix model tests ---

func TestResolveModelDirectDeepSeekForceHistory(t *testing.T) {
	got, ok := ResolveModel(nil, "deepseek-v4-flash-forcehistory")
	if !ok || got != "deepseek-v4-flash-forcehistory" {
		t.Fatalf("expected deepseek-v4-flash-forcehistory, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelAliasWithForceHistorySuffix(t *testing.T) {
	// gpt-4.1 alias + -forcehistory suffix should resolve to deepseek-v4-flash-forcehistory
	got, ok := ResolveModel(nil, "gpt-4.1-forcehistory")
	if !ok || got != "deepseek-v4-flash-forcehistory" {
		t.Fatalf("expected alias gpt-4.1-forcehistory -> deepseek-v4-flash-forcehistory, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelClaudeAliasWithForceHistorySuffix(t *testing.T) {
	got, ok := ResolveModel(nil, "claude-sonnet-4-6-forcehistory")
	if !ok || got != "deepseek-v4-flash-forcehistory" {
		t.Fatalf("expected alias claude-sonnet-4-6-forcehistory -> deepseek-v4-flash-forcehistory, got ok=%v model=%q", ok, got)
	}
}

func TestIsForceHistoryModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4-flash-forcehistory", true},
		{"deepseek-v4-pro-search-forcehistory", true},
		{"deepseek-v4-flash", false},
		{"deepseek-v4-flash-nothinking", false},
		{"deepseek-v4-flash-autodelete", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsForceHistoryModel(tc.model); got != tc.want {
			t.Fatalf("IsForceHistoryModel(%q)=%v want=%v", tc.model, got, tc.want)
		}
	}
}

func TestGetModelConfigStripsForceHistorySuffix(t *testing.T) {
	thinking, search, ok := GetModelConfig("deepseek-v4-flash-forcehistory")
	if !ok || !thinking || search {
		t.Fatalf("expected deepseek-v4-flash-forcehistory to behave as flash (thinking=true, search=false), got thinking=%v search=%v ok=%v", thinking, search, ok)
	}
	thinking, search, ok = GetModelConfig("deepseek-v4-pro-search-forcehistory")
	if !ok || !thinking || !search {
		t.Fatalf("expected deepseek-v4-pro-search-forcehistory to behave as pro-search (thinking=true, search=true), got thinking=%v search=%v ok=%v", thinking, search, ok)
	}
}

func TestGetModelTypeStripsForceHistorySuffix(t *testing.T) {
	mt, ok := GetModelType("deepseek-v4-flash-forcehistory")
	if !ok || mt != "default" {
		t.Fatalf("expected deepseek-v4-flash-forcehistory -> default, got mt=%q ok=%v", mt, ok)
	}
	mt, ok = GetModelType("deepseek-v4-pro-forcehistory")
	if !ok || mt != "expert" {
		t.Fatalf("expected deepseek-v4-pro-forcehistory -> expert, got mt=%q ok=%v", mt, ok)
	}
}

func TestDeepSeekModelsIncludesForceHistoryVariants(t *testing.T) {
	// 验证 /v1/models 列表包含 -forcehistory 变体
	expected := []string{
		"deepseek-v4-flash-forcehistory",
		"deepseek-v4-pro-forcehistory",
		"deepseek-v4-flash-search-forcehistory",
		"deepseek-v4-pro-search-forcehistory",
		"deepseek-v4-vision-forcehistory",
	}
	seen := map[string]bool{}
	for _, m := range DeepSeekModels {
		seen[m.ID] = true
	}
	for _, id := range expected {
		if !seen[id] {
			t.Fatalf("expected %q in DeepSeekModels, got: %v", id, seen)
		}
	}
}

// --- 双后缀 / 三后缀叠加测试 ---

func TestDeepSeekModelsIncludesAllStackedVariants(t *testing.T) {
	// 验证 /v1/models 列表包含每个 base 模型的全部 12 种合法后缀组合
	// 5 base × 12 = 60 个变体（4 个后缀，nothinking 与 thinkinginject 互斥）
	if got, want := len(DeepSeekModels), 60; got != want {
		t.Fatalf("DeepSeekModels count = %d, want %d (5 base × 12 combinations)", got, want)
	}
	// 抽样验证各种叠加形态：单后缀 / 双后缀 / 三后缀
	expected := []string{
		"deepseek-v4-flash-thinkinginject",
		"deepseek-v4-flash-forcehistory-autodelete",
		"deepseek-v4-flash-forcehistory-thinkinginject",
		"deepseek-v4-flash-nothinking-forcehistory",
		"deepseek-v4-flash-nothinking-autodelete",
		"deepseek-v4-flash-nothinking-forcehistory-autodelete",
		"deepseek-v4-flash-forcehistory-autodelete-thinkinginject",
		"deepseek-v4-pro-search-forcehistory-autodelete",
		"deepseek-v4-pro-search-nothinking-forcehistory-autodelete",
		"deepseek-v4-pro-search-forcehistory-autodelete-thinkinginject",
		"deepseek-v4-vision-nothinking-forcehistory-autodelete",
		"deepseek-v4-vision-thinkinginject",
		"deepseek-v4-vision-forcehistory-thinkinginject",
	}
	seen := map[string]bool{}
	for _, m := range DeepSeekModels {
		seen[m.ID] = true
	}
	for _, id := range expected {
		if !seen[id] {
			t.Fatalf("expected %q in DeepSeekModels, got: %v", id, seen)
		}
	}
	// 验证 nothinking+thinkinginject 组合不会被生成
	forbidden := []string{
		"deepseek-v4-flash-nothinking-thinkinginject",
		"deepseek-v4-flash-nothinking-forcehistory-thinkinginject",
		"deepseek-v4-flash-nothinking-autodelete-thinkinginject",
		"deepseek-v4-flash-nothinking-forcehistory-autodelete-thinkinginject",
	}
	for _, id := range forbidden {
		if seen[id] {
			t.Fatalf("expected %q NOT in DeepSeekModels (nothinking+thinkinginject is meaningless), got present", id)
		}
	}
}

func TestResolveModelDirectDeepSeekStackedDouble(t *testing.T) {
	got, ok := ResolveModel(nil, "deepseek-v4-flash-forcehistory-autodelete")
	if !ok || got != "deepseek-v4-flash-forcehistory-autodelete" {
		t.Fatalf("expected deepseek-v4-flash-forcehistory-autodelete, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelDirectDeepSeekStackedTriple(t *testing.T) {
	got, ok := ResolveModel(nil, "deepseek-v4-pro-nothinking-forcehistory-autodelete")
	if !ok || got != "deepseek-v4-pro-nothinking-forcehistory-autodelete" {
		t.Fatalf("expected deepseek-v4-pro-nothinking-forcehistory-autodelete, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelAliasWithStackedSuffix(t *testing.T) {
	// gpt-4.1 alias + 双后缀 -> deepseek-v4-flash-forcehistory-autodelete
	got, ok := ResolveModel(nil, "gpt-4.1-forcehistory-autodelete")
	if !ok || got != "deepseek-v4-flash-forcehistory-autodelete" {
		t.Fatalf("expected alias gpt-4.1-forcehistory-autodelete -> deepseek-v4-flash-forcehistory-autodelete, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelAliasWithTripleSuffix(t *testing.T) {
	// claude-sonnet-4-6 alias + 三后缀 -> deepseek-v4-flash-nothinking-forcehistory-autodelete
	got, ok := ResolveModel(nil, "claude-sonnet-4-6-nothinking-forcehistory-autodelete")
	if !ok || got != "deepseek-v4-flash-nothinking-forcehistory-autodelete" {
		t.Fatalf("expected alias claude-sonnet-4-6-nothinking-forcehistory-autodelete -> deepseek-v4-flash-nothinking-forcehistory-autodelete, got ok=%v model=%q", ok, got)
	}
}

// TestResolveModelNormalizesSuffixOrder 验证非规范顺序的输入也能正确解析。
// 用户可能传 deepseek-v4-flash-autodelete-forcehistory（autodelete 在前），
// 解析后应识别出两个后缀并返回 true（虽然不会重新规范化输出，但 IsXxxModel 应正确）。
func TestResolveModelNormalizesSuffixOrder(t *testing.T) {
	// 直接传 DeepSeek 模型名，不论顺序如何都应被识别为 supported
	got, ok := ResolveModel(nil, "deepseek-v4-flash-autodelete-forcehistory")
	if !ok {
		t.Fatalf("expected non-canonical order to still resolve, got ok=%v model=%q", ok, got)
	}
	if got != "deepseek-v4-flash-autodelete-forcehistory" {
		t.Fatalf("expected direct hit returns input as-is, got %q", got)
	}
	// 后缀位应被正确识别
	if !IsAutoDeleteModel(got) {
		t.Fatalf("IsAutoDeleteModel(%q) = false, want true", got)
	}
	if !IsForceHistoryModel(got) {
		t.Fatalf("IsForceHistoryModel(%q) = false, want true", got)
	}
	if IsNoThinkingModel(got) {
		t.Fatalf("IsNoThinkingModel(%q) = true, want false", got)
	}
}

func TestIsAutoDeleteModelStacked(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4-flash-autodelete", true},
		{"deepseek-v4-flash-forcehistory-autodelete", true},
		{"deepseek-v4-flash-nothinking-autodelete", true},
		{"deepseek-v4-flash-nothinking-forcehistory-autodelete", true},
		{"deepseek-v4-pro-search-autodelete", true},
		{"deepseek-v4-flash", false},
		{"deepseek-v4-flash-nothinking", false},
		{"deepseek-v4-flash-forcehistory", false},
		{"deepseek-v4-flash-nothinking-forcehistory", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsAutoDeleteModel(tc.model); got != tc.want {
			t.Fatalf("IsAutoDeleteModel(%q)=%v want=%v", tc.model, got, tc.want)
		}
	}
}

func TestIsForceHistoryModelStacked(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4-flash-forcehistory", true},
		{"deepseek-v4-flash-forcehistory-autodelete", true},
		{"deepseek-v4-flash-nothinking-forcehistory", true},
		{"deepseek-v4-flash-nothinking-forcehistory-autodelete", true},
		{"deepseek-v4-pro-search-forcehistory", true},
		{"deepseek-v4-flash", false},
		{"deepseek-v4-flash-nothinking", false},
		{"deepseek-v4-flash-autodelete", false},
		{"deepseek-v4-flash-nothinking-autodelete", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsForceHistoryModel(tc.model); got != tc.want {
			t.Fatalf("IsForceHistoryModel(%q)=%v want=%v", tc.model, got, tc.want)
		}
	}
}

func TestIsNoThinkingModelStacked(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4-flash-nothinking", true},
		{"deepseek-v4-flash-nothinking-forcehistory", true},
		{"deepseek-v4-flash-nothinking-autodelete", true},
		{"deepseek-v4-flash-nothinking-forcehistory-autodelete", true},
		{"deepseek-v4-flash", false},
		{"deepseek-v4-flash-forcehistory", false},
		{"deepseek-v4-flash-autodelete", false},
		{"deepseek-v4-flash-forcehistory-autodelete", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsNoThinkingModel(tc.model); got != tc.want {
			t.Fatalf("IsNoThinkingModel(%q)=%v want=%v", tc.model, got, tc.want)
		}
	}
}

func TestGetModelConfigStripsStackedSuffixes(t *testing.T) {
	// 三后缀模型仍应正确识别 base 与 thinking 配置
	thinking, search, ok := GetModelConfig("deepseek-v4-flash-nothinking-forcehistory-autodelete")
	if !ok || thinking || search {
		t.Fatalf("expected deepseek-v4-flash-nothinking-forcehistory-autodelete to behave as flash non-thinking (thinking=false, search=false), got thinking=%v search=%v ok=%v", thinking, search, ok)
	}
	thinking, search, ok = GetModelConfig("deepseek-v4-pro-search-forcehistory-autodelete")
	if !ok || !thinking || !search {
		t.Fatalf("expected deepseek-v4-pro-search-forcehistory-autodelete to behave as pro-search (thinking=true, search=true), got thinking=%v search=%v ok=%v", thinking, search, ok)
	}
}

func TestGetModelTypeStripsStackedSuffixes(t *testing.T) {
	mt, ok := GetModelType("deepseek-v4-flash-nothinking-forcehistory-autodelete")
	if !ok || mt != "default" {
		t.Fatalf("expected deepseek-v4-flash-nothinking-forcehistory-autodelete -> default, got mt=%q ok=%v", mt, ok)
	}
	mt, ok = GetModelType("deepseek-v4-pro-search-forcehistory-autodelete")
	if !ok || mt != "expert" {
		t.Fatalf("expected deepseek-v4-pro-search-forcehistory-autodelete -> expert, got mt=%q ok=%v", mt, ok)
	}
	mt, ok = GetModelType("deepseek-v4-vision-nothinking-forcehistory-autodelete")
	if !ok || mt != "vision" {
		t.Fatalf("expected deepseek-v4-vision-nothinking-forcehistory-autodelete -> vision, got mt=%q ok=%v", mt, ok)
	}
}

func TestOllamaCapabilitiesModelsIncludesAllStackedVariants(t *testing.T) {
	// 验证 OllamaCapabilitiesModels 包含全部 60 条记录（5 base × 12 combinations）
	if got, want := len(OllamaCapabilitiesModels), 60; got != want {
		t.Fatalf("OllamaCapabilitiesModels count = %d, want %d", got, want)
	}
	// 抽样：vision 三后缀全开仍应保留 vision 能力，但 nothinking 应去掉 thinking
	var visionTriple *OllamaCapabilitiesModelInfo
	for i := range OllamaCapabilitiesModels {
		if OllamaCapabilitiesModels[i].ID == "deepseek-v4-vision-nothinking-forcehistory-autodelete" {
			visionTriple = &OllamaCapabilitiesModels[i]
			break
		}
	}
	if visionTriple == nil {
		t.Fatal("expected deepseek-v4-vision-nothinking-forcehistory-autodelete in OllamaCapabilitiesModels")
	}
	caps := map[string]bool{}
	for _, c := range visionTriple.Capabilities {
		caps[c] = true
	}
	if !caps["tools"] {
		t.Fatalf("expected tools capability, got %v", visionTriple.Capabilities)
	}
	if !caps["vision"] {
		t.Fatalf("expected vision capability, got %v", visionTriple.Capabilities)
	}
	if caps["thinking"] {
		t.Fatalf("expected nothinking variant to drop thinking, got %v", visionTriple.Capabilities)
	}
}

// --- -thinkinginject 后缀测试 ---

func TestResolveModelDirectDeepSeekThinkingInject(t *testing.T) {
	got, ok := ResolveModel(nil, "deepseek-v4-flash-thinkinginject")
	if !ok || got != "deepseek-v4-flash-thinkinginject" {
		t.Fatalf("expected deepseek-v4-flash-thinkinginject, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelDirectDeepSeekQuadSuffix(t *testing.T) {
	// 三后缀（forcehistory + autodelete + thinkinginject）
	got, ok := ResolveModel(nil, "deepseek-v4-flash-forcehistory-autodelete-thinkinginject")
	if !ok || got != "deepseek-v4-flash-forcehistory-autodelete-thinkinginject" {
		t.Fatalf("expected triple-suffix (forcehistory+autodelete+thinkinginject), got ok=%v model=%q", ok, got)
	}
}

// TestResolveModelDirectHitOnNoThinkingAndThinkingInjectCombo 验证 nothinking+thinkinginject
// 组合的"直接 DeepSeek 风格 ID"会被 ResolveModel 接受（因为 base 模型合法），
// 但该 ID 不会出现在 /v1/models 列表中（suffixCombinations 已剔除该组合）。
//
// 真正的降级行为在 alias 路径生效，见 TestResolveModelAliasWithQuadSuffix。
// 直接命中路径不经过 withSuffixes 重新拼接（保留用户输入原样），但运行时
// ApplyThinkingInjection 的 stdReq.Thinking 检查会短路（nothinking 让 Thinking=false）。
func TestResolveModelDirectHitOnNoThinkingAndThinkingInjectCombo(t *testing.T) {
	// 直接命中路径：原样返回（不规范化），但该 ID 不在 /v1/models 列表中
	got, ok := ResolveModel(nil, "deepseek-v4-flash-nothinking-forcehistory-autodelete-thinkinginject")
	if !ok {
		t.Fatalf("expected direct hit to still resolve (base is valid), got ok=%v", ok)
	}
	if got != "deepseek-v4-flash-nothinking-forcehistory-autodelete-thinkinginject" {
		t.Fatalf("expected direct hit returns input as-is, got %q", got)
	}
	// 但这个 ID 不在 DeepSeekModels 列表中（被 suffixCombinations 排除）
	for _, m := range DeepSeekModels {
		if m.ID == got {
			t.Fatalf("expected %q NOT in DeepSeekModels (nothinking+thinkinginject is meaningless), but found", got)
		}
	}
}

func TestResolveModelAliasWithThinkingInjectSuffix(t *testing.T) {
	// gpt-4.1 alias + -thinkinginject -> deepseek-v4-flash-thinkinginject
	got, ok := ResolveModel(nil, "gpt-4.1-thinkinginject")
	if !ok || got != "deepseek-v4-flash-thinkinginject" {
		t.Fatalf("expected alias gpt-4.1-thinkinginject -> deepseek-v4-flash-thinkinginject, got ok=%v model=%q", ok, got)
	}
}

func TestResolveModelAliasWithQuadSuffix(t *testing.T) {
	// claude-sonnet-4-6 alias + nothinking+forcehistory+autodelete+thinkinginject（矛盾组合）
	// 应自动降级为 deepseek-v4-flash-nothinking-forcehistory-autodelete（nothinking 优先，丢弃 thinkinginject）
	got, ok := ResolveModel(nil, "claude-sonnet-4-6-nothinking-forcehistory-autodelete-thinkinginject")
	if !ok {
		t.Fatalf("expected alias + quad suffix to resolve (downgraded), got ok=%v", ok)
	}
	if got != "deepseek-v4-flash-nothinking-forcehistory-autodelete" {
		t.Fatalf("expected nothinking+thinkinginject combo to be downgraded to nothinking only, got %q", got)
	}
}

func TestIsThinkingInjectModelStacked(t *testing.T) {
	// 注意：nothinking+thinkinginject 组合不会从 /v1/models 列表生成，
	// 但用户可能直接传入这种字符串。parseModelSuffixes 宽容识别，
	// IsThinkingInjectModel 仍返回 true（但运行时不会触发注入，因为 stdReq.Thinking=false）。
	cases := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4-flash-thinkinginject", true},
		{"deepseek-v4-flash-forcehistory-thinkinginject", true},
		{"deepseek-v4-flash-autodelete-thinkinginject", true},
		{"deepseek-v4-flash-forcehistory-autodelete-thinkinginject", true},
		{"deepseek-v4-pro-search-thinkinginject", true},
		{"deepseek-v4-flash-nothinking-thinkinginject", true},                         // 用户直传的矛盾组合，谓词仍返回 true
		{"deepseek-v4-flash-nothinking-forcehistory-autodelete-thinkinginject", true}, // 同上
		{"deepseek-v4-flash", false},
		{"deepseek-v4-flash-nothinking", false},
		{"deepseek-v4-flash-forcehistory", false},
		{"deepseek-v4-flash-autodelete", false},
		{"deepseek-v4-flash-forcehistory-autodelete", false},
		{"deepseek-v4-flash-nothinking-forcehistory-autodelete", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsThinkingInjectModel(tc.model); got != tc.want {
			t.Fatalf("IsThinkingInjectModel(%q)=%v want=%v", tc.model, got, tc.want)
		}
	}
}

func TestGetModelConfigStripsThinkingInjectSuffix(t *testing.T) {
	// thinkinginject 后缀不影响 thinking/search 配置
	thinking, search, ok := GetModelConfig("deepseek-v4-flash-thinkinginject")
	if !ok || !thinking || search {
		t.Fatalf("expected deepseek-v4-flash-thinkinginject to behave as flash (thinking=true, search=false), got thinking=%v search=%v ok=%v", thinking, search, ok)
	}
	thinking, search, ok = GetModelConfig("deepseek-v4-pro-search-forcehistory-autodelete-thinkinginject")
	if !ok || !thinking || !search {
		t.Fatalf("expected quad-suffix pro-search to behave as pro-search (thinking=true, search=true), got thinking=%v search=%v ok=%v", thinking, search, ok)
	}
}

func TestGetModelTypeStripsThinkingInjectSuffix(t *testing.T) {
	mt, ok := GetModelType("deepseek-v4-flash-thinkinginject")
	if !ok || mt != "default" {
		t.Fatalf("expected deepseek-v4-flash-thinkinginject -> default, got mt=%q ok=%v", mt, ok)
	}
	mt, ok = GetModelType("deepseek-v4-pro-thinkinginject")
	if !ok || mt != "expert" {
		t.Fatalf("expected deepseek-v4-pro-thinkinginject -> expert, got mt=%q ok=%v", mt, ok)
	}
	mt, ok = GetModelType("deepseek-v4-vision-thinkinginject")
	if !ok || mt != "vision" {
		t.Fatalf("expected deepseek-v4-vision-thinkinginject -> vision, got mt=%q ok=%v", mt, ok)
	}
}

// TestResolveModelNormalizesQuadSuffixOrder 验证非规范顺序也能解析。
// 注意：nothinking+thinkinginject 组合在 alias 解析后会被 withSuffixes 自动降级为只有 nothinking。
func TestResolveModelNormalizesQuadSuffixOrder(t *testing.T) {
	// 用户传 thinkinginject 在最前面，也能识别。这是 alias 路径，会走 withSuffixes 降级。
	// 期望：nothinking 保留，thinkinginject 被丢弃。
	got, ok := ResolveModel(nil, "gpt-4.1-thinkinginject-autodelete-forcehistory-nothinking")
	if !ok {
		t.Fatalf("expected non-canonical order to still resolve, got ok=%v model=%q", ok, got)
	}
	if got != "deepseek-v4-flash-nothinking-forcehistory-autodelete" {
		t.Fatalf("expected downgraded to nothinking only (no thinkinginject), got %q", got)
	}
	// 解析后的 resolved ID 不应带 thinkinginject 后缀
	if IsThinkingInjectModel(got) {
		t.Fatalf("IsThinkingInjectModel(%q) = true, want false (should be downgraded)", got)
	}
	if !IsAutoDeleteModel(got) {
		t.Fatalf("IsAutoDeleteModel(%q) = false, want true", got)
	}
	if !IsForceHistoryModel(got) {
		t.Fatalf("IsForceHistoryModel(%q) = false, want true", got)
	}
	if !IsNoThinkingModel(got) {
		t.Fatalf("IsNoThinkingModel(%q) = false, want true", got)
	}
}
