package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

// TestGenerateRefreshToken 验证 token 解码后为 32 字节，采用适合 Cookie 的
// URL 安全编码，且连续两次生成不同；测试不读取配置、数据库或真实会话。
func TestGenerateRefreshToken(t *testing.T) {
	first, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("生成 refresh token 失败：%v", err)
	}
	second, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("再次生成 refresh token 失败：%v", err)
	}
	if first == second {
		t.Fatal("两次生成的 refresh token 不应相同")
	}

	for _, raw := range []string{first, second} {
		decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
		if err != nil {
			t.Fatal("refresh token 应使用不带填充的 URL 安全 Base64 编码")
		}
		if len(decoded) != 32 {
			t.Fatalf("每个 refresh token 应由 32 字节随机数据组成，实际为 %d 字节", len(decoded))
		}
	}
}

// TestHashRefreshToken 验证供仓储保存的哈希等于 token 字符串的 SHA-256；
// 同一个 token 得到相同哈希，不同 token 得到不同哈希。
func TestHashRefreshToken(t *testing.T) {
	const raw = "test-refresh-token-only"
	want := sha256.Sum256([]byte(raw))
	got := HashRefreshToken(raw)
	if got != want {
		t.Fatal("refresh token 哈希应等于 SHA-256(原始 token 字符串)")
	}
	if HashRefreshToken(raw) != got {
		t.Fatal("同一个 token 应得到相同哈希，以便数据库查找")
	}
	if HashRefreshToken(raw+"-other") == got {
		t.Fatal("不同 token 不应得到同一哈希")
	}
}
