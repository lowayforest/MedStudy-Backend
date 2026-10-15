package httpapi

import (
	"context"
	"crypto/rand"
	"net/http"
)

type requestIDKey struct{}

// RequestIDFromContext 取出当前请求的 ID；没有时返回空字符串
func RequestIDFromContext(ctx context.Context) string {
	value := ctx.Value(requestIDKey{})
	id, ok := value.(string)
	if !ok {
		return ""
	}
	return id
}

// RequestIDMiddleware 为每个请求生成 ID，并传给下一个 Handler。
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
