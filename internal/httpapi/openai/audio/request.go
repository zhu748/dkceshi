package audio

import (
	"fmt"
	"strings"

	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

const (
	// OpenAI's /v1/audio/speech caps input at 4096 characters. Keep the same
	// bound: every recitation is a full completion, so oversized input also
	// means a large token and audio bill.
	maxTTSInputRunes  = 4096
	maxTTSRequestBody = 1 << 20
	// maxTTSSegmentRunes keeps each upstream message below the TTS service's
	// roughly one-minute audio limit. 160 Chinese characters are about 40 s of
	// speech, leaving headroom for a slower voice or a longer recitation.
	maxTTSSegmentRunes       = 160
	minTTSSegmentRunes       = 30
	maxTTSSegmentRunesConfig = 500
	maxTTSSegments           = 32
	// defaultTTSModel is the cheapest web model with thinking disabled. The
	// recitation only needs verbatim copying, not reasoning.
	defaultTTSModel  = "deepseek-v4.1-flash-nothinking"
	ttsReciteSurface = "tts"
)

type speechRequest struct {
	Model          string   `json:"model"`
	Input          string   `json:"input"`
	Voice          string   `json:"voice"`
	ResponseFormat string   `json:"response_format"`
	Speed          *float64 `json:"speed"`
}

var voiceAliases = map[string]string{
	"贝壳": dsclient.TTSVoiceMira,
	"白浪": dsclient.TTSVoiceEcho,
	"海星": dsclient.TTSVoiceStella,
	"暗潮": dsclient.TTSVoiceTide,

	"alloy":   dsclient.TTSVoiceMira,
	"echo":    dsclient.TTSVoiceEcho,
	"fable":   dsclient.TTSVoiceStella,
	"onyx":    dsclient.TTSVoiceTide,
	"nova":    dsclient.TTSVoiceStella,
	"shimmer": dsclient.TTSVoiceMira,
}

func (h *Handler) resolveModel(raw string) (string, error) {
	model := strings.TrimSpace(raw)
	if model == "" || strings.HasPrefix(strings.ToLower(model), "tts-") {
		model = defaultTTSModel
	}
	resolved, ok := config.ResolveModel(h.Store, model)
	if !ok {
		return "", fmt.Errorf("model %q is not available", raw)
	}
	return resolved, nil
}

func normalizeVoice(raw string) (string, error) {
	voice := strings.ToLower(strings.TrimSpace(raw))
	if voice == "" {
		return dsclient.TTSVoiceMira, nil
	}
	if dsclient.IsValidTTSVoice(voice) {
		return voice, nil
	}
	if mapped, ok := voiceAliases[voice]; ok {
		return mapped, nil
	}
	return "", fmt.Errorf("unsupported voice %q (supported: %s; Chinese: 贝壳/白浪/海星/暗潮)", raw, strings.Join(dsclient.TTSVoices(), ", "))
}

func normalizeFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "opus", "ogg", "ogg_opus":
		return "opus", nil
	case "wav", "wave":
		return "wav", nil
	default:
		return "", fmt.Errorf("unsupported response_format %q (supported: opus, wav)", raw)
	}
}
