package auth

import (
	"ds2api/internal/httpapi/clientaddr"
	"ds2api/internal/httpapi/requestbody"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	authn "ds2api/internal/auth"
)

func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		signed := false
		if strings.HasPrefix(strings.ToLower(header), "bearer ") {
			_, err := authn.VerifyJWTWithStore(strings.TrimSpace(header[7:]), h.Store)
			signed = err == nil
		}
		var authErr error
		if !signed {
			if header != "" && !h.allowPasswordAttempt(w, r) {
				return
			}
			authErr = authn.VerifyAdminRequestWithStore(r, h.Store)
		}
		if err := authErr; err != nil {
			if h.WebUIFallback != nil && shouldServeSPAForBrowserNavigation(r) {
				if h.WebUIFallback(w, r) {
					return
				}
			}
			writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": err.Error()})
			return
		}
		if !signed && header != "" {
			h.logins.reset(clientaddr.IP(r))
		}
		next.ServeHTTP(w, r)
	})
}

// shouldServeSPAForBrowserNavigation reports whether the request looks like a
// browser navigation (full page load) rather than an XHR/fetch API call.
// Browser navigation requests carry Accept: text/html; fetch() defaults to
// Accept: */*. This lets us serve index.html when a user hard-refreshes a SPA
// route that collides with a protected admin GET endpoint (e.g. /admin/settings),
// while still returning 401 JSON for unauthenticated API calls.
func shouldServeSPAForBrowserNavigation(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.Method != http.MethodGet {
		return false
	}
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		return false
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "text/html")
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.allowPasswordAttempt(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		status := http.StatusBadRequest
		if requestbody.IsTooLarge(err) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]any{"detail": "invalid login request"})
		return
	}
	adminKey, _ := req["admin_key"].(string)
	expireHours := intFrom(req["expire_hours"])
	if !authn.VerifyAdminCredential(adminKey, h.Store) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": "Invalid admin key"})
		return
	}
	// Bound the caller-requested lifetime. Without a cap a single login could
	// mint a decades-long admin JWT; 30 days is far beyond any "remember me"
	// need while keeping a leaked token revocable on a sane schedule.
	const maxExpireHours = 24 * 30
	if expireHours > maxExpireHours {
		expireHours = maxExpireHours
	}
	h.logins.reset(clientaddr.IP(r))
	token, err := authn.CreateJWTWithStore(expireHours, h.Store)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	if expireHours <= 0 {
		expireHours = h.Store.AdminJWTExpireHours()
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": token, "expires_in": expireHours * 3600})
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": "No credentials provided"})
		return
	}
	token := strings.TrimSpace(header[7:])
	payload, err := authn.VerifyJWTWithStore(token, h.Store)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": err.Error()})
		return
	}
	exp, _ := payload["exp"].(float64)
	remaining := int64(exp) - time.Now().Unix()
	if remaining < 0 {
		remaining = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "expires_at": int64(exp), "remaining_seconds": remaining})
}

func (h *Handler) getVercelConfig(w http.ResponseWriter, _ *http.Request) {
	saved := h.Store.Snapshot().Vercel
	token, tokenSource := firstConfiguredValue(
		[2]string{"env", os.Getenv("VERCEL_TOKEN")},
		[2]string{"config", saved.Token},
	)
	projectID, _ := firstConfiguredValue(
		[2]string{"env", os.Getenv("VERCEL_PROJECT_ID")},
		[2]string{"config", saved.ProjectID},
	)
	teamID, _ := firstConfiguredValue(
		[2]string{"env", os.Getenv("VERCEL_TEAM_ID")},
		[2]string{"config", saved.TeamID},
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"has_token":     token != "",
		"token_preview": maskSecretPreview(token),
		"token_source":  nilIfEmpty(tokenSource),
		"project_id":    projectID,
		"team_id":       nilIfEmpty(teamID),
	})
}

func firstConfiguredValue(values ...[2]string) (string, string) {
	for _, pair := range values {
		value := strings.TrimSpace(pair[1])
		if value != "" {
			return value, strings.TrimSpace(pair[0])
		}
	}
	return "", ""
}

func (h *Handler) allowPasswordAttempt(w http.ResponseWriter, r *http.Request) bool {
	if ok, retry := h.logins.allow(clientaddr.IP(r), time.Now()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"detail": "Too many login attempts; retry later"})
		return false
	}
	return true
}
