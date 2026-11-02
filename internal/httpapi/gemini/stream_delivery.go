package gemini

import (
	"strings"

	"ds2api/internal/completionruntime"
	"ds2api/internal/responsehistory"
	"ds2api/internal/sse"
	streamengine "ds2api/internal/stream"
	"ds2api/internal/toolstream"
)

func (s *geminiStreamRuntime) rejectedOutput() string {
	text, _, _, _, thinking := s.accumulator.Snapshot()
	return completionruntime.ToolRetrySource(text, thinking)
}

func (s *geminiStreamRuntime) sendParts(parts []map[string]any) {
	s.sendChunk(map[string]any{
		"candidates": []map[string]any{{
			"index":   0,
			"content": map[string]any{"role": "model", "parts": parts},
		}},
		"modelVersion": s.model,
	})
}

func (s *geminiStreamRuntime) emitStreamText(text string) {
	text = cleanVisibleOutput(text, s.stripReferenceMarkers)
	if text == "" || s.searchEnabled && sse.IsCitation(text) {
		return
	}
	if strings.TrimSpace(text) != "" {
		s.answerDelivered = true
	}
	s.sendParts([]map[string]any{{"text": text}})
}

//nolint:unused // retained for native Gemini stream handling path.
func (s *geminiStreamRuntime) onParsed(parsed sse.LineResult) streamengine.ParsedDecision {
	if !parsed.Parsed {
		return streamengine.ParsedDecision{}
	}
	if parsed.ResponseMessageID > 0 {
		s.responseMessageID = parsed.ResponseMessageID
	}
	if parsed.ContentFilter || parsed.ErrorMessage != "" || parsed.Stop {
		if parsed.ContentFilter {
			s.contentFilter = true
		}
		if parsed.ErrorMessage != "" {
			s.upstreamErr = parsed.ErrorMessage
		}
		return streamengine.ParsedDecision{Stop: true}
	}

	s.accumulator.SetToolMarker(s.toolSieve.Marker)
	accumulated := s.accumulator.Apply(parsed)
	for _, p := range accumulated.Parts {
		if s.bufferContent {
			continue
		}
		if p.Type == "thinking" {
			if p.VisibleText != "" {
				s.sendParts([]map[string]any{{"text": p.VisibleText, "thought": true}})
			}
			continue
		}
		if p.RawText == "" || p.CitationOnly {
			continue
		}
		for _, event := range toolstream.ProcessChunk(&s.toolSieve, p.RawText, s.toolNames) {
			if event.Deferred {
				s.deferredText.WriteString(event.Content)
				continue
			}
			if len(event.ToolCalls) > 0 || len(event.ToolCallDeltas) > 0 {
				s.toolCallsPending = true
				continue
			}
			if !s.toolCallsPending {
				s.emitStreamText(event.Content)
			}
		}
	}
	if s.history != nil {
		rawText, text, rawThinking, thinking, detectionThinking := s.accumulator.Snapshot()
		s.history.Progress(
			responsehistory.ThinkingForArchive(rawThinking, detectionThinking, thinking),
			responsehistory.TextForArchive(rawText, text),
		)
	}
	// 上游内容规则（复读死循环 / 编造的 `[Tool]:` 角色标记）在此统一收口：
	// 结束读取，保留已生成内容走正常收尾。
	return streamengine.StopForCut(accumulated.Cut, "gemini_stream", "gemini", accumulated.ContentSeen)
}
