package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lowayforest/MedStudy-Backend/internal/auth"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// CreateUser 将已校验、已哈希的注册数据写入数据库，并返回创建的用户
func (r *UserRepository) CreateUser(ctx context.Context, input auth.CreateUserInput) (auth.User, error) {
	// 定义SQL
	const query = `insert into public.users (username, password_hash, timezone) values ($1, $2, $3) returning id::text, username, timezone, created_at`
	var created auth.User
	err := r.pool.QueryRow(ctx, query, input.Username, input.PasswordHash, input.Timezone).Scan(
		&created.ID,
		&created.Username,
		&created.Timezone,
		&created.CreatedAt,
	)

	if err != nil {
		return auth.User{}, err
	}

	return created, nil

}
