# PostgreSQL 初始化与 Navicat 导入

## 1. 文件与适用范围

- 初始化脚本：[000001_initial_schema.sql](../migrations/000001_initial_schema.sql)。
- 数据库约束测试：[000001_initial_schema_test.sql](../tests/sql/000001_initial_schema_test.sql)。
- 隔离验证工具：[verify_schema.sh](../scripts/verify_schema.sh)。

脚本面向 PostgreSQL 18+、UTF8 编码和当前 v1 接口契约，是第一版完整的初始结构，不是 MySQL 脚本。

要了解每张表的字段、关系、索引、来源快照、去重和事务设计，请阅读 [数据库详细设计](DATABASE_DESIGN.md)。本页主要说明如何连接与导入。

只导入到尚未初始化 MedStudy 表结构的数据库。脚本不创建数据库、不修改数据库登录密码、不创建管理员/演示业务账号，不包含 `DROP` 或 `TRUNCATE`，也不自动导入测试数据。

整个初始化在一个事务内。再次导入会因对象已存在报错，不会覆盖现有表。不要用 `CREATE TABLE IF NOT EXISTS` 隐藏结构差异；后续变更写成新的版本化 SQL。暂未实现 Go migration runner、迁移 checksum 校验或自动部署执行。

## 2. 连接本地数据库

当前 `compose.yaml` 的端口映射是 `127.0.0.1:13445:5432`。Navicat 新建 **PostgreSQL** 连接：

| 配置 | 值 |
| --- | --- |
| 主机 | `127.0.0.1` |
| 端口 | `13445` |
| 用户 | `medstudy` |
| 密码 | 已设置的数据库账号密码，不是 Mac 登录密码 |
| 初始数据库 | `medstudy` |

先测试连接。Navicat 运行在 Mac 上，所以使用映射端口 `13445`；容器内部客户端使用 `5432`。

## 3. 导入顺序

1. 打开连接，明确选择 **medstudy 数据库**，不要选到 `postgres` 或其他数据库。
2. 使用「运行 SQL 文件 / Execute SQL File」，选择 `migrations/000001_initial_schema.sql`，编码选择 UTF-8。不同 Navicat 版本菜单文字可能不同。
3. 执行完整文件，不要只执行选中的一小段，不勾选「出错继续 / Continue on error」。遇到错误停止执行并检查报错；若查询连接仍处于失败事务，先执行 `ROLLBACK;`，不要通过删除约束强行导入。
4. 执行成功后刷新 `public` schema 下的表列表。
5. 在查询窗口执行下面的只读检查：

```sql
SELECT current_database(), current_user;

SELECT version, name, applied_at
FROM public.schema_migrations
ORDER BY version;

SELECT tablename
FROM pg_catalog.pg_tables
WHERE schemaname = 'public'
ORDER BY tablename;
```

应看到数据库 `medstudy`、版本 `1 / initial_schema` 和 **14 张表**，初始业务数据为空。

不需要导入 `tests/sql/` 中的文件。导入脚本不会启动后端、实现 HTTP 认证或自动生成业务数据。

## 4. 表的职责

| 表 | 用途 |
| --- | --- |
| `schema_migrations` | 数据库结构版本记录 |
| `users` | 用户、Argon2id 密码哈希、时区、数据 revision |
| `sessions` | refresh token 哈希、轮换和 token family 状态 |
| `ai_credentials` | 个人 DeepSeek Key 的 AES-GCM 密文 |
| `folders` | material / exam 资料夹 |
| `documents` | 解析后的文档文本与自动计算的字符数 |
| `flashcards` | 智能卡片、来源快照、去重键与 SRS 状态 |
| `quick_quizzes` | 已入库快刷题及 reviewed 状态 |
| `error_questions` | 错题、答案、解析及来源 |
| `error_flashcards` | 错题与卡片的关联 |
| `review_history` | 持久卡片和临时题的复习事件 |
| `safety_backups` | 恢复前的用户业务快照/对象存储引用 |
| `idempotency_keys` | 24 小时请求幂等状态和已完成响应 |
| `import_previews` | 导入/迁移 preview 的一次性 token 哈希与加密暂存 |

`import_previews` 是为 preview/一次性导入协议补充的内部存储表，不改变接口字段。临时考试和强化题按契约不新增持久题目表。看板由现有表聚合，不保存第二份统计真相。

## 5. 重要设计约定

### 所属关系与来源快照

- 每条业务 repository 查询仍必须带 `user_id`，建表不会自动实现账号权限。本版未启用 RLS，也未创建独立应用数据库角色；生产应用不能沿用开发管理员账号。
- 文档通过 `(user_id, folder_id)` 复合外键关联资料夹，不能跨用户引用；删除资料夹级联删除文档。
- 卡片、错题和快刷的 `source_doc_id/source_folder_id` 以及名称是历史快照。另有内部 `source_doc_ref_id/source_folder_ref_id` 保存当前存活资源的同用户外键。
- 来源触发器绑定当前同用户资源并拒绝其他用户的当前资源。来源删除后仅将 ref 字段置空，保留历史 ID/名称；避免把不同历史资料夹的错题去重键合并。
- 已删除的来源 ID 允许从旧快照恢复，此时 ref 为空。新建接口仍必须由 service 检查指定来源当前存在且属于当前用户，不能把历史恢复规则当作绕过权限检查的方法。
- 错题与卡片关联两端有同用户外键。删除错题只删除关联，不删除卡片；删除卡片会删除卡片自己的复习记录及错题关联。
- `review_history.subject_id` 对应 HTTP 的 `cardId`。持久卡片通过生成的 `flashcard_id` 外键保护；临时题与已删除快刷题的事件允许保留历史 UUID。quickQuiz 评分写入前的归属检查由 service 完成。

### 标准化与去重

- 卡片按用户范围先匹配完整来源键，未命中时匹配标准化内容；**不同来源但相同内容仍然去重**，与当前前端一致。
- 来源唯一索引和内容唯一索引同时保护并发写入。`originId` 按契约是字符串，不能强制外键到临时考试题；空白 originId 由 Go 规范为 NULL。
- 标准化规则为 NFKC、Unicode trim、小写和合并空白。数据库生成去重键；Go 实现应通过中英文、Unicode 和大小写测试与数据库函数对齐，不能假设所有语言/locale 的大小写转换完全一致。
- 长正文使用 SHA-256 索引，避免 B-tree 文本键大小限制。service 在冲突后比较标准化原文，避免将理论上的哈希碰撞误判为重复。
- 错题仅按 `trim(question) + sourceFolderId` 去重；无来源的 NULL 也参与唯一性判断。没有擅自增加错题内容的 NFKC 或大小写规则。
- 用户名暂按同一 NFKC/trim/小写/空白标准化函数生成唯一键，原始显示名保留。注册与登录查询必须使用同一规则。

### 数据类型与应用职责

- ID 使用 `uuid`，默认 `gen_random_uuid()`，旧快照合法 UUID 可以显式插入；不依赖额外数据库扩展。
- 时间使用 `timestamptz`；HTTP 边界转换成 epoch 毫秒。不要把毫秒整数直接写进时间列。
- `options` 使用 `jsonb` 字符串对象；没有选项用 SQL NULL，不是字符串 `"null"`。
- 密码只存 Go 生成的完整 Argon2id 编码；数据库的前缀 CHECK 不等于完整密码哈希校验。注册明文密码的字符/字节限制由 Go 检查。
- IANA 时区有效性、请求体/文本大小等可配置限额由 Go 检查，不能仅靠 SQL 长度约束。
- 文档字符数由数据库生成；名称写入前由 Go trim，新建文档正文至少 10 字符。历史 folderId 允许为 NULL，但新建接口仍要求有效资料夹。
- AI Key 和导入暂存使用 AES-GCM 密文、12 字节 nonce、密钥版本；加密主密钥不进入数据库。同一密钥下的 nonce 唯一性由加密实现保证。
- refresh token/import token 仅存 32 字节 SHA-256 哈希，不存原文；会话轮换、同 family 校验、重放吊销、有效期与消费锁由 service 完成。
- `updated_at` 由触发器更新；`users.revision` 不自动更新，service 必须在写入/恢复事务中递增。
- SRS + 复习记录、幂等完成响应 + 业务写入、导入替换 + 安全备份 + token 消费均需要应用事务；建表不替代业务实现。
- 幂等 response 存业务 JSON；不缓存密码、token、AI Key 或 `Set-Cookie`。过期记录清理和异常未完成请求处理在实现 service 时安排。
- 至少保留最近三份恢复前安全备份由 service 实现；用户业务快照不替代 PostgreSQL 全库备份。

## 6. 本地隔离验证

```bash
bash scripts/verify_schema.sh
```

工具使用本机已有的 `postgres:18` 镜像，创建一个临时容器；无发布端口、网络隔离、数据在 tmpfs，绝不连接当前 Compose 的 `medstudy` 数据库。检查首次导入、关键约束、删除规则和重复导入保护。测试结束会删除它自己创建的临时容器。

临时容器的 trust 认证仅用于完全隔离的测试，不用于开发数据库或生产部署。工具没有配置 CI；按需运行。

## 7. Go 配置与真实连接验证

本地配置由 `internal/config.LoadFile` 读取，连接由 `internal/platform/postgres.Open` 验证。两者已组装进 `cmd/api.run`，2026-10-11 用户确认实际启动验收成功。真实 `.env` 保持被 Git 忽略，只提交无秘密的 [.env.example](../.env.example)。

本地文件应包含 `DATABASE_URL` 与 `POSTGRES_PASSWORD`：连接地址不内嵌密码，密码单独保存。保留已有正确密码，不用为运行测试复制或修改密码。当前本机连接地址使用端口 `13445`；`sslmode=disable` 仅用于本机开发，不作为生产 TLS 配置。

在项目根目录运行：

```bash
MEDSTUDY_TEST_ENV_FILE=/Users/a-void/Code/MedStudy-Backend/.env \
  go test -v ./internal/platform/postgres \
  -run '^TestOpenIntegration$' -count=1 -timeout 15s
```

`MEDSTUDY_TEST_ENV_FILE` 是我们定义的测试选择项，不是 Go 标准变量。测试在明确选择文件后读取配置，把地址和密码作为参数传入新版 `Open`；不设置进程级密码环境，不需要手动 source，也不自动扫描任意 `.env`。文件方式优先于直接设置 `MEDSTUDY_TEST_DATABASE_URL`。不使用文件时，测试从 `MEDSTUDY_TEST_DATABASE_URL` 和 `PGPASSWORD` 读取地址与密码。

`go test` 的测试进程以被测包目录为工作目录，`.env` 与 `./.env` 都不会指向项目根目录。应传绝对路径；在项目根目录也可以写 `MEDSTUDY_TEST_ENV_FILE="$PWD/.env"`，由终端先展开为绝对路径。

测试只连接并执行 `SELECT 1`，不建表、不修改数据，不验证全部表是否已导入。它实际连接当前本地开发库，不是假定存在一个独立测试库；未来有写入的数据库集成测试必须单独隔离。

未指定文件或连接环境时显示 SKIP，不算真实连接通过。预期结果是 `PASS: TestOpenIntegration`；只分享测试结果，不分享真实文件、连接密码或密钥。此测试不能替代尚未实现的 API 鉴权、用户隔离和持久化验收。

2026-10-10：用户确认使用正确配置路径后的真实连接测试通过；助手复核普通 Go 测试与 vet 通过。当前 API 启动流程尚未接入配置与数据库。

上述独立真实连接测试记录对应原两参数版本；当前已改为显式密码参数，普通 Go 测试与 vet 再次通过。新版独立 TestOpenIntegration 未重复运行；用户确认下述实际启动和健康检查成功，启动中的 Open/Ping 验证了新版连接路径，不代表业务写入已验收。

### 本地 API 启动验收

2026-10-11：启动实现与失败路径测试完成，普通测试、vet 及 API/连接包竞态检查通过。用户将监听地址改为 `127.0.0.1:11345`，并确认实际启动与健康检查成功；成功路径结果来自用户运行反馈。启动方法如下，在项目根目录执行：

```bash
go run ./cmd/api
```

应先看到数据库连接成功，再看到访问地址和启动 Logo。此命令会读取根目录的 .env，连接本地开发库，不自动建表或插入业务数据；不要分享密码或完整配置。在另一终端执行：

```bash
curl --noproxy '*' -i http://127.0.0.1:11345/health/live
```

应返回 200 与 `{"status":"ok"}`。`--noproxy '*'` 使本地验证绕过代理；仅看到 Logo 不代表端口绑定成功，必须检查实际请求。HTTP 服务当前仅有健康检查路由，尚未实现鉴权、资料夹 API、生产环境配置或优雅停机。

### 资料夹仓储的独立写入测试

在项目根目录执行，先确认 Docker 已启动且本机已有 `postgres:18` 镜像：

```bash
MEDSTUDY_TEST_FOLDER_DOCKER=1 go test -v ./internal/platform/postgres \
  -run '^TestFolderRepository' -count=1 -timeout 2m
```

测试代码位于 [folder_repository_test.go](../internal/platform/postgres/folder_repository_test.go)。这里的环境变量只开启独立测试，不设置数据库密码，也不选取 `.env`。该测试不接收开发库 URL，不复用 Compose 的 db：它自己创建临时容器，以 tmpfs 保存数据，只发布随机的 `127.0.0.1` 端口，使用无实际价值的测试凭据，导入真实迁移，再创建测试用户和资料夹。镜像使用 `--pull=never`，不会自动下载。

测试检查 INSERT/RETURNING/Scan、UUID 与创建时间、持久化字段、不同用户 ID、特殊字符参数，以及外键和 CHECK 的失败路径。事务版另检查每次成功创建仅递增当前用户 revision 一次，插入失败时版本不变；在临时库准备 bigint 最大版本的测试用户，使版本递增失败，确认前面的资料夹插入一并回滚。结束时先关闭连接池，再核实专用标签并删除本次创建的容器；这些临时测试数据会随之丢弃，不影响项目容器和数据卷。若清理失败，测试会明确提示检查带 `medstudy.folder-test` 标签的临时容器。

不设置 `MEDSTUDY_TEST_FOLDER_DOCKER=1` 时，真实写入测试显示 SKIP，不代表通过。2026-10-13：普通测试、vet 与仓储相关 race 检查通过；随后用户确认独立 Docker 写入测试成功，此结果来自用户反馈，助手未重复运行。该测试通过不代表 HTTP 鉴权、用户隔离和 revision 事务已经实现。

后续用户已实现事务创建，助手扩展上述 revision/回滚测试，编译、vet、全部普通测试及 folder/postgres 相关 race 检查通过。用户随后确认新版事务隔离测试 PASS，真实结果来自用户反馈，助手未重复启动容器或连接开发库；HTTP 鉴权及资料夹路由仍未实现。

## 8. 参考

- [PostgreSQL 外键、复合约束与列级 SET NULL](https://www.postgresql.org/docs/18/ddl-constraints.html)
- [PostgreSQL Unicode 文本处理](https://www.postgresql.org/docs/18/functions-string.html)
- [PostgreSQL 唯一索引与 NULLS NOT DISTINCT](https://www.postgresql.org/docs/18/indexes-unique.html)
- [Navicat 官方：运行 SQL 文件](https://help.navicat.com/hc/en-us/articles/218303897-How-to-import-SQL-files-script-files-or-command-files-into-the-server)

具体测试结果以运行记录为准；约束测试通过不意味着尚未实现的 API 已经完成。
