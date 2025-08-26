package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-portfolio/websocket-chat/internal/auth"
)

type ctxKey string

const CtxUserKey ctxKey = "user"

// =========================
// AuthMiddleware проверяет cookie JWT
// =========================
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing auth cookie"})
			return
		}

		userName, err := auth.ParseJWT(c.Value)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
			return
		}

		ctx := context.WithValue(r.Context(), CtxUserKey, userName)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
