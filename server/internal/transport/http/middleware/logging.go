package middleware

import (
	"bufio"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/edhases/oxide-server/internal/logging"
)

// statusWriter captures the status code and the number of response bytes.
type statusWriter struct {
	http.ResponseWriter
	status   int
	written  int64
	hijacked bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.written += int64(n)
	return n, err
}

// Hijack/Flush/Unwrap keep WebSocket upgrades and http.ResponseController
// working through the wrapper; gorilla/websocket requires http.Hijacker.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	w.hijacked = true
	return hj.Hijack()
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// RequestLogger emits exactly one structured record per request.
//
// Only r.URL.Path is logged, never r.URL.RequestURI or r.URL.RawQuery: OAuth
// callbacks carry `code`/`state` and Telegram carries `hash` in the query
// string, and those are single-use credentials that must not land in logs.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}

		next.ServeHTTP(sw, r)

		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		if sw.hijacked {
			// WebSocket upgrade: the 101 is written straight to the connection.
			status = http.StatusSwitchingProtocols
		}

		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelWarn
		}

		traceID, userID := "", ""
		if st := reqStateFrom(r.Context()); st != nil {
			traceID, userID = st.values()
		}

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.Int64("bytes", sw.written),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("remote_ip", extractIP(r)),
			slog.String("trace_id", traceID),
		}
		if userID != "" {
			attrs = append(attrs, slog.String("user_id", userID))
		}

		logging.L().LogAttrs(r.Context(), level, "http_request", attrs...)
	})
}
