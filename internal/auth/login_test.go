package auth

import (
	"context"
	"errors"
	"testing"
)

// TestServiceLogin 验证登录服务使用仓储取出的哈希校验密码，
// 只在密码正确时返回用户；测试不连接数据库，也不签发 token。
func TestServiceLogin(t *testing.T) {
	hash, err := HashPassword("Abc!defg")
	if err != nil {
		t.Fatal("生成测试密码哈希失败")
	}
	want := User{ID: "00000000-0000-4000-8000-000000000001", Username: "Alice"}
	repo := &fakeUserRepository{foundUser: want, foundHash: hash}
	service := NewService(repo)

	got, err := service.Login(context.Background(), LoginInput{Username: "Alice", Password: "Abc!defg"})
	if err != nil || got != want || repo.findCalls != 1 {
		t.Fatalf("正确密码应登录成功并返回用户：err=%v", err)
	}
	got, err = service.Login(context.Background(), LoginInput{Username: "Alice", Password: "Wrong!123"})
	if !errors.Is(err, ErrInvalidCredentials) || got != (User{}) || repo.findCalls != 2 {
		t.Fatal("错误密码应返回统一的凭证错误和零值用户")
	}
}

// TestServiceLoginMissingUser 验证不存在的用户名与密码错误返回同一种错误，
// 避免让调用方通过错误类型判断账号是否存在。
func TestServiceLoginMissingUser(t *testing.T) {
	repo := &fakeUserRepository{findErr: ErrUserNotFound}
	got, err := NewService(repo).Login(context.Background(), LoginInput{
		Username: "Nobody", Password: "Abc!defg",
	})
	if !errors.Is(err, ErrInvalidCredentials) || got != (User{}) || repo.findCalls != 1 {
		t.Fatal("用户名不存在应返回统一的凭证错误和零值用户")
	}
}

// TestServiceLoginRepositoryFailure 验证数据库等非凭证错误不会被伪装为密码错误，
// 也不能把仓储同时返回的部分用户当作已登录用户。
func TestServiceLoginRepositoryFailure(t *testing.T) {
	wantErr := errors.New("测试用数据库故障")
	repo := &fakeUserRepository{foundUser: User{ID: "partial-fixture-id"}, findErr: wantErr}
	got, err := NewService(repo).Login(context.Background(), LoginInput{
		Username: "Alice", Password: "Abc!defg",
	})
	if !errors.Is(err, wantErr) || got != (User{}) || repo.findCalls != 1 {
		t.Fatal("仓储故障应保留原始错误并返回零值用户")
	}
}
