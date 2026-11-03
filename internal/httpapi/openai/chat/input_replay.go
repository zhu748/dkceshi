package chat

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
)

func (h *Handler) withInputReplay(req promptcompat.StandardRequest, a *auth.RequestAuth, raw map[string]any, trace string, route promptcompat.ImageRoute) promptcompat.StandardRequest {
	route.RestoreModelName(&req)
	return files.WithReplay(req, a, raw, func(ctx context.Context, working map[string]any) (promptcompat.StandardRequest, error) {
		if err := h.preprocessInlineTextFilesForExpert(ctx, a, working); err != nil {
			return promptcompat.StandardRequest{}, err
		}
		route.StripImages(working)
		out, err := promptcompat.NormalizeOpenAIChatRequest(h.Store, working, trace, a.ToolMarker)
		route.RestoreModelName(&out)
		return out, err
	})
}
