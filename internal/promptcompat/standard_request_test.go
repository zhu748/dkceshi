package promptcompat

import (
	"encoding/json"
	"testing"
)

func TestStandardRequestCompletionPayloadSetsModelTypeFromResolvedModel(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		thinking  bool
		search    bool
		modelType string
	}{
		{name: "default", model: "deepseek-v4-flash", thinking: false, search: false, modelType: "default"},
		{name: "default_nothinking", model: "deepseek-v4-flash-nothinking", thinking: false, search: false, modelType: "default"},
		{name: "expert", model: "deepseek-v4-pro", thinking: true, search: false, modelType: "expert"},
		{name: "vision", model: "deepseek-v4-vision", thinking: true, search: false, modelType: "vision"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := StandardRequest{
				ResolvedModel: tc.model,
				FinalPrompt:   "hello",
				Thinking:      tc.thinking,
				Search:        tc.search,
				RefFileIDs:    []string{"file-a", "file-b"},
				PassThrough: map[string]any{
					"temperature": 0.3,
				},
			}

			payload := req.CompletionPayload("session-123")
			m, ok := payload.(*OrderedJSONMap)
			if !ok {
				t.Fatalf("expected *OrderedJSONMap, got %T", payload)
			}

			if got := m.M["model_type"]; got != tc.modelType {
				t.Fatalf("expected model_type %s, got %#v", tc.modelType, got)
			}
			if got := m.M["chat_session_id"]; got != "session-123" {
				t.Fatalf("unexpected chat_session_id: %#v", got)
			}
			if got := m.M["thinking_enabled"]; got != tc.thinking {
				t.Fatalf("unexpected thinking_enabled: %#v", got)
			}
			if got := m.M["search_enabled"]; got != tc.search {
				t.Fatalf("unexpected search_enabled: %#v", got)
			}
			if got := m.M["temperature"]; got != 0.3 {
				t.Fatalf("expected passthrough temperature, got %#v", got)
			}
			refFileIDs, ok := m.M["ref_file_ids"].([]any)
			if !ok {
				t.Fatalf("expected ref_file_ids slice, got %#v", m.M["ref_file_ids"])
			}
			if len(refFileIDs) != 2 || refFileIDs[0] != "file-a" || refFileIDs[1] != "file-b" {
				t.Fatalf("unexpected ref_file_ids: %#v", refFileIDs)
			}
			// 真实 Android App 抓包对齐：必须存在 audio_id / preempt / action 三个字段
			if _, ok := m.M["audio_id"]; !ok {
				t.Fatalf("expected audio_id field present (App 对齐)")
			}
			if _, ok := m.M["preempt"]; !ok {
				t.Fatalf("expected preempt field present (App 对齐)")
			}
			if got := m.M["preempt"]; got != false {
				t.Fatalf("expected preempt=false, got %#v", got)
			}
			if _, ok := m.M["action"]; !ok {
				t.Fatalf("expected action field present (App 对齐)")
			}
		})
	}
}

// TestStandardRequestCompletionPayloadFieldOrder 验证 JSON 序列化后的字段顺序与
// 真实 Android App 抓包一致：
//
//	{"chat_session_id":...,"parent_message_id":null,"prompt":...,"ref_file_ids":[],
//	 "thinking_enabled":true,"search_enabled":true,"audio_id":null,"preempt":false,
//	 "model_type":"default","action":null}
func TestStandardRequestCompletionPayloadFieldOrder(t *testing.T) {
	req := StandardRequest{
		ResolvedModel: "deepseek-v4-flash",
		FinalPrompt:   "你好",
		Thinking:      true,
		Search:        true,
		RefFileIDs:    []string{},
	}
	payload := req.CompletionPayload("2a28bdf6-9387-43f2-b9f0-6e3ff2dd300b")
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	expected := `{"chat_session_id":"2a28bdf6-9387-43f2-b9f0-6e3ff2dd300b","parent_message_id":null,"prompt":"你好","ref_file_ids":[],"thinking_enabled":true,"search_enabled":true,"audio_id":null,"preempt":false,"model_type":"default","action":null}`
	if string(raw) != expected {
		t.Fatalf("field order mismatch:\n got: %s\nwant: %s", string(raw), expected)
	}
}
