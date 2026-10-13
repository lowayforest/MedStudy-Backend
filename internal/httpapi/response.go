package httpapi

import "net/http"

type responseMeta struct {
	RequestID string `json:"requestId"`
}

type successResponse struct {
	Data any          `json:"data"`
	Meta responseMeta `json:"meta"`
}

// writeSuccess 将 data 和 requestID 封装为统一成功响应，并以 status 写出。
// JSON 编码或写入失败时返回错误。
func writeSuccess(w http.ResponseWriter, status int, requestID string, data any) error {
	response := successResponse{
		Data: data,
		Meta: responseMeta{RequestID: requestID},
	}

	return writeJSON(w, status, response)

}
