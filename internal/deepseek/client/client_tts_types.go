package client

import (
	"fmt"
	"strings"
	"time"
)

// TTS voice ids exposed by the DeepSeek web client. They are account-level
// server-side settings; the WebSocket query carries no voice parameter.
const (
	TTSVoiceMira   = "mira"
	TTSVoiceEcho   = "echo"
	TTSVoiceStella = "stella"
	TTSVoiceTide   = "tide"
)

var ttsVoiceIDs = map[string]struct{}{
	TTSVoiceMira:   {},
	TTSVoiceEcho:   {},
	TTSVoiceStella: {},
	TTSVoiceTide:   {},
}

// TTSVoices returns the supported voice ids in the order the web client shows
// them.
func TTSVoices() []string {
	return []string{TTSVoiceMira, TTSVoiceEcho, TTSVoiceStella, TTSVoiceTide}
}

// IsValidTTSVoice reports whether voiceID is one of the four upstream voices.
func IsValidTTSVoice(voiceID string) bool {
	_, ok := ttsVoiceIDs[strings.TrimSpace(voiceID)]
	return ok
}

const (
	maxTTSAudioBytes = 64 << 20
	maxTTSFrames     = 200000
)

// TTSOptions controls one speech synthesis request.
type TTSOptions struct {
	// Voice switches the account voice before the stream starts. Empty keeps
	// the account's current voice (the server default is mira).
	Voice string
	// Format is the upstream audio format. Only "opus" is currently known.
	Format string
	// ReadTimeout bounds each individual WebSocket read.
	ReadTimeout time.Duration
}

// TTSAudio is the raw upstream result: a sequence of bare Opus packets plus
// the metadata echoed by the ready event.
type TTSAudio struct {
	Frames  [][]byte
	AudioID string
	VoiceID string
	Format  string
	TraceID string
}

type ttsEvent struct {
	Event   string `json:"event"`
	AudioID string `json:"audio_id"`
	Format  string `json:"format"`
	VoiceID string `json:"voice_id"`
	TraceID string `json:"trace_id"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
}

// TTSServiceError is the upstream's explicit failure event. It is retryable:
// the message may not have been committed yet when the stream opens, and a
// fresh ticket plus a short wait usually succeeds.
type TTSServiceError struct {
	Code int
	Msg  string
}

func (e *TTSServiceError) Error() string {
	return fmt.Sprintf("tts service error: code=%d msg=%s", e.Code, e.Msg)
}
