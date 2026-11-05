package auth

import "time"

// CreateSessionInput 是创建 refresh 会话时交给数据库仓储的数据
// UserID 来自已验证的用户，TokenHash 是原始 token 字符串的 SHA-256 哈希
// ExpiresAt 由上层认证逻辑决定，仓储只负责保存
type CreateSessionInput struct {
	UserID    string
	TokenHash [32]byte
	ExpiresAt time.Time
}

// Session 表示数据库中一条 refresh 会话
// ID 标识这一条会话，FamilyID 标识它所属的 token 轮换链
// 这里不保存发给浏览器的原始 refresh token
type Session struct {
	ID        string
	UserID    string
	FamilyID  string
	ExpiresAt time.Time
}
