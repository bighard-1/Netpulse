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

func (h *Handler) handleUpsertTopology(w http.ResponseWriter, r *http.Request) {
	var t db.TopologyLink
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if t.Protocol == "" {
		t.Protocol = "LLDP"
	}
	id, err := h.repo.UpsertTopologyLink(r.Context(), t)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *Handler) handleListTopology(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListTopologyLinks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleUpsertAlertRule(w http.ResponseWriter, r *http.Request) {
	var ar db.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&ar); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
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

func (h *Handler) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListAlertRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
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
		cpu, _ := h.repo.GetDeviceHistory(r.Context(), d.ID, start, end)
		mem, _ := h.repo.GetDeviceHistory(r.Context(), d.ID, start, end)
		b.WriteString(fmt.Sprintf("%d,%s,%s,%d,%d\n", d.ID, d.IP, d.Status, len(cpu), len(mem)))
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="netpulse_report.csv"`)
	_, _ = io.WriteString(w, b.String())
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
