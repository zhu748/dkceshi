package config

import (
        "strings"
        "time"
)

type ModelInfo struct {
        ID         string `json:"id"`
        Object     string `json:"object"`
        Created    int64  `json:"created"`
        OwnedBy    string `json:"owned_by"`
        Permission []any  `json:"permission,omitempty"`
}
type OllamaModelInfo struct {
        Name       string `json:"name"`
        Model      string `json:"model"`
        Size       int64  `json:"size"`
        ModifiedAt string `json:"modified_at"`
}
type OllamaCapabilitiesModelInfo struct {
        ID           string   `json:"id"`
        Capabilities []string `json:"capabilities"`
}

type ModelAliasReader interface {
        ModelAliases() map[string]string
}

const noThinkingModelSuffix     = "-nothinking"
const autoDeleteModelSuffix    = "-autodelete"
const forceHistoryModelSuffix  = "-forcehistory"

var deepSeekBaseModels = []ModelInfo{
        {ID: "deepseek-v4-flash", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-pro", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-flash-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-pro-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-vision", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
}

var OllamaCapabilitiesModels = []OllamaCapabilitiesModelInfo{
        {ID: "deepseek-v4-flash", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-pro", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-flash-search", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-pro-search", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-vision", Capabilities: []string{"tools", "thinking", "vision"}},
        {ID: "deepseek-v4-flash-nothinking", Capabilities: []string{"tools"}},
        {ID: "deepseek-v4-pro-nothinking", Capabilities: []string{"tools"}},
        {ID: "deepseek-v4-flash-search-nothinking", Capabilities: []string{"tools"}},
        {ID: "deepseek-v4-pro-search-nothinking", Capabilities: []string{"tools"}},
        {ID: "deepseek-v4-vision-nothinking", Capabilities: []string{"tools", "vision"}},
        {ID: "deepseek-v4-flash-autodelete", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-pro-autodelete", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-flash-search-autodelete", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-pro-search-autodelete", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-vision-autodelete", Capabilities: []string{"tools", "thinking", "vision"}},
        {ID: "deepseek-v4-flash-forcehistory", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-pro-forcehistory", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-flash-search-forcehistory", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-pro-search-forcehistory", Capabilities: []string{"tools", "thinking"}},
        {ID: "deepseek-v4-vision-forcehistory", Capabilities: []string{"tools", "thinking", "vision"}},
}

var DeepSeekModels = appendNoThinkingVariants(deepSeekBaseModels)
var OllamaModels = mapToOllamaModels(DeepSeekModels)
var claudeBaseModels = []ModelInfo{
        // Current aliases
        {ID: "claude-opus-4-6", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-sonnet-4-6", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-haiku-4-5", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},

        // Claude 4.x snapshots and prior aliases kept for compatibility
        {ID: "claude-sonnet-4-5", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-opus-4-1", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-opus-4-1-20250805", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-opus-4-0", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-opus-4-20250514", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-sonnet-4-5-20250929", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-sonnet-4-0", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-sonnet-4-20250514", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-haiku-4-5-20251001", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},

        // Claude 3.x (legacy/deprecated snapshots and aliases)
        {ID: "claude-3-7-sonnet-latest", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-3-7-sonnet-20250219", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-3-5-sonnet-latest", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-3-5-sonnet-20240620", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-3-5-sonnet-20241022", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-3-opus-20240229", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-3-sonnet-20240229", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-3-5-haiku-latest", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-3-5-haiku-20241022", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},
        {ID: "claude-3-haiku-20240307", Object: "model", Created: 1715635200, OwnedBy: "anthropic"},

        // DeepSeek 系列直连模型 —— 让 Anthropic SDK 直接传 model="deepseek-v4-flash-autodelete"
        // 给 /v1/messages 也能识别并触发 auto-delete。OwnedBy 用 "deepseek" 与 OpenAI 端一致。
        {ID: "deepseek-v4-flash", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-pro", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-flash-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-pro-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-vision", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
}

var ClaudeModels = appendNoThinkingVariants(claudeBaseModels)

func GetModelConfig(model string) (thinking bool, search bool, ok bool) {
        baseModel, noThinking := splitNoThinkingModel(model)
        baseModel, _ = splitAutoDeleteModel(baseModel)
        baseModel, _ = splitForceHistoryModel(baseModel)
        if baseModel == "" {
                return false, false, false
        }
        switch baseModel {
        case "deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4-vision":
                return !noThinking, false, true
        case "deepseek-v4-flash-search", "deepseek-v4-pro-search":
                return !noThinking, true, true
        default:
                return false, false, false
        }
}

func GetModelType(model string) (modelType string, ok bool) {
        baseModel, _ := splitNoThinkingModel(model)
        baseModel, _ = splitAutoDeleteModel(baseModel)
        baseModel, _ = splitForceHistoryModel(baseModel)
        switch baseModel {
        case "deepseek-v4-flash", "deepseek-v4-flash-search":
                return "default", true
        case "deepseek-v4-pro", "deepseek-v4-pro-search":
                return "expert", true
        case "deepseek-v4-vision":
                return "vision", true
        default:
                return "", false
        }
}

func IsSupportedDeepSeekModel(model string) bool {
        _, _, ok := GetModelConfig(model)
        return ok
}

func IsNoThinkingModel(model string) bool {
        _, noThinking := splitNoThinkingModel(model)
        return noThinking
}

// IsAutoDeleteModel 判断模型 ID 是否带 -autodelete 后缀。
// 调用方在响应完成后据此触发 chat_session/delete 删除本次对话，
// 与全局 auto_delete.mode 联动：suffix 模型仅当全局 mode=none 时生效，
// 全局 mode=single/all 时保持全局策略不变。
func IsAutoDeleteModel(model string) bool {
        _, autoDelete := splitAutoDeleteModel(model)
        return autoDelete
}

// IsForceHistoryModel 判断模型 ID 是否带 -forcehistory 后缀。
// 命中时，调用方应无视全局 current_input_file.enabled 开关，
// 在本次请求中强制启用历史拆分（上传历史为文件）。
func IsForceHistoryModel(model string) bool {
        _, forceHistory := splitForceHistoryModel(model)
        return forceHistory
}

func DefaultModelAliases() map[string]string {
        return map[string]string{
                // OpenAI GPT / ChatGPT families
                "chatgpt-4o":          "deepseek-v4-flash",
                "gpt-4":               "deepseek-v4-flash",
                "gpt-4-turbo":         "deepseek-v4-flash",
                "gpt-4-turbo-preview": "deepseek-v4-flash",
                "gpt-4.5-preview":     "deepseek-v4-flash",
                "gpt-4o":              "deepseek-v4-flash",
                "gpt-4o-mini":         "deepseek-v4-flash",
                "gpt-4.1":             "deepseek-v4-flash",
                "gpt-4.1-mini":        "deepseek-v4-flash",
                "gpt-4.1-nano":        "deepseek-v4-flash",
                "gpt-5":               "deepseek-v4-flash",
                "gpt-5-chat":          "deepseek-v4-flash",
                "gpt-5.1":             "deepseek-v4-flash",
                "gpt-5.1-chat":        "deepseek-v4-flash",
                "gpt-5.2":             "deepseek-v4-flash",
                "gpt-5.2-chat":        "deepseek-v4-flash",
                "gpt-5.3-chat":        "deepseek-v4-flash",
                "gpt-5.4":             "deepseek-v4-flash",
                "gpt-5.5":             "deepseek-v4-flash",
                "gpt-5-mini":          "deepseek-v4-flash",
                "gpt-5-nano":          "deepseek-v4-flash",
                "gpt-5.4-mini":        "deepseek-v4-flash",
                "gpt-5.4-nano":        "deepseek-v4-flash",
                "gpt-5-pro":           "deepseek-v4-pro",
                "gpt-5.2-pro":         "deepseek-v4-pro",
                "gpt-5.4-pro":         "deepseek-v4-pro",
                "gpt-5.5-pro":         "deepseek-v4-pro",
                "gpt-5-codex":         "deepseek-v4-pro",
                "gpt-5.1-codex":       "deepseek-v4-pro",
                "gpt-5.1-codex-mini":  "deepseek-v4-pro",
                "gpt-5.1-codex-max":   "deepseek-v4-pro",
                "gpt-5.2-codex":       "deepseek-v4-pro",
                "gpt-5.3-codex":       "deepseek-v4-pro",
                "codex-mini-latest":   "deepseek-v4-pro",

                // OpenAI reasoning / research families
                "o1":                    "deepseek-v4-pro",
                "o1-preview":            "deepseek-v4-pro",
                "o1-mini":               "deepseek-v4-pro",
                "o1-pro":                "deepseek-v4-pro",
                "o3":                    "deepseek-v4-pro",
                "o3-mini":               "deepseek-v4-pro",
                "o3-pro":                "deepseek-v4-pro",
                "o3-deep-research":      "deepseek-v4-pro-search",
                "o4-mini":               "deepseek-v4-pro",
                "o4-mini-deep-research": "deepseek-v4-pro-search",

                // Claude current and historical aliases
                "claude-opus-4-6":            "deepseek-v4-pro",
                "claude-opus-4-1":            "deepseek-v4-pro",
                "claude-opus-4-1-20250805":   "deepseek-v4-pro",
                "claude-opus-4-0":            "deepseek-v4-pro",
                "claude-opus-4-20250514":     "deepseek-v4-pro",
                "claude-sonnet-4-6":          "deepseek-v4-flash",
                "claude-sonnet-4-5":          "deepseek-v4-flash",
                "claude-sonnet-4-5-20250929": "deepseek-v4-flash",
                "claude-sonnet-4-0":          "deepseek-v4-flash",
                "claude-sonnet-4-20250514":   "deepseek-v4-flash",
                "claude-haiku-4-5":           "deepseek-v4-flash",
                "claude-haiku-4-5-20251001":  "deepseek-v4-flash",
                "claude-3-7-sonnet":          "deepseek-v4-flash",
                "claude-3-7-sonnet-latest":   "deepseek-v4-flash",
                "claude-3-7-sonnet-20250219": "deepseek-v4-flash",
                "claude-3-5-sonnet":          "deepseek-v4-flash",
                "claude-3-5-sonnet-latest":   "deepseek-v4-flash",
                "claude-3-5-sonnet-20240620": "deepseek-v4-flash",
                "claude-3-5-sonnet-20241022": "deepseek-v4-flash",
                "claude-3-5-haiku":           "deepseek-v4-flash",
                "claude-3-5-haiku-latest":    "deepseek-v4-flash",
                "claude-3-5-haiku-20241022":  "deepseek-v4-flash",
                "claude-3-opus":              "deepseek-v4-pro",
                "claude-3-opus-20240229":     "deepseek-v4-pro",
                "claude-3-sonnet":            "deepseek-v4-flash",
                "claude-3-sonnet-20240229":   "deepseek-v4-flash",
                "claude-3-haiku":             "deepseek-v4-flash",
                "claude-3-haiku-20240307":    "deepseek-v4-flash",

                // Gemini current and historical text / multimodal models
                "gemini-pro":            "deepseek-v4-pro",
                "gemini-pro-vision":     "deepseek-v4-vision",
                "gemini-pro-latest":     "deepseek-v4-pro",
                "gemini-flash-latest":   "deepseek-v4-flash",
                "gemini-1.5-pro":        "deepseek-v4-pro",
                "gemini-1.5-flash":      "deepseek-v4-flash",
                "gemini-1.5-flash-8b":   "deepseek-v4-flash",
                "gemini-2.0-flash":      "deepseek-v4-flash",
                "gemini-2.0-flash-lite": "deepseek-v4-flash",
                "gemini-2.5-pro":        "deepseek-v4-pro",
                "gemini-2.5-flash":      "deepseek-v4-flash",
                "gemini-2.5-flash-lite": "deepseek-v4-flash",
                "gemini-3.1-pro":        "deepseek-v4-pro",
                "gemini-3-pro":          "deepseek-v4-pro",
                "gemini-3-flash":        "deepseek-v4-flash",
                "gemini-3.1-flash":      "deepseek-v4-flash",
                "gemini-3.1-flash-lite": "deepseek-v4-flash",

                "llama-3.1-70b-instruct": "deepseek-v4-flash",
                "qwen-max":               "deepseek-v4-flash",
        }
}

func ResolveModel(store ModelAliasReader, requested string) (string, bool) {
        model := lower(strings.TrimSpace(requested))
        if model == "" {
                return "", false
        }
        aliases := loadModelAliases(store)
        if IsSupportedDeepSeekModel(model) {
                return model, true
        }
        if mapped, ok := aliases[model]; ok && IsSupportedDeepSeekModel(mapped) {
                return mapped, true
        }
        baseModel, noThinking := splitNoThinkingModel(model)
        baseModel, autoDelete := splitAutoDeleteModel(baseModel)
        baseModel, forceHistory := splitForceHistoryModel(baseModel)
        if baseModel != model {
                // 用户传入了带后缀的形式（如 gpt-4.1-nothinking / gpt-4.1-autodelete /
                // gpt-4.1-forcehistory）。
                if mapped, ok := aliases[baseModel]; ok && IsSupportedDeepSeekModel(mapped) {
                        mapped = withNoThinkingVariant(mapped, noThinking)
                        mapped = withAutoDeleteVariant(mapped, autoDelete)
                        mapped = withForceHistoryVariant(mapped, forceHistory)
                        return mapped, true
                }
        }
        return "", false
}

func lower(s string) string {
        b := []byte(s)
        for i, c := range b {
                if c >= 'A' && c <= 'Z' {
                        b[i] = c + 32
                }
        }
        return string(b)
}

func OpenAIModelsResponse() map[string]any {
        return map[string]any{"object": "list", "data": DeepSeekModels}
}

func OpenAIModelByID(store ModelAliasReader, id string) (ModelInfo, bool) {
        canonical, ok := ResolveModel(store, id)
        if !ok {
                return ModelInfo{}, false
        }
        for _, model := range DeepSeekModels {
                if model.ID == canonical {
                        return model, true
                }
        }
        return ModelInfo{}, false
}

func OllamaModelsResponse() map[string]any {
        return map[string]any{"models": OllamaModels}
}

func OllamaModelByID(store ModelAliasReader, id string) (OllamaCapabilitiesModelInfo, bool) {
        canonical, ok := ResolveModel(store, id)
        if !ok {
                return OllamaCapabilitiesModelInfo{}, false
        }
        for _, model := range OllamaCapabilitiesModels {
                if model.ID == canonical {
                        return model, true
                }
        }
        return OllamaCapabilitiesModelInfo{}, false
}

func ClaudeModelsResponse() map[string]any {
        resp := map[string]any{"object": "list", "data": ClaudeModels}
        if len(ClaudeModels) > 0 {
                resp["first_id"] = ClaudeModels[0].ID
                resp["last_id"] = ClaudeModels[len(ClaudeModels)-1].ID
        } else {
                resp["first_id"] = nil
                resp["last_id"] = nil
        }
        resp["has_more"] = false
        return resp
}

func appendNoThinkingVariants(models []ModelInfo) []ModelInfo {
        return appendModelVariants(models)
}

// appendModelVariants 为每个 base model 生成 -nothinking / -autodelete / -forcehistory
// 三个后缀变体。
//   - -nothinking：禁用 thinking 字段
//   - -autodelete：响应完成后自动调用 chat_session/delete 删除本次对话
//   - -forcehistory：无视全局 current_input_file.enabled，本次请求强制启用历史拆分
//
// 注意：三个后缀不叠加，避免组合爆炸。
func appendModelVariants(models []ModelInfo) []ModelInfo {
        out := make([]ModelInfo, 0, len(models)*4)
        for _, model := range models {
                out = append(out, model)

                noThinkingVariant := model
                noThinkingVariant.ID = withNoThinkingVariant(model.ID, true)
                out = append(out, noThinkingVariant)

                autoDeleteVariant := model
                autoDeleteVariant.ID = withAutoDeleteVariant(model.ID, true)
                out = append(out, autoDeleteVariant)

                forceHistoryVariant := model
                forceHistoryVariant.ID = withForceHistoryVariant(model.ID, true)
                out = append(out, forceHistoryVariant)
        }
        return out
}
func mapToOllamaModels(models []ModelInfo) []OllamaModelInfo {
        out := make([]OllamaModelInfo, 0, len(models))
        for _, model := range models {
                var modifiedAt string
                if model.Created > 0 {
                        modifiedAt = time.Unix(model.Created, 0).Format(time.RFC3339)
                }
                ollamaModel := OllamaModelInfo{
                        Name:       model.ID,
                        Model:      model.ID,
                        Size:       0,
                        ModifiedAt: modifiedAt,
                }
                out = append(out, ollamaModel)
        }
        return out
}

func splitNoThinkingModel(model string) (string, bool) {
        model = lower(strings.TrimSpace(model))
        if strings.HasSuffix(model, noThinkingModelSuffix) {
                return strings.TrimSuffix(model, noThinkingModelSuffix), true
        }
        return model, false
}

func withNoThinkingVariant(model string, enabled bool) string {
        baseModel, _ := splitNoThinkingModel(model)
        if !enabled {
                return baseModel
        }
        if baseModel == "" {
                return ""
        }
        return baseModel + noThinkingModelSuffix
}

// splitAutoDeleteModel 剥离 -autodelete 后缀。
// 注意：只剥最外层，且不会同时识别 -nothinking-autodelete 组合（设计上不支持叠加）。
func splitAutoDeleteModel(model string) (string, bool) {
        model = lower(strings.TrimSpace(model))
        if strings.HasSuffix(model, autoDeleteModelSuffix) {
                return strings.TrimSuffix(model, autoDeleteModelSuffix), true
        }
        return model, false
}

// withAutoDeleteVariant 在 base model 上追加 -autodelete 后缀（若 enabled=true）。
// 若 base model 已经带 -autodelete 后缀，不会重复追加。
func withAutoDeleteVariant(model string, enabled bool) string {
        baseModel, _ := splitAutoDeleteModel(model)
        if !enabled {
                return baseModel
        }
        if baseModel == "" {
                return ""
        }
        return baseModel + autoDeleteModelSuffix
}

// splitForceHistoryModel 剥离 -forcehistory 后缀。
// 与 -nothinking / -autodelete 一样只剥最外层，不支持叠加。
func splitForceHistoryModel(model string) (string, bool) {
        model = lower(strings.TrimSpace(model))
        if strings.HasSuffix(model, forceHistoryModelSuffix) {
                return strings.TrimSuffix(model, forceHistoryModelSuffix), true
        }
        return model, false
}

// withForceHistoryVariant 在 base model 上追加 -forcehistory 后缀（若 enabled=true）。
// 若 base model 已经带 -forcehistory 后缀，不会重复追加。
func withForceHistoryVariant(model string, enabled bool) string {
        baseModel, _ := splitForceHistoryModel(model)
        if !enabled {
                return baseModel
        }
        if baseModel == "" {
                return ""
        }
        return baseModel + forceHistoryModelSuffix
}

func loadModelAliases(store ModelAliasReader) map[string]string {
        aliases := DefaultModelAliases()
        if store != nil {
                for k, v := range store.ModelAliases() {
                        aliases[lower(strings.TrimSpace(k))] = lower(strings.TrimSpace(v))
                }
        }
        return aliases
}
