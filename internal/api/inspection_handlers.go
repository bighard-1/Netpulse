package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (h *Handler) handleInspectionBundle(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	devices, _ := h.repo.ListDevicesWithStatus(r.Context())
	alerts, _ := h.repo.ListAlertEvents(r.Context(), 100, "")
	events, _ := h.repo.GetRecentEvents(r.Context(), 100)
	audits, _ := h.repo.ListAuditLogs(r.Context(), 100)
	settings, _ := h.repo.GetRuntimeSettings(r.Context())

	payload := map[string]any{
		"generated_at": now.Format(time.RFC3339),
		"summary": map[string]any{
			"devices_total": len(devices),
			"alerts_total":  len(alerts),
			"events_total":  len(events),
			"audits_total":  len(audits),
		},
		"runtime_settings": settings,
		"devices":          devices,
		"alerts":           alerts,
		"events":           events,
		"audits":           audits,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="netpulse_inspection_bundle_%s.json"`, now.Format("20060102_150405")))
	_, _ = w.Write(raw)
}
