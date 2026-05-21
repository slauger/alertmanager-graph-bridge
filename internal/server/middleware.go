package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

// recoverMiddleware turns a panic in any downstream handler into a 500 response
// instead of crashing the connection, and records it.
func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.metrics.PanicsRecovered.Inc()
				s.logger.Error("recovered from panic in HTTP handler",
					"method", r.Method, "path", r.URL.Path, "panic", rec)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// authMiddleware enforces bearer-token authentication when a token is
// configured. With an empty token the webhook endpoint is unauthenticated.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.BearerToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		header := r.Header.Get("Authorization")
		token := strings.TrimPrefix(header, bearerPrefix)
		valid := strings.HasPrefix(header, bearerPrefix) &&
			subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.BearerToken)) == 1
		if !valid {
			s.metrics.WebhookRequests.WithLabelValues("unauthorized").Inc()
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
