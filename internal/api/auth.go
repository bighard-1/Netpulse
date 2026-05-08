package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"netpulse/internal/db"
)

type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Client   string `json:"client"`
	jwt.RegisteredClaims
}

type ctxKey string

const userCtxKey ctxKey = "auth_user"

type AuthUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Client   string `json:"client"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) handleLogin(client string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if h.isLocked(req.Username) {
			writeError(w, http.StatusTooManyRequests, "account temporarily locked, retry later")
			return
		}
		u, err := h.repo.GetUserByUsername(r.Context(), req.Username)
		if err != nil || u == nil {
			h.recordFail(req.Username)
			writeError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
			h.recordFail(req.Username)
			writeError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		h.clearFail(req.Username)
		if client == "mobile" && u.Role != "user" {
			writeError(w, http.StatusForbidden, "mobile only supports normal user login")
			return
		}
		if client == "web" && u.Role == "admin" {
			// allowed
		}
		token, err := h.issueToken(u.Username, u.Role, client)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token issue failed")
			return
		}
		_ = h.repo.AddAuditLog(r.Context(), db.AuditLog{
			UserID:     &u.ID,
			Action:     "LOGIN",
			Target:     "client=" + client,
			Method:     r.Method,
			Path:       r.URL.Path,
			IP:         clientIP(r),
			StatusCode: http.StatusOK,
			DurationMS: 0,
			Client:     client,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"token": token,
			"user": map[string]string{
				"username": u.Username,
				"role":     u.Role,
			},
		})
	}
}

func (h *Handler) isLocked(username string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	until, ok := h.lockedUntil[username]
	return ok && until.After(time.Now())
}

func (h *Handler) recordFail(username string) {
	if username == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fails[username]++
	if h.fails[username] >= 5 {
		h.lockedUntil[username] = time.Now().Add(15 * time.Minute)
		h.fails[username] = 0
	}
}

func (h *Handler) clearFail(username string) {
	if username == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.fails, username)
	delete(h.lockedUntil, username)
}

func (h *Handler) issueToken(username, role, client string) (string, error) {
	claims := Claims{
		Username: username,
		Role:     role,
		Client:   client,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(h.jwtSecret))
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		raw := strings.TrimPrefix(auth, "Bearer ")
		token, err := jwt.ParseWithClaims(raw, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(h.jwtSecret), nil
		})
		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		claims, ok := token.Claims.(*Claims)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid token claims")
			return
		}
		dbUser, err := h.repo.GetUserByUsername(r.Context(), claims.Username)
		if err != nil || dbUser == nil {
			writeError(w, http.StatusUnauthorized, "invalid token user")
			return
		}
		// Always trust current DB role so role changes take effect immediately.
		u := AuthUser{ID: dbUser.ID, Username: claims.Username, Role: dbUser.Role, Client: claims.Client}
		ctx := context.WithValue(r.Context(), userCtxKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r.Context())
		if u.Role != "admin" {
			writeError(w, http.StatusForbidden, "admin only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(ctx context.Context) AuthUser {
	v := ctx.Value(userCtxKey)
	if v == nil {
		return AuthUser{}
	}
	u, _ := v.(AuthUser)
	return u
}

func tokenClient(ctx context.Context) string {
	u := currentUser(ctx)
	if u.Client == "" {
		return "unknown"
	}
	return u.Client
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}
