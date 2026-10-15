package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lowayforest/MedStudy-Backend/internal/folder"
)

type folderHandlerRepository struct {
	calls  int
	ctx    context.Context
	userID string
	input  folder.CreateInput
	err    error
}

// Create 记录 Handler 经 Service 传来的可信用户、请求 context 和标准化输入。
// 这是仅存在于测试内存中的假仓储；不会访问数据库。
func (repo *folderHandlerRepository) Create(ctx context.Context, userID string, input folder.CreateInput) (folder.Folder, error) {
	repo.calls++
	repo.ctx = ctx
	repo.userID = userID
	repo.input = input
	if repo.err != nil {
		return folder.Folder{}, repo.err
	}
	return folder.Folder{
		ID:        "00000000-0000-4000-8000-000000000010",
		UserID:    userID,
		Name:      input.Name,
		Type:      input.Type,
		CreatedAt: time.UnixMilli(1787712000123),
	}, nil
}

// TestFolderHandlerCreateSuccess 验证合法请求携带可信用户 ID 和 context 进入 Service，返回 201 响应。
// 请求中仅有 name/type；使用内存请求与假仓储，不注册路由或连接数据库。
func TestFolderHandlerCreateSuccess(t *testing.T) {
	repo := &folderHandlerRepository{}
	handler := NewFolderHandler(folder.NewService(repo))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/folders", strings.NewReader(`{"name":" 口腔解剖 ","type":"material"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	if err := handler.Create(recorder, request, "trusted-user", "test-request-id"); err != nil {
		t.Fatalf("创建资料夹响应写入失败：%v", err)
	}
	if repo.calls != 1 || repo.userID != "trusted-user" || repo.input.Name != "口腔解剖" || repo.input.Type != "material" || repo.ctx != request.Context() {
		t.Fatalf("Service 未收到可信用户、请求 context 或标准化输入：%+v", repo)
	}
	result := recorder.Result()
	t.Cleanup(func() { _ = result.Body.Close() })
	if result.StatusCode != http.StatusCreated || result.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("成功响应状态或类型不正确：%d，%q", result.StatusCode, result.Header.Get("Content-Type"))
	}
	var body struct {
		Data folderResponse `json:"data"`
		Meta responseMeta   `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析成功响应失败：%v", err)
	}
	if body.Data.ID != "00000000-0000-4000-8000-000000000010" || body.Data.Name != "口腔解剖" || body.Data.Type != "material" || body.Data.CreatedAt != 1787712000123 || body.Meta.RequestID != "test-request-id" {
		t.Fatalf("成功响应内容不正确：%+v", body)
	}
	if strings.Contains(recorder.Body.String(), "trusted-user") {
		t.Fatal("响应不应暴露可信用户 ID")
	}
}

// TestFolderHandlerCreateFailures 验证缺少身份、非法输入和仓储故障分别映射为受控 HTTP 错误。
// 每例创建独立的内存请求和假仓储；失败前不应调用仓储，数据库故障详情不应出现在响应中。
func TestFolderHandlerCreateFailures(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		body       string
		repoErr    error
		wantStatus int
		wantCode   string
		wantField  string
		wantCalls  int
	}{
		{
			name: "缺少可信身份返回 401", userID: "", body: `{"name":"英语","type":"material"}`,
			wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "空白名称返回字段错误", userID: "trusted-user", body: `{"name":"  ","type":"material"}`,
			wantStatus: http.StatusBadRequest, wantCode: "validation_failed", wantField: "name",
		},
		{
			name: "非法类型返回字段错误", userID: "trusted-user", body: `{"name":"英语","type":"other"}`,
			wantStatus: http.StatusBadRequest, wantCode: "validation_failed", wantField: "type",
		},
		{
			name: "客户端用户 ID 不被接受", userID: "trusted-user", body: `{"name":"英语","type":"material","userID":"other-user"}`,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_json",
		},
		{
			name: "仓储错误返回安全的 500", userID: "trusted-user", body: `{"name":"英语","type":"material"}`,
			repoErr: errors.New("private repository diagnostic"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error", wantCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &folderHandlerRepository{err: tt.repoErr}
			handler := NewFolderHandler(folder.NewService(repo))
			request := httptest.NewRequest(http.MethodPost, "/api/v1/folders", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			if err := handler.Create(recorder, request, tt.userID, "test-request-id"); err != nil {
				t.Fatalf("写入受控错误响应失败：%v", err)
			}
			if repo.calls != tt.wantCalls {
				t.Fatalf("仓储调用次数不正确：得到 %d，期望 %d", repo.calls, tt.wantCalls)
			}
			result := recorder.Result()
			t.Cleanup(func() { _ = result.Body.Close() })
			if result.StatusCode != tt.wantStatus || result.Header.Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("错误响应状态或类型不正确：%d，%q", result.StatusCode, result.Header.Get("Content-Type"))
			}
			var body struct {
				Error errorDetails `json:"error"`
				Meta  responseMeta `json:"meta"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("解析错误响应失败：%v", err)
			}
			if body.Error.Code != tt.wantCode || body.Error.Message == "" || body.Meta.RequestID != "test-request-id" {
				t.Fatalf("错误码、消息或 requestId 不正确：%+v", body)
			}
			if tt.wantField != "" && body.Error.Fields[tt.wantField] == "" {
				t.Fatalf("缺少 %s 字段错误：%+v", tt.wantField, body.Error.Fields)
			}
			if strings.Contains(recorder.Body.String(), "private repository diagnostic") {
				t.Fatal("响应泄露了仓储内部错误详情")
			}
		})
	}
}
