package middleware

import "net/http"

// DefaultMaxBodyBytes is the global ceiling for request bodies (1 MiB).
const DefaultMaxBodyBytes int64 = 1 << 20

// BodyLimit rejects oversized request bodies with 413 before a handler can
// allocate them. Known bodies (Content-Length) are rejected up front; streamed
// or chunked bodies are cut off with http.MaxBytesReader.
//
// max <= 0 disables the limit.
func BodyLimit(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if max <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > max {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				_, _ = w.Write([]byte(`{"error":"request body too large"}`))
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, max)
			}
			next.ServeHTTP(w, r)
		})
	}
}
