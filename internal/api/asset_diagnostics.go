package api

import (
	"net/http"
)

func (h *Handler) handleAssetLoadDiagnostics(w http.ResponseWriter, r *http.Request) {
	report, err := h.repo.DiagnoseAssetLoad(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
