package auth

import "context"

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
