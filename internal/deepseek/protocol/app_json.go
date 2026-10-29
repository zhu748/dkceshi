package protocol

import (
	"bytes"
	"encoding/json"
	"net/url"
	"sort"
)

var completionKeyOrder = []string{"chat_session_id", "parent_message_id", "prompt", "ref_file_ids", "thinking_enabled", "search_enabled", "audio_id", "preempt", "model_type", "action"}

// MarshalAppJSON applies wire conventions after shared prompt assembly. It does
// not mutate the standard request, which is also used for history and retries.
func MarshalAppJSON(endpoint string, payload any) ([]byte, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	values, ok := payload.(map[string]any)
	if !ok {
		return marshalAppValue(payload)
	}
	var order []string
	switch parsed.Path {
	case DeepSeekCompletionTargetPath:
		order = completionKeyOrder
		copyValues := make(map[string]any, len(values)+1)
		for key, value := range values {
			copyValues[key] = value
		}
		if _, exists := copyValues["audio_id"]; !exists {
			copyValues["audio_id"] = nil
		}
		if copyValues["parent_message_id"] != nil {
			copyValues["model_type"] = nil
		}
		values = copyValues
	case "/api/v0/chat_session/delete":
		order = []string{"chat_session_id", "chat_session_ids"}
	case "/api/v0/users/login":
		// App 2.7.1（UsersLoginWithEmailAndPwdRequest / UsersLoginWithPhoneAndPwdRequest）
		// kotlinx.serialization 字段序：email → password → device_id → os；
		// 手机号为 mobile → area_code → password → device_id → os。credential_id 缺省不发送。
		order = []string{"email", "mobile", "area_code", "password", "device_id", "os"}
	case DeepSeekCheckDeviceTargetPath:
		// App 2.7.1 CheckDeviceRequest 字段序：device_model → device_id。
		order = []string{"device_model", "device_id"}
	default:
		return marshalAppValue(payload)
	}
	seen := make(map[string]bool, len(order))
	keys := make([]string, 0, len(values))
	for _, key := range order {
		if _, exists := values[key]; exists {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	var extra []string
	for key := range values {
		if !seen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)
	var output bytes.Buffer
	output.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			output.WriteByte(',')
		}
		name, _ := json.Marshal(key)
		output.Write(name)
		output.WriteByte(':')
		value, err := marshalAppValue(values[key])
		if err != nil {
			return nil, err
		}
		output.Write(value)
	}
	output.WriteByte('}')
	return output.Bytes(), nil
}

func marshalAppValue(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
