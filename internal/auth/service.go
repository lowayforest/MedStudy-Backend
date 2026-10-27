package auth

import (
	"context"
	"errors"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (User, error) {
	// 1. PrepareRegisterInput(input)；失败返回 User{} 和错误
	prepared, err := PrepareRegisterInput(input)
	if err != nil {
		return User{}, err
	}
	// 2. HashPassword(校验后的 Password)；失败同样返回零值用户和错误
	passwordHash, err := HashPassword(prepared.Password)
	if err != nil {
		return User{}, err
	}
	// 3. 将 Username、PasswordHash、Timezone 装入 CreateUserInput
	createInput := CreateUserInput{Username: prepared.Username, PasswordHash: passwordHash, Timezone: prepared.Timezone}

	// 4. 调用 s.repo.CreateUser(ctx, ...) 并返回结果
	user, err := s.repo.CreateUser(ctx, createInput)

	if err != nil {
		return User{}, err
	}

	return user, nil
}

// Login 校验用户名和密码，成功时返回用户；不在这里签发 token
func (s *Service) Login(ctx context.Context, input LoginInput) (User, error) {
	// 根据用户名读取账号和密码哈希
	user, passwordHash, err := s.repo.FindByUsername(ctx, input.Username)
	// 区分账号不存在与数据库异常
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	// 用用户输入的原始密码校验哈希

	isCorrect, err := VerifyPassword(input.Password, passwordHash)

	// 区分校验异常与密码不匹配
	if err != nil {
		return User{}, err
	}

	if !isCorrect {
		return User{}, ErrInvalidCredentials
	}

	return user, nil
}
