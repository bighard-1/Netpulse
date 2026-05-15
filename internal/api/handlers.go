package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
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
		pr.Use(h.slowRequestMiddleware(1200 * time.Millisecond))
		pr.With(h.requirePermission("device.read")).Get("/api/devices", h.handleListDevices)
		pr.With(h.requirePermission("device.read")).Get("/api/search", h.handleGlobalSearch)
		pr.Get("/api/devices/{id}", h.handleGetDevice)
		pr.Get("/api/devices/{id}/capabilities", h.handleGetDeviceCapabilities)
		pr.With(h.requirePermission("device.read")).Get("/api/devices/{id}/diagnose", h.handleDiagnoseDevice)
		pr.With(h.requirePermission("device.write")).Post("/api/devices/precheck", h.handlePrecheckDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("ADD_DEVICE")).Post("/api/devices", h.handleAddDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_DEVICE")).Put("/api/devices/{id}", h.handleUpdateDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("IMPORT_DEVICES")).Post("/api/devices/import", h.handleImportDevices)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("DELETE_DEVICE")).Delete("/api/devices/{id}", h.handleDeleteDevice)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_DEVICE_REMARK")).Put("/api/devices/{id}/remark", h.handleUpdateDeviceRemark)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_INTERFACE_REMARK")).Put("/api/interfaces/{id}/remark", h.handleUpdateInterfaceRemark)
		pr.With(h.requirePermission("device.write"), h.auditMiddleware("UPDATE_INTERFACE_PROFILE")).Put("/api/interfaces/{id}", h.handleUpdateInterfaceProfile)
		pr.With(h.requirePermission("metrics.read")).Get("/api/metrics/history", h.handleMetricsHistory)
		pr.With(h.requirePermission("logs.read")).Get("/api/devices/{id}/logs", h.handleDeviceLogs)
		pr.Get("/api/events/recent", h.handleRecentEvents)
		pr.With(h.requirePermission("logs.read")).Get("/api/alerts/events", h.handleListAlertEvents)
		pr.With(h.adminOnly).Put("/api/alerts/events/{id}", h.handleUpdateAlertEventWorkflow)
		pr.Get("/api/system/health", h.handleSystemHealthTrend)
		pr.With(h.adminOnly).Get("/api/system/ops", h.handleSystemOps)
		pr.With(h.adminOnly).Get("/api/system/inspection-bundle", h.handleInspectionBundle)
		pr.Get("/api/system/backup", h.rateLimit("backup", 10, time.Minute, h.handleSystemBackup))
		pr.With(h.auditMiddleware("RESTORE_SYSTEM")).Post("/api/system/restore", h.rateLimit("restore", 5, time.Minute, h.handleSystemRestore))
		pr.With(h.adminOnly).Get("/api/audit-logs", h.handleAuditLogs)
		pr.With(h.adminOnly).Get("/api/audit/logs", h.handleAuditLogs)
		pr.With(h.adminOnly).Get("/api/users", h.handleListUsers)
		pr.With(h.adminOnly).Post("/api/users", h.handleCreateUser)
		pr.With(h.adminOnly).Put("/api/users/{id}", h.handleUpdateUser)
		pr.With(h.adminOnly).Delete("/api/users/{id}", h.handleDeleteUser)
		pr.With(h.adminOnly).Get("/api/users/{id}/permissions", h.handleListUserPermissions)
		pr.With(h.adminOnly).Put("/api/users/{id}/permissions", h.handleReplaceUserPermissions)
		pr.With(h.adminOnly).Get("/api/admin/users", h.handleListUsers)
		pr.With(h.adminOnly).Post("/api/admin/users", h.handleCreateUser)
		pr.With(h.adminOnly).Get("/api/templates", h.handleListTemplates)
		pr.With(h.adminOnly).Post("/api/templates", h.handleCreateTemplate)
		pr.With(h.adminOnly).Get("/api/alerts/rules", h.handleListAlertRules)
		pr.With(h.adminOnly).Post("/api/alerts/rules", h.handleUpsertAlertRule)
		pr.With(h.adminOnly).Delete("/api/alerts/rules/{id}", h.handleDeleteAlertRule)
		pr.With(h.adminOnly).Get("/api/reports/summary", h.handleReportSummary)
		pr.With(h.adminOnly).Post("/api/discovery/scan", h.handleDiscoveryScan)
		pr.With(h.adminOnly).Post("/api/devices/{id}/config/snapshot", h.handleConfigSnapshot)
		pr.With(h.adminOnly).Post("/api/system/backup/drill", h.handleBackupDrill)
		pr.With(h.adminOnly).Get("/api/system/backup/drill/reports", h.handleBackupDrillReports)
		pr.With(h.adminOnly).Get("/api/settings/runtime", h.handleGetRuntimeSettings)
		pr.With(h.adminOnly).Put("/api/settings/runtime", h.handleUpdateRuntimeSettings)
	})

	return r
}

func (h *Handler) slowRequestMiddleware(threshold time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			cost := time.Since(start)
			if cost >= threshold {
				log.Printf("[slow-api] method=%s path=%s cost_ms=%d ip=%s", r.Method, r.URL.Path, cost.Milliseconds(), clientIP(r))
			}
		})
	}
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

func (h *Handler) handleGetDeviceCapabilities(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	item, err := h.repo.GetDeviceCapability(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "device capability not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

type addDeviceRequest struct {
	IP              string `json:"ip"`
	Name            string `json:"name"`
	TemplateID      *int64 `json:"template_id,omitempty"`
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
	PollIntervalSec int    `json:"poll_interval_sec"`
	CPUThreshold    float64 `json:"cpu_threshold"`
	MemThreshold    float64 `json:"mem_threshold"`
	Remark          string `json:"remark"`
}

func validateSNMPRequest(req addDeviceRequest) error {
	if req.SNMPVersion != "3" {
		if strings.TrimSpace(req.Community) == "" {
			return fmt.Errorf("snmp v1/v2c requires community")
		}
		return nil
	}
	if strings.TrimSpace(req.V3Username) == "" {
		return fmt.Errorf("snmp v3 requires v3_username")
	}
	level := strings.TrimSpace(req.V3SecurityLevel)
	if level == "" {
		level = "noAuthNoPriv"
	}
	switch level {
	case "noAuthNoPriv":
		return nil
	case "authNoPriv", "authPriv":
		if strings.TrimSpace(req.V3AuthProtocol) == "" {
			return fmt.Errorf("snmp v3 requires v3_auth_protocol for selected security level")
		}
		if strings.TrimSpace(req.V3AuthPassword) == "" {
			return fmt.Errorf("snmp v3 requires v3_auth_password for selected security level")
		}
		if level == "authPriv" {
			if strings.TrimSpace(req.V3PrivProtocol) == "" {
				return fmt.Errorf("snmp v3 authPriv requires v3_priv_protocol")
			}
			if strings.TrimSpace(req.V3PrivPassword) == "" {
				return fmt.Errorf("snmp v3 authPriv requires v3_priv_password")
			}
		}
		return nil
	default:
		return fmt.Errorf("invalid snmp v3 security level")
	}
}

type updateRemarkRequest struct {
	Remark string `json:"remark"`
}

type updateDeviceRequest struct {
	Name            string `json:"name"`
	Brand           string `json:"brand"`
	Remark          string `json:"remark"`
	MaintenanceMode bool   `json:"maintenance_mode"`
	PollIntervalSec int    `json:"poll_interval_sec"`
	CPUThreshold    float64 `json:"cpu_threshold"`
	MemThreshold    float64 `json:"mem_threshold"`
}

type updateInterfaceRequest struct {
	Name   *string `json:"name"`
	Remark *string `json:"remark"`
}

func (h *Handler) handlePrecheckDevice(w http.ResponseWriter, r *http.Request) {
	var req addDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.IP == "" {
		writeError(w, http.StatusBadRequest, "ip is required")
		return
	}
	if req.SNMPVersion == "" {
		req.SNMPVersion = "2c"
	}
	if req.SNMPPort <= 0 {
		req.SNMPPort = 161
	}
	if req.PollIntervalSec < 0 {
		req.PollIntervalSec = 0
	}
	if req.PollIntervalSec > 3600 {
		req.PollIntervalSec = 3600
	}
	if req.CPUThreshold < 0 {
		req.CPUThreshold = 0
	}
	if req.CPUThreshold > 100 {
		req.CPUThreshold = 100
	}
	if req.MemThreshold < 0 {
		req.MemThreshold = 0
	}
	if req.MemThreshold > 100 {
		req.MemThreshold = 100
	}
	if err := validateSNMPRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
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
	poll, err := h.collector.PollDevice(req.IP, opt)
	if err != nil {
		msg := strings.ToLower(err.Error())
		hint := "请检查SNMP参数"
		switch {
		case strings.Contains(msg, "timeout"):
			hint = "设备响应超时，请检查网络连通、ACL、防火墙或SNMP端口"
		case strings.Contains(msg, "authentication"), strings.Contains(msg, "community"), strings.Contains(msg, "authorization"):
			hint = "认证失败，请核对v3用户名/认证协议/密码或v1/v2c团体字串"
		case strings.Contains(msg, "connect"):
			hint = "连接失败，请检查IP与端口可达性"
		case strings.Contains(msg, "oid"), strings.Contains(msg, "ifname"), strings.Contains(msg, "counter"):
			hint = "设备OID读取异常，请检查型号兼容与SNMP视图权限"
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_SNMP_PRECHECK",
			"error":   err.Error(),
			"message": "snmp precheck failed",
			"hint":    hint,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":    "snmp precheck ok",
		"cpu_usage":  poll.CPUUsage,
		"mem_usage":  poll.MemoryUsage,
		"interfaces": len(poll.Interfaces),
	})
}

func (h *Handler) handleListDevices(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListDevicesWithStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleGlobalSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	var ctxDeviceID int64
	if raw := strings.TrimSpace(r.URL.Query().Get("device_id")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
			ctxDeviceID = v
		}
	}
	items, err := h.repo.GlobalSearch(r.Context(), q, 120, ctxDeviceID)
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
	if req.Name == "" {
		req.Name = req.IP
	}
	if req.SNMPPort <= 0 {
		req.SNMPPort = 161
	}
	if err := validateSNMPRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	deviceID, err := h.repo.AddDevice(r.Context(), db.Device{
		IP:          req.IP,
		Name:        req.Name,
		TemplateID:  req.TemplateID,
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
		PollIntervalSec: req.PollIntervalSec,
		CPUThreshold: req.CPUThreshold,
		MemThreshold: req.MemThreshold,
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
				IfName:        itf.IfName,
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

func (h *Handler) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	var req updateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
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
	name := req.Name
	if name == "" {
		name = item.Name
	}
	brand := req.Brand
	if brand == "" {
		brand = item.Brand
	}
	if req.PollIntervalSec < 0 {
		req.PollIntervalSec = 0
	}
	if req.PollIntervalSec > 3600 {
		req.PollIntervalSec = 3600
	}
	if req.CPUThreshold < 0 {
		req.CPUThreshold = 0
	}
	if req.CPUThreshold > 100 {
		req.CPUThreshold = 100
	}
	if req.MemThreshold < 0 {
		req.MemThreshold = 0
	}
	if req.MemThreshold > 100 {
		req.MemThreshold = 100
	}
	if err := h.repo.UpdateDevice(r.Context(), db.Device{
		ID:              id,
		Name:            name,
		Brand:           brand,
		Remark:          req.Remark,
		MaintenanceMode: req.MaintenanceMode,
		PollIntervalSec: req.PollIntervalSec,
		CPUThreshold:    req.CPUThreshold,
		MemThreshold:    req.MemThreshold,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "device updated"})
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

func (h *Handler) handleUpdateInterfaceProfile(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid interface id")
		return
	}
	var req updateInterfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Name == nil && req.Remark == nil {
		writeError(w, http.StatusBadRequest, "name or remark is required")
		return
	}
	if err := h.repo.UpdateInterfaceProfile(r.Context(), id, req.Name, req.Remark); err != nil {
		if err.Error() == "interface name conflict in this device" {
			writeError(w, http.StatusConflict, "端口名称在本资产内已存在，请更换")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "interface updated"})
}

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
			"type":     metricType,
			"id":       id,
			"start":    start,
			"end":      end,
			"interval": interval,
			"maxPoints": maxPoints,
			"data":      items,
		})
	case "traffic":
		items, err := h.repo.GetInterfaceHistory(r.Context(), id, start, end, interval, maxPoints)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"type":     metricType,
			"id":       id,
			"start":    start,
			"end":      end,
			"interval": interval,
			"maxPoints": maxPoints,
			"data":      items,
		})
	case "storage":
		items, err := h.repo.GetDeviceStorageHistory(r.Context(), id, start, end, interval, maxPoints)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"type":     metricType,
			"id":       id,
			"start":    start,
			"end":      end,
			"interval": interval,
			"maxPoints": maxPoints,
			"data":      items,
		})
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: cpu, mem, traffic, storage")
	}
}

func (h *Handler) handleDeviceLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	level := strings.TrimSpace(r.URL.Query().Get("level"))
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, e := strconv.Atoi(raw); e == nil && n > 0 {
			limit = n
		}
	}
	items, err := h.repo.GetDeviceLogsFiltered(r.Context(), id, level, source, limit)
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
	header := make([]byte, 2)
	n, err := io.ReadFull(file, header)
	if err != nil || n != 2 {
		writeError(w, http.StatusBadRequest, "invalid gzip file")
		return
	}
	if header[0] != 0x1f || header[1] != 0x8b {
		writeError(w, http.StatusBadRequest, "restore file must be .sql.gz")
		return
	}
	restoreReader := io.MultiReader(bytes.NewReader(header), file)

	restoreCtx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()

	if err := h.system.Restore(restoreCtx, restoreReader); err != nil {
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
	if err := validatePasswordPolicy(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

type updateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username required")
		return
	}
	if req.Role != "admin" && req.Role != "user" {
		req.Role = "user"
	}
	var hashPtr *string
	if req.Password != "" {
		if err := validatePasswordPolicy(req.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "hash error")
			return
		}
		s := string(hash)
		hashPtr = &s
	}
	if err := h.repo.UpdateUser(r.Context(), id, req.Username, req.Role, hashPtr); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "user updated"})
}

func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	u := currentUser(r.Context())
	if u.ID == id {
		writeError(w, http.StatusBadRequest, "cannot delete self")
		return
	}
	if err := h.repo.DeleteUser(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "user deleted"})
}

func (h *Handler) handleListUserPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	items, err := h.repo.ListUserPermissions(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": items})
}

type userPermissionsReq struct {
	Permissions []string `json:"permissions"`
}

func (h *Handler) handleReplaceUserPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req userPermissionsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.repo.ReplaceUserPermissions(r.Context(), id, req.Permissions); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "permissions updated"})
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

func validatePasswordPolicy(password string) error {
	if len(password) < 10 {
		return fmt.Errorf("password must be at least 10 characters")
	}
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !(hasUpper && hasLower && hasDigit) {
		return fmt.Errorf("password must include uppercase, lowercase and number")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	code := "ERR_GENERIC"
	switch status {
	case http.StatusBadRequest:
		code = "ERR_BAD_REQUEST"
	case http.StatusUnauthorized:
		code = "ERR_UNAUTHORIZED"
	case http.StatusForbidden:
		code = "ERR_FORBIDDEN"
	case http.StatusNotFound:
		code = "ERR_NOT_FOUND"
	case http.StatusConflict:
		code = "ERR_CONFLICT"
	case http.StatusTooManyRequests:
		code = "ERR_RATE_LIMIT"
	case http.StatusInternalServerError:
		code = "ERR_INTERNAL"
	}
	writeJSON(w, status, map[string]string{
		"code":    code,
		"error":   msg,
		"message": msg,
		"hint":    "如需排查，请导出自助诊断报告并提供给运维",
	})
}
