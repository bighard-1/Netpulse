package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"netpulse/internal/db"
	"netpulse/internal/snmp"
)

type Handler struct {
	repo        *db.Repository
	collector   *snmp.Collector
	system      *SystemService
	jwtSecret   string
	mu          sync.Mutex
	fails       map[string]int
	lockedUntil map[string]time.Time
	rl          map[string][]time.Time
	jobsMu      sync.Mutex
	jobs        map[string]*SystemJob
	cacheMu     sync.Mutex
	topology    topologyCacheEntry
	history     map[string]historyCacheEntry
	cacheStats  cacheStats
	slowMu      sync.Mutex
	slowAPIs    []slowAPIRecord
}

type topologyCacheEntry struct {
	graph     *db.TopologyGraph
	expiresAt time.Time
}

type historyCacheEntry struct {
	payload   map[string]any
	expiresAt time.Time
}

type cacheStats struct {
	TopologyHits   int64 `json:"topology_hits"`
	TopologyMiss   int64 `json:"topology_miss"`
	HistoryHits    int64 `json:"history_hits"`
	HistoryMiss    int64 `json:"history_miss"`
	HistoryEntries int   `json:"history_entries"`
}

type slowAPIRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	StatusCode int       `json:"status_code"`
	DurationMS int64     `json:"duration_ms"`
	IP         string    `json:"ip"`
}

func NewHandler(repo *db.Repository, collector *snmp.Collector, system *SystemService, jwtSecret string) *Handler {
	return &Handler{
		repo: repo, collector: collector, system: system, jwtSecret: jwtSecret,
		fails: map[string]int{}, lockedUntil: map[string]time.Time{}, rl: map[string][]time.Time{},
		jobs: map[string]*SystemJob{}, history: map[string]historyCacheEntry{},
	}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(h.serverTimeZoneHeaderMiddleware())

	r.Post("/api/login", h.rateLimit("login", 20, time.Minute, h.handleLogin("web")))
	r.Post("/api/auth/login", h.rateLimit("login", 20, time.Minute, h.handleLogin("web")))
	r.Post("/api/auth/mobile/login", h.rateLimit("login", 20, time.Minute, h.handleLogin("mobile")))

	r.Group(func(pr chi.Router) {
		pr.Use(h.authMiddleware)
		pr.Use(h.slowRequestMiddleware(1200 * time.Millisecond))
		pr.With(h.requirePermission("device.read")).Get("/api/devices", h.handleListDevices)
		pr.With(h.requirePermission("device.read")).Get("/api/search", h.handleGlobalSearch)
		pr.With(h.requirePermission("device.read")).Get("/api/topology", h.handleGetTopology)
		pr.With(h.adminOnly, h.auditMiddleware("CREATE_TOPOLOGY_NODE")).Post("/api/topology/nodes", h.handleCreateTopologyNode)
		pr.With(h.adminOnly, h.auditMiddleware("UPDATE_TOPOLOGY_NODE")).Put("/api/topology/nodes/{id}", h.handleUpdateTopologyNode)
		pr.With(h.adminOnly, h.auditMiddleware("DELETE_TOPOLOGY_NODE")).Delete("/api/topology/nodes/{id}", h.handleDeleteTopologyNode)
		pr.With(h.adminOnly, h.auditMiddleware("CREATE_TOPOLOGY_EDGE")).Post("/api/topology/edges", h.handleCreateTopologyEdge)
		pr.With(h.adminOnly, h.auditMiddleware("UPDATE_TOPOLOGY_EDGE")).Put("/api/topology/edges/{id}", h.handleUpdateTopologyEdge)
		pr.With(h.adminOnly, h.auditMiddleware("DELETE_TOPOLOGY_EDGE")).Delete("/api/topology/edges/{id}", h.handleDeleteTopologyEdge)
		pr.Get("/api/devices/{id}", h.handleGetDevice)
		pr.Get("/api/devices/{id}/capabilities", h.handleGetDeviceCapabilities)
		pr.With(h.requirePermission("device.read")).Get("/api/devices/{id}/diagnose", h.handleDiagnoseDevice)
		pr.With(h.requirePermission("device.read")).Get("/api/devices/{id}/diagnose/traffic-bias", h.handleDiagnoseTrafficBias)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("PRECHECK_DEVICE")).Post("/api/devices/precheck", h.handlePrecheckDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("ADD_DEVICE")).Post("/api/devices", h.handleAddDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_DEVICE")).Put("/api/devices/{id}", h.handleUpdateDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("IMPORT_DEVICES")).Post("/api/devices/import", h.handleImportDevices)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("DELETE_DEVICE")).Delete("/api/devices/{id}", h.handleDeleteDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_DEVICE_REMARK")).Put("/api/devices/{id}/remark", h.handleUpdateDeviceRemark)
		pr.With(h.requirePermission("device.read")).Get("/api/interfaces/{id}", h.handleGetInterface)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_INTERFACE_REMARK")).Put("/api/interfaces/{id}/remark", h.handleUpdateInterfaceRemark)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_INTERFACE_PROFILE")).Put("/api/interfaces/{id}", h.handleUpdateInterfaceProfile)
		pr.With(h.requirePermission("metrics.read")).Get("/api/metrics/history", h.handleMetricsHistory)
		pr.With(h.requirePermission("logs.read")).Get("/api/devices/{id}/logs", h.handleDeviceLogs)
		pr.Get("/api/events/recent", h.handleRecentEvents)
		pr.With(h.requirePermission("logs.read")).Get("/api/alerts/events", h.handleListAlertEvents)
		pr.With(h.adminOnly, h.auditMiddleware("UPDATE_ALERT_EVENT")).Put("/api/alerts/events/{id}", h.handleUpdateAlertEventWorkflow)
		pr.With(h.adminOnly).Get("/api/diagnostics/asset-load", h.handleAssetLoadDiagnostics)
		pr.Get("/api/system/health", h.handleSystemHealthTrend)
		pr.With(h.adminOnly).Get("/api/system/ops", h.handleSystemOps)
		pr.With(h.adminOnly).Get("/api/system/inspection-bundle", h.handleInspectionBundle)
		pr.With(h.adminOnly).Get("/api/system/backup", h.rateLimit("backup", 10, time.Minute, h.handleSystemBackup))
		pr.With(h.adminOnly, h.auditMiddleware("START_BACKUP_JOB")).Post("/api/system/backup/jobs", h.handleStartBackupJob)
		pr.With(h.adminOnly).Get("/api/system/backup/jobs/{id}/download", h.handleDownloadBackupJob)
		pr.With(h.adminOnly).Get("/api/system/jobs", h.handleListSystemJobs)
		pr.With(h.adminOnly).Get("/api/system/jobs/{id}", h.handleGetSystemJob)
		pr.With(h.adminOnly, h.auditMiddleware("RESTORE_SYSTEM")).Post("/api/system/restore", h.rateLimit("restore", 5, time.Minute, h.handleSystemRestore))
		pr.With(h.adminOnly, h.auditMiddleware("RESTORE_SYSTEM")).Post("/api/system/restore/jobs", h.rateLimit("restore_job", 5, time.Minute, h.handleStartRestoreJob))
		pr.With(h.adminOnly).Get("/api/audit-logs", h.handleAuditLogs)
		pr.With(h.adminOnly).Get("/api/audit/logs", h.handleAuditLogs)
		pr.With(h.adminOnly).Get("/api/users", h.handleListUsers)
		pr.With(h.adminOnly, h.auditMiddleware("CREATE_USER")).Post("/api/users", h.handleCreateUser)
		pr.With(h.adminOnly, h.auditMiddleware("UPDATE_USER")).Put("/api/users/{id}", h.handleUpdateUser)
		pr.With(h.adminOnly, h.auditMiddleware("DELETE_USER")).Delete("/api/users/{id}", h.handleDeleteUser)
		pr.With(h.adminOnly).Get("/api/users/{id}/permissions", h.handleListUserPermissions)
		pr.With(h.adminOnly, h.auditMiddleware("REPLACE_USER_PERMISSIONS")).Put("/api/users/{id}/permissions", h.handleReplaceUserPermissions)
		pr.With(h.adminOnly).Get("/api/admin/users", h.handleListUsers)
		pr.With(h.adminOnly, h.auditMiddleware("CREATE_USER")).Post("/api/admin/users", h.handleCreateUser)
		pr.With(h.adminOnly).Get("/api/templates", h.handleListTemplates)
		pr.With(h.adminOnly, h.auditMiddleware("CREATE_TEMPLATE")).Post("/api/templates", h.handleCreateTemplate)
		pr.With(h.adminOnly).Get("/api/alerts/rules", h.handleListAlertRules)
		pr.With(h.adminOnly, h.auditMiddleware("UPSERT_ALERT_RULE")).Post("/api/alerts/rules", h.handleUpsertAlertRule)
		pr.With(h.adminOnly, h.auditMiddleware("DELETE_ALERT_RULE")).Delete("/api/alerts/rules/{id}", h.handleDeleteAlertRule)
		pr.With(h.adminOnly).Get("/api/reports/summary", h.handleReportSummary)
		pr.With(h.adminOnly, h.auditMiddleware("DISCOVERY_SCAN")).Post("/api/discovery/scan", h.handleDiscoveryScan)
		pr.With(h.adminOnly, h.auditMiddleware("CONFIG_SNAPSHOT")).Post("/api/devices/{id}/config/snapshot", h.handleConfigSnapshot)
		pr.With(h.adminOnly, h.auditMiddleware("RUN_BACKUP_DRILL")).Post("/api/system/backup/drill", h.handleBackupDrill)
		pr.With(h.adminOnly, h.auditMiddleware("START_BACKUP_DRILL_JOB")).Post("/api/system/backup/drill/jobs", h.handleStartBackupDrillJob)
		pr.With(h.adminOnly).Get("/api/system/backup/drill/reports", h.handleBackupDrillReports)
		pr.With(h.adminOnly).Get("/api/settings/runtime", h.handleGetRuntimeSettings)
		pr.With(h.adminOnly, h.auditMiddleware("UPDATE_RUNTIME_SETTINGS")).Put("/api/settings/runtime", h.handleUpdateRuntimeSettings)
	})

	return r
}

func (h *Handler) serverTimeZoneHeaderMiddleware() func(http.Handler) http.Handler {
	tz := strings.TrimSpace(os.Getenv("TZ"))
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Server-Timezone", tz)
			next.ServeHTTP(w, r)
		})
	}
}

func (h *Handler) slowRequestMiddleware(threshold time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			cost := time.Since(start)
			if cost >= threshold {
				h.recordSlowAPI(slowAPIRecord{
					Timestamp:  time.Now(),
					Method:     r.Method,
					Path:       r.URL.Path,
					StatusCode: rec.status,
					DurationMS: cost.Milliseconds(),
					IP:         clientIP(r),
				})
				log.Printf("[slow-api] method=%s path=%s status=%d cost_ms=%d ip=%s", r.Method, r.URL.Path, rec.status, cost.Milliseconds(), clientIP(r))
			}
		})
	}
}

func (h *Handler) recordSlowAPI(item slowAPIRecord) {
	h.slowMu.Lock()
	defer h.slowMu.Unlock()
	h.slowAPIs = append([]slowAPIRecord{item}, h.slowAPIs...)
	if len(h.slowAPIs) > 80 {
		h.slowAPIs = h.slowAPIs[:80]
	}
}

func (h *Handler) recentSlowAPIs(limit int) []slowAPIRecord {
	if limit <= 0 || limit > 80 {
		limit = 30
	}
	h.slowMu.Lock()
	defer h.slowMu.Unlock()
	if len(h.slowAPIs) < limit {
		limit = len(h.slowAPIs)
	}
	out := make([]slowAPIRecord, limit)
	copy(out, h.slowAPIs[:limit])
	return out
}

func (h *Handler) rateLimit(key string, limit int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		h.mu.Lock()
		q := h.rl[key]
		cutoff := now.Add(-window)
		filtered := q[:0]
		for _, t := range q {
			if t.After(cutoff) {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) >= limit {
			h.rl[key] = filtered
			h.mu.Unlock()
			writeError(w, http.StatusTooManyRequests, "too many requests, retry later")
			return
		}
		filtered = append(filtered, now)
		h.rl[key] = filtered
		h.mu.Unlock()
		next(w, r)
	}
}

func parseIDParam(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}

func parseTime(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (h *Handler) auditMiddleware(action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if rec.status >= http.StatusBadRequest {
				return
			}
			u := currentUser(r.Context())
			var uid *int64
			if u.ID > 0 {
				uid = &u.ID
			}
			target := r.URL.Path
			_ = h.repo.AddAuditLog(r.Context(), db.AuditLog{
				UserID:     uid,
				Action:     action,
				Target:     target,
				Method:     r.Method,
				Path:       r.URL.Path,
				IP:         clientIP(r),
				StatusCode: rec.status,
				DurationMS: time.Since(start).Milliseconds(),
				Client:     tokenClient(r.Context()),
			})
		})
	}
}

func (h *Handler) requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := currentUser(r.Context())
			if u.Client == "mobile" && isWritePermission(permission) {
				writeError(w, http.StatusForbidden, "mobile client is read-only")
				return
			}
			ok, err := h.repo.HasPermission(r.Context(), u.ID, u.Role, permission)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "permission check failed")
				return
			}
			if !ok {
				writeError(w, http.StatusForbidden, "permission denied")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isWritePermission(permission string) bool {
	p := strings.ToLower(strings.TrimSpace(permission))
	return strings.HasSuffix(p, ".write")
}
