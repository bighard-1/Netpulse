package api

import (
	"encoding/json"
	"net/http"

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
