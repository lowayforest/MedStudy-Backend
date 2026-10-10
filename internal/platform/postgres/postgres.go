package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	//在这里实现连接初始化
	if strings.TrimSpace(connString) == "" {
		return nil, errors.New("数据库连接配置不能为空")
	}
	pool, err := pgxpool.New(ctx, connString)

	if err != nil {
		return nil, err
	}
	err = pool.Ping(ctx)
	if err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
