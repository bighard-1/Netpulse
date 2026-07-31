package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"netpulse/internal/db"
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
	trendBackfill, _ := h.repo.GetTrafficTrendBackfillOverview(ctx)
	storageOverview, _ := h.repo.GetStorageOverview(ctx)
	cleanupPreview, _ := h.repo.CleanupOperationalData(ctx, db.OperationalDataRetention{}, true)
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
		"trend_backfill":    trendBackfill,
		"storage_overview":  storageOverview,
		"cleanup_preview":   cleanupPreview,
		"cache_summary":     h.snapshotCacheStats(),
		"slow_apis":         h.recentSlowAPIs(30),
		"recent_jobs":       h.listSystemJobs(10),
		"build":             h.buildInfo(),
	})
}

type systemCleanupRequest struct {
	DryRun                bool `json:"dry_run"`
	AuditLogDays          int  `json:"audit_log_days"`
	DeviceLogDays         int  `json:"device_log_days"`
	ResolvedAlertDays     int  `json:"resolved_alert_days"`
	SystemHealthDays      int  `json:"system_health_days"`
	BackupDrillDays       int  `json:"backup_drill_days"`
	CapabilityHistoryDays int  `json:"capability_history_days"`
}

func (h *Handler) handleSystemCleanup(w http.ResponseWriter, r *http.Request) {
	var req systemCleanupRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	retention := db.OperationalDataRetention{
		AuditLogDays:          req.AuditLogDays,
		DeviceLogDays:         req.DeviceLogDays,
		ResolvedAlertDays:     req.ResolvedAlertDays,
		SystemHealthDays:      req.SystemHealthDays,
		BackupDrillDays:       req.BackupDrillDays,
		CapabilityHistoryDays: req.CapabilityHistoryDays,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := h.repo.CleanupOperationalData(ctx, retention, req.DryRun)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
