package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, connString string, password string) (*pgxpool.Pool, error) {
	//在这里实现连接初始化
	if strings.TrimSpace(connString) == "" {
		return nil, errors.New("数据库连接配置不能为空")
	}
	if password == "" {
		return nil, errors.New("数据库密码配置不能为空")
	}
	poolConnfig, err := pgxpool.ParseConfig(connString)

	if err != nil {
		return nil, err
	}

	poolConnfig.ConnConfig.Password = password

	pool, err := pgxpool.NewWithConfig(ctx, poolConnfig)

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
