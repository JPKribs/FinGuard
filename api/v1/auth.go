package v1

import (
	"net/http"
	"strings"
)

// MARK: authMiddleware
// Authorize transactions using middleware and the admin token.
func (a *APIServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := a.extractToken(r)
		expectedToken := a.cfg.Server.AdminToken

		if expectedToken == "" {
			a.respondWithError(w, http.StatusInternalServerError, "Admin token not configured")
			return
		}

		if token != expectedToken {
			a.respondWithError(w, http.StatusUnauthorized, "Invalid or missing authentication token")
			return
		}

		next(w, r)
	}
}

// MARK: extractToken
// Get the token from the configuration.
func (a *APIServer) extractToken(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	return r.URL.Query().Get("token")
}
