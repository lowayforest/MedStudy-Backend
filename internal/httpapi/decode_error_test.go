package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWriteDecodeError 验证 JSON 解析错误映射为契约中的 400、413、415 和安全错误码。
// 使用内存响应记录器；包装错误模拟调用链，不发送真实请求或连接数据库。
func TestWriteDecodeError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "包装的媒体类型错误映射到 415",
			err:        fmt.Errorf("内部上下文不应外泄: %w", ErrUnsupportedMediaType),
			wantStatus: http.StatusUnsupportedMediaType,
			wantCode:   "unsupported_media_type",
		},
		{
			name:       "包装的请求体超限错误映射到 413",
			err:        fmt.Errorf("内部上下文不应外泄: %w", &http.MaxBytesError{Limit: maxJSONBodyBytes}),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantCode:   "request_too_large",
		},
		{
			name:       "其他解析错误映射到 400",
			err:        errors.New("内部上下文不应外泄"),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			if err := writeDecodeError(recorder, "test-request-id", tt.err); err != nil {
				t.Fatalf("写入受控错误响应失败：%v", err)
			}
			result := recorder.Result()
			t.Cleanup(func() { _ = result.Body.Close() })
			if result.StatusCode != tt.wantStatus {
				t.Fatalf("状态码不正确：得到 %d，期望 %d", result.StatusCode, tt.wantStatus)
			}
			if result.Header.Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("错误响应必须是 JSON：%q", result.Header.Get("Content-Type"))
			}
			var body struct {
				Error errorDetails `json:"error"`
				Meta  responseMeta `json:"meta"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("解析错误响应失败：%v", err)
			}
			if body.Error.Code != tt.wantCode || body.Error.Message == "" || body.Error.Retryable || body.Meta.RequestID != "test-request-id" {
				t.Fatalf("错误详情或 requestId 不正确：%+v", body)
			}
			if strings.Contains(recorder.Body.String(), "内部上下文不应外泄") {
				t.Fatal("响应泄露了原始内部错误文字")
			}
		})
	}
}
