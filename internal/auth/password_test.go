package auth

import (
	"strings"
	"testing"
)

// TestHashAndVerifyPassword 验证注册密码保存为带随机盐的 Argon2id 编码，
// 正确密码可以验证，错误密码不能验证；测试只使用虚构密码，不读写数据库。
func TestHashAndVerifyPassword(t *testing.T) {
	const password = "Abc!defg"
	first, err := HashPassword(password)
	if err != nil {
		t.Fatalf("生成密码哈希失败：%v", err)
	}
	if !strings.HasPrefix(first, "$argon2id$") {
		t.Fatal("数据库中的密码值应该是 Argon2id 编码")
	}
	if first == password || strings.Contains(first, password) {
		t.Fatal("密码哈希不能保存明文密码")
	}

	second, err := HashPassword(password)
	if err != nil {
		t.Fatalf("再次生成密码哈希失败：%v", err)
	}
	if first == second {
		t.Fatal("相同密码的两次哈希应该因随机盐而不同")
	}

	match, err := VerifyPassword(password, first)
	if err != nil || !match {
		t.Fatalf("正确密码验证失败：match=%v, err=%v", match, err)
	}
	match, err = VerifyPassword("Wrong!123", first)
	if err != nil || match {
		t.Fatalf("错误密码不应匹配：match=%v, err=%v", match, err)
	}
}

// TestVerifyPasswordRejectsMalformedHash 验证数据库中的哈希格式损坏时返回错误，
// 不把格式错误误判为登录成功；测试不连接数据库。
func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	match, err := VerifyPassword("Abc!defg", "not-an-argon2id-hash")
	if err == nil || match {
		t.Fatalf("损坏的哈希应返回错误且不匹配：match=%v, err=%v", match, err)
	}
}
