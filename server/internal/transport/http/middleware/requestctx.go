package middleware

import (
	"context"
	"net/http"
	"sync"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// RequestIDHeader is echoed on every response so clients can correlate a
// failure with the server-side log line.
const RequestIDHeader = "X-Request-Id"

// reqState is shared by the whole handler chain of a single request. Handlers
// run on derived contexts (r.WithContext), so a plain context value set deep in
// the chain is invisible to middleware that wraps it; the shared pointer is not.
type reqState struct {
	mu      sync.RWMutex
	traceID string
	userID  string
}

type reqStateKey struct{}

type userIDKey struct{}

func (s *reqState) setUserID(id string) {
	s.mu.Lock()
	s.userID = id
	s.mu.Unlock()
}

func (s *reqState) values() (traceID, userID string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.traceID, s.userID
}

func reqStateFrom(ctx context.Context) *reqState {
	if s, ok := ctx.Value(reqStateKey{}).(*reqState); ok {
		return s
	}
	return nil
}

// RequestContext resolves the correlation id for the request: chi's own request
// id when chi's RequestID middleware ran first, otherwise a fresh UUID. The id
// is exposed via TraceID and returned to the client as X-Request-Id.
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := chimiddleware.GetReqID(r.Context())
		if traceID == "" {
			traceID = uuid.NewString()
		}

		w.Header().Set(RequestIDHeader, traceID)

		ctx := context.WithValue(r.Context(), reqStateKey{}, &reqState{traceID: traceID})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TraceID returns the correlation id, or "" outside of a request context.
func TraceID(ctx context.Context) string {
	if s := reqStateFrom(ctx); s != nil {
		id, _ := s.values()
		return id
	}
	return ""
}

// UserID returns the authenticated user id when some middleware already resolved
// it (AuthMiddleware, or any caller of WithUserID).
func UserID(ctx context.Context) (string, bool) {
	if s := reqStateFrom(ctx); s != nil {
		if _, uid := s.values(); uid != "" {
			return uid, true
		}
	}
	if uid, ok := ctx.Value(userIDKey{}).(string); ok && uid != "" {
		return uid, true
	}
	if id, ok := GetUserIDFromContext(ctx); ok {
		return id.String(), true
	}
	return "", false
}

// WithUserID records the authenticated user so the request logger can attach it
// to the log record. It must be called with the request context, e.g. from
// AuthMiddleware; the value is published through the shared reqState and thus
// visible to RequestLogger after the handler returns.
func WithUserID(ctx context.Context, userID string) context.Context {
	if s := reqStateFrom(ctx); s != nil {
		s.setUserID(userID)
		return ctx
	}
	return context.WithValue(ctx, userIDKey{}, userID)
}
