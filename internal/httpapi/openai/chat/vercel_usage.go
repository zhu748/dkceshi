package chat

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"ds2api/internal/assistantturn"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tokenusage"
	"ds2api/internal/util"
)

// The lease owns the original context and file metadata. The bridge sends only
// the actual follow-up prompts, so history and attachments are counted once.
func vercelUsagePrompt(std promptcompat.StandardRequest, req map[string]any) string {
	prompt := std.PromptTokenText
	if prompt == "" {
		prompt = std.FinalPrompt
	}
	if retries, ok := req["retry_prompts"].([]any); ok {
		var out strings.Builder
		out.WriteString(prompt)
		for _, raw := range retries {
			if retry, ok := raw.(string); ok {
				out.WriteByte('\n')
				out.WriteString(retry)
			}
		}
		return out.String()
	}
	return prompt
}

func vercelPromptTokens(std promptcompat.StandardRequest) int {
	return assistantturn.BuildUsage(std.ResponseModel, vercelUsagePrompt(std, nil), "", "", std.RefFileTokens).InputTokens
}

func finalizeVercelUsage(std promptcompat.StandardRequest, req map[string]any) map[string]any {
	if util.IntFrom(req["usage_accounting_version"]) != 1 || asString(req["error"]) != "" {
		return nil
	}
	if req["usage"] == nil && asString(req["thinking"]) == "" && asString(req["content"]) == "" {
		return nil // A lease returned before generation consumes no model tokens.
	}
	usage := assistantturn.BuildUsageFromUpstream(std.ResponseModel, vercelUsagePrompt(std, req), asString(req["thinking"]), asString(req["content"]), std.RefFileTokens, tokenusage.FromMap(req["upstream_usage"]), asString(req["finish_reason"]) != "stopped" && asString(req["finish_reason"]) != "upstream_interrupted")
	result := assistantturn.OpenAIChatUsage(assistantturn.Turn{Usage: usage})
	req["usage"] = result
	return result
}

// Count usage before the terminal frame without releasing the live lease or
// declaring its response delivered. Final release carries the write evidence.
func (h *Handler) previewStreamUsage(w http.ResponseWriter, r *http.Request, id string, req map[string]any) {
	lease, ok := h.lookupStreamLease(id)
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "stream lease not found")
		return
	}
	writeVercelUsageReply(w, h.finalizeLeaseUsage(r.Context(), lease, req))
}

// Complete the response before potentially slow remote cleanup. Content-Length
// lets the bridge consume the full JSON while this handler still owns its slot.
func writeVercelUsageReply(w http.ResponseWriter, usage map[string]any) {
	body, err := json.Marshal(map[string]any{"success": true, "usage": usage})
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "usage encoding failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if _, err := w.Write(body); err != nil {
		config.Logger.Warn("[vercel_release] usage delivery failed", "error", err)
	}
	if err := http.NewResponseController(w).Flush(); err != nil && err != http.ErrNotSupported {
		config.Logger.Warn("[vercel_release] usage flush failed", "error", err)
	}
}
