package promptcompat

import (
        "fmt"
        "time"

        "ds2api/internal/config"
)

type StandardRequest struct {
        Surface                 string
        RequestedModel          string
        ResolvedModel           string
        ResponseModel           string
        Messages                []any
        HistoryText             string
        PromptTokenText         string
        CurrentInputFileApplied bool
        CurrentInputFileID      string
        CurrentToolsFileID      string
        ToolsRaw                any
        FinalPrompt             string
        ToolNames               []string
        ToolChoice              ToolChoicePolicy
        Stream                  bool
        Thinking                bool
        Search                  bool
        RefFileIDs              []string
        RefFileTokens           int
        PassThrough             map[string]any

        // EditReuse 以下字段用于 edit_message 会话复用

        // EditReuseEnabled 标识是否启用了 edit_message 复用（模型带 -v4f 后缀）。
        EditReuseEnabled bool
        // MessagesFingerprint 是 messages 前 N-1 条的指纹，用于匹配已有会话。
        MessagesFingerprint string
        // LastUserMessage 是 messages 中最后一条用户消息的文本内容，
        // edit_message 时只发这段文字而非完整对话。
        LastUserMessage string
}

type ToolChoiceMode string

const (
        ToolChoiceAuto     ToolChoiceMode = "auto"
        ToolChoiceNone     ToolChoiceMode = "none"
        ToolChoiceRequired ToolChoiceMode = "required"
        ToolChoiceForced   ToolChoiceMode = "forced"
)

type ToolChoicePolicy struct {
        Mode       ToolChoiceMode
        ForcedName string
        Allowed    map[string]struct{}
}

func DefaultToolChoicePolicy() ToolChoicePolicy {
        return ToolChoicePolicy{Mode: ToolChoiceAuto}
}

func (p ToolChoicePolicy) IsNone() bool {
        return p.Mode == ToolChoiceNone
}

func (p ToolChoicePolicy) IsRequired() bool {
        return p.Mode == ToolChoiceRequired || p.Mode == ToolChoiceForced
}

func (p ToolChoicePolicy) Allows(name string) bool {
        if len(p.Allowed) == 0 {
                return true
        }
        _, ok := p.Allowed[name]
        return ok
}

// CompletionPayload 构建发往 DeepSeek /api/v0/chat/completion 的请求体。
//
// 字段顺序严格对齐真实 Android App 抓包结果：
//
//      {"chat_session_id":...,"parent_message_id":null,"prompt":...,"ref_file_ids":[],
//       "thinking_enabled":true,"search_enabled":true,"audio_id":null,"preempt":false,
//       "model_type":"default","action":null}
//
// 返回 *OrderedJSONMap 而非 map[string]any，以保留字段顺序；同时通过
// json.Marshal 时按插入顺序输出，对齐 App 行为。
//
// 兼容说明：调用方若需按 key 读取，可使用 .Get(key) 或 .AsMap()[key]。
func (r StandardRequest) CompletionPayload(sessionID string) any {
        modelID := r.ResolvedModel
        if modelID == "" {
                modelID = r.RequestedModel
        }
        modelType := "default"
        if resolvedType, ok := config.GetModelType(modelID); ok {
                modelType = resolvedType
        }
        refFileIDs := make([]any, 0, len(r.RefFileIDs))
        for _, fileID := range r.RefFileIDs {
                if fileID == "" {
                        continue
                }
                refFileIDs = append(refFileIDs, fileID)
        }
        m := NewOrderedJSONMap()
        // 严格按 App 抓包顺序写入
        m.Set("chat_session_id", sessionID)
        m.Set("parent_message_id", nil)
        m.Set("prompt", r.FinalPrompt)
        m.Set("ref_file_ids", refFileIDs)
        m.Set("thinking_enabled", r.Thinking)
        m.Set("search_enabled", r.Search)
        m.Set("audio_id", nil)
        m.Set("preempt", false)
        m.Set("model_type", modelType)
        m.Set("action", nil)
        // passthrough 字段（如 temperature）追加在末尾
        for k, v := range r.PassThrough {
                m.Set(k, v)
        }
        return m
}

// EditMessagePayload 构建 edit_message 请求体。
// 与 CompletionPayload 的关键区别：
//   - 使用 message_id 替代 parent_message_id
//   - prompt 仅含最新用户消息文本（LastUserMessage），而非完整对话
//   - 新增 client_stream_id 字段
func (r StandardRequest) EditMessagePayload(sessionID string, messageID int) any {
        modelID := r.ResolvedModel
        if modelID == "" {
                modelID = r.RequestedModel
        }
        modelType := "default"
        if resolvedType, ok := config.GetModelType(modelID); ok {
                modelType = resolvedType
        }
        m := NewOrderedJSONMap()
        // 严格按 App 抓包顺序
        m.Set("chat_session_id", sessionID)
        m.Set("message_id", messageID)
        m.Set("prompt", r.LastUserMessage)
        m.Set("thinking_enabled", r.Thinking)
        m.Set("search_enabled", r.Search)
        m.Set("client_stream_id", generateClientStreamID())
        m.Set("action", nil)
        // passthrough 字段追加在末尾
        for k, v := range r.PassThrough {
                m.Set(k, v)
        }
        return m
}

// generateClientStreamID 生成格式为 YYYYMMDD-<hex> 的客户端流 ID。
func generateClientStreamID() string {
        now := time.Now()
        datePart := now.Format("20060102")
        nano := now.UnixNano()
        hexPart := fmt.Sprintf("%012x", nano&0xFFFFFFFFFFFF)
        return datePart + "-" + hexPart
}
