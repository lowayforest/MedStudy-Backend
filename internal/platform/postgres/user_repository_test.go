package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lowayforest/MedStudy-Backend/internal/auth"
)

// TestUserRepositoryCreateIntegration 验证注册用户确实写入隔离数据库，
// 只保存 Argon2id 哈希，并由数据库生成 ID、创建时间和规范化用户名唯一约束。
// 只有显式开启时才启动测试专用 PostgreSQL 容器，不读取 .env 或开发数据库。
func TestUserRepositoryCreateIntegration(t *testing.T) {
	if os.Getenv("MEDSTUDY_TEST_USER_DOCKER") != "1" {
		t.Skip("未开启独立 Docker 用户写入测试，真实注册插入尚未验收")
	}
	pool := newIsolatedFolderTestDatabase(t)
	repo := NewUserRepository(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	passwordHash, err := auth.HashPassword("Abc!defg")
	if err != nil {
		t.Fatal("生成测试密码哈希失败")
	}
	created, err := repo.CreateUser(ctx, auth.CreateUserInput{
		Username: "Alice", PasswordHash: passwordHash, Timezone: "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("合法用户写入失败：%v", err)
	}
	if created.ID == "" || created.Username != "Alice" ||
		created.Timezone != "Asia/Shanghai" || created.CreatedAt.IsZero() {
		t.Fatal("应返回数据库生成的用户 ID、创建时间及保存的用户名和时区")
	}

	// 再读数据库，确认不是只返回了对象、却没有真正保存；密码列不能是明文。
	var storedHash string
	err = pool.QueryRow(ctx, `select password_hash from public.users where id = $1`, created.ID).Scan(&storedHash)
	if err != nil || storedHash != passwordHash || storedHash == "Abc!defg" {
		t.Fatal("数据库应保存传入的 Argon2id 哈希，不能保存明文密码")
	}

	// 登录查询应使用数据库的规范化用户名规则，而不是只按显示名精确匹配。
	found, foundHash, err := repo.FindByUsername(ctx, "  ALICE  ")
	if err != nil || found != created || foundHash != passwordHash {
		t.Fatal("应按规范化用户名找到同一用户及其密码哈希，供登录服务验证")
	}

	// 数据库会规范化用户名，因此大小写不同的同名用户也不能重复注册。
	_, err = repo.CreateUser(ctx, auth.CreateUserInput{
		Username: "alice", PasswordHash: passwordHash, Timezone: "Asia/Shanghai",
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatal("规范化后的重复用户名应由数据库唯一约束拒绝")
	}
}
