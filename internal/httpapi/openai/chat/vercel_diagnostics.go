package chat

import (
	"context"
	"ds2api/internal/util"
	"net/http"

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
		tooldiagnostics.Configure(ctx, std.ResponseModel, lease.Auth.CallerID, lease.Auth.AccountID, true, std.ToolsRaw, std.ToolChoice)
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
	tooldiagnostics.Configure(ctx, lease.Standard.ResponseModel, caller, account, true, lease.Standard.ToolsRaw, lease.Standard.ToolChoice)
	return ctx
}

func finishLeaseDiagnostics(r *http.Request, lease streamLease, req map[string]any) {
	if lease.Diagnostics == nil {
		return
	}
	ctx := leaseDiagnosticContext(r.Context(), lease, req)
	message := asString(req["error"])
	status := 200
	if message != "" {
		code := asString(req["error_code"])
		if code == "" {
			code = "stream_error"
		}
		tooldiagnostics.TransportFailure(ctx, tooldiagnostics.Observation{Relevant: len(lease.Standard.ToolNames) > 0, Model: lease.Standard.ResponseModel, Raw: asString(req["content"]), ErrorCode: code, Error: message})
		status = 502
	}
	lease.Diagnostics.Finish(status, nil, asString(req["finish_reason"]) == "stopped")
}

func (h *Handler) leaseRepairTimeoutMS(id string) int {
	h.leaseMu.Lock()
	defer h.leaseMu.Unlock()
	if lease, ok := h.streamLeases[id]; ok && lease.RepairSettings.TimeoutSeconds > 0 {
		return lease.RepairSettings.TimeoutSeconds * 1000
	}
	return 10000
}
