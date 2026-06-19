package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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
