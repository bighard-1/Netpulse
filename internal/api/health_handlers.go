package api

import (
	"context"
	"net/http"
	"time"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func (h *Handler) buildInfo() map[string]string {
	return map[string]string{
		"version":    Version,
		"commit":     Commit,
		"build_time": BuildTime,
	}
}

func (h *Handler) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.buildInfo())
}

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := http.StatusOK
	dbOK := true
	dbErr := ""
	if err := h.repo.Ping(ctx); err != nil {
		status = http.StatusServiceUnavailable
		dbOK = false
		dbErr = err.Error()
	}

	writeJSON(w, status, map[string]any{
		"status":     map[bool]string{true: "ok", false: "degraded"}[dbOK],
		"database":   map[string]any{"ok": dbOK, "error": dbErr},
		"uptime_sec": int64(time.Since(h.startedAt).Seconds()),
		"started_at": h.startedAt.Format(time.RFC3339),
		"build":      h.buildInfo(),
	})
}
