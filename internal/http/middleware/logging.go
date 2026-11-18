package middleware

import (
	"log/slog"
	"net/http"
)

const unmatchedRoute = "unmatched"

func Logging(logger *slog.Logger, resolve RouteResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			track(w, r, next, resolve(r), func(result requestResult) {
				route := result.route
				if route == "" {
					route = unmatchedRoute
				}

				level := slog.LevelInfo
				if result.status >= http.StatusInternalServerError {
					level = slog.LevelError
				}

				logger.Log(r.Context(), level, "request handled",
					slog.String("method", r.Method),
					slog.String("route", route),
					slog.Int("status", result.status),
					slog.Int64("duration_ms", result.duration.Milliseconds()),
				)
			})
		})
	}
}
