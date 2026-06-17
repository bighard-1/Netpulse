package api

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (h *Handler) handleSystemOps(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	devs, _ := h.repo.ListDevicesWithStatus(ctx)
	events, _ := h.repo.GetRecentEvents(ctx, 50)
	audits, _ := h.repo.ListAuditLogs(ctx, 50)
	lastMetricAt, ingestDelaySec, _ := h.repo.GetMetricsIngestStatus(ctx)
	pollSummary, _ := h.repo.GetOpsPollSummary(ctx, time.Hour)
	trafficSummary, _ := h.repo.GetTrafficSampleSummary(ctx, time.Hour)
	trafficRollups, _ := h.repo.GetTrafficRollupStatuses(ctx)
	openAlerts := 0
	for _, e := range events {
		l := strings.ToUpper(strings.TrimSpace(e.Level))
		if l == "ERROR" || l == "WARNING" || strings.Contains(strings.ToUpper(e.Message), "DOWN") {
			openAlerts++
		}
	}
	var lastAudit string
	if len(audits) > 0 {
		lastAudit = audits[0].Timestamp.Format(time.RFC3339)
	}
	var lastEvent string
	if len(events) > 0 {
		lastEvent = events[0].CreatedAt.Format(time.RFC3339)
	}
	lastMetric := ""
	if !lastMetricAt.IsZero() {
		lastMetric = lastMetricAt.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_total":      len(devs),
		"open_alert_events": openAlerts,
		"recent_events":     len(events),
		"recent_audits":     len(audits),
		"last_event_at":     lastEvent,
		"last_audit_at":     lastAudit,
		"last_metric_at":    lastMetric,
		"ingest_delay_sec":  ingestDelaySec,
		"poll_summary":      pollSummary,
		"traffic_summary":   trafficSummary,
		"traffic_rollups":   trafficRollups,
		"cache_summary":     h.snapshotCacheStats(),
		"slow_apis":         h.recentSlowAPIs(30),
		"recent_jobs":       h.listSystemJobs(10),
	})
}
