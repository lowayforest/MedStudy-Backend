package auth

import "context"

type Repository interface {
	//创建用户
	CreateUser(ctx context.Context, input CreateUserInput) (User, error)

	//使用用户名查找用户
	FindByUsername(ctx context.Context, username string) (User, string, error)
}
