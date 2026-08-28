package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// apiKeyMiddleware lets a request through only when X-Api-Key matches the
// APP_API_KEY secret. Compares sha256 digests via ConstantTimeCompare so the
// secret length is not leaked.
func apiKeyMiddleware(next http.Handler, apiKey string) http.Handler {
	keySum := sha256.Sum256([]byte(apiKey))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-Api-Key")
		if provided == "" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
			return
		}
		providedSum := sha256.Sum256([]byte(provided))
		if subtle.ConstantTimeCompare(providedSum[:], keySum[:]) != 1 {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
