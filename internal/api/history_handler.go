package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"netpulse/internal/db"
)

func (h *Handler) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	requestStarted := time.Now()
	metricType := r.URL.Query().Get("type")
	idStr := r.URL.Query().Get("id")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	interval := strings.TrimSpace(r.URL.Query().Get("interval"))
	maxPoints := 0
	if mp := strings.TrimSpace(r.URL.Query().Get("max_points")); mp != "" {
		v, err := strconv.Atoi(mp)
		if err != nil || v <= 0 {
			writeError(w, http.StatusBadRequest, "invalid max_points")
			return
		}
		if v > 10000 {
			v = 10000
		}
		maxPoints = v
	}

	if metricType == "" || idStr == "" || startStr == "" || endStr == "" {
		writeError(w, http.StatusBadRequest, "type, id, start, end are required")
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	start, err := parseTime(startStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start, use RFC3339")
		return
	}
	end, err := parseTime(endStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end, use RFC3339")
		return
	}
	if !end.After(start) {
		writeError(w, http.StatusBadRequest, "end must be after start")
		return
	}

	switch metricType {
	case "cpu", "mem":
		items, err := h.repo.GetDeviceHistory(r.Context(), id, start, end, interval, maxPoints)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		diag := historyDiagnostic{
			RangeLabel:      historyRangeLabel(end.Sub(start)),
			SourceTable:     "metrics",
			SampledInterval: sampledIntervalLabel(interval, "原始(自动)"),
			CacheHit:        false,
			DurationMS:      time.Since(requestStarted).Milliseconds(),
			PointCount:      len(items),
		}
		setHistoryDiagnosticHeaders(w, diag)
		writeJSON(w, http.StatusOK, withHistoryDiagnostics(map[string]any{
			"type":      metricType,
			"id":        id,
			"start":     start,
			"end":       end,
			"interval":  interval,
			"maxPoints": maxPoints,
			"data":      items,
		}, diag))
	case "traffic":
		span := end.Sub(start)
		if span > db.MaxTrafficHistoryRange {
			writeError(w, http.StatusBadRequest, "流量历史最长仅支持查询近2年")
			return
		}
		cacheKey := historyCacheKey(metricType, id, start, end, interval, maxPoints)
		if cached, ok := h.getCachedHistory(cacheKey); ok {
			payload := cloneHistoryPayload(cached)
			diag := historyDiagnostic{
				RangeLabel:      stringValue(payload["range_label"], historyRangeLabel(span)),
				SourceTable:     stringValue(payload["source_table"], "metrics"),
				SampledInterval: stringValue(payload["sampled_interval"], sampledIntervalLabel(interval, "原始(自动)")),
				CacheHit:        true,
				DurationMS:      time.Since(requestStarted).Milliseconds(),
				PointCount:      intValue(payload["point_count"]),
			}
			setHistoryDiagnosticHeaders(w, diag)
			writeJSON(w, http.StatusOK, withHistoryDiagnostics(payload, diag))
			return
		}
		queryCtx := r.Context()
		cancel := func() {}
		if span > 24*time.Hour {
			// Long-range traffic reads are served from compact rollups, but a cold
			// cache or an incomplete rollup can still require a fallback query.
			// Ten seconds is too aggressive for a busy production TimescaleDB and
			// was the direct cause of the 7/30-day chart failures.
			timeout := 30 * time.Second
			if span > 31*24*time.Hour {
				timeout = 45 * time.Second
			}
			queryCtx, cancel = context.WithTimeout(r.Context(), timeout)
		}
		defer cancel()
		items, err := h.repo.GetInterfaceHistory(queryCtx, id, start, end, interval, maxPoints)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sourceTable := "metrics"
		if span > 31*24*time.Hour {
			sourceTable = "traffic_1h"
		} else if span > 24*time.Hour {
			sourceTable = "traffic_5m"
		}
		sampledInterval := sampledIntervalForSource(interval, sourceTable)
		diag := historyDiagnostic{
			RangeLabel:      historyRangeLabel(span),
			SourceTable:     sourceTable,
			SampledInterval: sampledInterval,
			CacheHit:        false,
			DurationMS:      time.Since(requestStarted).Milliseconds(),
			PointCount:      len(items),
		}
		payload := map[string]any{
			"type":                metricType,
			"id":                  id,
			"start":               start,
			"end":                 end,
			"interval":            interval,
			"sampled_interval":    sampledInterval,
			"source_table":        sourceTable,
			"aggregation_pending": sourceTable != "metrics" && len(items) == 0,
			"maxPoints":           maxPoints,
			"data":                items,
		}
		withHistoryDiagnostics(payload, diag)
		cacheTTL := historyCacheTTLFor(start, end)
		if sourceTable != "metrics" && len(items) == 0 {
			cacheTTL = historyCacheTTL
		}
		h.setCachedHistory(cacheKey, payload, cacheTTL)
		setHistoryDiagnosticHeaders(w, diag)
		writeJSON(w, http.StatusOK, payload)
	case "storage":
		items, err := h.repo.GetDeviceStorageHistory(r.Context(), id, start, end, interval, maxPoints)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		diag := historyDiagnostic{
			RangeLabel:      historyRangeLabel(end.Sub(start)),
			SourceTable:     "metrics",
			SampledInterval: sampledIntervalLabel(interval, "原始(自动)"),
			CacheHit:        false,
			DurationMS:      time.Since(requestStarted).Milliseconds(),
			PointCount:      len(items),
		}
		setHistoryDiagnosticHeaders(w, diag)
		writeJSON(w, http.StatusOK, withHistoryDiagnostics(map[string]any{
			"type":      metricType,
			"id":        id,
			"start":     start,
			"end":       end,
			"interval":  interval,
			"maxPoints": maxPoints,
			"data":      items,
		}, diag))
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: cpu, mem, traffic, storage")
	}
}

type historyDiagnostic struct {
	RangeLabel      string
	SourceTable     string
	SampledInterval string
	CacheHit        bool
	DurationMS      int64
	PointCount      int
}

func sampledIntervalForSource(interval, sourceTable string) string {
	switch sourceTable {
	case "traffic_1h":
		return sampledIntervalLabel(interval, "1h(预聚合)")
	case "traffic_5m":
		return sampledIntervalLabel(interval, "5m(预聚合)")
	default:
		return sampledIntervalLabel(interval, "原始(自动)")
	}
}

func sampledIntervalLabel(interval, fallback string) string {
	if v := strings.TrimSpace(interval); v != "" {
		return v
	}
	return fallback
}

func historyRangeLabel(span time.Duration) string {
	switch {
	case span <= 24*time.Hour:
		return "today_or_custom_short"
	case span <= 8*24*time.Hour:
		return "7d"
	case span <= 32*24*time.Hour:
		return "30d"
	case span <= 370*24*time.Hour:
		return "1y"
	default:
		return "long_range"
	}
}

func cloneHistoryPayload(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+8)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func withHistoryDiagnostics(payload map[string]any, diag historyDiagnostic) map[string]any {
	payload["range_label"] = diag.RangeLabel
	payload["query_duration_ms"] = diag.DurationMS
	payload["point_count"] = diag.PointCount
	payload["cache_hit"] = diag.CacheHit
	if diag.SourceTable != "" {
		payload["source_table"] = diag.SourceTable
	}
	if diag.SampledInterval != "" {
		payload["sampled_interval"] = diag.SampledInterval
	}
	return payload
}

func setHistoryDiagnosticHeaders(w http.ResponseWriter, diag historyDiagnostic) {
	w.Header().Set("X-NetPulse-History-Range", diag.RangeLabel)
	w.Header().Set("X-NetPulse-History-Source", diag.SourceTable)
	w.Header().Set("X-NetPulse-History-Interval", diag.SampledInterval)
	w.Header().Set("X-NetPulse-History-Cache-Hit", strconv.FormatBool(diag.CacheHit))
	w.Header().Set("X-NetPulse-Query-Duration-Ms", strconv.FormatInt(diag.DurationMS, 10))
	w.Header().Set("X-NetPulse-History-Point-Count", strconv.Itoa(diag.PointCount))
}

func stringValue(v any, fallback string) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}

func intValue(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

const historyCacheTTL = 20 * time.Second
const historyCacheMaxEntries = 128

func historyCacheKey(metricType string, id int64, start, end time.Time, interval string, maxPoints int) string {
	// Round end time slightly so repeated refreshes within one short UI cycle can
	// reuse the same result without changing the public API contract.
	bucketSeconds := int64(15)
	span := end.Sub(start)
	switch {
	case span > 7*24*time.Hour:
		bucketSeconds = 300
	case span > 24*time.Hour:
		bucketSeconds = 120
	}
	endBucket := end.Unix() / bucketSeconds
	return fmt.Sprintf("%s:%d:%d:%d:%s:%d", metricType, id, start.Unix(), endBucket, strings.TrimSpace(interval), maxPoints)
}

func historyCacheTTLFor(start, end time.Time) time.Duration {
	span := end.Sub(start)
	switch {
	case span > 7*24*time.Hour:
		return 10 * time.Minute
	case span > 24*time.Hour:
		return 2 * time.Minute
	default:
		return historyCacheTTL
	}
}

func (h *Handler) getCachedHistory(key string) (map[string]any, bool) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if h.history == nil {
		h.history = map[string]historyCacheEntry{}
	}
	entry, ok := h.history[key]
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			delete(h.history, key)
		}
		h.cacheStats.HistoryMiss++
		return nil, false
	}
	h.cacheStats.HistoryHits++
	return entry.payload, true
}

func (h *Handler) setCachedHistory(key string, payload map[string]any, ttl time.Duration) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if h.history == nil {
		h.history = map[string]historyCacheEntry{}
	}
	now := time.Now()
	for k, entry := range h.history {
		if now.After(entry.expiresAt) {
			delete(h.history, k)
		}
	}
	if len(h.history) >= historyCacheMaxEntries {
		var oldestKey string
		var oldest time.Time
		for k, entry := range h.history {
			if oldestKey == "" || entry.expiresAt.Before(oldest) {
				oldestKey = k
				oldest = entry.expiresAt
			}
		}
		if oldestKey != "" {
			delete(h.history, oldestKey)
		}
	}
	if ttl <= 0 {
		ttl = historyCacheTTL
	}
	h.history[key] = historyCacheEntry{payload: payload, expiresAt: now.Add(ttl)}
	h.cacheStats.HistoryEntries = len(h.history)
}

func (h *Handler) snapshotCacheStats() cacheStats {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if h.history == nil {
		h.history = map[string]historyCacheEntry{}
	}
	now := time.Now()
	for k, entry := range h.history {
		if now.After(entry.expiresAt) {
			delete(h.history, k)
		}
	}
	out := h.cacheStats
	out.HistoryEntries = len(h.history)
	return out
}
