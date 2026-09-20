package server

import (
	"log/slog"
	"net/http"
	"time"
)

// loggingMiddleware traces each API request at DEBUG. It's not INFO because
// the UI polls /bot/status and would drown the log.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slog.Default().Enabled(r.Context(), slog.LevelDebug) {
			next.ServeHTTP(w, r)
			return
		}

		rec := &statusRecorder{ResponseWriter: w}
		start := time.Now()

		next.ServeHTTP(rec, r)

		status := rec.status
		if status == 0 {
			// A handler that writes nothing gets net/http's implicit 200.
			status = http.StatusOK
		}

		slog.Debug("api request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"durationMs", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

// statusRecorder remembers the first status a handler writes.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
