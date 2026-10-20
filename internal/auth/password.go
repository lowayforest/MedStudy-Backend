package auth

import (
	"github.com/alexedwards/argon2id"
)

// HashPassword 将注册密码转换为可保存到数据库的 Argon2id 哈希
func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

// VerifyPassword 检查登录密码是否与数据库中的哈希匹配
func VerifyPassword(password, encodedHash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, encodedHash)
}
