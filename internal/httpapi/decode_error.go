package httpapi

import (
	"errors"
	"net/http"
)

// writeDecodeError 将 JSON 请求解析错误转换为统一 HTTP 错误响应。
// 只返回安全的错误消息；响应写入失败时返回错误。
func writeDecodeError(w http.ResponseWriter, requestID string, err error) error {
	// 判断错误类型，选出状态码和 errorDetails
	status := http.StatusBadRequest
	details := errorDetails{
		Code:    "invalid_json",
		Message: "请求体不是有效 JSON",
	}

	var tooLarge *http.MaxBytesError

	if errors.Is(err, ErrUnsupportedMediaType) {
		status = http.StatusUnsupportedMediaType
		details = errorDetails{
			Code:    "unsupported_media_type",
			Message: "不支持的 Content-Type/文档格式",
		}
	}

	if errors.As(err, &tooLarge) {
		status = http.StatusRequestEntityTooLarge
		details = errorDetails{
			Code:    "request_too_large",
			Message: "上传文件、JSON body 或 AI 上下文过大",
		}
	}

	// 最后调用 writeError

	return writeError(w, status, requestID, details)
}
