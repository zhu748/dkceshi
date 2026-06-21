package sse

import (
        "fmt"
)

// LineResult is the normalized parse result for one DeepSeek SSE line.
type LineResult struct {
        Parsed                     bool
        Stop                       bool
        ContentFilter              bool
        EditLimitHit               bool
        ErrorMessage               string
        Parts                      []ContentPart
        ToolDetectionThinkingParts []ContentPart
        NextType                   string
        ResponseMessageID          int
        RequestMessageID           int
}

// ParseDeepSeekContentLine centralizes one-line DeepSeek SSE parsing for both
// streaming and non-streaming handlers.
func ParseDeepSeekContentLine(raw []byte, thinkingEnabled bool, currentType string) LineResult {
        chunk, done, parsed := ParseDeepSeekSSELine(raw)
        if !parsed {
                return LineResult{NextType: currentType}
        }
        if done {
                return LineResult{Parsed: true, Stop: true, NextType: currentType}
        }
        if errObj, hasErr := chunk["error"]; hasErr {
                return LineResult{
                        Parsed:       true,
                        Stop:         true,
                        ErrorMessage: fmt.Sprintf("%v", errObj),
                        NextType:     currentType,
                }
        }
        if code, _ := chunk["code"].(string); code == "content_filter" {
                return LineResult{
                        Parsed:        true,
                        Stop:          true,
                        ContentFilter: true,
                        NextType:      currentType,
                }
        }
        if hasContentFilterStatus(chunk) {
                return LineResult{
                        Parsed:        true,
                        Stop:          true,
                        ContentFilter: true,
                        NextType:      currentType,
                }
        }
        parts, detectionThinkingParts, finished, nextType := ParseSSEChunkForContentDetailed(chunk, thinkingEnabled, currentType)
        parts = filterLeakedContentFilterParts(parts)
        detectionThinkingParts = filterLeakedContentFilterParts(detectionThinkingParts)
        var respMsgID int
        var reqMsgID int
        observeResponseMessageID(chunk, &respMsgID)
        observeRequestMessageID(chunk, &reqMsgID)

        // 检测 edit_limit hint 事件
        editLimit := false
        if finishReason, ok := chunk["finish_reason"]; ok {
                if fr, ok := finishReason.(string); ok && fr == "edit_limit" {
                        editLimit = true
                }
        }

        return LineResult{
                Parsed:                     true,
                Stop:                       finished || editLimit,
                EditLimitHit:               editLimit,
                Parts:                      parts,
                ToolDetectionThinkingParts: detectionThinkingParts,
                NextType:                   nextType,
                ResponseMessageID:          respMsgID,
                RequestMessageID:           reqMsgID,
        }
}
