package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRequestIDMiddleware 验证中间件把同一个服务端生成的 ID 放入响应头和下游请求，
// 并且不同请求使用不同 ID，不信任客户端自带的同名请求头。
func TestRequestIDMiddleware(t *testing.T) {
	var downstreamID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstreamID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
	handler := RequestIDMiddleware(next)

	var previousID string
	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "/example", nil)
		request.Header.Set("X-Request-ID", "client-controlled")
		recorder := httptest.NewRecorder()
		downstreamID = ""

		handler.ServeHTTP(recorder, request)

		responseID := recorder.Header().Get("X-Request-ID")
		if responseID == "" {
			t.Fatal("响应头应包含服务端生成的请求 ID")
		}
		if responseID == "client-controlled" {
			t.Fatal("服务端不能把客户端提供的请求 ID 当作自己的 ID")
		}
		if downstreamID != responseID {
			t.Fatalf("下游读取的 ID = %q，与响应头 ID = %q 不一致", downstreamID, responseID)
		}
		if previousID != "" && responseID == previousID {
			t.Fatal("不同请求不应重复使用同一个请求 ID")
		}
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("下游状态码 = %d，期望 %d", recorder.Code, http.StatusNoContent)
		}
		previousID = responseID
	}
}

// TestRequestIDFromContextMissing 验证请求尚未经过中间件时，读取请求 ID 安全地返回空字符串。
func TestRequestIDFromContextMissing(t *testing.T) {
	if id := RequestIDFromContext(context.Background()); id != "" {
		t.Fatalf("缺少请求 ID 时返回 %q，期望空字符串", id)
	}
}
