package claude

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
)

func (h *Handler) withInputReplay(req promptcompat.StandardRequest, a *auth.RequestAuth, raw map[string]any) promptcompat.StandardRequest {
	return files.WithReplay(req, a, raw, func(_ context.Context, working map[string]any) (promptcompat.StandardRequest, error) {
		norm, err := normalizeClaudeRequest(h.Store, working, a.ToolMarker)
		out := norm.Standard
		return out, err
	})
}
