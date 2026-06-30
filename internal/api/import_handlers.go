package api

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"netpulse/internal/db"
)

func (h *Handler) handleImportDevices(w http.ResponseWriter, r *http.Request) {
	reader := csv.NewReader(r.Body)
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid csv")
		return
	}
	created := 0
	header := map[string]int{}
	if len(records) > 0 {
		for i, name := range records[0] {
			header[strings.ToLower(strings.TrimSpace(name))] = i
		}
	}
	field := func(rec []string, names ...string) string {
		for _, name := range names {
			if idx, ok := header[name]; ok && idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
		}
		return ""
	}
	for i, rec := range records {
		if i == 0 {
			continue
		}
		if len(rec) < 1 {
			continue
		}
		port := 161
		if raw := field(rec, "snmp_port", "port"); raw != "" {
			if p, err := strconv.Atoi(raw); err == nil && p > 0 {
				port = p
			}
		}
		readCommunity := field(rec, "read_community", "community")
		dev := db.Device{
			IP:             field(rec, "ip"),
			Name:           field(rec, "name"),
			Brand:          field(rec, "brand"),
			Community:      readCommunity,
			WriteCommunity: field(rec, "write_community"),
			SNMPVersion:    field(rec, "snmp_version"),
			Remark:         field(rec, "remark"),
			SNMPPort:       port,
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
