package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"netpulse/internal/db"
)

func (h *Handler) handleRecentEvents(w http.ResponseWriter, r *http.Request) {
	limit := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	filter := db.EventFilter{
		Limit:      limit,
		DeviceName: strings.TrimSpace(r.URL.Query().Get("device_name")),
		EventType:  strings.TrimSpace(r.URL.Query().Get("event_type")),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("device_id")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			filter.DeviceID = id
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("start")); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			filter.Start = &t
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("end")); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			filter.End = &t
		}
	}
	items, err := h.repo.QueryRecentEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  items,
		"limit": limit,
	})
}
