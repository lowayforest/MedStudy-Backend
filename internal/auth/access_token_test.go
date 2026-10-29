package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testAccessTokenSecret = "0123456789abcdef0123456789abcdef"

// TestAccessTokenRoundTrip 验证签发的 token 能恢复用户 ID，并且签名算法、
// 用途和有效期符合接口契约；测试密钥仅用于测试，不作为运行时配置。
func TestAccessTokenRoundTrip(t *testing.T) {
	manager, err := NewAccessTokenManager([]byte(testAccessTokenSecret))
	if err != nil {
		t.Fatalf("创建 access token 管理器失败：%v", err)
	}

	userID := "00000000-0000-4000-8000-000000000001"
	raw, err := manager.Issue(userID)
	if err != nil {
		t.Fatalf("签发 access token 失败：%v", err)
	}

	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(_ *jwt.Token) (any, error) {
		return []byte(testAccessTokenSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer("medstudy-backend"),
		jwt.WithAudience("medstudy-api"),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid {
		t.Fatalf("签发的 JWT 应具有有效签名和约定的 iss、aud、exp：%v", err)
	}
	if claims.Subject != userID || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatalf("JWT 应含用户 ID、iat 和 exp：%+v", claims)
	}
	if got := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time); got != 15*time.Minute {
		t.Fatalf("JWT 有效期应为 15 分钟，实际为 %v", got)
	}

	gotUserID, err := manager.Verify(raw)
	if err != nil || gotUserID != userID {
		t.Fatalf("应能验证自己签发的 token 并取得用户 ID：userID=%q, err=%v", gotUserID, err)
	}
}

// TestAccessTokenRejectsInvalid 验证签名正确并不代表 token 一定适用于本 API：
// 过期、错误签发方或接收方、缺少必要字段及非预期签名算法都必须拒绝。
func TestAccessTokenRejectsInvalid(t *testing.T) {
	manager, err := NewAccessTokenManager([]byte(testAccessTokenSecret))
	if err != nil {
		t.Fatalf("创建 access token 管理器失败：%v", err)
	}

	now := time.Now()
	tests := []struct {
		name       string
		change     func(*jwt.RegisteredClaims)
		method     jwt.SigningMethod
		signingKey []byte
	}{
		{name: "已过期", change: func(c *jwt.RegisteredClaims) { c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute)) }},
		{name: "错误签发方", change: func(c *jwt.RegisteredClaims) { c.Issuer = "other-service" }},
		{name: "错误接收方", change: func(c *jwt.RegisteredClaims) { c.Audience = jwt.ClaimStrings{"other-api"} }},
		{name: "缺少过期时间", change: func(c *jwt.RegisteredClaims) { c.ExpiresAt = nil }},
		{name: "缺少用户 ID", change: func(c *jwt.RegisteredClaims) { c.Subject = "" }},
		{name: "错误签名算法", method: jwt.SigningMethodHS384},
		{name: "错误签名密钥", signingKey: []byte("fedcba9876543210fedcba9876543210")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := jwt.RegisteredClaims{
				Issuer:    "medstudy-backend",
				Subject:   "00000000-0000-4000-8000-000000000001",
				Audience:  jwt.ClaimStrings{"medstudy-api"},
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			}
			if tt.change != nil {
				tt.change(&claims)
			}
			method := tt.method
			if method == nil {
				method = jwt.SigningMethodHS256
			}
			key := tt.signingKey
			if key == nil {
				key = []byte(testAccessTokenSecret)
			}
			raw, err := jwt.NewWithClaims(method, claims).SignedString(key)
			if err != nil {
				t.Fatalf("准备测试 token 失败：%v", err)
			}
			if userID, err := manager.Verify(raw); err == nil {
				t.Fatalf("无效 token 必须被拒绝，实际返回 userID=%q", userID)
			}
		})
	}
}

// TestAccessTokenRejectsWeakInput 验证密钥和用户 ID 在签发前就被检查，
// 避免产生弱签名或无法归属到用户的 token。
func TestAccessTokenRejectsWeakInput(t *testing.T) {
	if _, err := NewAccessTokenManager([]byte(testAccessTokenSecret[:31])); err == nil {
		t.Fatal("少于 32 字节的签名密钥必须被拒绝")
	}
	manager, err := NewAccessTokenManager([]byte(testAccessTokenSecret))
	if err != nil {
		t.Fatalf("创建 access token 管理器失败：%v", err)
	}
	if _, err := manager.Issue(""); err == nil {
		t.Fatal("空用户 ID 不能用于签发 access token")
	}
}
