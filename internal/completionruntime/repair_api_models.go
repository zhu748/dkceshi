package completionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ds2api/internal/tooldiagnostics"
)

type RepairAPIModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type RepairAPIModels struct {
	Models    []RepairAPIModel `json:"models"`
	Truncated bool             `json:"truncated"`
}
type repairModelPage struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Lifecycle   string `json:"lifecycle"`
	} `json:"data"`
	Models []struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"displayName"`
		Methods     []string `json:"supportedGenerationMethods"`
	} `json:"models"`
	HasMore       bool   `json:"has_more"`
	LastID        string `json:"last_id"`
	NextPageToken string `json:"nextPageToken"`
}

// ListRepairAPIModels reads the configured provider using an unsaved draft. It
// never changes settings, and keeps model discovery separate from tool repair.
func ListRepairAPIModels(ctx context.Context, settings tooldiagnostics.RepairSettings) (RepairAPIModels, error) {
	out := RepairAPIModels{Models: []RepairAPIModel{}}
	endpoint, err := tooldiagnostics.RepairModelsEndpoint(settings)
	if err != nil {
		return out, err
	}
	timeout := 10 * time.Second
	if settings.TimeoutSeconds > 0 {
		timeout = time.Duration(settings.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	protocol := tooldiagnostics.RepairProtocol(settings.Protocol)
	seen, cursors := map[string]bool{}, map[string]bool{}
	cursor := ""
	for page := 0; page < 20; page++ {
		u, err := url.Parse(endpoint)
		if err != nil {
			return out, err
		}
		q := u.Query()
		if protocol == "claude" {
			q.Set("limit", "100")
			if cursor != "" {
				q.Set("after_id", cursor)
			}
		}
		if protocol == "gemini" {
			q.Set("pageSize", "100")
			if cursor != "" {
				q.Set("pageToken", cursor)
			}
		}
		u.RawQuery = q.Encode()
		data, err := doRepairAPIRequest(ctx, settings, http.MethodGet, u.String(), nil, 4<<20)
		if err != nil {
			return out, err
		}
		var response repairModelPage
		if json.Unmarshal(data, &response) != nil {
			return out, errors.New("repair API returned an invalid model list")
		}
		models, err := parseRepairModelPage(protocol, response)
		if err != nil {
			return out, err
		}
		for _, model := range models {
			if model.ID == "" || len(model.ID) > 256 || len(model.Name) > 1024 || seen[model.ID] {
				continue
			}
			if repairMetadataContainsKey(model.ID, settings.APIKey) || repairMetadataContainsKey(model.Name, settings.APIKey) {
				continue
			}
			if len(out.Models) >= 1000 {
				out.Truncated = true
				return out, nil
			}
			seen[model.ID] = true
			out.Models = append(out.Models, model)
		}
		next := ""
		switch protocol {
		case "claude":
			if response.HasMore {
				next = response.LastID
				if next == "" {
					return out, errors.New("claude model list omitted pagination cursor")
				}
			}
		case "gemini":
			next = response.NextPageToken
		default:
			out.Truncated = response.HasMore
			return out, nil
		}
		if next == "" {
			return out, nil
		}
		if cursors[next] || len(next) > 2048 || repairMetadataContainsKey(next, settings.APIKey) {
			return out, errors.New("repair model list returned an invalid pagination cursor")
		}
		cursors[next] = true
		cursor = next
	}
	out.Truncated = true
	return out, nil
}

func parseRepairModelPage(protocol string, page repairModelPage) ([]RepairAPIModel, error) {
	models := []RepairAPIModel{}
	if protocol != "gemini" {
		if page.Data == nil {
			return nil, errors.New("repair model list must contain a data array")
		}
		for _, model := range page.Data {
			if model.Lifecycle == "retired" {
				continue
			}
			name := model.DisplayName
			if name == "" {
				name = model.ID
			}
			models = append(models, RepairAPIModel{ID: model.ID, Name: name})
		}
		return models, nil
	}
	for _, model := range page.Models {
		compatible := len(model.Methods) == 0
		for _, method := range model.Methods {
			if method == "generateContent" {
				compatible = true
			}
		}
		if !compatible {
			continue
		}
		id := strings.TrimPrefix(model.Name, "models/")
		if id == "" || strings.ContainsAny(id, "/\\?#:") {
			continue
		}
		name := model.DisplayName
		if name == "" {
			name = id
		}
		models = append(models, RepairAPIModel{ID: id, Name: name})
	}
	return models, nil
}

func repairMetadataContainsKey(value, key string) bool {
	return key != "" && (value == key || len(key) >= 8 && strings.Contains(value, key))
}
