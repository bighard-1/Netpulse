package api

import "net/http"

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
