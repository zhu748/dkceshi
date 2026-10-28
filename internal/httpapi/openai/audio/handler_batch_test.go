package audio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

type mockTTSDS struct {
	stubDS
}

func (mockTTSDS) CreateSession(context.Context, *auth.RequestAuth, int) (string, error) {
	return "session-1", nil
}

func (mockTTSDS) GetPow(context.Context, *auth.RequestAuth, int) (string, error) {
	return "pow", nil
}

func (mockTTSDS) CallCompletion(context.Context, *auth.RequestAuth, map[string]any, string, int) (*http.Response, error) {
	time.Sleep(10 * time.Millisecond)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       ioNopCloserString("data: {\"response_message_id\":7,\"v\":\"朗读内容\"}\n\ndata: [DONE]\n\n"),
	}, nil
}

func (mockTTSDS) SynthesizeSpeech(context.Context, *auth.RequestAuth, string, int, dsclient.TTSOptions) (*dsclient.TTSAudio, error) {
	time.Sleep(10 * time.Millisecond)
	return &dsclient.TTSAudio{
		Frames:  [][]byte{{0x6b, 0x01, 0x02}, {0x6b, 0x03, 0x04}},
		VoiceID: "mira",
		AudioID: "audio-1",
	}, nil
}

func (mockTTSDS) DeleteSessionForToken(context.Context, string, string) (*dsclient.DeleteSessionResult, error) {
	return nil, nil
}

func ioNopCloserString(s string) *readCloser {
	return &readCloser{Reader: strings.NewReader(s)}
}

type readCloser struct {
	*strings.Reader
}

func (r *readCloser) Close() error { return nil }

// TestTTSBatchProcessingWithToolsOnlyAndGlobalLimit 模拟真实线上场景：
// 账号池中有 2 个 default 账号和 2 个 tools_only 账号，全局并发上限为 2。
// 当输入长文本（拆分出 6+ 个 segments）时，系统不能因为 segments 数量超过并发容量而报错，
// 应该正确过滤非 tools 账号、遵守全局在途上限，并通过分批并发处理顺利完成所有段落的朗读。
func TestTTSBatchProcessingWithToolsOnlyAndGlobalLimit(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[
			{"email":"def1@example.com","password":"pwd","token":"tok1","pool_type":"default"},
			{"email":"def2@example.com","password":"pwd","token":"tok2","pool_type":"default"},
			{"email":"tools1@example.com","password":"pwd","token":"tok3","pool_type":"tools_only"},
			{"email":"tools2@example.com","password":"pwd","token":"tok4","pool_type":"tools_only"}
		],
		"runtime":{"account_max_inflight":2,"account_max_queue":2,"global_max_inflight":2},
		"tts":{"enabled":true,"multi_account":true,"account_concurrency":true,"segment_chars":50}
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	resolver := auth.NewResolver(store, pool, func(_ context.Context, _ config.Account) (string, error) {
		return "tok", nil
	})
	h := &Handler{
		Auth:  resolver,
		DS:    mockTTSDS{},
		Store: store,
	}

	// 构造 600+ 字的长文本（约 12 个 segment）
	input := strings.Repeat("这是一段用于测试长文本分批朗读的完整段落句子。", 30)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"`+input+`"}`))
	req.Header.Set("Authorization", "Bearer managed-key")
	rec := httptest.NewRecorder()
	h.Speech(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d with body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/ogg" {
		t.Fatalf("expected Content-Type audio/ogg, got %q", ct)
	}
	segmentsHeader := rec.Header().Get("X-Ds2-Tts-Segments")
	if segmentsHeader == "" || segmentsHeader == "1" {
		t.Fatalf("expected multiple segments, got %q", segmentsHeader)
	}
}

// TestTTSBatchProcessingWhenPoolAtCapacityGraceful 验证当号池部分账号被外部在途请求占用时，
// 剩余可用容量依然能够分批并发处理完长文本，而不是因容量不足直接报错。
func TestTTSBatchProcessingWhenPoolAtCapacityGraceful(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["managed-key"],
		"accounts":[
			{"email":"acc1@example.com","password":"pwd","token":"tok1"},
			{"email":"acc2@example.com","password":"pwd","token":"tok2"}
		],
		"runtime":{"account_max_inflight":2,"account_max_queue":2,"global_max_inflight":3},
		"tts":{"enabled":true,"multi_account":true,"account_concurrency":true,"segment_chars":30}
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	resolver := auth.NewResolver(store, pool, func(_ context.Context, _ config.Account) (string, error) {
		return "tok", nil
	})
	h := &Handler{
		Auth:  resolver,
		DS:    mockTTSDS{},
		Store: store,
	}

	// 模拟外部已占用了 acc1 的 1 个并发槽位
	acc, ok := pool.AcquireKeepAlive("acc1@example.com")
	if !ok {
		t.Fatalf("failed to acquire keep alive slot")
	}
	defer pool.Release(acc.Identifier())

	input := strings.Repeat("第一句话测试。", 20) // 多段文本
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"input":"`+input+`"}`))
	req.Header.Set("Authorization", "Bearer managed-key")
	rec := httptest.NewRecorder()
	h.Speech(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d with body: %s", rec.Code, rec.Body.String())
	}
}
