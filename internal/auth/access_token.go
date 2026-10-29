package auth

import (
	"bytes"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AccessTokenManager struct {
	secret []byte
}

// NewAccessTokenManager 使用 secret 创建 HS256 access token 管理器，并保存密钥副本。
// secret 少于 32 字节时返回 nil 和错误。
func NewAccessTokenManager(secret []byte) (*AccessTokenManager, error) {
	if len(secret) < 32 {
		return nil, ErrJWTSecretTooShort
	}

	return &AccessTokenManager{secret: bytes.Clone(secret)}, nil
}

// Issue 以 userID 作为 sub 签发有效期为 15 分钟的 JWT。
// 成功返回 token 字符串；用户 ID 为空或签名失败时返回错误。
func (m *AccessTokenManager) Issue(userID string) (string, error) {
	now := time.Now()

	if userID == "" {
		return "", ErrEmptyUserID
	}

	claims := jwt.RegisteredClaims{
		Issuer:    "medstudy-backend",
		Subject:   userID,
		Audience:  jwt.ClaimStrings{"medstudy-api"},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString(m.secret)
}

// Verify 校验 raw 中的 JWT 签名、签发方、接收方和有效期。
// 成功返回 sub 中的用户 ID；token 无效或过期时返回错误。
func (m *AccessTokenManager) Verify(raw string) (string, error) {
	claims := &jwt.RegisteredClaims{}

	keyFunc := func(_ *jwt.Token) (any, error) {
		return m.secret, nil
	}

	parsed, err := jwt.ParseWithClaims(
		raw,
		claims,
		keyFunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer("medstudy-backend"),
		jwt.WithAudience("medstudy-api"),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)

	if err != nil {
		return "", ErrInvalidAccessToken
	}

	// 只有验证完成后，才能把 sub 当作可信的用户 ID
	if parsed == nil || !parsed.Valid || claims.Subject == "" || claims.IssuedAt == nil {
		return "", ErrInvalidAccessToken
	}

	return claims.Subject, nil
}
