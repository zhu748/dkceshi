package chat

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tokenusage"
	"strings"
)

func (h *Handler) newLeaseUsageResolver(a *auth.RequestAuth, sessionID string) tokenusage.Resolver {
	factory, ok := h.DS.(interface {
		NewUsageResolver(*auth.RequestAuth, string) tokenusage.Resolver
	})
	if !ok {
		return nil
	}
	return factory.NewUsageResolver(a, sessionID)
}

func (h *Handler) leaseUpstreamUsage(ctx context.Context, lease streamLease, req map[string]any) tokenusage.Snapshot {
	observed := tokenusage.FromMap(req["upstream_usage"])
	if lease.UsageResolver == nil || asString(req["error"]) != "" || len(observed.Messages) == 0 || asString(req["finish_reason"]) == "stopped" {
		return observed
	}
	id := observed.Messages[len(observed.Messages)-1].MessageID
	if usage, err := lease.UsageResolver.ResolveUpstreamUsage(ctx, id); err == nil {
		observed.SetRequest(usage)
	}
	return observed
}

func (h *Handler) finalizeLeaseUsage(ctx context.Context, lease streamLease, req map[string]any) map[string]any {
	req["upstream_usage"] = h.leaseUpstreamUsage(ctx, lease, req)
	return finalizeVercelUsage(lease.Standard, req)
}

func (h *Handler) updateStreamLeaseState(leaseID string, stdReq promptcompat.StandardRequest, sessionID string) {
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return
	}
	h.leaseMu.Lock()
	defer h.leaseMu.Unlock()
	lease, ok := h.streamLeases[leaseID]
	if !ok {
		return
	}
	lease.Standard = stdReq
	if lease.SessionID != sessionID {
		lease.UsageResolver = h.newLeaseUsageResolver(lease.Auth, sessionID)
	}
	lease.SessionID = sessionID
	h.streamLeases[leaseID] = lease
}
