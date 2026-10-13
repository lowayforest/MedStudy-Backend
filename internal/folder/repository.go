package folder

import "context"

// 定义仓储接口
type Repository interface {
	Create(ctx context.Context, userID string, input CreateInput) (Folder, error)
}
