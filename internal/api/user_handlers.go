package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"golang.org/x/crypto/bcrypt"
)

func (h *Handler) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListAuditLogs(r.Context(), 300)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	if err := validatePasswordPolicy(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Role != "admin" && req.Role != "user" {
		req.Role = "user"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash error")
		return
	}
	if err := h.repo.CreateUser(r.Context(), req.Username, string(hash), req.Role); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "user created"})
}

func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.repo.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

type updateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username required")
		return
	}
	if req.Role != "admin" && req.Role != "user" {
		req.Role = "user"
	}
	var hashPtr *string
	if req.Password != "" {
		if err := validatePasswordPolicy(req.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "hash error")
			return
		}
		s := string(hash)
		hashPtr = &s
	}
	if err := h.repo.UpdateUser(r.Context(), id, req.Username, req.Role, hashPtr); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "user updated"})
}

func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	u := currentUser(r.Context())
	if u.ID == id {
		writeError(w, http.StatusBadRequest, "cannot delete self")
		return
	}
	if err := h.repo.DeleteUser(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "user deleted"})
}

func (h *Handler) handleListUserPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	items, err := h.repo.ListUserPermissions(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": items})
}

type userPermissionsReq struct {
	Permissions []string `json:"permissions"`
}

func (h *Handler) handleReplaceUserPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var req userPermissionsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.repo.ReplaceUserPermissions(r.Context(), id, req.Permissions); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "permissions updated"})
}

func validatePasswordPolicy(password string) error {
	if len(password) < 10 {
		return fmt.Errorf("password must be at least 10 characters")
	}
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !(hasUpper && hasLower && hasDigit) {
		return fmt.Errorf("password must include uppercase, lowercase and number")
	}
	return nil
}
