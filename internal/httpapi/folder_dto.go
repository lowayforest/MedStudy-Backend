package httpapi

import "github.com/lowayforest/MedStudy-Backend/internal/folder"

// 客户端提交的创建参数
type createFolderRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// 返回给客户端的资料夹数据
type folderResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	CreatedAt int64  `json:"createdAt"`
}

// 将业务资料夹转化成HTTP响应数据
func newFolderResponse(value folder.Folder) folderResponse {
	return folderResponse{ID: value.ID, Name: value.Name, Type: value.Type, CreatedAt: value.CreatedAt.UnixMilli()}
}
