package auth

import "errors"

var (
	ErrUserNotFound       = errors.New("用户不存在")
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrJWTSecretTooShort  = errors.New("JWT 签名密钥至少需要 32 字节")
	ErrInvalidAccessToken = errors.New("无效的 access token")
	ErrEmptyUserID        = errors.New("用户 ID 不能为空")
)
