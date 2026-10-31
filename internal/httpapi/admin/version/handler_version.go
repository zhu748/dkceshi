package version

import (
	"net/http"
	"strings"
	"time"

	"ds2api/internal/version"
)

const latestReleaseAPI = "https://api.github.com/repos/ouqiting/ds2api/releases/latest"

type latestReleasePayload struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

func (h *Handler) getVersion(w http.ResponseWriter, _ *http.Request) {
	current, source := version.Current()
	check := h.checkRelease
	if check == nil {
		check = fetchLatestRelease
	}
	cached := h.releases.snapshot(check)
	resp := map[string]any{
		"success":              true,
		"current_version":      current,
		"current_tag":          version.Tag(current),
		"source":               source,
		"checked_at":           time.Now().UTC().Format(time.RFC3339),
		"update_check_pending": cached.pending,
	}
	if !cached.checkedAt.IsZero() {
		resp["latest_checked_at"] = cached.checkedAt.UTC().Format(time.RFC3339)
	}
	if cached.err != "" {
		resp["check_error"] = cached.err
	}
	if latest := strings.TrimSpace(cached.release.TagName); latest != "" {
		latestVersion := strings.TrimPrefix(latest, "v")
		resp["latest_tag"] = latest
		resp["latest_version"] = latestVersion
		resp["release_url"] = cached.release.HTMLURL
		resp["published_at"] = cached.release.PublishedAt
		resp["has_update"] = version.Compare(current, latestVersion) < 0
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}
