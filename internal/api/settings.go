package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type runtimeSettingsRequest struct {
	SNMPPollIntervalSec   int     `json:"snmp_poll_interval_sec"`
	SNMPDeviceTimeoutSec  int     `json:"snmp_device_timeout_sec"`
	StatusOnlineWindowSec int     `json:"status_online_window_sec"`
	AlertCPUThreshold     float64 `json:"alert_cpu_threshold"`
	AlertMemThreshold     float64 `json:"alert_mem_threshold"`
	AlertWebhookURL       string  `json:"alert_webhook_url"`
}

func (h *Handler) handleGetRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.repo.GetRuntimeSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *Handler) handleUpdateRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	var req runtimeSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if req.SNMPPollIntervalSec < 5 || req.SNMPPollIntervalSec > 3600 {
		writeError(w, http.StatusBadRequest, "snmp_poll_interval_sec must be between 5 and 3600")
		return
	}
	if req.SNMPDeviceTimeoutSec < 2 || req.SNMPDeviceTimeoutSec > 120 {
		writeError(w, http.StatusBadRequest, "snmp_device_timeout_sec must be between 2 and 120")
		return
	}
	if req.StatusOnlineWindowSec < 30 || req.StatusOnlineWindowSec > 3600 {
		writeError(w, http.StatusBadRequest, "status_online_window_sec must be between 30 and 3600")
		return
	}
	if req.AlertCPUThreshold <= 0 || req.AlertCPUThreshold > 100 {
		writeError(w, http.StatusBadRequest, "alert_cpu_threshold must be between 0 and 100")
		return
	}
	if req.AlertMemThreshold <= 0 || req.AlertMemThreshold > 100 {
		writeError(w, http.StatusBadRequest, "alert_mem_threshold must be between 0 and 100")
		return
	}

	kv := map[string]string{
		"snmp_poll_interval_sec":   strconv.Itoa(req.SNMPPollIntervalSec),
		"snmp_device_timeout_sec":  strconv.Itoa(req.SNMPDeviceTimeoutSec),
		"status_online_window_sec": strconv.Itoa(req.StatusOnlineWindowSec),
		"alert_cpu_threshold":      strconv.FormatFloat(req.AlertCPUThreshold, 'f', 2, 64),
		"alert_mem_threshold":      strconv.FormatFloat(req.AlertMemThreshold, 'f', 2, 64),
		"alert_webhook_url":        strings.TrimSpace(req.AlertWebhookURL),
	}
	if err := h.repo.UpsertSystemSettings(r.Context(), kv); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg, err := h.repo.GetRuntimeSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "runtime settings updated", "data": cfg})
}
