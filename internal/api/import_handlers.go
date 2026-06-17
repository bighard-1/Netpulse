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
