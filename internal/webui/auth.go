package webui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/junjiewwang/perf-analysis/pkg/auth"
	"github.com/junjiewwang/perf-analysis/pkg/config"
)

// authMiddleware returns an HTTP middleware that validates HMAC signed URL tokens.
// When auth is disabled, it passes all requests through without validation.
func authMiddleware(authCfg *config.ViewAuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for static assets
			if isStaticAsset(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			if !authCfg.Enabled || authCfg.Secret == "" {
				next.ServeHTTP(w, r)
				return
			}

			taskID := r.URL.Query().Get("task")
			token := r.URL.Query().Get("token")
			expStr := r.URL.Query().Get("exp")

			// If no task parameter, allow through (e.g., listing page without auth)
			if taskID == "" {
				next.ServeHTTP(w, r)
				return
			}

			// If task is specified but token is missing, deny
			if token == "" || expStr == "" {
				http.Error(w, "unauthorized: missing token or expiration", http.StatusUnauthorized)
				return
			}

			expireAt, err := strconv.ParseInt(expStr, 10, 64)
			if err != nil {
				http.Error(w, "unauthorized: invalid expiration", http.StatusUnauthorized)
				return
			}

			if !auth.ValidateViewToken(authCfg.Secret, taskID, expireAt, token) {
				http.Error(w, "unauthorized: invalid or expired token", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// iframeMiddleware returns an HTTP middleware that sets Content-Security-Policy
// frame-ancestors header based on allowed origins configuration.
func iframeMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowedOrigins) > 0 {
				origins := strings.Join(allowedOrigins, " ")
				w.Header().Set("Content-Security-Policy", "frame-ancestors 'self' "+origins)
				w.Header().Set("X-Frame-Options", "ALLOW-FROM "+allowedOrigins[0])
			} else {
				w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
				w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isStaticAsset checks if the request path is a static asset.
func isStaticAsset(path string) bool {
	staticPrefixes := []string{"/static/", "/css/", "/js/"}
	for _, prefix := range staticPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	staticExtensions := []string{".css", ".js", ".png", ".jpg", ".ico", ".svg", ".woff", ".woff2", ".ttf"}
	for _, ext := range staticExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// chainMiddleware chains multiple middleware in order (first applied outermost).
func chainMiddleware(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
