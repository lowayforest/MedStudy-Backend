package auth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSessionRepository struct {
	calls int
	input CreateSessionInput
	err   error
}

func (f *fakeSessionRepository) Create(_ context.Context, input CreateSessionInput) (Session, error) {
	f.calls++
	f.input = input
	if f.err != nil {
		return Session{}, f.err
	}
	return Session{
		ID: "session-fixture-id", UserID: input.UserID,
		FamilyID: "family-fixture-id", ExpiresAt: input.ExpiresAt,
	}, nil
}

// TestSessionServiceStart 验证登录后签发可验证的 access token
// 同时生成新 refresh token，并只把它的哈希和到期时间交给会话仓储
func TestSessionServiceStart(t *testing.T) {
	manager, err := NewAccessTokenManager(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal("准备测试用 JWT 管理器失败")
	}
	repo := &fakeSessionRepository{}
	const userID = "00000000-0000-4000-8000-000000000001"
	const ttl = 2 * time.Hour
	service := NewSessionService(repo, manager, ttl)
	before := time.Now()
	got, err := service.Start(context.Background(), userID)
	after := time.Now()
	if err != nil {
		t.Fatalf("创建登录会话失败：%v", err)
	}
	if repo.calls != 1 || repo.input.UserID != userID {
		t.Fatal("应为已验证用户恰好创建一条 refresh 会话")
	}
	if got.RefreshToken == "" || repo.input.TokenHash != HashRefreshToken(got.RefreshToken) {
		t.Fatal("应返回新 refresh token，并只把对应哈希交给仓储")
	}
	if repo.input.ExpiresAt.Before(before.Add(ttl)) || repo.input.ExpiresAt.After(after.Add(ttl)) ||
		!got.RefreshExpiresAt.Equal(repo.input.ExpiresAt) {
		t.Fatal("会话到期时间应按传入的 ttl 计算并返回给 HTTP 层")
	}
	verifiedUserID, err := manager.Verify(got.AccessToken)
	if err != nil || verifiedUserID != userID {
		t.Fatal("返回的 access token 应能验证出当前用户 ID")
	}
}

// TestSessionServiceStartStoreError 验证会话未保存成功时不返回任何 token
// 保留仓储错误，避免调用方把未持久化的 refresh token 发给浏览器
func TestSessionServiceStartStoreError(t *testing.T) {
	manager, err := NewAccessTokenManager(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal("准备测试用 JWT 管理器失败")
	}
	wantErr := errors.New("测试用数据库错误")
	repo := &fakeSessionRepository{err: wantErr}
	got, err := NewSessionService(repo, manager, time.Hour).Start(
		context.Background(), "00000000-0000-4000-8000-000000000001",
	)
	if !errors.Is(err, wantErr) || got != (TokenPair{}) {
		t.Fatal("会话保存失败时应返回原始错误和零值 TokenPair")
	}
	if repo.calls != 1 {
		t.Fatal("创建会话时应恰好调用一次仓储")
	}
}
