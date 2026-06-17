package api

import (
	"net/http"
	"strconv"
	"strings"
)

func (h *Handler) handleDeviceLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	level := strings.TrimSpace(r.URL.Query().Get("level"))
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, e := strconv.Atoi(raw); e == nil && n > 0 {
			limit = n
		}
	}
	items, err := h.repo.GetDeviceLogsFiltered(r.Context(), id, level, source, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}
