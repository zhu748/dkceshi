package tooldiagnostics

import (
	"ds2api/internal/completionruntime"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	adminshared "ds2api/internal/httpapi/admin/shared"
	"ds2api/internal/tooldiagnostics"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ Store *tooldiagnostics.Store }

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/tool-call-records", h.list)
	r.Get("/tool-call-records/{id}", h.detail)
	r.Delete("/tool-call-records/{id}", h.delete)
	r.Delete("/tool-call-records", h.clear)
	r.Put("/tool-call-records/settings", h.settings)
	r.Put("/tool-call-records/repair-settings", h.repairSettings)
	r.Post("/tool-call-records/repair-models", h.repairModels)
	r.Post("/tool-call-records/repair-test", h.repairTest)
}

func (h *Handler) available(w http.ResponseWriter) bool {
	if h.Store != nil {
		return true
	}
	adminshared.WriteJSON(w, 503, map[string]any{"detail": "tool diagnostics unavailable"})
	return false
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("page_size"))
	result := h.Store.List(tooldiagnostics.Query{Page: page, PageSize: size, Status: q.Get("status"), Tool: q.Get("tool"), Search: q.Get("search"), Pinned: q.Get("pinned") == "true"})
	w.Header().Set("Cache-Control", "no-store")
	adminshared.WriteJSON(w, 200, result)
}
func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	record, err := h.Store.Get(chi.URLParam(r, "id"))
	if err != nil {
		h.respondError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	adminshared.WriteJSON(w, 200, record)
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	h.result(w, h.Store.Delete(chi.URLParam(r, "id")))
}
func (h *Handler) clear(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	h.result(w, h.Store.Clear(r.URL.Query().Get("scope")))
}
func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
		Limit   int  `json:"limit"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		adminshared.WriteJSON(w, 400, map[string]any{"detail": "invalid settings"})
		return
	}
	if err := h.Store.SetSettings(tooldiagnostics.Settings{Enabled: body.Enabled, Limit: body.Limit}); err != nil {
		h.respondError(w, err)
		return
	}
	h.savedSettings(w)
}

func (h *Handler) repairSettings(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	var body tooldiagnostics.RepairSettings
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		adminshared.WriteJSON(w, 400, map[string]any{"detail": "invalid repair settings"})
		return
	}
	if err := h.Store.SetRepairSettings(body); err != nil {
		h.respondError(w, err)
		return
	}
	h.savedSettings(w)
}
func (h *Handler) result(w http.ResponseWriter, err error) {
	if err != nil {
		h.respondError(w, err)
		return
	}
	adminshared.WriteJSON(w, 200, map[string]bool{"success": true})
}
func (h *Handler) respondError(w http.ResponseWriter, err error) {
	status := 400
	if errors.Is(err, tooldiagnostics.ErrNotFound) {
		status = 404
	}
	adminshared.WriteJSON(w, status, map[string]any{"detail": err.Error()})
}

func (h *Handler) repairModels(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	var draft tooldiagnostics.RepairSettings
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
		adminshared.WriteJSON(w, 400, map[string]any{"detail": "invalid repair settings"})
		return
	}
	settings, err := h.Store.PreviewRepairSettings(draft)
	if err != nil {
		h.respondError(w, err)
		return
	}
	models, err := completionruntime.ListRepairAPIModels(r.Context(), settings)
	if err != nil {
		adminshared.WriteJSON(w, 502, map[string]any{"detail": err.Error()})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	adminshared.WriteJSON(w, 200, models)
}

func (h *Handler) savedSettings(w http.ResponseWriter) {
	settings, revision := h.Store.SettingsSnapshot()
	w.Header().Set("Cache-Control", "no-store")
	adminshared.WriteJSON(w, 200, map[string]any{"success": true, "settings": settings, "revision": revision})
}
