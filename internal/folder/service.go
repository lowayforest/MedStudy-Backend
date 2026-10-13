package folder

import (
	"context"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// 创建记录
func (s *Service) Create(ctx context.Context, userID string, input CreateInput) (Folder, error) {
	//校验准备
	prepared, err := PrepareCreateInput(input)
	if err != nil {
		return Folder{}, err
	}
	created, err := s.repo.Create(ctx, userID, prepared)
	if err != nil {
		return Folder{}, err
	}
	return created, err
}
