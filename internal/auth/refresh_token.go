package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// GenerateRefreshToken 生成 32 字节随机数据，并返回适合放入 Cookie 的 token 字符串
// 无法取得随机数据时返回错误
func GenerateRefreshToken() (string, error) {
	randomByte := make([]byte, 32)

	if _, err := rand.Read(randomByte); err != nil {
		return "", err
	}

	token := base64.RawURLEncoding.EncodeToString(randomByte)

	return token, nil

}

// HashRefreshToken 计算 raw token 字符串的 SHA-256 哈希
// raw 是发给客户端的 token；返回固定 32 字节的哈希，供数据库保存和查找
func HashRefreshToken(raw string) [32]byte {
	return sha256.Sum256([]byte(raw))
}
