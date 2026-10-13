package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWriteSuccess 验证创建结果使用约定的 data/meta.requestId 响应结构。
// 只使用内存响应记录器，不启动服务器或连接数据库；同时检查状态码和 JSON 字段名。
func TestWriteSuccess(t *testing.T) {
	recorder := httptest.NewRecorder()
	data := map[string]string{"id": "folder-id"}
	if err := writeSuccess(recorder, http.StatusCreated, "test-request-id", data); err != nil {
		t.Fatalf("写入成功响应失败：%v", err)
	}
	result := recorder.Result()
	t.Cleanup(func() { _ = result.Body.Close() })
	if result.StatusCode != http.StatusCreated {
		t.Fatalf("状态码不正确：得到 %d，期望 %d", result.StatusCode, http.StatusCreated)
	}
	if result.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("响应必须是 JSON：%q", result.Header.Get("Content-Type"))
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("解析响应失败：%v", err)
	}
	if len(envelope) != 2 {
		t.Fatalf("成功响应只应包含 data 和 meta：%s", recorder.Body.Bytes())
	}
	var actualData map[string]string
	if err := json.Unmarshal(envelope["data"], &actualData); err != nil || actualData["id"] != data["id"] {
		t.Fatalf("data 内容不正确：%s，解析错误：%v", envelope["data"], err)
	}
	var meta map[string]string
	if err := json.Unmarshal(envelope["meta"], &meta); err != nil {
		t.Fatalf("解析 meta 失败：%v", err)
	}
	if len(meta) != 1 || meta["requestId"] != "test-request-id" {
		t.Fatalf("meta 必须包含准确的 requestId 字段：%s", envelope["meta"])
	}
}

// TestWriteError 验证错误响应使用 error/meta.requestId，并保留准确的小写字段名。
// 两个内存案例分别检查无字段错误时省略 fields，以及有字段错误时包含 fields；不连接数据库。
func TestWriteError(t *testing.T) {
	tests := []struct {
		name       string
		details    errorDetails
		wantFields bool
	}{
		{
			name:       "无字段错误省略 fields",
			details:    errorDetails{Code: "invalid_json", Message: "请求体不是有效 JSON", Retryable: false},
			wantFields: false,
		},
		{
			name:       "字段错误包含 fields",
			details:    errorDetails{Code: "validation_failed", Message: "请求参数不符合要求", Fields: map[string]string{"name": "不能为空"}, Retryable: false},
			wantFields: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			if err := writeError(recorder, http.StatusBadRequest, "test-request-id", tt.details); err != nil {
				t.Fatalf("写入错误响应失败：%v", err)
			}
			result := recorder.Result()
			t.Cleanup(func() { _ = result.Body.Close() })
			if result.StatusCode != http.StatusBadRequest || result.Header.Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("状态码或 Content-Type 不正确：%d，%q", result.StatusCode, result.Header.Get("Content-Type"))
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || len(envelope) != 2 {
				t.Fatalf("错误响应必须包含 error 与 meta：%s，解析错误：%v", recorder.Body.Bytes(), err)
			}
			var details map[string]json.RawMessage
			if err := json.Unmarshal(envelope["error"], &details); err != nil {
				t.Fatalf("解析 error 失败：%v", err)
			}
			for _, field := range []string{"code", "message", "retryable"} {
				if _, ok := details[field]; !ok {
					t.Fatalf("error 缺少准确的 %s 字段：%s", field, envelope["error"])
				}
			}
			_, hasFields := details["fields"]
			wantCount := 3
			if tt.wantFields {
				wantCount++
			}
			if hasFields != tt.wantFields || len(details) != wantCount {
				t.Fatalf("error 的可选 fields 不符合预期：%s", envelope["error"])
			}
			var message string
			if err := json.Unmarshal(details["message"], &message); err != nil || message != tt.details.Message {
				t.Fatalf("message 内容不正确：%s，解析错误：%v", details["message"], err)
			}
			var retryable bool
			if err := json.Unmarshal(details["retryable"], &retryable); err != nil || retryable != tt.details.Retryable {
				t.Fatalf("retryable 内容不正确：%s，解析错误：%v", details["retryable"], err)
			}
			var meta map[string]string
			if err := json.Unmarshal(envelope["meta"], &meta); err != nil || len(meta) != 1 || meta["requestId"] != "test-request-id" {
				t.Fatalf("meta.requestId 不正确：%s，解析错误：%v", envelope["meta"], err)
			}
		})
	}
}
