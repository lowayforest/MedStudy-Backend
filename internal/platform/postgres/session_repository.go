package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lowayforest/MedStudy-Backend/internal/auth"
)

// SessionRepository 使用共享的 PostgreSQL 连接池保存 refresh 会话
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository 使用 pool 创建会话仓储
// pool 是调用方管理的共享连接池，返回的仓储不负责关闭它
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// Create 将 input 中的用户 ID、token 哈希和到期时间写入 sessions 表
// ctx 用于控制数据库操作的取消和超时
// 成功返回包含数据库生成的 ID 和 FamilyID 的 Session，失败返回零值 Session 和错误
func (r *SessionRepository) Create(ctx context.Context, input auth.CreateSessionInput) (auth.Session, error) {
	//定义SQL
	const query = `insert into public.sessions (user_id, token_hash, expires_at) values ($1, $2, $3) returning id::text, user_id::text, family_id::text, expires_at`
	var created auth.Session
	err := r.pool.QueryRow(ctx, query, input.UserID, input.TokenHash[:], input.ExpiresAt).Scan(
		&created.ID,
		&created.UserID,
		&created.FamilyID,
		&created.ExpiresAt,
	)

	if err != nil {
		return auth.Session{}, err
	}

	return created, nil

}
