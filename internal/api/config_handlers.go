package api

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

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
