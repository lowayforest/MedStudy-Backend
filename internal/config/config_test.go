package config

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testJWTSigningKey = "0123456789abcdef0123456789abcdef"

// withTestJWTSigningKey 为临时配置追加固定的测试密钥；它不用于真实签名或部署。
func withTestJWTSigningKey(content string) string {
	return content + "JWT_SIGNING_KEY_BASE64='" + base64.StdEncoding.EncodeToString([]byte(testJWTSigningKey)) + "'\n"
}

// TestLoadFile 验证数据库配置和 JWT 密钥的读取、解码与错误处理。
// 每个案例只读取临时文件并使用虚构凭据，不读取真实 .env，也不连接数据库。
func TestLoadFile(t *testing.T) {
	const databaseURL = "postgres://medstudy@127.0.0.1:13445/medstudy?sslmode=disable"
	const fakePassword = "fixture-only-password"
	tests := []struct {
		name         string
		content      string
		wantURL      string
		wantPassword string
		wantErr      bool
	}{
		// 三个必需项完整：应成功读取地址、密码和解码后的密钥。
		{
			name:         "valid",
			content:      withTestJWTSigningKey("DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD='" + fakePassword + "'\n"),
			wantURL:      databaseURL,
			wantPassword: fakePassword,
		},
		// 地址前后有空格：应去掉空格再保存到配置。
		{
			name:         "trim_database_url",
			content:      withTestJWTSigningKey("DATABASE_URL=' " + databaseURL + " '\nPOSTGRES_PASSWORD='" + fakePassword + "'\n"),
			wantURL:      databaseURL,
			wantPassword: fakePassword,
		},
		// 密码前后有空格：空格也属于密码，必须原样保留。
		{
			name:         "preserve_password_whitespace",
			content:      withTestJWTSigningKey("DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD=' " + fakePassword + " '\n"),
			wantURL:      databaseURL,
			wantPassword: " " + fakePassword + " ",
		},
		// 缺少连接地址：应返回错误和零值配置。
		{
			name:    "missing_database_url",
			content: withTestJWTSigningKey("POSTGRES_PASSWORD='" + fakePassword + "'\n"),
			wantErr: true,
		},
		// 连接地址只有空格：去空白后为空，应拒绝。
		{
			name:    "blank_database_url",
			content: withTestJWTSigningKey("DATABASE_URL='   '\nPOSTGRES_PASSWORD='" + fakePassword + "'\n"),
			wantErr: true,
		},
		// 缺少密码项：应拒绝，而不是使用不完整配置。
		{
			name:    "missing_password",
			content: withTestJWTSigningKey("DATABASE_URL='" + databaseURL + "'\n"),
			wantErr: true,
		},
		// 密码项存在但值为空：仍应拒绝。
		{
			name:    "empty_password",
			content: withTestJWTSigningKey("DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD=''\n"),
			wantErr: true,
		},
		// 文件中的密码引号没有闭合：应报告解析失败，错误不能泄露密码内容。
		{
			name:    "malformed_file",
			content: withTestJWTSigningKey("DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD='" + fakePassword + "\n"),
			wantErr: true,
		},
		// 签名密钥是必需配置，不能因为数据库配置有效就允许启动。
		{
			name:    "missing_jwt_signing_key",
			content: "DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD='" + fakePassword + "'\n",
			wantErr: true,
		},
		// Base64 必须能完整解码；错误信息不能回显文件中的密钥文本。
		{
			name:    "malformed_jwt_signing_key",
			content: "DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD='" + fakePassword + "'\nJWT_SIGNING_KEY_BASE64='not-base64!'\n",
			wantErr: true,
		},
		// 长度要按解码后的字节数检查，不能按 Base64 字符数检查。
		{
			name:    "short_jwt_signing_key",
			content: "DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD='" + fakePassword + "'\nJWT_SIGNING_KEY_BASE64='" + base64.StdEncoding.EncodeToString([]byte(testJWTSigningKey[:31])) + "'\n",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 准备本案例的临时配置，然后调用真正的 LoadFile。
			filename := writeTestEnvFile(t, test.content)
			cfg, err := LoadFile(filename)
			if test.wantErr {
				// 失败必须有错误、不含密码，并且不能返回半份配置。
				if err == nil {
					t.Fatal("无效配置应该返回错误")
				}
				if strings.Contains(err.Error(), fakePassword) ||
					strings.Contains(err.Error(), base64.StdEncoding.EncodeToString([]byte(testJWTSigningKey))) ||
					strings.Contains(err.Error(), "not-base64!") {
					t.Fatal("配置错误信息不能包含密码或签名密钥内容")
				}
				if cfg.DatabaseURL != "" || cfg.DatabasePassword != "" || len(cfg.JWTSigningKey) != 0 {
					t.Fatal("加载失败时应该返回零值 Config，不能返回半份配置")
				}
				return
			}
			// 成功必须没有错误，且读取出的三个字段都符合本案例预期。
			if err != nil {
				t.Fatal("合法配置不应该报错")
			}
			if cfg.DatabaseURL != test.wantURL {
				t.Fatal("数据库连接地址读取或 trim 不符合预期")
			}
			if cfg.DatabasePassword != test.wantPassword {
				t.Fatal("密码读取不符合预期，不能 trim 或修改密码内容")
			}
			if !bytes.Equal(cfg.JWTSigningKey, []byte(testJWTSigningKey)) {
				t.Fatal("JWT 签名密钥应为 Base64 解码后的原始字节")
			}
		})
	}
}

// TestLoadFileMissingFile 验证读取不存在的文件时返回错误和零值 Config。
// 临时目录由测试框架自动清理，不会删除项目文件。
func TestLoadFileMissingFile(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "missing.env"))
	if err == nil {
		t.Fatal("不存在的配置文件应该返回错误")
	}
	if cfg.DatabaseURL != "" || cfg.DatabasePassword != "" || len(cfg.JWTSigningKey) != 0 {
		t.Fatal("加载失败时应该返回零值 Config")
	}
}

// TestLoadFileDoesNotChangeEnvironment 验证只读取指定文件，不混入或覆盖进程环境变量。
func TestLoadFileDoesNotChangeEnvironment(t *testing.T) {
	const existingURL = "postgres://existing@127.0.0.1:1/existing"
	const fileURL = "postgres://medstudy@127.0.0.1:13445/medstudy?sslmode=disable"
	const existingPassword = "existing-fixture-password"
	// 准备与文件不同的环境值；t.Setenv 会在测试结束后恢复原值。
	t.Setenv("DATABASE_URL", existingURL)
	t.Setenv("POSTGRES_PASSWORD", existingPassword)
	t.Setenv("JWT_SIGNING_KEY_BASE64", "environment-fixture-only")

	filename := writeTestEnvFile(t, withTestJWTSigningKey("DATABASE_URL='"+fileURL+"'\nPOSTGRES_PASSWORD='file-fixture-password'\n"))
	cfg, err := LoadFile(filename)
	if err != nil {
		t.Fatal("读取合法文件不应该报错")
	}
	// 返回配置应来自文件，而进程环境中的原值应保持不变。
	if cfg.DatabaseURL != fileURL || cfg.DatabasePassword != "file-fixture-password" || !bytes.Equal(cfg.JWTSigningKey, []byte(testJWTSigningKey)) {
		t.Fatal("LoadFile 应读取指定文件，不应该混入进程环境中的值")
	}
	if os.Getenv("DATABASE_URL") != existingURL || os.Getenv("POSTGRES_PASSWORD") != existingPassword || os.Getenv("JWT_SIGNING_KEY_BASE64") != "environment-fixture-only" {
		t.Fatal("LoadFile 不应该修改进程环境变量")
	}
}

// writeTestEnvFile 创建仅当前用户可读写的临时配置文件；测试结束后目录自动清理。
// t.Helper 让失败位置指向调用此助手的测试代码，便于定位问题。
func writeTestEnvFile(t *testing.T, content string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "test.env")
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal("写入临时测试配置失败")
	}
	return filename
}
