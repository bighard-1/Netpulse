package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type runtimeSettingsRequest struct {
	SNMPPollIntervalSec   int     `json:"snmp_poll_interval_sec"`
	PollIntervalCoreSec   int     `json:"poll_interval_core_sec"`
	PollIntervalAggSec    int     `json:"poll_interval_agg_sec"`
	PollIntervalAccessSec int     `json:"poll_interval_access_sec"`
	SNMPDeviceTimeoutSec  int     `json:"snmp_device_timeout_sec"`
	StatusOnlineWindowSec int     `json:"status_online_window_sec"`
	WebIdleTimeoutMin     int     `json:"web_idle_timeout_min"`
	AlertCPUThreshold     float64 `json:"alert_cpu_threshold"`
	AlertMemThreshold     float64 `json:"alert_mem_threshold"`
	AlertWebhookURL       string  `json:"alert_webhook_url"`
	SNMPCalibrationMap    string  `json:"snmp_calibration_map"`
}

func runtimeInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
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

	current, err := h.repo.GetRuntimeSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.SNMPPollIntervalSec = runtimeInt(req.SNMPPollIntervalSec, current.SNMPPollIntervalSec)
	req.PollIntervalCoreSec = runtimeInt(req.PollIntervalCoreSec, current.PollIntervalCoreSec)
	req.PollIntervalAggSec = runtimeInt(req.PollIntervalAggSec, current.PollIntervalAggSec)
	req.PollIntervalAccessSec = runtimeInt(req.PollIntervalAccessSec, current.PollIntervalAccessSec)
	req.SNMPDeviceTimeoutSec = runtimeInt(req.SNMPDeviceTimeoutSec, current.SNMPDeviceTimeoutSec)
	req.StatusOnlineWindowSec = runtimeInt(req.StatusOnlineWindowSec, current.StatusOnlineWindowSec)
	req.WebIdleTimeoutMin = runtimeInt(req.WebIdleTimeoutMin, current.WebIdleTimeoutMin)
	if req.SNMPPollIntervalSec < 5 || req.SNMPPollIntervalSec > 3600 {
		writeError(w, http.StatusBadRequest, "snmp_poll_interval_sec must be between 5 and 3600")
		return
	}
	if req.PollIntervalCoreSec < 5 || req.PollIntervalCoreSec > 3600 {
		writeError(w, http.StatusBadRequest, "poll_interval_core_sec must be between 5 and 3600")
		return
	}
	if req.PollIntervalAggSec < 5 || req.PollIntervalAggSec > 3600 {
		writeError(w, http.StatusBadRequest, "poll_interval_agg_sec must be between 5 and 3600")
		return
	}
	if req.PollIntervalAccessSec < 5 || req.PollIntervalAccessSec > 3600 {
		writeError(w, http.StatusBadRequest, "poll_interval_access_sec must be between 5 and 3600")
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
	if req.WebIdleTimeoutMin < 5 || req.WebIdleTimeoutMin > 1440 {
		writeError(w, http.StatusBadRequest, "web_idle_timeout_min must be between 5 and 1440")
		return
	}
	if req.AlertCPUThreshold < 0 || req.AlertCPUThreshold > 100 {
		writeError(w, http.StatusBadRequest, "alert_cpu_threshold must be between 0 and 100")
		return
	}
	if req.AlertMemThreshold < 0 || req.AlertMemThreshold > 100 {
		writeError(w, http.StatusBadRequest, "alert_mem_threshold must be between 0 and 100")
		return
	}
	if strings.TrimSpace(req.SNMPCalibrationMap) != "" {
		var m map[string]float64
		if err := json.Unmarshal([]byte(req.SNMPCalibrationMap), &m); err != nil {
			writeError(w, http.StatusBadRequest, "snmp_calibration_map must be valid JSON object")
			return
		}
	}

	kv := map[string]string{
		"snmp_poll_interval_sec":   strconv.Itoa(req.SNMPPollIntervalSec),
		"poll_interval_core_sec":   strconv.Itoa(req.PollIntervalCoreSec),
		"poll_interval_agg_sec":    strconv.Itoa(req.PollIntervalAggSec),
		"poll_interval_access_sec": strconv.Itoa(req.PollIntervalAccessSec),
		"snmp_device_timeout_sec":  strconv.Itoa(req.SNMPDeviceTimeoutSec),
		"status_online_window_sec": strconv.Itoa(req.StatusOnlineWindowSec),
		"web_idle_timeout_min":     strconv.Itoa(req.WebIdleTimeoutMin),
		"alert_cpu_threshold":      strconv.FormatFloat(req.AlertCPUThreshold, 'f', 2, 64),
		"alert_mem_threshold":      strconv.FormatFloat(req.AlertMemThreshold, 'f', 2, 64),
		"alert_webhook_url":        strings.TrimSpace(req.AlertWebhookURL),
		"snmp_calibration_map":     strings.TrimSpace(req.SNMPCalibrationMap),
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
