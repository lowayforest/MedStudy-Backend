package auth

import (
	"context"
	"time"
)

// TokenPair 保存创建登录会话后交给 HTTP 层的两种令牌
// AccessToken 供客户端放入业务请求的 Authorization 头
// RefreshToken 用于设置 refresh Cookie，RefreshExpiresAt 用于设置 Cookie 到期时间
type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// SessionService 为已验证的用户签发令牌并保存 refresh 会话
// repo 负责保存 refresh token 的哈希，accessTokens 负责签发 JWT
// refreshTTL 是新会话的有效时长，由调用方在创建服务时传入
type SessionService struct {
	repo         SessionRepository
	accessTokens *AccessTokenManager
	refreshTTL   time.Duration
}

// NewSessionService 使用会话仓储、JWT 管理器和有效时长创建服务
// 返回保存这些依赖的 SessionService 指针
func NewSessionService(repo SessionRepository, accessTokens *AccessTokenManager, refreshTTL time.Duration) *SessionService {
	return &SessionService{repo: repo, accessTokens: accessTokens, refreshTTL: refreshTTL}
}

// Start 为已验证的 userID 签发令牌并保存 refresh 会话
// ctx 控制数据库操作，成功返回令牌及到期时间，失败返回零值 TokenPair 和错误
func (s *SessionService) Start(ctx context.Context, userID string) (TokenPair, error) {
	accessToken, err := s.accessTokens.Issue(userID)
	if err != nil {
		return TokenPair{}, err
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}

	hash := HashRefreshToken(refreshToken)
	expiresAt := time.Now().Add(s.refreshTTL)
	_, err = s.repo.Create(ctx, CreateSessionInput{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: expiresAt,
	}, nil
}
