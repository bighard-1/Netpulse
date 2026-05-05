package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

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
}

func NewHandler(repo *db.Repository, collector *snmp.Collector, system *SystemService, jwtSecret string) *Handler {
	return &Handler{
		repo: repo, collector: collector, system: system, jwtSecret: jwtSecret,
		fails: map[string]int{}, lockedUntil: map[string]time.Time{}, rl: map[string][]time.Time{},
	}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	r.Post("/api/login", h.rateLimit("login", 20, time.Minute, h.handleLogin("web")))
	r.Post("/api/auth/login", h.rateLimit("login", 20, time.Minute, h.handleLogin("web")))
	r.Post("/api/auth/mobile/login", h.rateLimit("login", 20, time.Minute, h.handleLogin("mobile")))

	r.Group(func(pr chi.Router) {
		pr.Use(h.authMiddleware)
		pr.With(h.requirePermission("device.read")).Get("/api/devices", h.handleListDevices)
		pr.Get("/api/devices/{id}", h.handleGetDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("ADD_DEVICE")).Post("/api/devices", h.handleAddDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("IMPORT_DEVICES")).Post("/api/devices/import", h.handleImportDevices)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("DELETE_DEVICE")).Delete("/api/devices/{id}", h.handleDeleteDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_DEVICE_REMARK")).Put("/api/devices/{id}/remark", h.handleUpdateDeviceRemark)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_INTERFACE_REMARK")).Put("/api/interfaces/{id}/remark", h.handleUpdateInterfaceRemark)
		pr.With(h.requirePermission("metrics.read")).Get("/api/metrics/history", h.handleMetricsHistory)
		pr.With(h.requirePermission("logs.read")).Get("/api/devices/{id}/logs", h.handleDeviceLogs)
		pr.Get("/api/system/backup", h.rateLimit("backup", 10, time.Minute, h.handleSystemBackup))
		pr.With(h.auditMiddleware("RESTORE_SYSTEM")).Post("/api/system/restore", h.rateLimit("restore", 5, time.Minute, h.handleSystemRestore))
		pr.With(h.adminOnly).Get("/api/audit-logs", h.handleAuditLogs)
		pr.With(h.adminOnly).Get("/api/audit/logs", h.handleAuditLogs)
		pr.With(h.adminOnly).Get("/api/users", h.handleListUsers)
		pr.With(h.adminOnly).Post("/api/users", h.handleCreateUser)
		pr.With(h.adminOnly).Get("/api/admin/users", h.handleListUsers)
		pr.With(h.adminOnly).Post("/api/admin/users", h.handleCreateUser)
		pr.With(h.adminOnly).Get("/api/templates", h.handleListTemplates)
		pr.With(h.adminOnly).Post("/api/templates", h.handleCreateTemplate)
		pr.With(h.adminOnly).Get("/api/topology", h.handleListTopology)
		pr.With(h.adminOnly).Post("/api/topology", h.handleUpsertTopology)
		pr.With(h.adminOnly).Get("/api/alerts/rules", h.handleListAlertRules)
		pr.With(h.adminOnly).Post("/api/alerts/rules", h.handleUpsertAlertRule)
		pr.With(h.adminOnly).Get("/api/reports/summary", h.handleReportSummary)
		pr.With(h.adminOnly).Post("/api/discovery/scan", h.handleDiscoveryScan)
		pr.With(h.adminOnly).Post("/api/devices/{id}/config/snapshot", h.handleConfigSnapshot)
		pr.With(h.adminOnly).Post("/api/system/backup/drill", h.handleBackupDrill)
		pr.With(h.adminOnly).Get("/api/system/backup/drill/reports", h.handleBackupDrillReports)
	})

	return r
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

func (h *Handler) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	item, err := h.repo.GetDeviceByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type addDeviceRequest struct {
	IP              string `json:"ip"`
	Brand           string `json:"brand"`
	Community       string `json:"community"`
	SNMPVersion     string `json:"snmp_version"`
	SNMPPort        int    `json:"snmp_port"`
	V3Username      string `json:"v3_username"`
	V3AuthProtocol  string `json:"v3_auth_protocol"`
	V3AuthPassword  string `json:"v3_auth_password"`
	V3PrivProtocol  string `json:"v3_priv_protocol"`
	V3PrivPassword  string `json:"v3_priv_password"`
	V3SecurityLevel string `json:"v3_security_level"`
	Remark          string `json:"remark"`
}

type updateRemarkRequest struct {
	Remark string `json:"remark"`
}

func (h *Handler) handleListDevices(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListDevicesWithStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleAddDevice(w http.ResponseWriter, r *http.Request) {
	var req addDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.IP == "" || req.Brand == "" {
		writeError(w, http.StatusBadRequest, "ip, brand are required")
		return
	}
	if req.SNMPVersion == "" {
		req.SNMPVersion = "2c"
	}
	if req.SNMPPort <= 0 {
		req.SNMPPort = 161
	}
	if req.SNMPVersion != "3" && req.Community == "" {
		writeError(w, http.StatusBadRequest, "snmp v1/v2c requires community")
		return
	}

	deviceID, err := h.repo.AddDevice(r.Context(), db.Device{
		IP:          req.IP,
		Brand:       req.Brand,
		Community:   req.Community,
		SNMPVersion: req.SNMPVersion,
		SNMPPort:    req.SNMPPort,
		V3Username:  req.V3Username,
		V3AuthProto: req.V3AuthProtocol,
		V3AuthPass:  req.V3AuthPassword,
		V3PrivProto: req.V3PrivProtocol,
		V3PrivPass:  req.V3PrivPassword,
		V3SecLevel:  req.V3SecurityLevel,
		Remark:      req.Remark,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Trigger immediate SNMP interface discovery.
	opt := snmp.PollOptions{
		Brand:       req.Brand,
		SNMPVersion: req.SNMPVersion,
		Port:        req.SNMPPort,
		Community:   req.Community,
		V3Username:  req.V3Username,
		V3AuthProto: req.V3AuthProtocol,
		V3AuthPass:  req.V3AuthPassword,
		V3PrivProto: req.V3PrivProtocol,
		V3PrivPass:  req.V3PrivPassword,
		V3SecLevel:  req.V3SecurityLevel,
	}
	ifs, err := h.collector.FetchInterfacesWithOptions(req.IP, opt)
	if err == nil {
		list := make([]db.Interface, 0, len(ifs))
		for _, itf := range ifs {
			list = append(list, db.Interface{
				DeviceID: deviceID,
				Index:    itf.IfIndex,
				Name:     itf.IfName,
			})
		}
		_ = h.repo.SyncInterfaces(context.Background(), deviceID, list)
	}
	// Trigger immediate polling once, so status and charts are available without waiting for next worker tick.
	if poll, err := h.collector.PollDevice(req.IP, opt); err == nil {
		metrics := make([]db.InterfaceMetric, 0, len(poll.Interfaces))
		for _, itf := range poll.Interfaces {
			metrics = append(metrics, db.InterfaceMetric{
				IfIndex:       itf.IfIndex,
				CPUUsage:      poll.CPUUsage,
				MemoryUsage:   poll.MemoryUsage,
				TrafficInBps:  0,
				TrafficOutBps: 0,
			})
		}
		if len(metrics) > 0 {
			_ = h.repo.SaveMetrics(context.Background(), deviceID, poll.PolledAt, metrics)
		}
		_ = h.repo.AddDeviceLog(context.Background(), deviceID, "INFO", "[OK] 设备添加后首次采集成功")
	} else {
		_ = h.repo.AddDeviceLog(context.Background(), deviceID, "ERROR", fmt.Sprintf("[POLL_FAILED] 设备添加后首次采集失败: %v", err))
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      deviceID,
		"message": "device created",
	})
}

func (h *Handler) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	if err := h.repo.DeleteDevice(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "device deleted"})
}

func (h *Handler) handleUpdateDeviceRemark(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	var req updateRemarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := h.repo.UpdateDeviceRemark(r.Context(), id, req.Remark); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "device remark updated"})
}

func (h *Handler) handleUpdateInterfaceRemark(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid interface id")
		return
	}
	var req updateRemarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := h.repo.UpdateInterfaceRemark(r.Context(), id, req.Remark); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "interface remark updated"})
}

func (h *Handler) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	metricType := r.URL.Query().Get("type")
	idStr := r.URL.Query().Get("id")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

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
		items, err := h.repo.GetDeviceHistory(r.Context(), id, start, end)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"type":  metricType,
			"id":    id,
			"start": start,
			"end":   end,
			"data":  items,
		})
	case "traffic":
		items, err := h.repo.GetInterfaceHistory(r.Context(), id, start, end)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"type":  metricType,
			"id":    id,
			"start": start,
			"end":   end,
			"data":  items,
		})
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: cpu, mem, traffic")
	}
}

func (h *Handler) handleDeviceLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	items, err := h.repo.GetDeviceLogs(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleSystemBackup(w http.ResponseWriter, r *http.Request) {
	filePath, filename, err := h.system.Backup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(filePath)

	f, err := os.Open(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("open backup file: %v", err))
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeContent(w, r, filename, time.Now(), f)
}

func (h *Handler) handleSystemRestore(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	restoreCtx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()

	if err := h.system.Restore(restoreCtx, file); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "restore completed"})
}

func (h *Handler) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListAuditLogs(r.Context(), 300)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	if req.Role != "admin" && req.Role != "user" {
		req.Role = "user"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash error")
		return
	}
	if err := h.repo.CreateUser(r.Context(), req.Username, string(hash), req.Role); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "user created"})
}

func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.repo.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func parseIDParam(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}

func parseTime(v string) (time.Time, error) {
	return time.Parse(time.RFC3339, v)
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
			ok, err := h.repo.HasPermission(r.Context(), u.Role, permission)
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
