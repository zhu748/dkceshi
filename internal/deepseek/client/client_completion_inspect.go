package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

type inspectedStreamBody struct {
	io.Reader
	io.Closer
}

func detectMutedCompletion(body io.ReadCloser) (io.ReadCloser, bool, float64, error) {
	if body == nil {
		return nil, false, 0, nil
	}
	reader := bufio.NewReader(body)
	first, err := reader.Peek(1)
	if err != nil && err != io.EOF {
		return &inspectedStreamBody{reader, body}, false, 0, err
	}
	if len(first) == 0 || first[0] != '{' {
		return &inspectedStreamBody{reader, body}, false, 0, nil
	}
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	replay := &inspectedStreamBody{io.MultiReader(bytes.NewReader(data), reader), body}
	if err != nil {
		return replay, false, 0, err
	}
	if len(data) > 1<<20 {
		return replay, false, 0, nil
	}
	var parsed map[string]any
	if json.Unmarshal(data, &parsed) != nil {
		return replay, false, 0, nil
	}
	if isMutedJSONResponse(parsed) {
		return replay, true, extractMuteUntil(parsed), nil
	}
	return replay, false, 0, nil
}
func isMutedJSONResponse(resp map[string]any) bool {
	if resp == nil {
		return false
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		return false
	}
	bizCode := intFrom(data["biz_code"])
	bizMsg := strings.ToLower(strings.TrimSpace(getStringFromMap(data, "biz_msg")))
	if bizCode == 5 || bizCode == 14 || strings.Contains(bizMsg, "muted") {
		return true
	}
	bizData, _ := data["biz_data"].(map[string]any)
	if bizData != nil {
		if isMuted, _ := bizData["is_muted"].(float64); isMuted == 1 {
			return true
		}
	}
	return false
}

func extractMuteUntil(resp map[string]any) float64 {
	if resp == nil {
		return 0
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		return 0
	}
	if muteUntil, ok := data["mute_until"].(float64); ok && muteUntil > 0 {
		return muteUntil
	}
	bizData, _ := data["biz_data"].(map[string]any)
	if bizData != nil {
		if muteUntil, ok := bizData["mute_until"].(float64); ok && muteUntil > 0 {
			return muteUntil
		}
	}
	return 0
}

func getStringFromMap(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
