package sse

import (
	"log/slog"
	"net/http"
	"strings"

	dsprotocol "ds2api/internal/deepseek/protocol"
	"ds2api/internal/util"
)

// CollectResult holds the aggregated text and thinking content from a
// DeepSeek SSE stream, consumed to completion (non-streaming use case).
type CollectResult struct {
	Text                  string
	Thinking              string
	ToolDetectionThinking string
	ContentFilter         bool
	UpstreamError         string
	ReadError             error
	CitationLinks         map[int]string
	ResponseMessageID     int
	// FirstResponseMessageID is the id the completion started with. When
	// auto-continue appends segments, each continuation reports its own
	// response_message_id and ResponseMessageID ends up naming the last
	// segment. The TTS endpoint reads the original assistant message, so it
	// needs this value instead.
	FirstResponseMessageID int
	// Cut reports that collection was cut short by a content rule (see
	// ContentPolicy). The content collected before the cut is returned as-is;
	// the cut fragment and everything after it was dropped.
	Cut CutReason
}

// CollectStream fully consumes a DeepSeek SSE response and separates
// thinking content from text content. This replaces the duplicated
// stream-collection logic in openai.handleNonStream, claude.collectDeepSeek,
// and admin.testAccount.
//
// The caller is responsible for closing resp.Body unless closeBody is true.
func CollectStream(resp *http.Response, thinkingEnabled bool, closeBody bool, prompts ...string) CollectResult {
	if closeBody {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				slog.Warn("[sse] response body close failed", "error", err)
			}
		}()
	}
	text := strings.Builder{}
	thinking := strings.Builder{}
	toolDetectionThinking := strings.Builder{}
	contentFilter := false
	upstreamError := ""
	stopped := false
	policy := ContentPolicy{}
	if len(prompts) > 0 {
		policy.Prompt = prompts[0]
	}
	collector := newCitationLinkCollector()
	responseMessageID := 0
	firstResponseMessageID := 0
	currentType := "text"
	if thinkingEnabled {
		currentType = "thinking"
	}
	readErr := dsprotocol.ScanSSELines(resp, func(line []byte) bool {
		chunk, done, parsed := ParseDeepSeekSSELine(line)
		if parsed && !done {
			collector.ingestChunk(chunk)
			observeResponseMessageIDWithFirst(chunk, &responseMessageID, &firstResponseMessageID)
		}
		if done {
			return false
		}
		if stopped {
			return true
		}
		if !parsed {
			// Not a data:/JSON line at all; skip the second parse entirely.
			return true
		}
		// The line was already decoded above; reuse the chunk instead of
		// unmarshalling the same payload a second time per line.
		result := ParseDeepSeekContentChunk(chunk, thinkingEnabled, currentType)
		currentType = result.NextType
		if !result.Parsed {
			return true
		}
		if result.Stop {
			if result.ContentFilter {
				contentFilter = true
			}
			if result.ErrorMessage != "" && upstreamError == "" {
				upstreamError = result.ErrorMessage
			}
			// Keep scanning to collect late-arriving citation metadata lines
			// that can appear after response/status=FINISHED, but stop as soon
			// as [DONE] arrives.
			stopped = true
			return true
		}
		for _, p := range result.Parts {
			if p.Type == "thinking" {
				trimmed := TrimContinuationOverlap(thinking.String(), p.Text)
				thinking.WriteString(trimmed)
			} else {
				trimmed := TrimContinuationOverlap(text.String(), p.Text)
				// 内容规则可能在此截断正文通道：编造的 `[Tool]:` 角色标记及其后
				// 全是虚构对话，复读死循环之后的输出也已退化。两种情况都只保留
				// 截断点之前的正文。
				trimmed = policy.Feed(trimmed)
				if trimmed == "" {
					continue
				}
				text.WriteString(trimmed)
			}
		}
		for _, p := range result.ToolDetectionThinkingParts {
			trimmed := TrimContinuationOverlap(toolDetectionThinking.String(), p.Text)
			toolDetectionThinking.WriteString(trimmed)
		}
		if policy.Cut() != CutNone {
			// A content rule ended the text channel: stop reading and keep what
			// was collected so the caller's normal finish path (tool sieve /
			// repair / usage) still runs.
			return false
		}
		return true
	})
	// FINISHED freezes the answer while we continue reading late citations.
	// A failure before that point means the answer is incomplete; a failure
	// after a terminal status must not turn a completed answer into an error.
	if stopped {
		readErr = nil
	}
	text.WriteString(policy.Flush())
	return CollectResult{
		Text:                   text.String(),
		Thinking:               thinking.String(),
		ToolDetectionThinking:  toolDetectionThinking.String(),
		ContentFilter:          contentFilter,
		UpstreamError:          upstreamError,
		ReadError:              readErr,
		CitationLinks:          collector.build(),
		ResponseMessageID:      responseMessageID,
		FirstResponseMessageID: firstResponseMessageID,
		Cut:                    policy.Cut(),
	}
}

// observeResponseMessageID extracts the response_message_id from a parsed SSE
// chunk. It mirrors the extraction logic in client_continue.go's observe
// method, checking top-level response_message_id, v.response.message_id, and
// message.response.message_id.
func observeResponseMessageID(chunk map[string]any, out *int) {
	observeResponseMessageIDWithFirst(chunk, out, nil)
}

// observeResponseMessageIDWithFirst is observeResponseMessageID plus a record
// of the first id seen. Continuation streams report a fresh id per segment, so
// callers that need the original message (TTS) must keep the first one.
func observeResponseMessageIDWithFirst(chunk map[string]any, out *int, first *int) {
	if chunk == nil || out == nil {
		return
	}
	set := func(id int) {
		if id <= 0 {
			return
		}
		*out = id
		if first != nil && *first == 0 {
			*first = id
		}
	}
	set(util.IntFrom(chunk["response_message_id"]))
	v, _ := chunk["v"].(map[string]any)
	if response, _ := v["response"].(map[string]any); response != nil {
		set(util.IntFrom(response["message_id"]))
	}
	if message, _ := chunk["message"].(map[string]any); message != nil {
		if response, _ := message["response"].(map[string]any); response != nil {
			set(util.IntFrom(response["message_id"]))
		}
	}
}
