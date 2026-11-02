package responses

import (
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/sse"
	"ds2api/internal/toolstream"
)

func (s *responsesStreamRuntime) nextSequence() int {
	s.sequence++
	return s.sequence
}

func (s *responsesStreamRuntime) sendEvent(event string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	if _, ok := payload["sequence_number"]; !ok {
		payload["sequence_number"] = s.nextSequence()
	}
	s.output.JSON(s.w, s.rc, s.canFlush, event, payload)
}

func (s *responsesStreamRuntime) sendCreated() {
	s.sendEvent("response.created", openaifmt.BuildResponsesCreatedPayload(s.responseID, s.model))
}

func (s *responsesStreamRuntime) sendDone() {
	s.output.Write(s.w, s.rc, s.canFlush, []byte("data: [DONE]\n\n"))
}

func (s *responsesStreamRuntime) processToolStreamEvents(events []toolstream.Event, emitContent bool, _ bool) {
	for _, evt := range events {
		if evt.Deferred {
			s.deferredText.WriteString(evt.Content)
			continue
		}
		if emitContent && evt.Content != "" && !s.toolCallsEmitted && !s.toolCallsPending {
			// 已发出 tool_calls 后，工具调用块之后的尾巴正文不再透传，
			// 避免工具调用最后几行结果泄漏到正文。
			cleaned := cleanVisibleDelta(evt.Content, s.stripReferenceMarkers)
			if cleaned != "" && (!s.searchEnabled || !sse.IsCitation(cleaned)) {
				s.emitTextDelta(cleaned)
			}
		}
		if len(evt.ToolCallDeltas) > 0 || evt.ToolBoundary || len(evt.ToolCalls) > 0 {
			s.toolCallsPending = true
		}
	}
}
