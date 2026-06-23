package auditlog

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyActor
	keyClientIP
)

// WithRequestID stores the request id in the context for later audit events.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// WithActor stores the identified caller (user/SA id or username) in the context.
func WithActor(ctx context.Context, a string) context.Context {
	return context.WithValue(ctx, keyActor, a)
}

// WithClientIP stores the resolved client IP in the context.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, keyClientIP, ip)
}

func requestIDFrom(ctx context.Context) string { s, _ := ctx.Value(keyRequestID).(string); return s }
func clientIPFrom(ctx context.Context) string  { s, _ := ctx.Value(keyClientIP).(string); return s }

func actorFrom(ctx context.Context) string {
	if s, ok := ctx.Value(keyActor).(string); ok && s != "" {
		return s
	}
	return "anonymous"
}

// clientIP resolves the caller's source address: first hop of X-Forwarded-For,
// then X-Real-IP, then the connection's RemoteAddr.
func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i >= 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	return r.RemoteAddr
}

// HTTPMiddleware seeds request_id and client_ip into the request context.
// Actor is added later by each service's auth middleware once identified.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithRequestID(r.Context(), r.Header.Get("X-Request-ID"))
		ctx = WithClientIP(ctx, clientIP(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
