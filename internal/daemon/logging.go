package daemon

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ssh-tunnel-manager/internal/logging"
)

type loggedResponse struct {
	http.ResponseWriter
	status int
}

func (w *loggedResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggedResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *loggedResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		response := &loggedResponse{ResponseWriter: w}
		next.ServeHTTP(response, r)
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		level := slog.LevelInfo
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			level = slog.LevelDebug
		}
		if status >= 400 {
			level = slog.LevelWarn
		}
		if status >= 500 {
			level = slog.LevelError
		}
		// Never log request/response bodies, headers, or query strings (credentials).
		ctx := s.tunnelContext()
		logging.FromContext(ctx).Log(ctx, level, "API request", "method", r.Method, "path", r.URL.EscapedPath(), "status", status, "elapsed", time.Since(start))
	})
}
