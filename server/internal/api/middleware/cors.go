package middleware

import (
	"net/http"
	"strings"
)

// CORS lets a browser-hosted client call the API from another origin.
//
// The app on a phone has no origin and never needs this; the app in a
// browser during development does, and so would a web client later. With
// "*" in the list any origin is reflected back. Authentication is a header
// the page sets itself, never a cookie, so reflecting an origin exposes
// nothing a cookie-based API would.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowAll := false
	allowed := map[string]bool{}
	for _, o := range origins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		if len(origins) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || allowed[origin]) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Dev-User")
				h.Set("Access-Control-Expose-Headers", "Retry-After")
				h.Set("Access-Control-Max-Age", "600")
				if r.Method == http.MethodOptions && strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")) != "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
