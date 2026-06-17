package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"netpulse/internal/db"
)

func (h *Handler) handleReportSummary(w http.ResponseWriter, r *http.Request) {
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			end = t
		}
	}
	devices, err := h.repo.ListDevicesWithStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var b strings.Builder
	b.WriteString("device_id,ip,status,cpu_points,mem_points\n")
	for _, d := range devices {
		cpu, _ := h.repo.GetDeviceHistory(r.Context(), d.ID, start, end, "1m", 300)
		mem, _ := h.repo.GetDeviceHistory(r.Context(), d.ID, start, end, "1m", 300)
		b.WriteString(fmt.Sprintf("%d,%s,%s,%d,%d\n", d.ID, d.IP, d.Status, len(cpu), len(mem)))
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="netpulse_report.csv"`)
	_, _ = io.WriteString(w, b.String())
}
func (h *Handler) handleSystemHealthTrend(w http.ResponseWriter, r *http.Request) {
	limit := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := h.repo.GetSystemHealthTrend(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(items) == 0 {
		devs, derr := h.repo.ListDevicesWithStatus(r.Context())
		if derr == nil && len(devs) > 0 {
			total := len(devs)
			online := 0
			for _, d := range devs {
				if strings.EqualFold(d.Status, "online") {
					online++
				}
			}
			availability := (float64(online) / float64(total)) * 100
			items = append(items, db.SystemHealthPoint{
				Timestamp:    time.Now(),
				Score:        availability,
				ActiveAlerts: 0,
				Availability: availability,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  items,
		"limit": limit,
	})
}
