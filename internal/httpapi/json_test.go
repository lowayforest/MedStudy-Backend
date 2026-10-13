package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDecodeJSONAcceptsSingleObject 验证合法 JSON 能填入请求 DTO。
// Content-Type 带 charset，正文前后有空白；这里只用内存中的 HTTP 请求，不连接数据库。
func TestDecodeJSONAcceptsSingleObject(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/folders", strings.NewReader(" \n{\"name\":\"英语\",\"type\":\"material\"}\n "))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	recorder := httptest.NewRecorder()

	var body createFolderRequest
	if err := decodeJSON(recorder, request, &body); err != nil {
		t.Fatalf("合法 JSON 应解码成功：%v", err)
	}
	if body.Name != "英语" || body.Type != "material" {
		t.Fatalf("请求 DTO 内容不正确：%+v", body)
	}
}

// TestDecodeJSONRejectsInvalidInput 验证媒体类型、字段和正文边界均会拒绝非法请求。
// 每个子测试创建独立的内存请求和 DTO，不使用数据库或共享状态。
func TestDecodeJSONRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantErr     error
	}{
		{name: "非 JSON 媒体类型", contentType: "text/plain", body: `{}`, wantErr: ErrUnsupportedMediaType},
		{name: "未知字段", contentType: "application/json", body: `{"name":"英语","type":"material","extra":true}`},
		{name: "损坏的 JSON", contentType: "application/json", body: `{"name":`},
		{name: "空请求体", contentType: "application/json", body: ""},
		{name: "第二个 JSON 值", contentType: "application/json", body: `{} {}`, wantErr: ErrMultipleJSONValues},
		{name: "结尾损坏的 JSON", contentType: "application/json", body: `{} {`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/folders", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", tt.contentType)
			var body createFolderRequest
			err := decodeJSON(httptest.NewRecorder(), request, &body)
			if err == nil {
				t.Fatal("非法请求应返回错误")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("错误类型不正确：得到 %v，期望 %v", err, tt.wantErr)
			}
			if tt.name == "结尾损坏的 JSON" && errors.Is(err, ErrMultipleJSONValues) {
				t.Fatal("结尾解析错误应原样返回，不能误报为第二个完整 JSON 值")
			}
		})
	}
}

// TestDecodeJSONRejectsOversizedBody 验证超过 1 MiB 的正文在读取时返回大小限制错误。
// 测试只分配内存构造请求，不启动服务器，也不修改持久数据。
func TestDecodeJSONRejectsOversizedBody(t *testing.T) {
	oversized := `{"name":"` + strings.Repeat("a", maxJSONBodyBytes) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/folders", strings.NewReader(oversized))
	request.Header.Set("Content-Type", "application/json")
	var body createFolderRequest
	err := decodeJSON(httptest.NewRecorder(), request, &body)
	var tooLarge *http.MaxBytesError
	if !errors.As(err, &tooLarge) {
		t.Fatalf("超大正文应返回 *http.MaxBytesError，得到 %v", err)
	}
	if tooLarge.Limit != maxJSONBodyBytes {
		t.Fatalf("请求体大小上限不正确：得到 %d，期望 %d", tooLarge.Limit, maxJSONBodyBytes)
	}
}
