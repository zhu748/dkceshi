package audio

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/shared"
)

func (h *Handler) writeSpeechResponse(w http.ResponseWriter, plan speechPlan, outcomes []segmentOutcome) {
	segments, format := plan.segments, plan.format
	var err error
	var frames [][]byte
	var recitedParts []string
	lastVoiceID := ""
	lastAudioID := ""
	for i, outcome := range outcomes {
		config.Logger.Debug("[tts] segment frames",
			"segment", i+1, "segments", len(outcomes), "frames", len(outcome.frames))
		frames = append(frames, outcome.frames...)
		if outcome.text != "" {
			recitedParts = append(recitedParts, outcome.text)
		}
		if outcome.voiceID != "" {
			lastVoiceID = outcome.voiceID
		}
		if outcome.audioID != "" {
			lastAudioID = outcome.audioID
		}
	}
	config.Logger.Info("[tts] synthesized",
		"segments", len(segments), "frames", len(frames), "voice", lastVoiceID)
	var audioBytes []byte
	contentType := "audio/ogg"
	if format == "wav" {
		audioBytes, err = dsclient.EncodeWAVFromOpus(frames)
		contentType = "audio/wav"
	} else {
		audioBytes, err = dsclient.EncodeOggOpus(frames)
	}
	if err != nil {
		config.Logger.Warn("[tts] encode audio failed", "format", format, "error", err)
		shared.WriteOpenAIErrorWithCode(w, http.StatusBadGateway, "failed to encode audio", "tts_encode_error")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(audioBytes)))
	w.Header().Set("Cache-Control", "no-store")
	// Expose the TTS metadata to cross-origin callers as well; the admin page
	// is same-origin, but the endpoint is usable from any OpenAI-compatible
	// client that happens to run in a browser.
	w.Header().Set("Access-Control-Expose-Headers", "X-Ds2-Tts-Voice, X-Ds2-Tts-Audio-Id, X-Ds2-Tts-Format, X-Ds2-Tts-Text-Base64, X-Ds2-Tts-Segments, X-Ds2-Tts-Frames")
	if lastVoiceID != "" {
		w.Header().Set("X-Ds2-Tts-Voice", lastVoiceID)
	}
	if lastAudioID != "" {
		w.Header().Set("X-Ds2-Tts-Audio-Id", lastAudioID)
	}
	w.Header().Set("X-Ds2-Tts-Format", format)
	w.Header().Set("X-Ds2-Tts-Segments", strconv.Itoa(len(segments)))
	w.Header().Set("X-Ds2-Tts-Frames", strconv.Itoa(len(frames)))
	// The model may rewrite the recitation; expose what was actually spoken
	// so callers can verify it. Base64 keeps arbitrary text out of the header
	// grammar. Headers are capped at a few KB by most proxies, so skip the
	// field when it would be oversized.
	recitedText := strings.Join(recitedParts, "\n")
	if encoded := base64.RawURLEncoding.EncodeToString([]byte(recitedText)); encoded != "" && len(encoded) <= 4096 {
		w.Header().Set("X-Ds2-Tts-Text-Base64", encoded)
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(audioBytes); err != nil {
		config.Logger.Warn("[tts] write response failed", "error", err)
	}
}
