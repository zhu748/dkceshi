package tooldiagnostics

import (
	"encoding/json"
	"errors"
	"strings"
)

func validateRepairExtraBody(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if len(raw) > 8192 {
		return errors.New("repair extra parameters exceed 8192 bytes")
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return errors.New("repair extra parameters must be a JSON object")
	}
	blocked := map[string]bool{"functions": true, "functioncall": true, "responseformat": true, "model": true, "messages": true, "prompt": true, "system": true, "input": true, "tools": true, "toolchoice": true, "stream": true, "n": true, "parentmessageid": true, "reffileids": true, "contents": true, "systeminstruction": true, "toolconfig": true, "cachedcontent": true, "candidates": true}
	for key := range values {
		if blocked[strings.ReplaceAll(strings.ToLower(key), "_", "")] {
			return errors.New("repair extra parameters cannot override request context, model or output protocol")
		}
	}
	generationCount := 0
	for key, generation := range values {
		if strings.ReplaceAll(strings.ToLower(key), "_", "") != "generationconfig" {
			continue
		}
		generationCount++
		if generationCount > 1 {
			return errors.New("duplicate generationConfig aliases")
		}
		var config map[string]json.RawMessage
		if json.Unmarshal(generation, &config) != nil || config == nil {
			return errors.New("generationConfig must be a JSON object")
		}
		for name, count := range config {
			normalized := strings.ReplaceAll(strings.ToLower(name), "_", "")
			if normalized == "responsemimetype" || normalized == "responseschema" || normalized == "responsejsonschema" {
				return errors.New("repair output must remain plain text")
			}
			if strings.ReplaceAll(strings.ToLower(name), "_", "") == "candidatecount" && string(count) != "1" {
				return errors.New("repair generation must return one candidate")
			}
		}
	}
	return nil
}
