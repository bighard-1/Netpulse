package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"netpulse/internal/db"
)

func (h *Handler) handleUpsertAlertRule(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var ar db.AlertRule
	// New schema payload
	if v, ok := raw["id"]; ok {
		if n, err := toInt64(v); err == nil {
			ar.ID = n
		}
	}
	ar.Name = strings.TrimSpace(toString(raw["name"]))
	ar.Scope = strings.TrimSpace(toString(raw["scope"]))
	if ar.Scope == "" {
		ar.Scope = "global"
	}
	if v, ok := raw["device_id"]; ok {
		if n, err := toInt64(v); err == nil && n > 0 {
			ar.DeviceID = &n
		}
	}
	if v, ok := raw["cpu_threshold"]; ok {
		if f, err := toFloat64(v); err == nil {
			ar.CPUThreshold = &f
		}
	}
	if v, ok := raw["mem_threshold"]; ok {
		if f, err := toFloat64(v); err == nil {
			ar.MemThreshold = &f
		}
	}
	if v, ok := raw["traffic_threshold"]; ok {
		if n, err := toInt64(v); err == nil {
			ar.TrafficThreshold = &n
		}
	}
	ar.MuteStart = strings.TrimSpace(toString(raw["mute_start"]))
	ar.MuteEnd = strings.TrimSpace(toString(raw["mute_end"]))
	ar.NotifyWebhook = strings.TrimSpace(toString(raw["notify_webhook"]))
	if v, ok := raw["enabled"]; ok {
		ar.Enabled = toBool(v)
	} else {
		ar.Enabled = true
	}

	// Backward compatibility: old frontend payload(metric/op/threshold/silence_sec)
	if ar.CPUThreshold == nil && ar.MemThreshold == nil && ar.TrafficThreshold == nil {
		metric := strings.TrimSpace(strings.ToLower(toString(raw["metric"])))
		if metric != "" {
			f, _ := toFloat64(raw["threshold"])
			switch metric {
			case "cpu":
				ar.CPUThreshold = &f
			case "mem":
				ar.MemThreshold = &f
			case "traffic", "traffic_util":
				n := int64(f)
				ar.TrafficThreshold = &n
			}
		}
	}
	if ar.Name == "" {
		ar.Name = "告警策略"
	}
	if ar.Scope == "" {
		ar.Scope = "global"
	}
	id, err := h.repo.UpsertAlertRule(r.Context(), ar)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case float64:
		return int64(x), nil
	case json.Number:
		return x.Int64()
	case string:
		return strconv.ParseInt(strings.TrimSpace(x), 10, 64)
	default:
		return 0, fmt.Errorf("not int64")
	}
}

func toFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case json.Number:
		return x.Float64()
	case string:
		return strconv.ParseFloat(strings.TrimSpace(x), 64)
	default:
		return 0, fmt.Errorf("not float64")
	}
}

func toBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true") || strings.TrimSpace(x) == "1"
	case float64:
		return x != 0
	case int:
		return x != 0
	default:
		return false
	}
}

func (h *Handler) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListAlertRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	if err := h.repo.DeleteAlertRule(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "rule deleted"})
}
func (h *Handler) handleListAlertEvents(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	items, err := h.repo.ListAlertEvents(r.Context(), limit, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "limit": limit})
}

type alertWorkflowReq struct {
	Action         string `json:"action"`
	Assignee       string `json:"assignee"`
	Note           string `json:"note"`
	SilenceMinutes int    `json:"silence_minutes"`
}

func (h *Handler) handleUpdateAlertEventWorkflow(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid alert event id")
		return
	}
	var req alertWorkflowReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.repo.UpdateAlertEventWorkflow(r.Context(), id, req.Action, req.Assignee, req.Note, req.SilenceMinutes); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "alert workflow updated"})
}
