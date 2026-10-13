package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lowayforest/MedStudy-Backend/internal/folder"
)

//创建资料夹仓储

type FolderRepository struct {
	pool *pgxpool.Pool
}

// 创建函数
func NewFolderRepository(pool *pgxpool.Pool) *FolderRepository {
	return &FolderRepository{pool: pool}
}

//把一条新资料夹记录插入数据库

func (r *FolderRepository) Create(ctx context.Context, userID string, input folder.CreateInput) (folder.Folder, error) {

	//定义SQL
	const query = `insert into public.folders (user_id, name, type) values ($1, $2, $3) returning id::text, user_id::text, name, type, created_at`

	// 执行sql之前开启事务
	tx, err := r.pool.Begin(ctx)

	if err != nil {
		return folder.Folder{}, err
	}

	// 退出时撤销未提交的事务
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)

	}()

	//锁定用户行
	_, err = tx.Exec(ctx, `select id from public.users where id = $1 for update`, userID)

	if err != nil {
		return folder.Folder{}, err
	}

	//接受数据库返回字段
	var created folder.Folder

	err = tx.QueryRow(
		ctx, query, userID, input.Name, input.Type,
	).Scan(
		&created.ID,
		&created.UserID,
		&created.Name,
		&created.Type,
		&created.CreatedAt,
	)

	if err != nil {
		return folder.Folder{}, err
	}

	//递增用户版本
	tag, err := tx.Exec(ctx, `update public.users set revision = revision + 1 where id = $1`, userID)

	if err != nil {
		return folder.Folder{}, err
	}

	//恰好更新为一个用户才继续提交执行
	if tag.RowsAffected() != 1 {
		return folder.Folder{}, errors.New("更新用户数据版本失败")
	}

	//提交事务
	if err := tx.Commit(ctx); err != nil {
		return folder.Folder{}, err
	}

	return created, nil

}
