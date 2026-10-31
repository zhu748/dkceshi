package audio

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"ds2api/internal/assistantturn"
)

// speechPlan is validated without acquiring an account or contacting upstream.
type speechPlan struct {
	voice, format, model string
	segments             []string
	multiAccount         bool
	perAccount           int
}

func (h *Handler) prepareSpeech(req speechRequest) (speechPlan, *assistantturn.OutputError) {
	invalid := func(message string) (speechPlan, *assistantturn.OutputError) {
		return speechPlan{}, &assistantturn.OutputError{Status: http.StatusBadRequest, Message: message}
	}
	if strings.TrimSpace(req.Input) == "" {
		return invalid("input is required")
	}
	if utf8.RuneCountInString(req.Input) > maxTTSInputRunes {
		return invalid(fmt.Sprintf("input must be at most %d characters", maxTTSInputRunes))
	}
	if h.Store != nil && !h.Store.TTSEnabled() {
		return speechPlan{}, &assistantturn.OutputError{
			Status: http.StatusServiceUnavailable, Message: "speech synthesis is disabled", Code: "tts_disabled",
		}
	}
	voice, err := normalizeVoice(req.Voice)
	if err != nil {
		return invalid(err.Error())
	}
	format, err := normalizeFormat(req.ResponseFormat)
	if err != nil {
		return invalid(err.Error())
	}
	if req.Speed != nil && *req.Speed != 1 {
		return invalid("speed is not supported by the upstream voice")
	}
	model, err := h.resolveModel(req.Model)
	if err != nil {
		return invalid(err.Error())
	}

	segmentChars := maxTTSSegmentRunes
	multiAccount, accountConcurrency := true, true
	if h.Store != nil {
		if n := h.Store.TTSSegmentChars(); n > 0 {
			segmentChars = n
		}
		multiAccount = h.Store.TTSMultiAccountEnabled()
		accountConcurrency = h.Store.TTSAccountConcurrencyEnabled()
	}
	if segmentChars < minTTSSegmentRunes {
		segmentChars = minTTSSegmentRunes
	} else if segmentChars > maxTTSSegmentRunesConfig {
		segmentChars = maxTTSSegmentRunesConfig
	}
	segments := splitTTSText(req.Input, segmentChars)
	if len(segments) == 0 {
		return invalid("input is required")
	}
	if len(segments) > maxTTSSegments {
		return invalid(fmt.Sprintf("input produces too many segments (%d > %d), please reduce input length or increase segment_chars in settings", len(segments), maxTTSSegments))
	}
	return speechPlan{
		voice: voice, format: format, model: model, segments: segments,
		multiAccount: multiAccount, perAccount: resolvePerAccountLimit(h.Store, accountConcurrency),
	}, nil
}
