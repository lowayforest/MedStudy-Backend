package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withStartupJWTSigningKey 为启动测试加入固定的假签名密钥，不依赖真实 .env。
func withStartupJWTSigningKey(content string) string {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	return content + "JWT_SIGNING_KEY_BASE64='" + key + "'\n"
}

// TestRunMissingConfig 验证指定配置文件不存在时，run 返回错误而不是继续启动服务。
// 只使用临时目录中的不存在路径，不读取真实 .env，不连接数据库。
func TestRunMissingConfig(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "missing.env")
	if err := run(filename); err == nil {
		t.Fatal("配置文件不存在时，启动流程应该返回错误")
	}
}

// TestRunInvalidConfig 验证配置不完整或格式错误时，run 停止启动且错误不泄露密码。
// 每个案例写入临时文件；临时目录由测试框架清理，不使用开发数据库。
func TestRunInvalidConfig(t *testing.T) {
	const fakePassword = "startup-fixture-only-password"
	tests := []struct {
		name    string
		content string
	}{
		// 有密码但缺少数据库地址：不能初始化数据库，应返回错误。
		{name: "missing_database_url", content: withStartupJWTSigningKey("POSTGRES_PASSWORD='" + fakePassword + "'\n")},
		// 有地址但密码为空：不能继续启动 HTTP 服务，应返回错误。
		{name: "empty_password", content: withStartupJWTSigningKey("DATABASE_URL='postgres://fixture@127.0.0.1:1/fixture'\nPOSTGRES_PASSWORD=''\n")},
		// 密码引号未闭合：解析失败，应返回受控错误，不暴露原始密码行。
		{name: "malformed_file", content: withStartupJWTSigningKey("POSTGRES_PASSWORD='" + fakePassword + "\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filename := writeStartupConfig(t, test.content)
			err := run(filename)
			if err == nil {
				t.Fatal("无效配置应该使启动失败")
			}
			if strings.Contains(err.Error(), fakePassword) {
				t.Fatal("启动错误不能泄露密码内容")
			}
		})
	}
}

// TestRunDatabaseFailure 验证配置可读但目标不是 PostgreSQL 时，run 返回受控的数据库初始化错误。
// 使用随机本地端口的假 TCP 服务：接受后立即关闭连接，不读取真实 .env、不连接真实数据库。
// 本测试覆盖失败路径；不证明成功启动、真实密码认证或连接池清理已完成验收。
func TestRunDatabaseFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建本地假服务失败：%v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	// 设置与文件不同的假环境值，检查启动不会用文件密码覆盖进程环境。
	const existingPassword = "existing-startup-fixture-password"
	t.Setenv("PGPASSWORD", existingPassword)
	const filePassword = "file-startup-fixture-password"
	content := withStartupJWTSigningKey(fmt.Sprintf("DATABASE_URL='postgres://fixture@%s/fixture?sslmode=disable'\nPOSTGRES_PASSWORD='%s'\n", listener.Addr(), filePassword))
	filename := writeStartupConfig(t, content)
	err = run(filename)
	if err == nil {
		t.Fatal("数据库初始化失败时，不应该继续启动 HTTP 服务")
	}
	// 检查对外可见的受控描述，不把底层连接配置拼进错误。
	if err.Error() != "数据库初始化失败，请检查配置和数据库服务" {
		t.Fatal("数据库初始化失败应该返回约定的受控错误")
	}
	if strings.Contains(err.Error(), filePassword) || strings.Contains(err.Error(), listener.Addr().String()) {
		t.Fatal("启动错误不能包含密码或底层连接地址")
	}
	if os.Getenv("PGPASSWORD") != existingPassword {
		t.Fatal("启动流程不应该修改进程密码环境变量")
	}
}

// writeStartupConfig 为启动测试写入私有临时配置；测试结束后自动删除目录。
// t.Helper 使文件准备失败时的报错指向调用它的测试位置。
func writeStartupConfig(t *testing.T, content string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "startup.env")
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal("创建启动测试配置失败")
	}
	return filename
}
