package api

import (
	"net/http"
	"strings"

	"github.com/harshalvk/cage/internal/auth"
	"github.com/harshalvk/cage/internal/store"
)

func (a *API) AuthMiddleware(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "missing Authorization header", http.StatusUnauthorized)
				return
			}

			rawKey := strings.TrimPrefix(authHeader, "Bearer ")
			keyHash := auth.HashKey(rawKey)

			valid, err := st.ValidateAPIKey(r.Context(), keyHash)
			if err != nil {
				http.Error(w, "failed to validate api key", http.StatusInternalServerError)
				return
			}
			if !valid {
				http.Error(w, "invalid or revoked api key", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
