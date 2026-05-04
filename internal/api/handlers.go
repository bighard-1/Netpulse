package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"netpulse/internal/db"
	"netpulse/internal/snmp"
)

type Handler struct {
	repo      *db.Repository
	collector *snmp.Collector
	system    *SystemService
	jwtSecret string
}

func NewHandler(repo *db.Repository, collector *snmp.Collector, system *SystemService, jwtSecret string) *Handler {
	return &Handler{repo: repo, collector: collector, system: system, jwtSecret: jwtSecret}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	r.Post("/api/auth/login", h.handleLogin("web"))
	r.Post("/api/auth/mobile/login", h.handleLogin("mobile"))

	r.Group(func(pr chi.Router) {
		pr.Use(h.authMiddleware)
		pr.Get("/api/devices", h.handleListDevices)
		pr.Post("/api/devices", h.handleAddDevice)
		pr.Delete("/api/devices/{id}", h.handleDeleteDevice)
		pr.Put("/api/devices/{id}/remark", h.handleUpdateDeviceRemark)
		pr.Put("/api/interfaces/{id}/remark", h.handleUpdateInterfaceRemark)
		pr.Get("/api/metrics/history", h.handleMetricsHistory)
		pr.Get("/api/devices/{id}/logs", h.handleDeviceLogs)
		pr.Get("/api/system/backup", h.handleSystemBackup)
		pr.Post("/api/system/restore", h.handleSystemRestore)
		pr.With(h.adminOnly).Get("/api/audit/logs", h.handleAuditLogs)
		pr.With(h.adminOnly).Get("/api/admin/users", h.handleListUsers)
		pr.With(h.adminOnly).Post("/api/admin/users", h.handleCreateUser)
	})

	return r
}

type addDeviceRequest struct {
	IP        string `json:"ip"`
	Brand     string `json:"brand"`
	Community string `json:"community"`
	Remark    string `json:"remark"`
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
	if req.IP == "" || req.Brand == "" || req.Community == "" {
		writeError(w, http.StatusBadRequest, "ip, brand, community are required")
		return
	}

	deviceID, err := h.repo.AddDevice(r.Context(), db.Device{
		IP:        req.IP,
		Brand:     req.Brand,
		Community: req.Community,
		Remark:    req.Remark,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Trigger immediate SNMP interface discovery.
	ifs, err := h.collector.FetchInterfaces(req.IP, req.Community)
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

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      deviceID,
		"message": "device created",
	})
	h.audit(r, "ADD_DEVICE", "device_id="+strconv.FormatInt(deviceID, 10))
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
	h.audit(r, "DELETE_DEVICE", "device_id="+strconv.FormatInt(id, 10))
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
	h.audit(r, "UPDATE_DEVICE_REMARK", "device_id="+strconv.FormatInt(id, 10))
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
	h.audit(r, "UPDATE_INTERFACE_REMARK", "interface_id="+strconv.FormatInt(id, 10))
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
	h.audit(r, "RESTORE_SYSTEM", "restore sql.gz")
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

func (h *Handler) audit(r *http.Request, action, detail string) {
	u := currentUser(r.Context())
	_ = h.repo.AddAuditLog(r.Context(), db.AuditLog{
		Username: u.Username,
		Action:   action,
		Method:   r.Method,
		Path:     r.URL.Path,
		IP:       clientIP(r),
		Detail:   detail,
	})
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
	h.audit(r, "CREATE_USER", "username="+req.Username+",role="+req.Role)
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
