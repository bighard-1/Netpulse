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
		writeJSON(w, http.StatusOK, map[string]any{
			"type":      metricType,
			"id":        id,
			"start":     start,
			"end":       end,
			"interval":  interval,
			"maxPoints": maxPoints,
			"data":      items,
		})
	case "traffic":
		span := end.Sub(start)
		if span > db.MaxTrafficHistoryRange {
			writeError(w, http.StatusBadRequest, "流量历史最长仅支持查询近2年")
			return
		}
		cacheKey := historyCacheKey(metricType, id, start, end, interval, maxPoints)
		if cached, ok := h.getCachedHistory(cacheKey); ok {
			writeJSON(w, http.StatusOK, cached)
			return
		}
		queryCtx := r.Context()
		cancel := func() {}
		if span > 24*time.Hour {
			queryCtx, cancel = context.WithTimeout(r.Context(), 10*time.Second)
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
		sampledInterval := strings.TrimSpace(interval)
		if sampledInterval == "" {
			if sourceTable == "traffic_1h" {
				sampledInterval = "1h(预聚合)"
			} else if sourceTable == "traffic_5m" {
				sampledInterval = "5m(预聚合)"
			} else {
				sampledInterval = "原始(自动)"
			}
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
		cacheTTL := historyCacheTTLFor(start, end)
		if sourceTable != "metrics" && len(items) == 0 {
			cacheTTL = historyCacheTTL
		}
		h.setCachedHistory(cacheKey, payload, cacheTTL)
		writeJSON(w, http.StatusOK, payload)
	case "storage":
		items, err := h.repo.GetDeviceStorageHistory(r.Context(), id, start, end, interval, maxPoints)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"type":      metricType,
			"id":        id,
			"start":     start,
			"end":       end,
			"interval":  interval,
			"maxPoints": maxPoints,
			"data":      items,
		})
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: cpu, mem, traffic, storage")
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
