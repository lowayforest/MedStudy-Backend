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
