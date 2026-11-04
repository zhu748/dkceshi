package promptcompat

import "testing"

func TestStandardRequestCompletionPayloadSetsModelTypeFromResolvedModel(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		thinking  bool
		search    bool
		modelType string
	}{
		{name: "default", model: "deepseek-v4.1-flash", thinking: false, search: false, modelType: "default"},
		{name: "default_nothinking", model: "deepseek-v4.1-flash-nothinking", thinking: false, search: false, modelType: "default"},
		{name: "search", model: "deepseek-v4.1-flash-search", thinking: true, search: true, modelType: "default"},
		{name: "merged", model: "deepseek-v4.1-flash", thinking: true, search: false, modelType: "default"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := StandardRequest{
				ResolvedModel: tc.model,
				FinalPrompt:   "hello",
				Thinking:      tc.thinking,
				Search:        tc.search,
				RefFileIDs:    []string{"file-a", "file-b"},
			}

			payload := req.CompletionPayload("session-123")

			if got := payload["model_type"]; got != tc.modelType {
				t.Fatalf("expected model_type %s, got %#v", tc.modelType, got)
			}
			if got := payload["chat_session_id"]; got != "session-123" {
				t.Fatalf("unexpected chat_session_id: %#v", got)
			}
			if got := payload["thinking_enabled"]; got != tc.thinking {
				t.Fatalf("unexpected thinking_enabled: %#v", got)
			}
			if got := payload["search_enabled"]; got != tc.search {
				t.Fatalf("unexpected search_enabled: %#v", got)
			}
			if got := payload["temperature"]; got != nil {
				t.Fatalf("sampling parameters must never reach the upstream payload, got temperature=%#v", got)
			}
			refFileIDs, ok := payload["ref_file_ids"].([]any)
			if !ok {
				t.Fatalf("expected ref_file_ids slice, got %#v", payload["ref_file_ids"])
			}
			if len(refFileIDs) != 2 || refFileIDs[0] != "file-a" || refFileIDs[1] != "file-b" {
				t.Fatalf("unexpected ref_file_ids: %#v", refFileIDs)
			}
		})
	}
}

// 上游 completion 请求体的键集必须与真实 App 完全一致：客户端传来的采样参数、
// 停止序列与思考强度档位只做兼容接收，任何一个都不得进入请求体。
func TestStandardRequestCompletionPayloadRejectsSamplingParameters(t *testing.T) {
	req := map[string]any{
		"model":                 "deepseek-v4.1-flash",
		"messages":              []any{map[string]any{"role": "user", "content": "hi"}},
		"temperature":           0.7,
		"top_p":                 0.9,
		"max_tokens":            4096,
		"max_completion_tokens": 2048,
		"presence_penalty":      0.5,
		"frequency_penalty":     0.5,
		"stop":                  []any{"\n\n"},
		"reasoning_effort":      "high",
	}
	std, err := NormalizeOpenAIChatRequest(nil, req, "")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	payload := std.CompletionPayload("session-abc")
	allowed := map[string]struct{}{
		"chat_session_id": {}, "parent_message_id": {}, "prompt": {},
		"ref_file_ids": {}, "thinking_enabled": {}, "search_enabled": {},
		"model_type": {}, "action": {}, "preempt": {},
	}
	for key := range payload {
		if _, ok := allowed[key]; !ok {
			t.Fatalf("unexpected key %q in upstream completion payload: %#v", key, payload[key])
		}
	}
	for _, key := range []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "presence_penalty", "frequency_penalty", "stop", "reasoning_effort"} {
		if _, exists := payload[key]; exists {
			t.Fatalf("sampling/effort parameter %q must never reach the upstream payload", key)
		}
	}
}
