package tooldiagnostics

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var repairVersionPath = regexp.MustCompile(`^v[0-9]+(beta[0-9]*|alpha[0-9]*)?$`)

func RepairProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		return "openai"
	}
	return protocol
}

func sameRepairOrigin(a, b string) bool {
	left, err := url.Parse(strings.TrimSpace(a))
	if err != nil {
		return false
	}
	right, err := url.Parse(strings.TrimSpace(b))
	return err == nil && strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

// repairBaseURL preserves gateway prefixes and accepts either a base URL or a
// generation endpoint. No credentials are ever moved into query parameters.
func repairBaseURL(base, protocol string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(base) > 2048 {
		return nil, errors.New("repair API URL must be HTTP(S), without credentials, query or fragment")
	}
	path := strings.TrimRight(u.Path, "/")
	for _, suffix := range []string{"/chat/completions", "/messages"} {
		path = strings.TrimSuffix(path, suffix)
	}
	if index := strings.LastIndex(path, "/models"); index >= 0 && (len(path) == index+7 || path[index+7] == '/') {
		path = path[:index]
	}
	if path == "" {
		if protocol == "gemini" {
			path = "/v1beta"
		} else {
			path = "/v1"
		}
	} else if protocol != "openai" {
		parts := strings.Split(path, "/")
		if !repairVersionPath.MatchString(parts[len(parts)-1]) {
			if protocol == "gemini" {
				path += "/v1beta"
			} else {
				path += "/v1"
			}
		}
	}
	u.Path, u.RawPath = path, ""
	return u, nil
}

// RepairEndpoint retains the OpenAI-only helper for existing callers.
func RepairEndpoint(base string) (string, error) {
	return RepairAPIEndpoint(RepairSettings{BaseURL: base, Protocol: "openai"})
}

func RepairAPIEndpoint(settings RepairSettings) (string, error) {
	protocol := RepairProtocol(settings.Protocol)
	u, err := repairBaseURL(settings.BaseURL, protocol)
	if err != nil {
		return "", err
	}
	switch protocol {
	case "openai":
		u.Path += "/chat/completions"
	case "claude":
		u.Path += "/messages"
	case "gemini":
		if err := validateGeminiRepairModel(settings.Model); err != nil {
			return "", err
		}
		u.Path += "/models/" + strings.TrimPrefix(settings.Model, "models/") + ":generateContent"
	default:
		return "", errors.New("unsupported repair protocol")
	}
	return u.String(), nil
}

func RepairModelsEndpoint(settings RepairSettings) (string, error) {
	protocol := RepairProtocol(settings.Protocol)
	if protocol != "openai" && protocol != "claude" && protocol != "gemini" {
		return "", errors.New("unsupported repair protocol")
	}
	u, err := repairBaseURL(settings.BaseURL, protocol)
	if err != nil {
		return "", err
	}
	u.Path += "/models"
	return u.String(), nil
}

func validateGeminiRepairModel(model string) error {
	model = strings.TrimPrefix(model, "models/")
	if model == "" || model == "." || model == ".." || strings.ContainsAny(model, "/\\?#:") {
		return errors.New("invalid Gemini repair model ID")
	}
	return nil
}
