package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadFile 验证配置读取、地址去空白、密码原样保留，以及无效配置的错误处理。
// 每个案例只读取临时文件并使用虚构密码，不读取真实 .env，也不连接数据库。
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
		// 两个必需项完整：应成功读取地址和密码。
		{
			name:         "valid",
			content:      "DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD='" + fakePassword + "'\n",
			wantURL:      databaseURL,
			wantPassword: fakePassword,
		},
		// 地址前后有空格：应去掉空格再保存到配置。
		{
			name:         "trim_database_url",
			content:      "DATABASE_URL=' " + databaseURL + " '\nPOSTGRES_PASSWORD='" + fakePassword + "'\n",
			wantURL:      databaseURL,
			wantPassword: fakePassword,
		},
		// 密码前后有空格：空格也属于密码，必须原样保留。
		{
			name:         "preserve_password_whitespace",
			content:      "DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD=' " + fakePassword + " '\n",
			wantURL:      databaseURL,
			wantPassword: " " + fakePassword + " ",
		},
		// 缺少连接地址：应返回错误和零值配置。
		{
			name:    "missing_database_url",
			content: "POSTGRES_PASSWORD='" + fakePassword + "'\n",
			wantErr: true,
		},
		// 连接地址只有空格：去空白后为空，应拒绝。
		{
			name:    "blank_database_url",
			content: "DATABASE_URL='   '\nPOSTGRES_PASSWORD='" + fakePassword + "'\n",
			wantErr: true,
		},
		// 缺少密码项：应拒绝，而不是使用不完整配置。
		{
			name:    "missing_password",
			content: "DATABASE_URL='" + databaseURL + "'\n",
			wantErr: true,
		},
		// 密码项存在但值为空：仍应拒绝。
		{
			name:    "empty_password",
			content: "DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD=''\n",
			wantErr: true,
		},
		// 文件中的密码引号没有闭合：应报告解析失败，错误不能泄露密码内容。
		{
			name:    "malformed_file",
			content: "DATABASE_URL='" + databaseURL + "'\nPOSTGRES_PASSWORD='" + fakePassword + "\n",
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
				if strings.Contains(err.Error(), fakePassword) {
					t.Fatal("配置错误信息不能包含密码内容")
				}
				if cfg.DatabaseURL != "" || cfg.DatabasePassword != "" {
					t.Fatal("加载失败时应该返回零值 Config，不能返回半份配置")
				}
				return
			}
			// 成功必须没有错误，且读取出的两个字段都符合本案例预期。
			if err != nil {
				t.Fatal("合法配置不应该报错")
			}
			if cfg.DatabaseURL != test.wantURL {
				t.Fatal("数据库连接地址读取或 trim 不符合预期")
			}
			if cfg.DatabasePassword != test.wantPassword {
				t.Fatal("密码读取不符合预期，不能 trim 或修改密码内容")
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
	if cfg.DatabaseURL != "" || cfg.DatabasePassword != "" {
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

	filename := writeTestEnvFile(t, "DATABASE_URL='"+fileURL+"'\nPOSTGRES_PASSWORD='file-fixture-password'\n")
	cfg, err := LoadFile(filename)
	if err != nil {
		t.Fatal("读取合法文件不应该报错")
	}
	// 返回配置应来自文件，而进程环境中的原值应保持不变。
	if cfg.DatabaseURL != fileURL || cfg.DatabasePassword != "file-fixture-password" {
		t.Fatal("LoadFile 应读取指定文件，不应该混入进程环境中的值")
	}
	if os.Getenv("DATABASE_URL") != existingURL || os.Getenv("POSTGRES_PASSWORD") != existingPassword {
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
