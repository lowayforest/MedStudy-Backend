package auth

import "context"

type Repository interface {
	CreateUser(ctx context.Context, input CreateUserInput) (User, error)
}
