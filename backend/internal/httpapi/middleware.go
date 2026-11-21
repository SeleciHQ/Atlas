package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"atlas/internal/platform/apperr"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.wrote {
		return
	}
	r.wrote = true
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	written, err := r.ResponseWriter.Write(data)
	r.bytes += written
	return written, err
}

func (r *responseRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

// observe logs every request with structured fields and recovers panics into
// problem responses when the response has not started yet.
func observe(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &responseRecorder{ResponseWriter: w}
			requestID := middleware.GetReqID(r.Context())
			if requestID != "" {
				recorder.Header().Set("X-Request-Id", requestID)
			}

			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(r.Context(), "panic_recovered",
						"panic", fmt.Sprint(recovered),
						"stack", string(debug.Stack()),
						"request_id", requestID,
					)
					if !recorder.wrote {
						WriteError(recorder, r, logger, apperr.New(apperr.KindInternal, "internal_error", "An unexpected error occurred."))
					}
				}

				level := slog.LevelInfo
				switch {
				case recorder.Status() >= http.StatusInternalServerError:
					level = slog.LevelError
				case recorder.Status() >= http.StatusBadRequest:
					level = slog.LevelWarn
				}
				if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
					level = slog.LevelDebug
				}

				logger.Log(r.Context(), level, "http_request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", recorder.Status(),
					"bytes", recorder.bytes,
					"duration_ms", time.Since(start).Milliseconds(),
					"request_id", requestID,
					"remote_ip", r.RemoteAddr,
				)
			}()

			next.ServeHTTP(recorder, r)
		})
	}
}
