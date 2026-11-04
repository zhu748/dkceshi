package protocol

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestAppDefaultsAndCompletionJSONMatchCapture(t *testing.T) {
	raw, err := os.ReadFile("../../../tests/fixtures/android_app_2_7_1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Client   map[string]string
		Requests []struct {
			Path     string
			BodyKeys []string `json:"body_keys"`
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	var shared sharedConstants
	if err := json.Unmarshal(sharedConstantsJSON, &shared); err != nil {
		t.Fatal(err)
	}
	client := normalizeClientConstants(shared.Client)
	// The HAR describes a specific device; it must not become the global default.
	client.DeviceModel = fixture.Client["x-device-model"]
	headers := buildBaseHeaders(client, shared.BaseHeaders)
	for key, want := range fixture.Client {
		found := ""
		for name, value := range headers {
			if strings.EqualFold(name, key) {
				found = value
			}
		}
		if found != want {
			t.Fatalf("%s=%q; want %q", key, found, want)
		}
	}
	for _, parent := range []any{nil, 42} {
		payload := map[string]any{"chat_session_id": "test-session", "parent_message_id": parent, "prompt": "hello <工具>", "ref_file_ids": []string{"image-1", "image-2", "image-3", "image-4"}, "thinking_enabled": true, "search_enabled": false, "preempt": false, "model_type": "default", "action": nil}
		body, err := MarshalAppJSON(DeepSeekCompletionURL, payload)
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		if _, err := decoder.Token(); err != nil {
			t.Fatal(err)
		}
		var keys []string
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				t.Fatal(err)
			}
			keys = append(keys, key.(string))
			var value any
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
		}
		if !reflect.DeepEqual(keys, fixture.Requests[0].BodyKeys) {
			t.Fatalf("keys %v", keys)
		}
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatal(err)
		}
		if parent != nil && parsed["model_type"] != nil {
			t.Fatal("continuation must use null model_type")
		}
		if parent == nil && parsed["model_type"] != "default" {
			t.Fatal("new conversation lost model_type")
		}
		if parsed["audio_id"] != nil || len(parsed["ref_file_ids"].([]any)) != 4 {
			t.Fatal("audio or photo references mismatch")
		}
		if payload["model_type"] != "default" {
			t.Fatal("wire conversion mutated shared prompt payload")
		}
	}
}

func TestNovemberCompletionVariantsMatchCapture(t *testing.T) {
	raw, err := os.ReadFile("../../../tests/fixtures/android_app_2_7_1_2026_11_04.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Requests []struct {
			Path string
			Keys []string `json:"body_keys"`
		} `json:"request_shapes"`
		Variants []map[string]any `json:"completion_variants"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, request := range fixture.Requests {
		if request.Path == DeepSeekCompletionTargetPath {
			keys = request.Keys
		}
	}
	if len(keys) == 0 || len(fixture.Variants) == 0 {
		t.Fatal("capture fixture is missing completion observations")
	}
	for _, variant := range fixture.Variants {
		var parent any = 42
		if variant["parent_is_null"] == true {
			parent = nil
		}
		refs := make([]string, int(variant["ref_count"].(float64)))
		for i := range refs {
			refs[i] = "synthetic-file"
		}
		payload := map[string]any{"chat_session_id": "synthetic-session", "parent_message_id": parent, "prompt": "测试\u2028<内容>", "ref_file_ids": refs, "model_type": "default"}
		for _, key := range []string{"thinking_enabled", "search_enabled", "preempt", "action"} {
			payload[key] = variant[key]
		}
		body, err := MarshalAppJSON(DeepSeekCompletionURL, payload)
		if err != nil {
			t.Fatal(err)
		}
		var actual map[string]any
		if err := json.Unmarshal(body, &actual); err != nil {
			t.Fatal(err)
		}
		position := -1
		for _, key := range keys {
			next := bytes.Index(body, []byte(`"`+key+`":`))
			if next <= position {
				t.Fatalf("unexpected completion key ordering: %s", body)
			}
			position = next
		}
		for _, key := range []string{"thinking_enabled", "search_enabled", "audio_id", "preempt", "model_type", "action"} {
			if !reflect.DeepEqual(actual[key], variant[key]) {
				t.Fatalf("%s: got %v want %v", key, actual[key], variant[key])
			}
		}
		if len(actual["ref_file_ids"].([]any)) != len(refs) || actual["prompt"] != payload["prompt"] || payload["model_type"] != "default" {
			t.Fatal("wire conversion changed the prompt, references or shared payload")
		}
	}
}
