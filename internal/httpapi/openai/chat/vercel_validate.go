package chat

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"ds2api/internal/assistantturn"
	"ds2api/internal/completionruntime"
	"ds2api/internal/config"
	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/jsonvalue"
	"ds2api/internal/sse"
)

// Node forwards raw candidate output to this lease-bound endpoint so tool
// argument validation, repair and numeric precision have one implementation.
func (h *Handler) handleVercelStreamValidate(w http.ResponseWriter, r *http.Request) {
	if !config.IsVercel() {
		http.NotFound(w, r)
		return
	}
	secret := vercelInternalSecret()
	provided := strings.TrimSpace(r.Header.Get("X-Ds2-Internal-Token"))
	if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(provided)) != 1 {
		writeOpenAIError(w, 401, "unauthorized internal request")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, openAIGeneralMaxSize)
	var req map[string]any
	if err := jsonvalue.Decode(r.Body, &req); err != nil {
		writeOpenAIError(w, 400, "invalid json")
		return
	}
	lease, ok := h.lookupStreamLease(asString(req["lease_id"]))
	if !ok {
		writeOpenAIError(w, 404, "stream lease not found")
		return
	}
	std := lease.Standard
	turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: asString(req["content"]), Thinking: asString(req["thinking"]), ToolDetectionThinking: asString(req["detection_thinking"])}, assistantturn.BuildOptions{Model: std.ResponseModel, Prompt: std.PromptTokenText, ToolNames: std.ToolNames, ToolsRaw: std.ToolsRaw, ToolChoice: std.ToolChoice, ToolMarker: std.ToolMarker, ToolCallRepair: completionruntime.NewToolCallRepairInvoker(h.DS, lease.Auth, h.ChatHistory), ToolCallRepairCtx: r.Context()})
	if turn.Error != nil {
		writeOpenAIErrorWithCode(w, turn.Error.Status, turn.Error.Message, turn.Error.Code)
		return
	}
	writeJSON(w, 200, map[string]any{"tool_calls": openaifmt.FormatValidatedChatToolCalls(turn.ToolCalls, std.ToolsRaw), "content": completionruntime.ValidatedToolText(turn, std.ToolNames, std.ToolMarker, false), "usage": assistantturn.OpenAIChatUsage(turn)})
}
