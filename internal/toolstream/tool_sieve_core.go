package toolstream

import (
	"ds2api/internal/toolcall"
)

func processChunk(state *State, chunk string, toolNames []string) []Event {
	if state == nil {
		return nil
	}
	if chunk != "" {
		state.pending.WriteString(state.normalizeIncoming(chunk))
	}
	events := make([]Event, 0, 2)
	if len(state.pendingToolCalls) > 0 {
		events = append(events, Event{ToolCalls: state.pendingToolCalls})
		state.pendingToolRaw = ""
		state.pendingToolCalls = nil
	}

	for {
		if state.capturing {
			if state.pending.Len() > 0 {
				state.capture.WriteString(state.pending.String())
				state.pending.Reset()
			}
			prefix, calls, suffix, ready := consumeToolCapture(state, toolNames, false)
			if !ready {
				break
			}
			state.capture.Reset()
			state.capturing = false
			state.resetIncrementalToolState()
			if len(calls) > 0 {
				if prefix != "" {
					state.noteText(prefix)
					events = append(events, Event{Content: prefix, Deferred: shouldDeferToolText(state, prefix)})
				}
				// Emit the completed tool call immediately so the runtime can mark
				// the turn as tool-call emitting before the capture suffix (tail
				// prose after the tool block) is released as content. Deferring
				// to the next ProcessChunk call would let same-chunk tail text
				// slip through as visible content in the same onParsed pass.
				events = append(events, Event{ToolCalls: calls})
				state.pendingToolRaw = ""
				if suffix != "" {
					state.pending.WriteString(suffix)
				}
				continue
			}
			if prefix != "" {
				state.noteText(prefix)
				events = append(events, Event{Content: prefix, Deferred: shouldDeferToolText(state, prefix)})
			}
			if suffix != "" {
				state.pending.WriteString(suffix)
			}
			continue
		}

		pending := state.pending.String()
		if pending == "" {
			break
		}
		start := findToolSegmentStart(state, pending)
		if state.DeferMalformed {
			literal := toolcall.FindUnfinishedToolLiteralStart(pending)
			partial := findPartialXMLToolTagStart(pending)
			if literal >= 0 && !insideCodeFenceWithState(state, pending[:literal]) && (start < 0 || literal < start) && (partial < 0 || literal < partial) {
				prefix := pending[:literal]
				state.pending.Reset()
				state.pending.WriteString(pending[literal:])
				state.noteText(prefix)
				if prefix != "" {
					events = append(events, Event{Content: prefix})
				}
				break
			}
		}
		if start == holdToolSegmentStart {
			break
		}
		if start >= 0 {
			prefix := pending[:start]
			if prefix != "" {
				resetMarkdownSpan := shouldResetUnclosedMarkdownPrefix(state, prefix, pending[start:])
				state.noteText(prefix)
				if resetMarkdownSpan {
					state.markdownCodeSpanTicks = 0
				}
				events = append(events, Event{Content: prefix, Deferred: shouldDeferToolText(state, prefix)})
			}
			state.pending.Reset()
			state.capture.WriteString(pending[start:])
			state.capturing = true
			state.resetIncrementalToolState()
			continue
		}

		safe, hold := splitSafeContentForToolDetection(state, pending)
		if safe == "" {
			break
		}
		state.pending.Reset()
		state.pending.WriteString(hold)
		state.noteText(safe)
		events = append(events, Event{Content: safe})
	}

	return events
}

func flush(state *State, toolNames []string) []Event {
	if state == nil {
		return nil
	}
	if tail := state.flushNormalizer(); tail != "" {
		state.pending.WriteString(tail)
	}
	events := processChunk(state, "", toolNames)
	if state.pending.Len() > 0 && state.markdownCodeSpanTicks > 0 {
		// At end of stream, an unmatched backtick is literal Markdown text.
		// Re-scan pending content so a real tool call after that stray
		// backtick is not permanently hidden by inline-code state.
		state.markdownCodeSpanTicks = 0
		events = append(events, processChunk(state, "", toolNames)...)
	}
	if len(state.pendingToolCalls) > 0 {
		events = append(events, Event{ToolCalls: state.pendingToolCalls})
		state.pendingToolRaw = ""
		state.pendingToolCalls = nil
	}
	if state.capturing {
		consumedPrefix, consumedCalls, consumedSuffix, ready := consumeToolCapture(state, toolNames, true)
		if ready {
			if consumedPrefix != "" {
				state.noteText(consumedPrefix)
				events = append(events, Event{Content: consumedPrefix, Deferred: state.DeferMalformed && len(consumedCalls) == 0})
			}
			if len(consumedCalls) > 0 {
				events = append(events, Event{ToolCalls: consumedCalls})
			}
			if consumedSuffix != "" {
				state.noteText(consumedSuffix)
				events = append(events, Event{Content: consumedSuffix})
			}
		} else {
			content := state.capture.String()
			if content != "" {
				// A never-completed capture is released as text (repair /
				// verbatim-output contract), but its DSML tags are still alien
				// markup that must never reach the client: strip them here so
				// the flush-time release cannot leak a lone `<｜｜DSML｜｜ calls>`
				// as a delta the finalize pass can no longer retract.
				content = toolcall.StripDSMLResidue(content)
				recovered := toolcall.SanitizeLooseCDATA(content)
				if recovered != content {
					if prefix, calls, suffix, recoveredReady := consumeXMLToolCapture(recovered, toolNames); recoveredReady && len(calls) > 0 {
						if prefix != "" {
							state.noteText(prefix)
							events = append(events, Event{Content: prefix, Deferred: shouldDeferToolText(state, prefix)})
						}
						events = append(events, Event{ToolCalls: calls})
						if suffix != "" {
							state.noteText(suffix)
							events = append(events, Event{Content: suffix})
						}
					} else {
						// If capture never resolved into a real tool call, release
						// the buffered text instead of swallowing it.
						state.noteText(content)
						events = append(events, Event{Content: content, Deferred: state.DeferMalformed})
					}
				} else {
					// If capture never resolved into a real tool call, release the
					// buffered text instead of swallowing it.
					state.noteText(content)
					events = append(events, Event{Content: content, Deferred: state.DeferMalformed})
				}
			}
		}
		state.capture.Reset()
		state.capturing = false
		state.resetIncrementalToolState()
	}
	if state.pending.Len() > 0 {
		content := state.pending.String()
		// Held-back partial DSML fragments (a tag whose `>` never arrived) used
		// to be released verbatim here; strip them with the same final net.
		content = toolcall.StripDSMLResidue(content)
		// If pending never resolved into a real tool call, release it as text.
		state.noteText(content)
		events = append(events, Event{Content: content, Deferred: state.DeferMalformed})
		state.pending.Reset()
	}
	return events
}
