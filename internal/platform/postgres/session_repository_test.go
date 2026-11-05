package postgres

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/lowayforest/MedStudy-Backend/internal/auth"
)

// TestSessionRepositoryCreateIntegration 验证首次登录会话保存用户、token 哈希和到期时间
// 只有显式开启时才使用新建的隔离 PostgreSQL 容器，不读取开发数据库或 .env
func TestSessionRepositoryCreateIntegration(t *testing.T) {
	if os.Getenv("MEDSTUDY_TEST_SESSION_DOCKER") != "1" {
		t.Skip("未开启隔离数据库测试，真实会话写入尚未验收")
	}

	pool := newIsolatedFolderTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const userID = "00000000-0000-4000-8000-000000000001"
	_, err := pool.Exec(ctx, `insert into public.users (id, username, password_hash)
		values ($1, 'SessionFixture', '$argon2id$fixture-only-not-a-login-hash')`, userID)
	if err != nil {
		t.Fatal("准备隔离测试用户失败")
	}

	const raw = "session-fixture-token"
	// PostgreSQL 时间精度为微秒，测试时间先截断到相同精度
	expiresAt := time.Now().UTC().Truncate(time.Microsecond).Add(time.Hour)
	hash := auth.HashRefreshToken(raw)
	repo := NewSessionRepository(pool)
	created, err := repo.Create(ctx, auth.CreateSessionInput{
		UserID: userID, TokenHash: hash, ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("创建会话失败：%v", err)
	}
	if created.ID == "" || created.FamilyID == "" || created.UserID != userID ||
		!created.ExpiresAt.Equal(expiresAt) {
		t.Fatal("仓储应返回数据库生成的会话 ID、family ID，以及传入的用户和到期时间")
	}

	// 再读数据库，确认实际保存的是 32 字节哈希，而不是原始 token
	var storedHash []byte
	var storedUserID, storedFamilyID string
	var storedExpiresAt time.Time
	err = pool.QueryRow(ctx, `select user_id::text, token_hash, family_id::text, expires_at
		from public.sessions where id = $1`, created.ID).Scan(
		&storedUserID, &storedHash, &storedFamilyID, &storedExpiresAt,
	)
	if err != nil {
		t.Fatalf("查询已创建会话失败：%v", err)
	}
	if storedUserID != userID || storedFamilyID != created.FamilyID ||
		!storedExpiresAt.Equal(expiresAt) || !bytes.Equal(storedHash, hash[:]) {
		t.Fatal("数据库中的用户、family、到期时间或 token 哈希不符合预期")
	}
}
