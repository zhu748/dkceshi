package client

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	trans "ds2api/internal/deepseek/transport"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func (c *Client) dialTTSWebSocket(ctx context.Context, a *auth.RequestAuth, sessionID string, messageID int, ticket string, format string) (*trans.WSConn, *http.Response, error) {
	pathQuery := dsprotocol.DeepSeekTTSTargetPath +
		"?chat_session_id=" + url.QueryEscape(sessionID) +
		"&message_id=" + strconv.Itoa(messageID) +
		"&ticket=" + url.QueryEscape(ticket) +
		"&mode=manual" +
		"&format=" + url.QueryEscape(format)

	var dial trans.DialContextFunc
	if proxyCfg, ok := c.resolveProxyForAccount(a.Account); ok {
		var err error
		dial, err = proxyDialContext(proxyCfg)
		if err != nil {
			return nil, nil, err
		}
	} else if a.Account.NoProxy {
		dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	return trans.DialWebSocket(ctx, trans.WebSocketDialOptions{
		Host:           dsprotocol.DeepSeekHost,
		PathQuery:      pathQuery,
		DialContext:    dial,
		Origin:         "https://chat.deepseek.com",
		UserAgent:      c.authHeadersForAuth(a)["User-Agent"],
		AcceptLanguage: dsprotocol.AcceptLanguageFor(a.Account.Locale),
	})
}

// waitTTSReady consumes frames until the ready event arrives. Binary frames
// that race ahead of it are returned for the audio loop.
func waitTTSReady(ws *trans.WSConn, timeout time.Duration) (ttsEvent, [][]byte, error) {
	var pending [][]byte
	pendingBytes := 0
	for {
		if err := ws.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return ttsEvent{}, nil, err
		}
		opcode, payload, err := ws.ReadMessage()
		if err != nil {
			return ttsEvent{}, nil, fmt.Errorf("tts ready: %w", err)
		}
		switch opcode {
		case trans.WSOpBinary:
			pendingBytes += len(payload)
			if len(pending) >= maxTTSFrames || pendingBytes > maxTTSAudioBytes {
				return ttsEvent{}, nil, errors.New("tts pending audio frames exceed limit")
			}
			pending = append(pending, payload)
		case trans.WSOpText:
			var ev ttsEvent
			if err := json.Unmarshal(payload, &ev); err != nil {
				config.Logger.Warn("[tts] unexpected text frame before ready", "payload", string(payload), "error", err)
				continue
			}
			switch ev.Event {
			case "ready":
				return ev, pending, nil
			case "error":
				return ttsEvent{}, nil, &TTSServiceError{Code: ev.Code, Msg: ev.Msg}
			}
		}
	}
}

// readTTSAudio drains the binary frames, waits for the server finish event and
// acknowledges it. Sending the client finish earlier truncates the audio.
func readTTSAudio(ws *trans.WSConn, ready ttsEvent, pending [][]byte, opts TTSOptions) (*TTSAudio, error) {
	audio := &TTSAudio{
		AudioID: ready.AudioID,
		VoiceID: ready.VoiceID,
		Format:  ready.Format,
		TraceID: ready.TraceID,
	}
	if audio.Format == "" {
		audio.Format = opts.Format
	}
	if audio.Format == "" {
		audio.Format = "opus"
	}

	totalBytes := 0
	expectedSeq := uint32(0)
	sequenceGaps := 0
	addFrame := func(payload []byte) error {
		if len(payload) < 4 {
			return errors.New("tts binary frame too short")
		}
		seq := binary.BigEndian.Uint32(payload[:4])
		data := payload[4:]
		if len(data) == 0 {
			return nil
		}
		if len(audio.Frames) >= maxTTSFrames {
			return fmt.Errorf("tts returned more than %d frames", maxTTSFrames)
		}
		totalBytes += len(data)
		if totalBytes > maxTTSAudioBytes {
			return errors.New("tts audio exceeds size limit")
		}
		// Keep every frame in arrival order. The sequence number is metadata:
		// long streams repeat or skip it (one Opus packet can carry several
		// frames), and deduplicating by sequence silently dropped audio from
		// the middle of the reply.
		if seq != expectedSeq {
			sequenceGaps++
		}
		expectedSeq = seq + 1
		audio.Frames = append(audio.Frames, append([]byte(nil), data...))
		return nil
	}
	for _, payload := range pending {
		if err := addFrame(payload); err != nil {
			return nil, err
		}
	}

	finished := false
	for !finished {
		if err := ws.SetReadDeadline(time.Now().Add(opts.ReadTimeout)); err != nil {
			return nil, err
		}
		opcode, payload, err := ws.ReadMessage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("tts stream: %w", err)
		}
		switch opcode {
		case trans.WSOpBinary:
			if err := addFrame(payload); err != nil {
				return nil, err
			}
		case trans.WSOpText:
			var ev ttsEvent
			if err := json.Unmarshal(payload, &ev); err != nil {
				continue
			}
			switch ev.Event {
			case "finish":
				if ev.Code != 0 {
					return nil, &TTSServiceError{Code: ev.Code, Msg: ev.Msg}
				}
				finished = true
			case "error":
				return nil, fmt.Errorf("tts upstream error: code=%d msg=%s", ev.Code, ev.Msg)
			}
		}
	}
	if !finished {
		return nil, errors.New("tts stream ended before finish event")
	}
	if err := ws.WriteText([]byte(`{"event":"finish"}`)); err != nil {
		config.Logger.Warn("[tts] acknowledge finish failed", "error", err)
	}
	if sequenceGaps > 0 {
		config.Logger.Warn("[tts] non-contiguous frame sequence",
			"gaps", sequenceGaps, "frames", len(audio.Frames))
	}
	if len(audio.Frames) == 0 {
		return nil, errors.New("tts returned no audio")
	}
	return audio, nil
}
