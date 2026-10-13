package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lowayforest/MedStudy-Backend/internal/folder"
)

// TestFolderRepositoryReusesPool 检查构造函数保存传入的同一个连接池指针。
// 仅创建不主动连接的池，不读取 .env，也不连接真实数据库；结束时关闭池。
func TestFolderRepositoryReusesPool(t *testing.T) {
	pool := newUnconnectedFolderTestPool(t)
	repo := NewFolderRepository(pool)
	if repo == nil || repo.pool != pool {
		t.Fatal("仓储应该复用传入的连接池指针，不能复制或另建连接池")
	}
}

// TestFolderRepositoryCreateCanceled 检查已取消的操作返回取消原因和零值 Folder。
// context 在调用前取消，因此不执行 SQL，也不需要真实 PostgreSQL。
func TestFolderRepositoryCreateCanceled(t *testing.T) {
	repo := NewFolderRepository(newUnconnectedFolderTestPool(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	created, err := repo.Create(ctx, "00000000-0000-4000-8000-000000000001", folder.CreateInput{
		Name: "口腔解剖", Type: "material",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("已取消的创建操作应该保留 context.Canceled 错误")
	}
	if created != (folder.Folder{}) {
		t.Fatal("创建失败应该返回零值 Folder，不能把部分数据当作成功结果")
	}
}

// newUnconnectedFolderTestPool 关闭预建连接能力，配合已取消 context 使用。
// 地址是占位值，不使用开发库地址或密码；调用方不得用此池发起正常查询。
func newUnconnectedFolderTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("postgres://fixture@127.0.0.1:1/fixture?sslmode=disable")
	if err != nil {
		t.Fatal("解析测试占位配置失败")
	}
	cfg.MinConns = 0
	cfg.MinIdleConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("创建测试连接池失败")
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestFolderRepositoryIntegration 验证真实 INSERT、RETURNING、Scan、数据库约束及 revision 事务。
// 只有 MEDSTUDY_TEST_FOLDER_DOCKER=1 才运行；由测试新建临时 PostgreSQL 18 容器，
// 使用内存数据目录和随机本机端口，不读取 .env、不复用 Compose 容器或项目数据卷。
// 未开启时跳过，不代表真实插入已验收；结束时关闭连接池并删除专用临时容器。
func TestFolderRepositoryIntegration(t *testing.T) {
	if os.Getenv("MEDSTUDY_TEST_FOLDER_DOCKER") != "1" {
		t.Skip("未开启独立 Docker 写入测试，真实资料夹插入尚未验收")
	}
	pool := newIsolatedFolderTestDatabase(t)
	repo := NewFolderRepository(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const firstUser = "00000000-0000-4000-8000-000000000001"
	const secondUser = "00000000-0000-4000-8000-000000000002"
	// 只在本测试新建的数据库里准备用户，以满足 folders.user_id 外键。
	// 占位哈希仅满足表约束，不是真实密码哈希，不能用于注册或登录。
	_, err := pool.Exec(ctx, `insert into public.users (id, username, password_hash)
		values ($1, 'FolderFixtureOne', $3), ($2, 'FolderFixtureTwo', $3)`,
		firstUser, secondUser, "$argon2id$fixture-only-not-a-login-hash")
	if err != nil {
		t.Fatal("准备隔离测试用户失败")
	}

	uuidV4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seenIDs := make(map[string]bool)
	successes := []struct {
		name   string
		userID string
		input  folder.CreateInput
	}{
		// 中文资料夹：应保留名称和 material 类型，并返回数据库生成的 ID、时间。
		{name: "中文资料", userID: firstUser, input: folder.CreateInput{Name: "口腔解剖", Type: "material"}},
		// 另一用户的真题资料夹：应保存第二个用户 ID，不能写入固定用户。
		{name: "另一用户真题", userID: secondUser, input: folder.CreateInput{Name: "历年真题", Type: "exam"}},
		// 单引号等 SQL 特殊字符应作为普通名称保存，不能被拼接为 SQL 语法。
		{name: "名称包含引号", userID: firstUser, input: folder.CreateInput{Name: "资料 ' 引号; --", Type: "material"}},
	}
	for _, test := range successes {
		t.Run(test.name, func(t *testing.T) {
			// 同时记录目标用户和另一用户的状态，检查版本只更新当前用户。
			otherUser := secondUser
			if test.userID == secondUser {
				otherUser = firstUser
			}
			before := readFolderTestState(t, ctx, pool, test.userID)
			otherBefore := readFolderTestState(t, ctx, pool, otherUser)
			created, err := repo.Create(ctx, test.userID, test.input)
			if err != nil {
				t.Fatal("合法输入创建资料夹失败")
			}
			if !uuidV4.MatchString(created.ID) || seenIDs[created.ID] {
				t.Fatal("数据库应该为每条新记录生成不同的 UUID v4")
			}
			seenIDs[created.ID] = true
			if created.UserID != test.userID || created.Name != test.input.Name || created.Type != test.input.Type {
				t.Fatal("返回字段与输入不一致，请检查 SQL 参数和 Scan 列顺序")
			}
			if created.CreatedAt.IsZero() {
				t.Fatal("应该返回数据库生成的创建时间")
			}

			// 再读数据库，确认不是仅返回了一个看似正确、实际没有落库的对象。
			var stored folder.Folder
			err = pool.QueryRow(ctx, `select id::text, user_id::text, name, type, created_at
				from public.folders where id = $1 and user_id = $2`, created.ID, test.userID).Scan(
				&stored.ID, &stored.UserID, &stored.Name, &stored.Type, &stored.CreatedAt,
			)
			if err != nil {
				t.Fatal("创建成功后应该能在对应用户下读到资料夹")
			}
			if stored.ID != created.ID || stored.UserID != created.UserID || stored.Name != created.Name ||
				stored.Type != created.Type || !stored.CreatedAt.Equal(created.CreatedAt) {
				t.Fatal("返回结果应该与实际存储的五个字段一致")
			}
			// 必须提交一条资料夹及一次版本递增，不能少更新或重复递增。
			after := readFolderTestState(t, ctx, pool, test.userID)
			if after.folders != before.folders+1 || after.revision != before.revision+1 {
				t.Fatal("每次创建成功应提交一条资料夹，并让该用户 revision 恰好加 1")
			}
			if readFolderTestState(t, ctx, pool, otherUser) != otherBefore {
				t.Fatal("创建当前用户资料夹不能改变另一用户的数据或 revision")
			}
		})
	}

	failures := []struct {
		name   string
		userID string
		input  folder.CreateInput
		code   string
	}{
		// 用户不存在：外键拒绝写入，应保留 PostgreSQL 的 23503 错误。
		{name: "用户不存在", userID: "00000000-0000-4000-8000-000000000099", input: folder.CreateInput{Name: "资料", Type: "material"}, code: "23503"},
		// 类型非法：CHECK 约束拒绝写入，应保留 23514 错误。
		{name: "类型非法", userID: firstUser, input: folder.CreateInput{Name: "资料", Type: "other"}, code: "23514"},
		// 空名称：数据库也必须保护名称规则，不能落库。
		{name: "名称为空", userID: firstUser, input: folder.CreateInput{Name: "", Type: "material"}, code: "23514"},
		// 仓储接收已标准化输入；若上层遗漏 trim，数据库约束仍应拒绝首尾空白。
		{name: "名称未去空白", userID: firstUser, input: folder.CreateInput{Name: " 资料 ", Type: "material"}, code: "23514"},
	}
	for _, test := range failures {
		t.Run(test.name, func(t *testing.T) {
			// 保存两个已有用户的完整状态，失败后数据和版本都应保持不变。
			firstBefore := readFolderTestState(t, ctx, pool, firstUser)
			secondBefore := readFolderTestState(t, ctx, pool, secondUser)
			created, err := repo.Create(ctx, test.userID, test.input)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != test.code {
				t.Fatalf("应该保留数据库约束错误，预期 SQLSTATE 为 %s", test.code)
			}
			if created != (folder.Folder{}) {
				t.Fatal("数据库拒绝创建时应该返回零值 Folder")
			}
			if readFolderTestState(t, ctx, pool, firstUser) != firstBefore ||
				readFolderTestState(t, ctx, pool, secondUser) != secondBefore {
				t.Fatal("插入失败不能留下资料夹，也不能改变任一用户的 revision")
			}
		})
	}

	// 第二条写入失败：资料夹 INSERT 已执行，但 revision 更新溢出。
	// 这是验证事务原子性的关键场景：第一条写入也必须被回滚。
	// 只在专用临时容器创建边界用户，不修改迁移、不向开发库注入故障。
	t.Run("版本更新失败撤销资料夹", func(t *testing.T) {
		const userID = "00000000-0000-4000-8000-000000000003"
		const highestRevision int64 = 9223372036854775807
		_, err := pool.Exec(ctx, `insert into public.users (id, username, password_hash, revision)
			values ($1, 'FolderRollbackFixture', $2, $3)`,
			userID, "$argon2id$fixture-only-not-a-login-hash", highestRevision)
		if err != nil {
			t.Fatal("准备版本边界测试用户失败")
		}
		before := readFolderTestState(t, ctx, pool, userID)
		created, err := repo.Create(ctx, userID, folder.CreateInput{Name: "应该回滚的资料", Type: "material"})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "22003" {
			t.Fatal("版本加 1 溢出时应该保留 SQLSTATE 22003 错误，不能提交或吞掉原因")
		}
		if created != (folder.Folder{}) {
			t.Fatal("版本更新失败不能返回已经生成但未提交的资料夹对象")
		}
		// Create 返回前，defer 的 Rollback 应已执行；从连接池重新读取验证真实状态。
		if readFolderTestState(t, ctx, pool, userID) != before {
			t.Fatal("第二条写入失败时，应回滚先前资料夹插入，保持原版本不变")
		}
	})

	// 应只有三条合法记录，全部失败操作都不应留下额外记录。
	var count int
	if err := pool.QueryRow(ctx, "select count(*) from public.folders").Scan(&count); err != nil {
		t.Fatal("统计隔离测试资料夹失败")
	}
	if count != len(successes) {
		t.Fatalf("预期只有 %d 条成功记录，实际 %d 条", len(successes), count)
	}
}

// folderTestState 是隔离数据库里某个用户的状态快照，供事务前后比较。
type folderTestState struct {
	folders  int
	revision int64
}

// readFolderTestState 用独立于业务事务的连接池查询观察已提交状态。
// 仅由上述已新建临时容器的集成测试使用，不能传入开发数据库。
func readFolderTestState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID string) folderTestState {
	t.Helper()
	var state folderTestState
	err := pool.QueryRow(ctx, `select revision,
		(select count(*) from public.folders where user_id = $1)
		from public.users where id = $1`, userID).Scan(&state.revision, &state.folders)
	if err != nil {
		t.Fatal("读取隔离测试用户的资料夹数量和版本失败")
	}
	return state
}

// newIsolatedFolderTestDatabase 封装重复的 Docker 准备工作，不要求业务实现依赖 Docker。
// 必须新建容器；仅使用本机已有的 postgres:18 镜像，不自动下载、不读取开发配置。
func newIsolatedFolderTestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	const label = "medstudy.folder-test"
	const database = "medstudy_folder_test"
	const password = "fixture-only-container-password"
	output, err := exec.CommandContext(ctx, "docker", "run", "--detach", "--pull=never",
		"--label", label+"=temporary", "--tmpfs", "/var/lib/postgresql:rw",
		"--publish", "127.0.0.1::5432", "--env", "POSTGRES_DB="+database,
		"--env", "POSTGRES_PASSWORD="+password, "postgres:18").Output()
	if err != nil {
		t.Fatal("创建临时 PostgreSQL 失败：请先启动 Docker，并确认本机已有 postgres:18 镜像")
	}
	containerID := strings.TrimSpace(string(output))
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(containerID) {
		t.Fatal("Docker 未返回有效容器 ID，停止测试；请检查新建的测试容器")
	}
	t.Cleanup(func() {
		// 删除范围限于本函数创建的完整 ID，且必须核实专用临时标签。
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		marker, err := exec.CommandContext(cleanupCtx, "docker", "inspect", "--format",
			"{{index .Config.Labels \""+label+"\"}}", containerID).Output()
		if err != nil || strings.TrimSpace(string(marker)) != "temporary" {
			t.Error("无法核实测试容器标签，未执行删除；请手动检查带 medstudy.folder-test 标签的临时容器")
			return
		}
		if err := exec.CommandContext(cleanupCtx, "docker", "rm", "--force", containerID).Run(); err != nil {
			t.Error("临时测试容器清理失败，请手动检查；未操作项目容器或数据卷")
		}
	})

	// 只接受 Docker 为该新容器分配的回环地址，不能传入现有数据库 URL。
	output, err = exec.CommandContext(ctx, "docker", "port", containerID, "5432/tcp").Output()
	if err != nil {
		t.Fatal("读取临时数据库端口失败")
	}
	address := strings.TrimSpace(string(output))
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		t.Fatal("临时数据库必须只发布到本机回环地址")
	}
	cfg, err := pgxpool.ParseConfig(fmt.Sprintf("postgres://postgres@%s/%s?sslmode=disable", address, database))
	if err != nil {
		t.Fatal("解析临时数据库配置失败")
	}
	cfg.ConnConfig.Password = password
	cfg.ConnConfig.ConnectTimeout = time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("创建临时数据库连接池失败")
	}
	// Cleanup 后注册先执行：先关闭池，再删除容器，避免残留连接。
	t.Cleanup(pool.Close)
	for pool.Ping(ctx) != nil {
		select {
		case <-ctx.Done():
			t.Fatal("临时 PostgreSQL 未及时就绪")
		case <-time.After(200 * time.Millisecond):
		}
	}

	// 导入项目真实迁移，才能发现 SQL 列名、类型和约束与实现不一致的问题。
	schema, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000001_initial_schema.sql"))
	if err != nil {
		t.Fatal("读取初始迁移失败")
	}
	command := exec.CommandContext(ctx, "docker", "exec", "-i", containerID,
		"psql", "-X", "-U", "postgres", "-d", database, "-v", "ON_ERROR_STOP=1")
	command.Stdin = bytes.NewReader(schema)
	if err := command.Run(); err != nil {
		t.Fatal("向临时数据库导入迁移失败")
	}
	return pool
}
