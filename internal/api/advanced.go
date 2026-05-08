package api

import (
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"netpulse/internal/db"
)

func (h *Handler) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var t db.DeviceTemplate
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if t.Name == "" || t.Brand == "" {
		writeError(w, http.StatusBadRequest, "name and brand required")
		return
	}
	if t.SNMPVersion == "" {
		t.SNMPVersion = "2c"
	}
	if t.SNMPPort <= 0 {
		t.SNMPPort = 161
	}
	id, err := h.repo.CreateTemplate(r.Context(), t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListTemplates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleImportDevices(w http.ResponseWriter, r *http.Request) {
	reader := csv.NewReader(r.Body)
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid csv")
		return
	}
	created := 0
	for i, rec := range records {
		if i == 0 {
			continue
		}
		if len(rec) < 4 {
			continue
		}
		port := 161
		if len(rec) > 5 {
			if p, err := strconv.Atoi(strings.TrimSpace(rec[5])); err == nil && p > 0 {
				port = p
			}
		}
		dev := db.Device{
			IP:          strings.TrimSpace(rec[0]),
			Brand:       strings.TrimSpace(rec[1]),
			Community:   strings.TrimSpace(rec[2]),
			SNMPVersion: strings.TrimSpace(rec[3]),
			Remark:      "",
			SNMPPort:    port,
		}
		if len(rec) > 4 {
			dev.Remark = strings.TrimSpace(rec[4])
		}
		if dev.IP == "" || dev.Brand == "" {
			continue
		}
		if dev.SNMPVersion == "" {
			dev.SNMPVersion = "2c"
		}
		if _, err := h.repo.AddDevice(r.Context(), dev); err == nil {
			created++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created})
}

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

func (h *Handler) handleReportSummary(w http.ResponseWriter, r *http.Request) {
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			end = t
		}
	}
	devices, err := h.repo.ListDevicesWithStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var b strings.Builder
	b.WriteString("device_id,ip,status,cpu_points,mem_points\n")
	for _, d := range devices {
		cpu, _ := h.repo.GetDeviceHistory(r.Context(), d.ID, start, end, "1m")
		mem, _ := h.repo.GetDeviceHistory(r.Context(), d.ID, start, end, "1m")
		b.WriteString(fmt.Sprintf("%d,%s,%s,%d,%d\n", d.ID, d.IP, d.Status, len(cpu), len(mem)))
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="netpulse_report.csv"`)
	_, _ = io.WriteString(w, b.String())
}

func (h *Handler) handleSystemHealthTrend(w http.ResponseWriter, r *http.Request) {
	limit := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := h.repo.GetSystemHealthTrend(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  items,
		"limit": limit,
	})
}

func (h *Handler) handleRecentEvents(w http.ResponseWriter, r *http.Request) {
	limit := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items, err := h.repo.GetRecentEvents(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  items,
		"limit": limit,
	})
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

func (h *Handler) handleSystemOps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	devs, _ := h.repo.ListDevicesWithStatus(ctx)
	events, _ := h.repo.GetRecentEvents(ctx, 50)
	audits, _ := h.repo.ListAuditLogs(ctx, 50)
	openAlerts := 0
	for _, e := range events {
		l := strings.ToUpper(strings.TrimSpace(e.Level))
		if l == "ERROR" || l == "WARNING" || strings.Contains(strings.ToUpper(e.Message), "DOWN") {
			openAlerts++
		}
	}
	var lastAudit string
	if len(audits) > 0 {
		lastAudit = audits[0].Timestamp.Format(time.RFC3339)
	}
	var lastEvent string
	if len(events) > 0 {
		lastEvent = events[0].CreatedAt.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_total":      len(devs),
		"open_alert_events": openAlerts,
		"recent_events":     len(events),
		"recent_audits":     len(audits),
		"last_event_at":     lastEvent,
		"last_audit_at":     lastAudit,
	})
}

type discoveryReq struct {
	CIDR      string `json:"cidr"`
	Community string `json:"community"`
	Brand     string `json:"brand"`
}

func (h *Handler) handleDiscoveryScan(w http.ResponseWriter, r *http.Request) {
	var req discoveryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	_, ipnet, err := net.ParseCIDR(req.CIDR)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cidr")
		return
	}
	ips := ipsInCIDR(ipnet, 256)
	results := make([]map[string]any, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	for _, ip := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			conn, err := net.DialTimeout("tcp", ip+":161", 600*time.Millisecond)
			up := err == nil
			if conn != nil {
				_ = conn.Close()
			}
			if up {
				mu.Lock()
				results = append(results, map[string]any{"ip": ip, "snmp": true})
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"cidr": req.CIDR, "results": results})
}

func ipsInCIDR(ipnet *net.IPNet, limit int) []string {
	var out []string
	ip := ipnet.IP.To4()
	if ip == nil {
		return out
	}
	for i := 1; i < 254 && len(out) < limit; i++ {
		c := make(net.IP, len(ip))
		copy(c, ip)
		c[3] = byte(i)
		if ipnet.Contains(c) {
			out = append(out, c.String())
		}
	}
	return out
}

func (h *Handler) handleConfigSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		writeError(w, http.StatusBadRequest, "content required")
		return
	}
	sum := sha1.Sum([]byte(req.Content))
	hash := hex.EncodeToString(sum[:])
	if err := h.repo.SaveConfigSnapshot(r.Context(), id, hash, req.Content, ""); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hash": hash})
}

func (h *Handler) handleBackupDrill(w http.ResponseWriter, r *http.Request) {
	if err := RunBackupDrill(r.Context(), h.system, h.repo); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "message": "backup drill completed"})
}

func (h *Handler) handleBackupDrillReports(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListBackupDrillReports(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}
