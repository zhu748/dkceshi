package tooldiagnostics

import "strings"

// Remove only the selected protocol's complete endpoint suffix, preserving
// gateway components named models or messages and unversioned endpoints.
func repairEndpointBasePath(path, protocol string) (string, bool) {
	suffix := "/chat/completions"
	switch protocol {
	case "claude":
		suffix = "/messages"
	case "gemini":
		if index := strings.LastIndex(path, "/models/"); index >= 0 {
			model := strings.TrimSuffix(path[index+8:], ":generateContent")
			if strings.HasSuffix(path, ":generateContent") && !strings.Contains(model, "/") && validateGeminiRepairModel(model) == nil {
				return path[:index], true
			}
		}
		suffix = "/models"
	}
	return strings.TrimSuffix(path, suffix), strings.HasSuffix(path, suffix)
}
