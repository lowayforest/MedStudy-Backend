package httpapi

import (
	"errors"
	"net/http"

	"github.com/lowayforest/MedStudy-Backend/internal/folder"
)

type FolderHandler struct {
	service *folder.Service
}

func NewFolderHandler(service *folder.Service) *FolderHandler {
	return &FolderHandler{service: service}
}

// Create 读取创建资料夹请求，调用业务服务，并写入统一 HTTP 响应。
// userID 必须由服务端认证层提供；响应写入失败时返回错误。
func (h *FolderHandler) Create(w http.ResponseWriter, r *http.Request, userID, requestID string) error {
	if userID == "" {
		return writeError(w, http.StatusUnauthorized, requestID, errorDetails{Code: "unauthorized", Message: "请先登录"})
	}

	var body createFolderRequest

	if err := decodeJSON(w, r, &body); err != nil {
		return writeDecodeError(w, requestID, err)
	}

	created, err := h.service.Create(r.Context(), userID, folder.CreateInput{
		Name: body.Name,
		Type: body.Type,
	})

	if err != nil {
		if errors.Is(err, folder.ErrInvalidName) {
			return writeError(w, http.StatusBadRequest, requestID, errorDetails{
				Code:    "validation_failed",
				Message: "请求参数不符合要求",
				Fields:  map[string]string{"name": "名称去除首尾空格后须为 1 到 100 个字符"},
			})
		}

		if errors.Is(err, folder.ErrInvalidType) {
			return writeError(w, http.StatusBadRequest, requestID, errorDetails{
				Code:    "validation_failed",
				Message: "请求参数不符合要求",
				Fields:  map[string]string{"type": "类型只能是 material 或 exam"},
			})
		}

		return writeError(w, http.StatusInternalServerError, requestID, errorDetails{
			Code:    "internal_error",
			Message: "服务器内部错误",
		})
	}

	return writeSuccess(w, http.StatusCreated, requestID, newFolderResponse(created))
}
