package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeUserRepository struct {
	createCalls int
	createdWith CreateUserInput
	result      User
	err         error
	findCalls   int
	foundUser   User
	foundHash   string
	findErr     error
}

func (f *fakeUserRepository) CreateUser(_ context.Context, input CreateUserInput) (User, error) {
	f.createCalls++
	f.createdWith = input
	if f.err != nil {
		return f.result, f.err
	}
	return f.result, nil
}

func (f *fakeUserRepository) FindByUsername(_ context.Context, _ string) (User, string, error) {
	f.findCalls++
	return f.foundUser, f.foundHash, f.findErr
}

// TestServiceRegister 验证注册服务先清理和校验输入，再把密码哈希交给仓储；
// 返回的用户不包含密码，测试使用假仓储，不连接真实数据库。
func TestServiceRegister(t *testing.T) {
	want := User{
		ID:        "00000000-0000-4000-8000-000000000001",
		Username:  "Alice",
		Timezone:  "Asia/Shanghai",
		CreatedAt: time.Date(2026, time.October, 20, 10, 0, 0, 0, time.UTC),
	}
	repo := &fakeUserRepository{result: want}
	service := NewService(repo)

	got, err := service.Register(context.Background(), RegisterInput{
		Username: "  Alice  ", Password: "Abc!defg",
	})
	if err != nil {
		t.Fatalf("合法输入注册失败：%v", err)
	}
	if got != want {
		t.Fatal("注册应返回仓储创建的用户，不能返回未落库的占位用户")
	}
	if repo.createCalls != 1 {
		t.Fatal("合法注册应恰好调用一次仓储")
	}
	if repo.createdWith.Username != "Alice" || repo.createdWith.Timezone != "Asia/Shanghai" {
		t.Fatal("仓储应收到去除首尾空格的用户名和默认时区")
	}
	if !strings.HasPrefix(repo.createdWith.PasswordHash, "$argon2id$") ||
		repo.createdWith.PasswordHash == "Abc!defg" {
		t.Fatal("仓储只能收到 Argon2id 哈希，不能收到明文密码")
	}
	match, err := VerifyPassword("Abc!defg", repo.createdWith.PasswordHash)
	if err != nil || !match {
		t.Fatalf("仓储收到的哈希应能验证原密码：match=%v, err=%v", match, err)
	}
}

// TestServiceRegisterRejectsInvalid 验证非法输入在写库前就被拒绝，
// 不调用假仓储，也不把无效用户当作注册成功。
func TestServiceRegisterRejectsInvalid(t *testing.T) {
	repo := &fakeUserRepository{}
	got, err := NewService(repo).Register(context.Background(), RegisterInput{
		Username: "  Al  ", Password: "abcdefg!",
	})
	if err == nil || got != (User{}) {
		t.Fatal("密码缺少大写字母时应返回错误和零值用户")
	}
	if repo.createCalls != 0 {
		t.Fatal("输入无效时不能调用仓储")
	}
}

// TestServiceRegisterReturnsRepositoryError 验证仓储写入失败时保留原始错误，
// 不把没有保存成功的用户返回给调用方。
func TestServiceRegisterReturnsRepositoryError(t *testing.T) {
	wantErr := errors.New("测试用数据库错误")
	// 假仓储故意同时返回部分用户和错误；Service 不能把部分结果当作成功。
	repo := &fakeUserRepository{result: User{ID: "partial-fixture-id"}, err: wantErr}
	got, err := NewService(repo).Register(context.Background(), RegisterInput{
		Username: "Alice", Password: "Abc!defg", Timezone: "Asia/Shanghai",
	})
	if !errors.Is(err, wantErr) || got != (User{}) {
		t.Fatal("仓储失败时应返回零值用户并保留原始错误")
	}
	if repo.createCalls != 1 {
		t.Fatal("合法输入应调用一次仓储")
	}
}
