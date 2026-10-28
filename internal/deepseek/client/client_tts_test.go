package client

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"ds2api/internal/auth"
	trans "ds2api/internal/deepseek/transport"
)

func TestTTSVoiceValidation(t *testing.T) {
	voices := TTSVoices()
	if len(voices) != 4 {
		t.Fatalf("expected 4 voices, got %d", len(voices))
	}
	for _, v := range []string{"mira", "echo", "stella", "tide"} {
		if !IsValidTTSVoice(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	for _, v := range []string{"", "unknown", "alloy"} {
		if IsValidTTSVoice(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestTTSServiceErrorFormat(t *testing.T) {
	err := &TTSServiceError{Code: 3, Msg: "service_error"}
	if got := err.Error(); got != "tts service error: code=3 msg=service_error" {
		t.Fatalf("unexpected error format: %q", got)
	}
}

func TestTTSVoiceLockFor(t *testing.T) {
	c := &Client{}
	l1 := c.ttsVoiceLockFor("acc1")
	l2 := c.ttsVoiceLockFor("acc1")
	l3 := c.ttsVoiceLockFor("acc2")
	if l1 != l2 {
		t.Fatal("same account must return the same lock instance")
	}
	if l1 == l3 {
		t.Fatal("different accounts must return different locks")
	}
	lEmpty := c.ttsVoiceLockFor("")
	if lEmpty == nil {
		t.Fatal("empty account key must return non-nil lock")
	}
}

func TestPinTTSAccount(t *testing.T) {
	a := &auth.RequestAuth{AccountID: "acc1"}
	restore := pinTTSAccount(a)
	if a.TargetAccount != "acc1" {
		t.Fatalf("TargetAccount=%q want acc1", a.TargetAccount)
	}
	restore()
	if a.TargetAccount != "" {
		t.Fatalf("TargetAccount=%q want empty after restore", a.TargetAccount)
	}

	// An account the caller pinned explicitly must not be overwritten.
	b := &auth.RequestAuth{AccountID: "acc1", TargetAccount: "acc0"}
	restore = pinTTSAccount(b)
	if b.TargetAccount != "acc0" {
		t.Fatalf("explicit target overwritten: %q", b.TargetAccount)
	}
	restore()
	if b.TargetAccount != "acc0" {
		t.Fatalf("explicit target changed on restore: %q", b.TargetAccount)
	}

	// Direct-token callers have no account id; pinning must be a no-op.
	c := &auth.RequestAuth{}
	restore = pinTTSAccount(c)
	if c.TargetAccount != "" {
		t.Fatalf("empty account must not set a target: %q", c.TargetAccount)
	}
	restore()
}

func TestTTSVoiceCacheHelpers(t *testing.T) {
	c := &Client{}
	if _, ok := c.cachedTTSVoice("acc1"); ok {
		t.Fatal("empty cache must miss")
	}
	c.rememberTTSVoice("acc1", "mira")
	if got, ok := c.cachedTTSVoice("acc1"); !ok || got != "mira" {
		t.Fatalf("cachedTTSVoice=%q ok=%v", got, ok)
	}
	c.rememberTTSVoice("acc1", "echo")
	if got, _ := c.cachedTTSVoice("acc1"); got != "echo" {
		t.Fatalf("cache not updated: %q", got)
	}
	c.forgetTTSVoice("acc1")
	if _, ok := c.cachedTTSVoice("acc1"); ok {
		t.Fatal("forgetTTSVoice did not clear the entry")
	}
}

func writeServerFrameForTest(conn net.Conn, fin bool, opcode byte, payload []byte) error {
	var header [10]byte
	if fin {
		header[0] = 0x80 | opcode
	} else {
		header[0] = opcode
	}
	n := 2
	switch {
	case len(payload) < 126:
		header[1] = byte(len(payload))
	case len(payload) <= 0xffff:
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:], uint16(len(payload)))
		n = 4
	default:
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(len(payload)))
		n = 10
	}
	if _, err := conn.Write(header[:n]); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func TestWaitTTSReadyAndReadAudio(t *testing.T) {
	c1, c2 := net.Pipe()
	t.Cleanup(func() {
		if err := c1.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
		if err := c2.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
	})

	ws := trans.NewWSConnForTest(c1)

	// Build a valid 4-byte sequence header + opus frame
	frame0 := make([]byte, 8)
	binary.BigEndian.PutUint32(frame0[:4], 0)
	copy(frame0[4:], []byte{0x6b, 0x01, 0x11, 0x22})

	frame1 := make([]byte, 8)
	binary.BigEndian.PutUint32(frame1[:4], 1)
	copy(frame1[4:], []byte{0x6b, 0x01, 0x33, 0x44})

	go func() {
		// Non-json noise text frame before ready
		_ = writeServerFrameForTest(c2, true, trans.WSOpText, []byte("plain text noise"))
		// Early racing binary frame
		_ = writeServerFrameForTest(c2, true, trans.WSOpBinary, frame0)
		// Ready event
		readyJSON := `{"event":"ready","voice_id":"mira","format":"opus","audio_id":"a123"}`
		_ = writeServerFrameForTest(c2, true, trans.WSOpText, []byte(readyJSON))
		// Subsequent binary frame
		_ = writeServerFrameForTest(c2, true, trans.WSOpBinary, frame1)
		// Finish event
		finishJSON := `{"event":"finish","code":0,"msg":"success"}`
		_ = writeServerFrameForTest(c2, true, trans.WSOpText, []byte(finishJSON))
		// Drain client acknowledge frame from pipe
		buf := make([]byte, 256)
		_, _ = c2.Read(buf)
	}()

	ready, pending, err := waitTTSReady(ws, 2*time.Second)
	if err != nil {
		t.Fatalf("waitTTSReady failed: %v", err)
	}
	if ready.VoiceID != "mira" || ready.AudioID != "a123" {
		t.Fatalf("unexpected ready event: %+v", ready)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending frame, got %d", len(pending))
	}

	audio, err := readTTSAudio(ws, ready, pending, TTSOptions{Format: "opus", ReadTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("readTTSAudio failed: %v", err)
	}
	if len(audio.Frames) != 2 {
		t.Fatalf("expected 2 audio frames, got %d", len(audio.Frames))
	}
	if audio.VoiceID != "mira" || audio.AudioID != "a123" {
		t.Fatalf("unexpected audio metadata: %+v", audio)
	}
}

func TestWaitTTSReadyReturnsServiceError(t *testing.T) {
	c1, c2 := net.Pipe()
	t.Cleanup(func() {
		if err := c1.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
		if err := c2.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
	})

	ws := trans.NewWSConnForTest(c1)
	go func() {
		errorJSON := `{"event":"error","code":40001,"msg":"message not found"}`
		_ = writeServerFrameForTest(c2, true, trans.WSOpText, []byte(errorJSON))
	}()

	_, _, err := waitTTSReady(ws, 2*time.Second)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var svcErr *TTSServiceError
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *TTSServiceError, got %T: %v", err, err)
	}
	if svcErr.Code != 40001 {
		t.Fatalf("expected code 40001, got %d", svcErr.Code)
	}
}

func TestReadTTSAudioFinishErrorCode(t *testing.T) {
	c1, c2 := net.Pipe()
	t.Cleanup(func() {
		if err := c1.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
		if err := c2.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
	})

	ws := trans.NewWSConnForTest(c1)
	go func() {
		finishJSON := `{"event":"finish","code":3,"msg":"service_error"}`
		_ = writeServerFrameForTest(c2, true, trans.WSOpText, []byte(finishJSON))
	}()

	ready := ttsEvent{Event: "ready", VoiceID: "mira", Format: "opus"}
	_, err := readTTSAudio(ws, ready, nil, TTSOptions{Format: "opus", ReadTimeout: 2 * time.Second})
	if err == nil {
		t.Fatal("expected service_error, got nil")
	}
	var svcErr *TTSServiceError
	if !errors.As(err, &svcErr) || svcErr.Code != 3 {
		t.Fatalf("expected TTSServiceError code 3, got: %v", err)
	}
}

func TestWaitTTSReadyPendingFramesCap(t *testing.T) {
	c1, c2 := net.Pipe()
	t.Cleanup(func() {
		if err := c1.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
		if err := c2.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
	})

	ws := trans.NewWSConnForTest(c1)
	go func() {
		// Send frame exceeding maxTTSAudioBytes limit
		oversized := make([]byte, maxTTSAudioBytes+10)
		_ = writeServerFrameForTest(c2, true, trans.WSOpBinary, oversized)
	}()

	_, _, err := waitTTSReady(ws, 2*time.Second)
	if err == nil {
		t.Fatal("expected pending frame limit error, got nil")
	}
}
