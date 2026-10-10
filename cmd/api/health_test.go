package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthLive 验证 GET /health/live 返回 200、JSON 响应头及唯一字段 status=ok。
// 请求直接经过真实路由，不需要启动 HTTP 服务器或连接数据库。
func TestHealthLive(t *testing.T) {
	// 准备请求和响应记录器；记录器代替网络客户端接收响应。
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	recorder := httptest.NewRecorder()
	router := newRouter()

	// 调用路由，让它选择处理函数并把响应写入记录器。
	router.ServeHTTP(recorder, request)

	// 检查状态码和响应头，确认接口成功并声明返回 JSON。
	if recorder.Code != http.StatusOK {
		t.Fatalf("期待状态码: %d, 实际得到: %d", http.StatusOK, recorder.Code)
	}
	contetntType := recorder.Header().Get("Content-Type")
	if contetntType != "application/json; charset=utf-8" {
		t.Fatalf("相应正文不是有效的 JSON: %s", contetntType)
	}
	// 把响应 JSON 解码到 map，再检查字段数量与内容，而不是只看输出日志。
	var body map[string]string
	err := json.Unmarshal(recorder.Body.Bytes(), &body)
	if err != nil {
		t.Fatalf("相应正文不是有效 JSON: %v", err)
	}

	if len(body) != 1 || body["status"] != "ok" {
		t.Fatalf("期望正文只有 status=ok, 实际得到: %v", body)
	}
}

// TestHealthLiveMethodNotAllowed 验证向仅支持 GET 的健康接口发送 POST 时返回 405。
func TestHealthLiveMethodNotAllowed(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/health/live", nil)
	recorder := httptest.NewRecorder()
	router := newRouter()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("期待状态码: %d, 实际得到: %d", http.StatusMethodNotAllowed, recorder.Code)
	}

}

// TestRouteNotFound 验证 GET 请求访问未注册的 /not-found 路径时返回 404。
func TestRouteNotFound(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/not-found", nil)
	recorder := httptest.NewRecorder()
	router := newRouter()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("期望状态码 404, 实际得到: %d", recorder.Code)
	}
}
