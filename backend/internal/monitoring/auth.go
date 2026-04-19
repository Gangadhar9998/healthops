package monitoring

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

// authMiddleware enforces authentication via JWT Bearer tokens or Basic Auth.
// Public endpoints (login, healthz) are exempt.
// GET requests require authentication but allow any role.
// Mutating requests (POST/PUT/PATCH/DELETE) require "admin" role.
func authMiddleware(cfg AuthConfig, userStore *UserStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Public endpoints — no auth required
		if path == "/healthz" || path == "/readyz" || path == "/metrics" ||
			path == "/api/v1/auth/login" || strings.HasPrefix(path, "/assets/") ||
			path == "/" || path == "/favicon.ico" {
			next.ServeHTTP(w, r)
			return
		}

		// Serve frontend SPA files without auth
		if !strings.HasPrefix(path, "/api/") && path != "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		// If user store is available, use JWT auth
		if userStore != nil {
			claims := ExtractJWTClaims(r)
			if claims == nil {
				// Also try legacy Basic Auth for backward compatibility
				if cfg.Enabled && IsRequestAuthorized(cfg, r) {
					next.ServeHTTP(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"success":false,"error":{"code":401,"message":"authentication required"}}`))
				return
			}

			// Role-based: ops users can only GET
			if isMutatingMethod(r.Method) && claims.Role != RoleAdmin {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"success":false,"error":{"code":403,"message":"admin role required for this action"}}`))
				return
			}

			next.ServeHTTP(w, r)
			return
		}

		// Fallback: legacy Basic Auth
		if cfg.Enabled {
			if isMutatingMethod(r.Method) && !IsRequestAuthorized(cfg, r) {
				RequestAuth(w)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// basicAuthMiddleware returns middleware that enforces basic auth.
// Kept for backward compatibility.
func basicAuthMiddleware(cfg AuthConfig, next http.Handler) http.Handler {
	if !cfg.Enabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isMutatingMethod(r.Method) && !IsRequestAuthorized(cfg, r) {
			RequestAuth(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func IsRequestAuthorized(cfg AuthConfig, r *http.Request) bool {
	if !cfg.Enabled {
		return true
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}
	username, password, ok := parseBasicAuth(auth)
	if !ok {
		return false
	}

	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(cfg.Username)) == 1
	passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(cfg.Password)) == 1
	return usernameMatch && passwordMatch
}

// RequestAuth sends a 401 Unauthorized response with WWW-Authenticate header.
func RequestAuth(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="HealthOps"`)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("Unauthorized\n"))
}

// parseBasicAuth parses the Authorization header and extracts username and password.
// Returns ok=false if the header is malformed or not Basic auth.
func parseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	if !strings.HasPrefix(auth, prefix) {
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return
	}

	creds := strings.SplitN(string(decoded), ":", 2)
	if len(creds) != 2 {
		return
	}

	return creds[0], creds[1], true
}
