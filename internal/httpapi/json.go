package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

// 普通json最多1mib
const maxJSONBodyBytes = 1 << 20

var (
	ErrUnsupportedMediaType = errors.New("请求必须使用 application/json")
	ErrMultipleJSONValues   = errors.New("请求体只能包含一个 JSON 值")
)

// decodeJSON 将请求体中的单个 JSON 值解码到 dst 指向的对象
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	//解析Content-Type
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return ErrUnsupportedMediaType
	}
	//限制请求体大小
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)

	//创建解码器
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	//将第一个 json 值写入调用方提供的对象
	if err := decoder.Decode(dst); err != nil {
		return err
	}

	//确认后面没有第二个 JSON 值
	var extra any
	err = decoder.Decode(&extra)

	if errors.Is(err, io.EOF) {
		return nil
	}

	if err != nil {
		return err
	}

	return ErrMultipleJSONValues
}

// writeJSON 将 value 编码为 JSON，并以 status 作为 HTTP 状态码写入响应。
// 编码失败时不写响应；写入失败时返回错误，由调用方处理。
func writeJSON(w http.ResponseWriter, status int, value any) error {
	// 先用 json.Marshal(value) 编码
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// 成功后设置 Content-Type 为 application/json; charset=utf-8
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// 调用 w.WriteHeader(status)
	w.WriteHeader(status)
	// 最后用 w.Write(...) 写入编码结果
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	return nil
}
