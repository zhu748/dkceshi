package tooldiagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

// APIKey is persisted only in the private index. PublicSettings strips it from
// every admin response; an empty update preserves it unless ClearAPIKey is set.
type RepairSettings struct {
	Protocol       string          `json:"protocol"`
	ExtraBody      json.RawMessage `json:"extra_body,omitempty"`
	Mode           string          `json:"mode"`
	BaseURL        string          `json:"base_url,omitempty"`
	Model          string          `json:"model,omitempty"`
	APIKey         string          `json:"api_key,omitempty"`
	TimeoutSeconds int             `json:"timeout_seconds"`
	HasAPIKey      bool            `json:"has_api_key,omitempty"`
	ClearAPIKey    bool            `json:"clear_api_key,omitempty"`
}

func DefaultRepairSettings() RepairSettings {
	return RepairSettings{Mode: "original", Protocol: "openai", TimeoutSeconds: 10}
}

func mergeRepairSettings(previous, next *RepairSettings) (*RepairSettings, error) {
	return resolveRepairSettings(previous, next, true)
}

func resolveRepairSettings(previous, next *RepairSettings, requireModel bool) (*RepairSettings, error) {
	value := DefaultRepairSettings()
	if previous != nil {
		value = *previous
	}
	if next != nil {
		old := value
		value = *next
		if value.APIKey == "" && !value.ClearAPIKey && sameRepairOrigin(old.BaseURL, value.BaseURL) && RepairProtocol(old.Protocol) == RepairProtocol(value.Protocol) {
			value.APIKey = old.APIKey
		}
		if value.ClearAPIKey {
			value.APIKey = ""
		}
	}
	value.Mode, value.Model, value.BaseURL = strings.TrimSpace(value.Mode), strings.TrimSpace(value.Model), strings.TrimSpace(value.BaseURL)
	value.HasAPIKey, value.ClearAPIKey = false, false
	value.Protocol = RepairProtocol(value.Protocol)
	if value.Protocol != "openai" && value.Protocol != "claude" && value.Protocol != "gemini" {
		return nil, errors.New("repair protocol must be openai, claude or gemini")
	}
	if value.Mode == "" {
		value.Mode = "original"
	}
	if value.TimeoutSeconds == 0 {
		value.TimeoutSeconds = 10
	}
	if value.Mode != "original" && value.Mode != "custom" {
		return nil, errors.New("repair mode must be original or custom")
	}
	if value.TimeoutSeconds < 1 || value.TimeoutSeconds > 300 {
		return nil, errors.New("repair timeout must be between 1 and 300 seconds")
	}
	if len(value.Model) > 256 || len(value.APIKey) > 8192 || strings.IndexFunc(value.APIKey, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return nil, errors.New("invalid repair model or API key")
	}
	if err := validateRepairExtraBody(value.ExtraBody); err != nil {
		return nil, err
	}
	value.ExtraBody = append(json.RawMessage(nil), value.ExtraBody...)
	if value.BaseURL != "" {
		if _, err := repairBaseURL(value.BaseURL, value.Protocol); err != nil {
			return nil, err
		}
	}
	if value.Mode == "custom" && (value.BaseURL == "" || requireModel && value.Model == "") {
		return nil, errors.New("custom repair requires an API URL and model")
	}
	if value.Protocol == "gemini" && value.Model != "" {
		if err := validateGeminiRepairModel(value.Model); err != nil {
			return nil, err
		}
	}
	return &value, nil
}

func PublicSettings(settings Settings) Settings {
	if settings.Repair != nil {
		value := *settings.Repair
		value.HasAPIKey = value.APIKey != ""
		value.APIKey = ""
		value.ClearAPIKey = false
		value.ExtraBody = append(json.RawMessage(nil), value.ExtraBody...)
		settings.Repair = &value
	}
	return settings
}

func (s *Store) RepairSettings() RepairSettings {
	if s == nil {
		return DefaultRepairSettings()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadError != nil || s.state.Settings.Repair == nil {
		return DefaultRepairSettings()
	}
	value := *s.state.Settings.Repair
	value.ExtraBody = append(json.RawMessage(nil), value.ExtraBody...)
	return value
}

func (s *Store) SetRepairSettings(next RepairSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadError != nil {
		return s.loadError
	}
	merged, err := mergeRepairSettings(s.state.Settings.Repair, &next)
	if err != nil {
		return err
	}
	previous := s.state.Settings.Repair
	s.state.Settings.Repair = merged
	if err := s.commitLocked(nil); err != nil {
		s.state.Settings.Repair = previous
		return err
	}
	return nil
}

type repairContextKey struct{}

func WithRepairSettings(ctx context.Context, settings RepairSettings) context.Context {
	return context.WithValue(ctx, repairContextKey{}, settings)
}
func RepairSettingsFromContext(ctx context.Context) RepairSettings {
	if ctx != nil {
		if value, ok := ctx.Value(repairContextKey{}).(RepairSettings); ok {
			return value
		}
	}
	return DefaultRepairSettings()
}

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
	blocked := map[string]bool{"model": true, "messages": true, "prompt": true, "system": true, "input": true, "tools": true, "toolchoice": true, "stream": true, "n": true, "parentmessageid": true, "reffileids": true, "contents": true, "systeminstruction": true, "toolconfig": true, "cachedcontent": true, "candidates": true}
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
			if strings.ReplaceAll(strings.ToLower(name), "_", "") == "candidatecount" && string(count) != "1" {
				return errors.New("repair generation must return one candidate")
			}
		}
	}
	return nil
}

// PreviewRepairSettings resolves the saved credential for an unsaved draft,
// without requiring a model selection or changing persistent configuration.
func (s *Store) PreviewRepairSettings(next RepairSettings) (RepairSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadError != nil {
		return RepairSettings{}, s.loadError
	}
	next.Mode = "custom"
	next.Model = ""
	value, err := resolveRepairSettings(s.state.Settings.Repair, &next, false)
	if err != nil {
		return RepairSettings{}, err
	}
	return *value, nil
}

// Return normalized public settings and their revision under one lock.
func (s *Store) SettingsSnapshot() (Settings, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return PublicSettings(s.state.Settings), s.state.Revision
}
