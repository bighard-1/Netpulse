package api

import (
	"encoding/json"
	"net/http"
)

type updateInterfaceRequest struct {
	Name   *string `json:"name"`
	Remark *string `json:"remark"`
}

func (h *Handler) handleUpdateInterfaceRemark(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid interface id")
		return
	}
	var req updateRemarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := h.repo.UpdateInterfaceRemark(r.Context(), id, req.Remark); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "interface remark updated"})
}

func (h *Handler) handleGetInterface(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid interface id")
		return
	}
	item, err := h.repo.GetInterfaceByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "interface not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) handleUpdateInterfaceProfile(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid interface id")
		return
	}
	var req updateInterfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Name == nil && req.Remark == nil {
		writeError(w, http.StatusBadRequest, "name or remark is required")
		return
	}
	if err := h.repo.UpdateInterfaceProfile(r.Context(), id, req.Name, req.Remark); err != nil {
		if err.Error() == "interface name conflict in this device" {
			writeError(w, http.StatusConflict, "端口名称在本资产内已存在，请更换")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "interface updated"})
}
