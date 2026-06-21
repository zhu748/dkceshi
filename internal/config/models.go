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

const noThinkingModelSuffix = "-nothinking"
const autoDeleteModelSuffix = "-autodelete"
const forceHistoryModelSuffix = "-forcehistory"
const thinkingInjectModelSuffix = "-thinkinginject"
const editReuseModelSuffix = "-v4f"

var deepSeekBaseModels = []ModelInfo{
        {ID: "deepseek-v4-flash", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-pro", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-flash-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-pro-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
        {ID: "deepseek-v4-vision", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
}

// OllamaCapabilitiesModels 由 buildOllamaCapabilitiesModels() 在 init 阶段生成。
// 5 个基础模型 × 12 个后缀组合（-nothinking / -forcehistory / -autodelete / -thinkinginject
// 的合法子集；nothinking 与 thinkinginject 互斥）共 60 条记录。
// capabilities 仅由 base 模型类型和是否带 -nothinking 决定，其他后缀不影响能力位。
var OllamaCapabilitiesModels = buildOllamaCapabilitiesModels()

func buildOllamaCapabilitiesModels() []OllamaCapabilitiesModelInfo {
        out := make([]OllamaCapabilitiesModelInfo, 0, len(deepSeekBaseModels)*12)
        for _, base := range deepSeekBaseModels {
                for _, combo := range suffixCombinations {
                        id := withSuffixes(base.ID, combo.noThinking, combo.forceHistory, combo.autoDelete, combo.thinkingInject, false)
                        out = append(out, OllamaCapabilitiesModelInfo{
                                ID:           id,
                                Capabilities: capabilitiesFor(base.ID, combo.noThinking),
                        })
                }
        }
        return out
}

// capabilitiesFor 返回 Ollama 风格的能力数组。
//   - 所有 base 模型默认带 "tools"
//   - 非 -nothinking 变体带 "thinking"
//   - vision base 额外带 "vision"
func capabilitiesFor(baseID string, noThinking bool) []string {
        caps := []string{"tools"}
        if !noThinking {
                caps = append(caps, "thinking")
        }
        if baseID == "deepseek-v4-vision" {
                caps = append(caps, "vision")
        }
        return caps
}

// suffixCombinations 列举 (-nothinking, -forcehistory, -autodelete, -thinkinginject) 的合法组合。
// 共 12 种（不是 16 种）：-nothinking 与 -thinkinginject 互斥，因为 nothinking 让 stdReq.Thinking=false，
// 此时思考注入不会触发，-thinkinginject 后缀失去意义。
// 规范拼接顺序：base[-nothinking][-forcehistory][-autodelete][-thinkinginject]
var suffixCombinations = []struct {
        noThinking     bool
        forceHistory   bool
        autoDelete     bool
        thinkingInject bool
}{
        // 无 nothinking 的 8 种组合（thinkingInject 任意）
        {false, false, false, false},
        {false, false, false, true},
        {false, true, false, false},
        {false, true, false, true},
        {false, false, true, false},
        {false, false, true, true},
        {false, true, true, false},
        {false, true, true, true},
        // 带 nothinking 的 4 种组合（不带 thinkinginject）
        {true, false, false, false},
        {true, true, false, false},
        {true, false, true, false},
        {true, true, true, false},
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
        baseModel, noThinking, _, _, _, _ := parseModelSuffixes(model)
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
        baseModel, _, _, _, _, _ := parseModelSuffixes(model)
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

// IsNoThinkingModel 判断模型 ID 是否带 -nothinking 后缀（可与其他后缀叠加）。
func IsNoThinkingModel(model string) bool {
        _, noThinking, _, _, _, _ := parseModelSuffixes(model)
        return noThinking
}

// IsAutoDeleteModel 判断模型 ID 是否带 -autodelete 后缀（可与其他后缀叠加）。
// 调用方在响应完成后据此触发 chat_session/delete 删除本次对话，
// 与全局 auto_delete.mode 联动：suffix 模型仅当全局 mode=none 时生效，
// 全局 mode=single/all 时保持全局策略不变。
func IsAutoDeleteModel(model string) bool {
        _, _, _, autoDelete, _, _ := parseModelSuffixes(model)
        return autoDelete
}

// IsForceHistoryModel 判断模型 ID 是否带 -forcehistory 后缀（可与其他后缀叠加）。
// 命中时，调用方应无视全局 current_input_file.enabled 开关，
// 在本次请求中强制启用历史拆分（上传历史为文件）。
func IsForceHistoryModel(model string) bool {
        _, _, forceHistory, _, _, _ := parseModelSuffixes(model)
        return forceHistory
}

// IsThinkingInjectModel 判断模型 ID 是否带 -thinkinginject 后缀（可与其他后缀叠加）。
// 命中时，调用方应无视全局 thinking_injection.enabled=false 开关，
// 在本次请求中强制注入思考格式提示词（只要 stdReq.Thinking=true）。
// 全局 enabled=true 时跟随全局（后缀无额外效果）。
// 注意：-nothinking 与 -thinkinginject 互斥。当模型 ID 同时带两个后缀时（理论上不会从 /v1/models
// 列表生成，但可能由用户直接传入或 alias OR 产生），nothinking 让 stdReq.Thinking=false，
// 思考注入仍不触发——该谓词仍返回 true，但 ApplyThinkingInjection 的 stdReq.Thinking 检查会短路。
func IsThinkingInjectModel(model string) bool {
        _, _, _, _, thinkingInject, _ := parseModelSuffixes(model)
        return thinkingInject
}

// IsEditReuseModel 判断模型 ID 是否带 -v4f 后缀（可与其他后缀叠加）。
// 命中时，若同一对话上下文（messages 前 N-1 条一致）在远端仍有活跃会话，
// 则通过 DeepSeek 的 edit_message API 复用该会话，仅发送最新用户消息，
// 而非每次都新建会话并重发完整对话历史。
// 限制：不兼容历史拆分（current_input_file），因为 edit_message 无法修改文件引用。
func IsEditReuseModel(model string) bool {
        _, _, _, _, _, editReuse := parseModelSuffixes(model)
        return editReuse
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
        baseModel, noThinking, forceHistory, autoDelete, thinkingInject, editReuse := parseModelSuffixes(model)
        if baseModel != model {
                // 用户传入了带后缀的形式（如 gpt-4.1-nothinking / gpt-4.1-autodelete /
                // gpt-4.1-forcehistory / gpt-4.1-v4f 等任意组合）。
                if mapped, ok := aliases[baseModel]; ok && IsSupportedDeepSeekModel(mapped) {
                        // alias 目标可能本身就带后缀（例如自定义 alias 指向 deepseek-v4-flash-autodelete），
                        // 把请求的后缀与 alias 目标的后缀做 OR，再用规范顺序重新拼接。
                        mappedBase, mappedNoThinking, mappedForceHistory, mappedAutoDelete, mappedThinkingInject, mappedEditReuse := parseModelSuffixes(mapped)
                        return withSuffixes(
                                mappedBase,
                                noThinking || mappedNoThinking,
                                forceHistory || mappedForceHistory,
                                autoDelete || mappedAutoDelete,
                                thinkingInject || mappedThinkingInject,
                                editReuse || mappedEditReuse,
                        ), true
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

// appendModelVariants 为每个 base model 生成 (-nothinking, -forcehistory, -autodelete, -thinkinginject)
// 四个后缀的全部 12 种合法组合变体（nothinking 与 thinkinginject 互斥，不生成 16 种）。
//   - -nothinking：禁用 thinking 字段（与 thinking 模型概念上同级，作为模型家族标识）
//   - -forcehistory：无视全局 current_input_file.enabled，本次请求强制启用历史拆分
//   - -autodelete：响应完成后自动调用 chat_session/delete 删除本次对话
//   - -thinkinginject：无视全局 thinking_injection.enabled=false，本次请求强制注入思考格式提示词
//
// 规范拼接顺序：base[-nothinking][-forcehistory][-autodelete][-thinkinginject]
// 后缀可任意组合（除 nothinking+thinkinginject 外），例如 deepseek-v4-flash-forcehistory-autodelete-thinkinginject。
func appendModelVariants(models []ModelInfo) []ModelInfo {
        out := make([]ModelInfo, 0, len(models)*12)
        for _, model := range models {
                base, _, _, _, _, _ := parseModelSuffixes(model.ID)
                for _, combo := range suffixCombinations {
                        variant := model
                        variant.ID = withSuffixes(base, combo.noThinking, combo.forceHistory, combo.autoDelete, combo.thinkingInject, false)
                        out = append(out, variant)
                }
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

// parseModelSuffixes 把模型 ID 拆成 base + 四个后缀布尔位。
// 四个后缀可任意叠加，识别顺序无关：循环从末尾剥离已知后缀直到无匹配。
// 例如：
//
//      "deepseek-v4-flash-nothinking-forcehistory-autodelete-thinkinginject"
//        -> ("deepseek-v4-flash", true, true, true, true)
//      "deepseek-v4-flash-thinkinginject-autodelete-forcehistory"  // 非规范顺序也能解析
//        -> ("deepseek-v4-flash", false, true, true, true)
//      "gpt-4.1-nothinking"
//        -> ("gpt-4.1", true, false, false, false)
func parseModelSuffixes(model string) (base string, noThinking, forceHistory, autoDelete, thinkingInject, editReuse bool) {
        model = lower(strings.TrimSpace(model))
        for {
                if strings.HasSuffix(model, editReuseModelSuffix) {
                        editReuse = true
                        model = strings.TrimSuffix(model, editReuseModelSuffix)
                        continue
                }
                if strings.HasSuffix(model, thinkingInjectModelSuffix) {
                        thinkingInject = true
                        model = strings.TrimSuffix(model, thinkingInjectModelSuffix)
                        continue
                }
                if strings.HasSuffix(model, autoDeleteModelSuffix) {
                        autoDelete = true
                        model = strings.TrimSuffix(model, autoDeleteModelSuffix)
                        continue
                }
                if strings.HasSuffix(model, forceHistoryModelSuffix) {
                        forceHistory = true
                        model = strings.TrimSuffix(model, forceHistoryModelSuffix)
                        continue
                }
                if strings.HasSuffix(model, noThinkingModelSuffix) {
                        noThinking = true
                        model = strings.TrimSuffix(model, noThinkingModelSuffix)
                        continue
                }
                break
        }
        return model, noThinking, forceHistory, autoDelete, thinkingInject, editReuse
}

// withSuffixes 按规范顺序拼接 base 与四个后缀。
// 规范顺序：base[-nothinking][-forcehistory][-autodelete][-thinkinginject]
// 用于：生成 /v1/models 列表项、把 alias 解析结果重新规范拼接。
//
// 互斥约束：nothinking 与 thinkingInject 不能共存。nothinking 让 stdReq.Thinking=false，
// 此时思考注入不会触发，-thinkinginject 后缀失去意义。当两者同时为 true 时，保留 nothinking
// 丢弃 thinkingInject（nothinking 优先），用于自动降级用户传入的“矛盾组合”输入。
func withSuffixes(base string, noThinking, forceHistory, autoDelete, thinkingInject, editReuse bool) string {
        if base == "" {
                return ""
        }
        if noThinking {
                // 互斥：nothinking 优先，丢弃 thinkingInject
                thinkingInject = false
        }
        out := base
        if noThinking {
                out += noThinkingModelSuffix
        }
        if forceHistory {
                out += forceHistoryModelSuffix
        }
        if autoDelete {
                out += autoDeleteModelSuffix
        }
        if thinkingInject {
                out += thinkingInjectModelSuffix
        }
        if editReuse {
                out += editReuseModelSuffix
        }
        return out
}

// splitNoThinkingModel 剥离 -nothinking 后缀（兼容单后缀调用方）。
// 注意：在叠加后缀场景下，仅返回 base 与 noThinking 位，其余后缀会被一同剥离丢弃。
// 需要保留所有后缀信息时请直接使用 parseModelSuffixes。
func splitNoThinkingModel(model string) (string, bool) {
        base, noThinking, _, _, _, _ := parseModelSuffixes(model)
        return base, noThinking
}

func withNoThinkingVariant(model string, enabled bool) string {
        base, _, forceHistory, autoDelete, thinkingInject, editReuse := parseModelSuffixes(model)
        return withSuffixes(base, enabled, forceHistory, autoDelete, thinkingInject, editReuse)
}

// splitAutoDeleteModel 剥离 -autodelete 后缀（兼容单后缀调用方）。
// 注意：在叠加后缀场景下，仅返回 base 与 autoDelete 位，其余后缀会被一同剥离丢弃。
func splitAutoDeleteModel(model string) (string, bool) {
        base, _, _, autoDelete, _, _ := parseModelSuffixes(model)
        return base, autoDelete
}

func withAutoDeleteVariant(model string, enabled bool) string {
        base, noThinking, forceHistory, _, thinkingInject, editReuse := parseModelSuffixes(model)
        return withSuffixes(base, noThinking, forceHistory, enabled, thinkingInject, editReuse)
}

// splitForceHistoryModel 剥离 -forcehistory 后缀（兼容单后缀调用方）。
// 注意：在叠加后缀场景下，仅返回 base 与 forceHistory 位，其余后缀会被一同剥离丢弃。
func splitForceHistoryModel(model string) (string, bool) {
        base, _, forceHistory, _, _, _ := parseModelSuffixes(model)
        return base, forceHistory
}

func withForceHistoryVariant(model string, enabled bool) string {
        base, noThinking, _, autoDelete, thinkingInject, editReuse := parseModelSuffixes(model)
        return withSuffixes(base, noThinking, enabled, autoDelete, thinkingInject, editReuse)
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
