package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthLive(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	recorder := httptest.NewRecorder()
	router := newRouter()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("期待状态码: %d, 实际得到: %d", http.StatusOK, recorder.Code)
	}
	contetntType := recorder.Header().Get("Content-Type")
	if contetntType != "application/json; charset=utf-8" {
		t.Fatalf("相应正文不是有效的 JSON: %s", contetntType)
	}
	var body map[string]string
	err := json.Unmarshal(recorder.Body.Bytes(), &body)
	if err != nil {
		t.Fatalf("相应正文不是有效 JSON: %v", err)
	}

	if len(body) != 1 || body["status"] != "ok" {
		t.Fatalf("期望正文只有 status=ok, 实际得到: %v", body)
	}
}

func TestHealthLiveMethodNotAllowed(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/health/live", nil)
	recorder := httptest.NewRecorder()
	router := newRouter()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("期待状态码: %d, 实际得到: %d", http.StatusMethodNotAllowed, recorder.Code)
	}

}

func TestRouteNotFound(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/not-found", nil)
	recorder := httptest.NewRecorder()
	router := newRouter()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("期望状态码 404, 实际得到: %d", recorder.Code)
	}
}
