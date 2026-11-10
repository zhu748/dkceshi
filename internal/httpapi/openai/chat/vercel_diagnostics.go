package chat

import (
	"context"
	"ds2api/internal/util"
	"net/http"

	"ds2api/internal/assistantturn"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func (h *Handler) attachLeaseDiagnostics(id string, std promptcompat.StandardRequest) {
	h.leaseMu.Lock()
	defer h.leaseMu.Unlock()
	lease, ok := h.streamLeases[id]
	if !ok {
		return
	}
	lease.RepairSettings = h.ToolDiagnostics.RepairSettings()
	lease.Diagnostics = tooldiagnostics.NewRecorder(h.ToolDiagnostics, "chat.completions", true)
	ctx := tooldiagnostics.WithRecorder(context.Background(), lease.Diagnostics)
	if lease.Auth != nil {
		tooldiagnostics.Configure(ctx, std.ResponseModel, lease.Auth.CallerID, lease.Auth.AccountID, true, std.ToolsRaw, std.ToolChoice, std.ResolvedModel)
	}
	h.streamLeases[id] = lease
}

func leaseDiagnosticContext(ctx context.Context, lease streamLease, req map[string]any) context.Context {
	ctx = tooldiagnostics.WithRecorder(ctx, lease.Diagnostics)
	ctx = tooldiagnostics.WithRepairSettings(ctx, lease.RepairSettings)
	account, caller := "", ""
	if lease.Auth != nil {
		account, caller = lease.Auth.AccountID, lease.Auth.CallerID
	}
	number := 1
	if n, ok := req["diagnostic_attempt"]; ok {
		number = util.IntFrom(n)
	}
	tooldiagnostics.SelectAttempt(ctx, number, asString(req["diagnostic_retry_reason"]), account)
	ctx = tooldiagnostics.WithAttempt(ctx, number)
	tooldiagnostics.Configure(ctx, lease.Standard.ResponseModel, caller, account, true, lease.Standard.ToolsRaw, lease.Standard.ToolChoice, lease.Standard.ResolvedModel)
	return ctx
}

func finishLeaseDiagnostics(r *http.Request, lease streamLease, req map[string]any) {
	if lease.Diagnostics == nil {
		return
	}
	ctx := leaseDiagnosticContext(r.Context(), lease, req)
	message := asString(req["error"])
	stopped := asString(req["finish_reason"]) == "stopped"
	if req["tools_written"] == true {
		tooldiagnostics.ToolsWritten(ctx)
	}
	if req["response_written"] == true && message == "" {
		tooldiagnostics.ResponseWritten(ctx)
		stopped = false
	} else if req["delivery_version"] != nil && message == "" {
		stopped = true
	}
	status := 200
	if message != "" || stopped {
		code := asString(req["error_code"])
		if stopped && message == "" {
			code, message = "request_cancelled", "Request cancelled before tool delivery completed."
		}
		if code == "" {
			code = "stream_error"
		}
		std := lease.Standard
		assistantturn.ObserveFailedOutput(asString(req["content"]), asString(req["thinking"]), asString(req["detection_thinking"]), assistantturn.BuildOptions{Model: std.ResponseModel, Prompt: std.PromptTokenText, ToolNames: std.ToolNames, ToolsRaw: std.ToolsRaw, ToolChoice: std.ToolChoice, ToolMarker: std.ToolMarker, DiagnosticContext: ctx}, &assistantturn.OutputError{Status: 502, Code: code, Message: message})
		tooldiagnostics.Failure(ctx, code, message)
		if reason := lease.Auth.AccountSwitchFailureReason(); reason != "" && reason != "unmanaged_account" {
			tooldiagnostics.RetryDecision(ctx, reason, message)
		}
		status = 502
	}
	lease.Diagnostics.Finish(status, nil, stopped)
}

func (h *Handler) leaseRepairTimeoutMS(id string) int {
	h.leaseMu.Lock()
	defer h.leaseMu.Unlock()
	if lease, ok := h.streamLeases[id]; ok && lease.RepairSettings.TimeoutSeconds > 0 {
		return lease.RepairSettings.TimeoutSeconds * 1000
	}
	return 10000
}
