package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"netpulse/internal/db"
)

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

type updateRemarkRequest struct {
	Remark string `json:"remark"`
}

type updateDeviceRequest struct {
	IP                    string  `json:"ip"`
	Name                  string  `json:"name"`
	Brand                 string  `json:"brand"`
	Remark                string  `json:"remark"`
	MaintenanceMode       bool    `json:"maintenance_mode"`
	MonitoringPaused      bool    `json:"monitoring_paused"`
	MonitoringPauseReason string  `json:"monitoring_pause_reason"`
	DeviceTier            string  `json:"device_tier"`
	PollIntervalSec       int     `json:"poll_interval_sec"`
	CPUThreshold          float64 `json:"cpu_threshold"`
	MemThreshold          float64 `json:"mem_threshold"`
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

func (h *Handler) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := h.repo.DeleteDevice(ctx, id); err != nil {
		writeDeleteDeviceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "device deleted"})
}

func writeDeleteDeviceError(w http.ResponseWriter, err error) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "statement timeout"), strings.Contains(msg, "lock timeout"):
		writeErrorDetail(
			w,
			http.StatusGatewayTimeout,
			"ERR_DEVICE_DELETE_BUSY",
			"删除资产失败：数据库正在处理该资产的采集/查询数据，删除操作未能及时获得锁",
			"请先确认没有打开该资产详情或长周期图表，等待当前采集轮询结束后重试；若仍失败，可在系统设置的运营数据清理中先清理历史运营数据。",
		)
	case strings.Contains(msg, "duplicate key"):
		writeErrorDetail(
			w,
			http.StatusConflict,
			"ERR_DEVICE_DELETE_STATE",
			"删除资产失败：资产状态发生变化",
			"请刷新资产列表后确认该资产是否已被其他管理员处理。",
		)
	default:
		writeErrorDetail(
			w,
			http.StatusInternalServerError,
			"ERR_DEVICE_DELETE",
			"删除资产失败：资产归档或关联可见数据清理未完成",
			"请重试一次；如连续失败，请导出自助诊断报告，并提供资产ID、IP和当前时间给运维排查。",
		)
	}
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
	ip := strings.TrimSpace(req.IP)
	if ip == "" {
		ip = item.IP
	}
	if net.ParseIP(ip) == nil {
		writeError(w, http.StatusBadRequest, "invalid IP address")
		return
	}
	if ip != item.IP {
		existing, findErr := h.repo.FindDeviceByIP(r.Context(), ip)
		if findErr != nil {
			writeError(w, http.StatusInternalServerError, findErr.Error())
			return
		}
		if existing != nil && existing.ID != id {
			writeError(w, http.StatusConflict, "该 IP 已被其他资产使用")
			return
		}
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
	req.DeviceTier = normalizeDeviceTier(req.DeviceTier)
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
	req.MonitoringPauseReason = strings.TrimSpace(req.MonitoringPauseReason)
	if len([]rune(req.MonitoringPauseReason)) > 240 {
		writeError(w, http.StatusBadRequest, "monitoring_pause_reason is too long")
		return
	}
	if err := h.repo.UpdateDevice(r.Context(), db.Device{
		ID:                    id,
		IP:                    ip,
		Name:                  name,
		Brand:                 brand,
		Remark:                req.Remark,
		MaintenanceMode:       req.MaintenanceMode,
		MonitoringPaused:      req.MonitoringPaused,
		MonitoringPauseReason: req.MonitoringPauseReason,
		DeviceTier:            req.DeviceTier,
		PollIntervalSec:       req.PollIntervalSec,
		CPUThreshold:          req.CPUThreshold,
		MemThreshold:          req.MemThreshold,
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
