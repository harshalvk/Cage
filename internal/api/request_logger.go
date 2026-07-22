package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/harshalvk/cage/internal/metrics"
)

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		reqID := middleware.GetReqID(r.Context())

		next.ServeHTTP(ww, r)

		slog.Info("request completed",
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		routePattern := chi.RouteContext(r.Context()).RoutePattern()

		next.ServeHTTP(ww, r)

		metrics.RequestsTotal.WithLabelValues(routePattern, r.Method, fmt.Sprintf("%d", ww.Status())).Inc()
		metrics.RequestDuration.WithLabelValues(routePattern, r.Method).Observe(float64(time.Since(start).Seconds()))
	})
}
