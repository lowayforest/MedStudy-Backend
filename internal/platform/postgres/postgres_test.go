package postgres

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/lowayforest/MedStudy-Backend/internal/config"
)

// TestOpenRejectsInvalidConnectionString 验证无效地址使 Open 返回错误和 nil 连接池。
// 单元测试不连接开发数据库，不需要真实密码。
func TestOpenRejectsInvalidConnectionString(t *testing.T) {
	tests := []struct {
		name       string
		connString string
	}{
		// 地址为空：缺少必需配置，应拒绝。
		{name: "empty", connString: ""},
		// 地址只有空白：去除空白后仍为空，应拒绝。
		{name: "whitespace", connString: " \t\n "},
		// 端口不是数字：连接串无法解析，应拒绝。
		{name: "invalid_port", connString: "postgres://medstudy@127.0.0.1:not-a-port/medstudy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 限定执行时间并释放 context；即使实现误返回连接池，也安排清理。
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			pool, err := Open(ctx, test.connString)
			if pool != nil {
				t.Cleanup(pool.Close)
			}
			// 失败不能被当作成功，也不能把连接池返回给调用方使用。
			if err == nil {
				t.Fatal("无效连接配置应该返回错误")
			}
			if pool != nil {
				t.Fatal("连接初始化失败时应该返回 nil 连接池")
			}
		})
	}
}

// TestOpenRespectsCanceledContext 验证已取消的初始化返回 context.Canceled 和 nil 连接池。
// errors.Is 必须能识别取消原因，不能把它替换成丢失原因的普通错误。
func TestOpenRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// context 已取消，不需要真的连接这个地址。
	pool, err := Open(ctx, "postgres://medstudy@127.0.0.1:1/medstudy?sslmode=disable")
	if pool != nil {
		t.Cleanup(pool.Close)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("已经取消的操作应该返回可由 errors.Is 识别的 context.Canceled")
	}
	if pool != nil {
		t.Fatal("已取消的初始化不应该返回可用连接池")
	}
}

// TestOpenRejectsNonPostgresServer 验证 TCP 地址可访问但并非数据库时，Open 仍返回错误和 nil。
// 本测试仅使用本地假服务；监听器和可能返回的连接池都在测试结束时关闭。
func TestOpenRejectsNonPostgresServer(t *testing.T) {
	// 临时监听器只接受连接并立即关闭，不是数据库，也不发布固定端口。
	// 用它检查 Open 是否真的执行 Ping，而不是只创建连接池就返回成功。
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建本地测试监听器失败：%v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connString := fmt.Sprintf("postgres://medstudy@%s/medstudy?sslmode=disable", listener.Addr())
	pool, err := Open(ctx, connString)
	if pool != nil {
		t.Cleanup(pool.Close)
	}
	if err == nil {
		t.Fatal("目标不是 PostgreSQL 时，初始化必须报错，不能只创建连接池就视为连接成功")
	}
	if pool != nil {
		t.Fatal("Ping 失败时应该关闭连接池并返回 nil")
	}
}

// TestOpenIntegration 连接显式指定的真实 PostgreSQL，验证 Open 成功且 SELECT 1 返回整数 1。
// 这是只读集成测试；未选择数据库时跳过，不表示真实连接已经验收。
func TestOpenIntegration(t *testing.T) {
	// 必须显式选择数据库；只执行 SELECT 1，不建表、不写数据。
	// 未配置时跳过不代表已经通过真实数据库验收。
	connString := os.Getenv("MEDSTUDY_TEST_DATABASE_URL")
	if filename := os.Getenv("MEDSTUDY_TEST_ENV_FILE"); filename != "" {
		// 指定配置文件时，以该文件的地址和密码为准，不在日志中展示凭据。
		cfg, err := config.LoadFile(filename)
		if err != nil {
			t.Fatal("读取测试配置失败，请检查指定文件及 DATABASE_URL、POSTGRES_PASSWORD；不要公开密码")
		}
		connString = cfg.DatabaseURL
		// 只在此测试期间提供密码，t.Setenv 会在测试结束时恢复原环境。
		// 本地配置约定：地址不内嵌密码，密码来自独立 POSTGRES_PASSWORD。
		t.Setenv("PGPASSWORD", cfg.DatabasePassword)
	}
	if connString == "" {
		t.Skip("未设置 MEDSTUDY_TEST_ENV_FILE 或 MEDSTUDY_TEST_DATABASE_URL，真实数据库连接尚未验收")
	}

	// 连接和查询共用 5 秒超时；测试结束后取消 context 并关闭连接池。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := Open(ctx, connString)
	if pool != nil {
		t.Cleanup(pool.Close)
	}
	if err != nil {
		// 不把可能携带连接配置的原始错误打印到测试日志。
		t.Fatal("连接测试数据库失败，请检查连接配置、服务状态和凭据；不要公开连接串")
	}
	if pool == nil {
		t.Fatal("初始化成功时必须返回非 nil 的连接池")
	}

	// QueryRow 执行单行查询，Scan 把返回的那一列写入 result，再断言其值为 1。
	var result int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&result); err != nil {
		t.Fatal("测试数据库只读查询失败")
	}
	if result != 1 {
		t.Fatalf("SELECT 1 应返回 1，实际得到 %v", result)
	}
}
