# MedStudy：从零学习 Go 并完成生产级后端

> 目标：以 MedStudy 为唯一主线，从不会 Go 开始，逐步独立完成一个可测试、可部署、可迁移、可维护的前后端分离系统。

配套文档：

- [Git 分支入门与安全开发](GIT_BRANCH_GUIDE.md)
- [Go 后端 API 接口文档](GO_BACKEND_API.md)
- [OpenAPI 机器可读契约](openapi.yaml)

## 1. 先校准目标

完成一个项目不能让任何人“一次性完全掌握”整个 Go 生态，但如果你真正完成本文中的代码、练习、测试和复盘，你应当能够：

- 阅读和编写常见 Go 代码，理解值、指针、切片、映射、结构体、方法、接口、泛型和包。
- 使用 `context`、goroutine、channel、互斥锁，并知道什么时候不该使用并发。
- 用标准库构建 HTTP JSON API，理解中间件、认证、CORS、超时和优雅停机。
- 设计 PostgreSQL 表、索引和事务，使用 pgx 编写参数化 SQL，分析常见慢查询。
- 设计清晰的 handler/service/repository 边界，避免“为了分层而分层”。
- 编写单元、HTTP、数据库集成、竞态和模糊测试。
- 安全处理密码、令牌、AI Key、文件上传、日志和配置。
- 使用 Docker、CI、结构化日志、指标和健康检查部署服务。
- 解释 MedStudy 每个核心功能从请求到数据库的完整路径，并独立定位问题。

这相当于“具备独立承担常规 Go Web 后端工作的能力”，后续仍需通过更多项目和生产问题继续积累。

## 2. 当前项目与迁移目标

### 2.1 当前架构

现有项目是 React 19 + TypeScript 的纯前端应用：

```text
React 页面
  ├─ IndexedDB：资料、卡片、错题、快刷、复习记录
  ├─ DeepSeek：浏览器直接调用，Key 在 localStorage
  └─ 腾讯 COS：浏览器携带长期密钥同步 JSON 快照
```

主要问题：

- “用户名”只是数据空间名，没有认证，任何知道名字的人都可能碰到同名数据。
- DeepSeek Key 和 COS 长期密钥位于浏览器，公开部署风险较高。
- 多设备依靠全量 JSON 同步，存在冲突选择和覆盖风险。
- 数据查询、约束、审计、服务端备份和权限控制能力有限。

### 2.2 目标架构

```text
React SPA（继续复用现有界面）
        │ HTTPS / JSON
        ▼
Go API
  ├─ 认证、授权、输入校验、限流
  ├─ 资料、卡片、错题、快刷、复习与统计服务
  ├─ DeepSeek 代理和结构化输出校验
  ├─ 文档解析与旧快照迁移
  └─ 结构化日志、指标、健康检查
        │
        ▼
PostgreSQL（主数据）  +  对象存储（可选的备份/原文件）
```

前端不会立刻切换。两个项目使用完全独立的目录和 Git 历史：后端按当前决定直接在本项目的 `main` 开发并先部署到 staging；相邻的 `/Users/a-void/Code/MedStudy` 前端仓库继续从自己的 `main` 运行原本的 IndexedDB/COS 版本，并预留 `feat/go-api-integration` 作为未来适配分支。

## 3. 技术选择及原因

| 范围 | 选择 | 学习价值与理由 |
| --- | --- | --- |
| Go | Go 1.27.x | 当前稳定主版本；项目固定补丁版本，定期升级 |
| HTTP | 标准库 `net/http` + `http.ServeMux` | 先掌握 Go 原生 HTTP、方法路由和中间件，再学框架不会迷失 |
| 数据库 | PostgreSQL + `github.com/jackc/pgx/v5` | 显式 SQL、事务、连接池和 PostgreSQL 能力，避免初学时被 ORM 隐藏细节 |
| ID | UUID | 可保留当前前端 UUID，并便于迁移旧快照 |
| 迁移 | SQL 文件 + 小型 Go migration runner | 学会真正的 DDL、版本表和事务；以后可替换为 goose/tern |
| 日志 | 标准库 `log/slog` | 结构化、标准库内置、方便接日志平台 |
| 配置 | 环境变量 + 显式校验 | 遵守十二要素，密钥不进入仓库 |
| 测试 | `testing`、`httptest`、真实 PostgreSQL 集成测试 | 学会 Go 原生测试体系，不依赖大型测试框架 |
| API 契约 | OpenAPI 3.1 | 前后端并行、自动校验和生成客户端的共同协议 |
| 部署 | Docker Compose，后续再接 CI/CD | 与现有部署方式一致，便于渐进迁移 |

不在第一版引入微服务、消息队列、Kubernetes、复杂 DI 框架和 ORM。它们不是“高级”的同义词；当流量和边界真的需要时再引入。

## 4. 建议的目录结构

后端是独立仓库，代码直接放在项目根目录，不再套一层 `backend/`：

```text
MedStudy-Backend/
├── cmd/
│   ├── api/main.go                    # API 进程组装入口
│   └── migrate/main.go                # 数据库迁移命令
├── internal/
│   ├── app/                           # 启动、关闭、依赖组装
│   ├── config/                        # 环境变量读取和校验
│   ├── auth/                          # 密码、访问令牌、刷新会话
│   ├── httpapi/                       # router、handler、中间件、DTO
│   ├── folder/                        # 资料夹领域服务
│   ├── document/                      # 文档元数据、解析、存储
│   ├── flashcard/                     # 去重、SRS、复习事务
│   ├── errorbook/                     # 错题本
│   ├── quickquiz/                     # 模拟快刷
│   ├── dashboard/                     # 聚合统计
│   ├── ai/                            # DeepSeek 客户端与业务提示词
│   ├── backup/                        # 导出、恢复、安全备份
│   ├── legacy/                        # IndexedDB/COS 快照迁移
│   └── platform/                      # clock、id、加密等基础适配
├── migrations/                        # 按版本排序的 SQL
├── testdata/                          # 不含真实隐私数据的测试样本
├── docs/
├── go.mod
├── go.sum
├── Dockerfile
└── .env.example
```

相邻的 `../MedStudy` 是功能参考，不属于本仓库。后端开发期间 Codex 可以读取其中的类型和页面来核对行为，但不得修改它。

Go 的 `internal` 目录会阻止仓库外的包直接导入这里的实现，有助于维护边界。不要创建 `utils`、`common` 之类无限膨胀的杂物包；代码应放在它真正服务的业务概念旁边。

## 5. 学习方式：每个阶段都交付可运行成果

不要先连续看几周语法再开始项目。每个阶段都使用这个循环：

1. 用自己的话解释本阶段概念。
2. 先写一个失败测试或明确验收条件。
3. 实现能让测试通过的最小代码。
4. 运行格式化、测试和静态检查。
5. 阅读 diff，解释每一个错误分支。
6. 做一个小提交并写学习笔记。

常用质量命令最终应形成肌肉记忆：

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go test -cover ./...
```

安装安全扫描工具后再运行：

```bash
govulncheck ./...
```

## 6. 分阶段路线

时间只是参考。是否进入下一阶段，只看“验收标准”，不看花了几天。

### 阶段 0：Git、安全边界与需求冻结

学习：工作区、暂存区、提交、分支、合并、冲突、Pull Request、环境变量和密钥泄漏风险。

动手：

- 阅读 `docs/GIT_BRANCH_GUIDE.md`。
- 确认后端目录为 `/Users/a-void/Code/MedStudy-Backend`，分支为 `main`。
- 确认前端仓库的 `main` 保持干净，API 适配只允许在未来的 `feat/go-api-integration` 开始。
- 建立“现有功能 → API → 测试”追踪表。
- 确认 `.env`、数据库口令、DeepSeek Key 不会被提交。

验收：

```bash
git branch --show-current
git status
```

你能解释为什么后端仓库的任何分支都不可能直接改变另一个前端仓库；只有未来主动修改、合并并部署前端仓库时，线上页面才会变化。

### 阶段 1：Go 基础与第一个健康检查

学习：

- `go` 命令、module、package、import、导出规则。
- 变量、常量、基础类型、零值、`if`、`for`、`switch`。
- 函数、多返回值、`defer`、错误值。
- 数组、slice、map，以及 `append` 可能更换底层数组。
- struct、方法、指针和值接收者。

动手：

1. 在项目根目录创建 `go.mod`。
2. 实现 `GET /health/live` 返回 JSON。
3. 给 JSON 编码失败、方法不匹配设计统一处理。
4. 为 handler 写 `httptest` 测试。

验收：

```bash
go run ./cmd/api
curl -i http://localhost:8080/health/live
go test ./...
```

必须能逐行解释 `main`、`http.Handler`、`http.ResponseWriter`、`*http.Request` 和错误检查。

练习：手写 `normalizeFlashcardText` 的 Go 版本，先覆盖前后空格、连续空白和大小写；Unicode NFKC 在引入依赖后补齐。

### 阶段 2：接口、错误、依赖注入和可测试时间

学习：

- Go 接口是隐式实现；接口应由使用方定义，并保持小型。
- `errors.Is`、`errors.As`、`fmt.Errorf("...: %w", err)`。
- `context.Context` 的取消、截止时间和请求范围值。
- 构造函数注入；为什么不使用全局数据库和全局配置。

动手：

- 定义 `Clock` 和 `IDGenerator` 小接口。
- 定义领域错误：`ErrNotFound`、`ErrConflict`、`ErrUnauthorized`、`ValidationError`。
- 统一错误响应，并给每个响应加 `requestId`。
- 在测试中注入固定时间和固定 UUID。

验收：同一个输入的测试输出完全确定；日志保留底层错误，HTTP 响应不泄露 SQL、密钥或栈信息。

### 阶段 3：HTTP、JSON、中间件和服务生命周期

学习：

- ServeMux 的方法+路径路由、路径变量、header、状态码。
- JSON tag、DTO 与领域模型的区别、严格解码和 body 大小限制。
- 中间件闭包、panic recovery、请求日志、CORS。
- goroutine、channel、signal、HTTP server 超时与优雅停机。

动手：

- 实现 `writeJSON`、`decodeJSON`、`recover`、`requestID`、`accessLog`、`cors` 中间件。
- 对未知 JSON 字段报错；只允许一个 JSON 值。
- 配置 `ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout`、`IdleTimeout`。
- 收到 SIGTERM 时停止接收新请求并在截止时间内关闭。

验收：

- panic 被记录但进程不因单次请求退出。
- 非白名单 Origin 没有 CORS 许可头。
- 超大 body 返回 `413`。
- `go test -race ./...` 通过。

### 阶段 4：PostgreSQL、SQL、pgx 和数据库迁移

学习：

- 表、主键、外键、唯一约束、检查约束、索引。
- `NULL` 与 Go 零值不同。
- 参数化 SQL 防注入；不要用字符串拼接用户输入。
- 连接池不是单连接；事务必须在同一连接/事务对象执行。
- 隔离级别、原子性、死锁和幂等。

动手：

- 使用 Compose 启动只供开发的 PostgreSQL。
- 实现版本化 SQL migration runner。
- 建立 users、folders、documents、flashcards、review_history、error_questions、error_flashcards、quick_quizzes、sessions、ai_credentials、safety_backups 表。
- 实现 `/health/ready`，检查数据库连接但设置短超时。
- repository 接收 `context.Context`，所有查询显式列名。

验收：

- 全新数据库可一次迁移到最新版。
- 重复执行迁移不破坏数据。
- 外键、唯一约束和用户隔离有集成测试。
- 每个列表查询都解释对应索引。

### 阶段 5：认证、授权和个人 AI Key

学习：

- 密码只保存 Argon2id 哈希，不能加密后可逆保存。
- 短期 access token 与可轮换 refresh session 的职责。
- Bearer token、HttpOnly/Secure/SameSite Cookie、CORS 和 CSRF 的关系。
- 认证回答“你是谁”，授权回答“你能操作什么”。
- 加密、哈希、编码不是一回事。

动手：

- 注册、登录、刷新、退出、`GET /me`。
- access token 只放前端内存；refresh token 只通过 HttpOnly Cookie 传输，数据库只存其哈希。
- 刷新时轮换 token；发现已使用旧 token 时吊销整个 token family。
- 每条业务 SQL 都必须按 `user_id` 限定，不能“先按 id 查出再判断”。
- 支持服务器默认 DeepSeek Key，或将用户自带 Key 用 AES-GCM 加密后保存；API 永不返回明文。
- 给登录、刷新和 AI 接口限流。

验收：用户 A 即使拿到用户 B 的资源 UUID，也只能得到 `404`；日志、响应和数据库快照中不会出现密码或完整 Key。

### 阶段 6：资料库与文件上传

学习：

- REST 资源建模、分页、过滤、`multipart/form-data`。
- MIME 嗅探与扩展名不可信、路径穿越、压缩炸弹和上传限额。
- 流式读取与一次性 `io.ReadAll` 的内存差异。
- 何时将原文件放对象存储、何时只保留解析文本。

动手：

- 文件夹增删改查，类型仅允许 `material`、`exam`。
- 删除文件夹时事务级联删除文档，保持当前行为。
- 文档列表、详情、重命名、删除。
- 提供两个写入口：前端已解析文本的 JSON 接口，以及逐步实现的 multipart 原文件解析接口。
- TXT 先完成；PDF、DOCX 分别做适配器；旧二进制 DOC 不可靠时返回明确错误并提示转换。
- 扫描件或有效字符不足 10 个时返回 `document_text_too_short`。

验收：空文件、伪造 MIME、超限文件、扫描版 PDF、损坏 DOCX、删除他人文件和重复请求都有测试。

### 阶段 7：智能卡片、去重、SRS 与事务

学习：

- 领域不变量应同时由应用逻辑与数据库约束保护。
- 批量插入、事务、并发竞争和唯一索引。
- 浮点数比较、时间与时区、整数毫秒 DTO。

动手：

- 批量创建卡片，并按“来源键”或“规范化后的 front+back”去重。
- 实现当前 SRS 规则：

```text
good:
  repetitions=0 → interval=1
  repetitions=1 → interval=3
  其他 → round(interval * easeFactor)
  repetitions += 1, easeFactor += 0.1

hard:
  interval=max(1, round(interval * 1.2))
  easeFactor=max(1.3, easeFactor-0.15)

forgot:
  repetitions=0, interval=0
  easeFactor=max(1.3, easeFactor-0.2)
```

- 同一个事务更新卡片并写 review history。
- 删除卡片时级联删除它的复习记录；错题到卡片使用关联表维护。
- 查询到期卡片和薄弱卡片。

验收：

- 表驱动测试覆盖三种评分、边界和多轮复习。
- 两个并发请求不能创建重复卡片。
- 两个并发复习请求通过行锁或乐观版本控制避免丢更新。
- `go test -race ./...` 通过。

### 阶段 8：错题、快刷、考试记录和数据看板

学习：服务编排、聚合查询、数据库聚合与 N+1 查询、幂等写入。

动手：

- 错题 CRUD、按资料夹筛选、关联派生卡片。
- 快刷批量导入、编辑、删除、未复习列表、评分并写复习记录。
- 保留考试/强化题的临时前端会话，但通过通用 review event 接口记录评分。
- 看板返回卡片总数、间隔超过 21 天的掌握数、错题数、当天复习数、7 天趋势和薄弱卡片。
- 下载 TXT 仍可由前端完成，不必为了“后端化”移动纯展示逻辑。

验收：统计结果以用户时区计算；上海时区的跨日边界有测试；分页不会遗漏或重复记录。

### 阶段 9：DeepSeek 接入与可靠性

学习：

- 复用的 `http.Client`、连接池、context timeout。
- 上游错误映射、敏感信息脱敏、结构化 JSON 校验。
- 指数退避、抖动、幂等与“为什么不能盲目重试所有 POST”。
- goroutine 泄漏、并发上限、信号量和 backpressure。

动手：把当前六类 AI 能力移到后端：

1. 从资料生成考点卡片。
2. 根据真题风格和学习资料生成模拟快刷题。
3. 生成考试试卷。
4. 手动错题分析。
5. 从快刷题、考试题或错题生成智能卡片。
6. 薄弱考点归纳与强化练习。

要求：

- 提示词放服务端并版本化，不允许客户端提交任意 system prompt。
- 校验 model 白名单、文档归属、数量 1～100、总上下文大小。
- DeepSeek Key 只在发往上游的 Authorization header 中出现。
- 将现有 `missing_key`、`authentication`、`insufficient_balance`、`rate_limit`、`invalid_model`、`request_too_large`、`invalid_request`、`service_unavailable`、`network`、`timeout`、`invalid_response`、`unknown` 映射保留下来。
- 批量错题生成设置小并发上限，返回每项成功/失败结果，不因一项失败丢掉已完成项。

验收：测试使用假的 DeepSeek server，不调用真实网络；覆盖 401、402、404、429、5xx、超时、非法 JSON、空 choices 和 Key 脱敏。

### 阶段 10：备份、恢复与旧数据迁移

学习：schema version、校验、dry-run、幂等迁移、补偿与灾难恢复。

动手：

- 导出与当前 `medstudy-backup` v1 兼容的 JSON 快照。
- 导入分两步：preview 返回摘要和短期 confirm token，confirm 前先生成安全备份，再在事务中替换。
- 每个用户保留最近的恢复前备份；对象存储可作为服务端异地备份，但客户端不再持有 COS 长期密钥。
- 旧快照 dry-run 检查 UUID、引用、重复、大小、username 映射和 schemaVersion。
- 支持同一 `Idempotency-Key` 重试而不重复导入。

验收：随机中断恢复过程，原数据必须仍完整；新旧快照数量和抽样内容一致；备份恢复演练有书面记录。

### 阶段 11：完整测试金字塔

至少建立以下层次：

| 层次 | 重点 | 示例 |
| --- | --- | --- |
| 纯单元测试 | 无 I/O 的规则 | SRS、去重、错误映射、输入校验 |
| service 测试 | 编排和失败路径 | AI 成功但存卡失败、权限拒绝 |
| HTTP 测试 | 契约 | 状态码、JSON、认证、CORS、body 限制 |
| PostgreSQL 集成测试 | 真正约束和事务 | 并发去重、行锁、级联删除、用户隔离 |
| 契约测试 | OpenAPI | 所有响应符合 schema |
| 端到端测试 | 用户旅程 | 登录→导入资料→生成→复习→备份 |
| fuzz | 不可信输入 | 备份 JSON、AI JSON、文档元数据 |
| race | 数据竞态 | 并发 handler、缓存和关闭流程 |

提交前后端总检查：

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
govulncheck ./...

cd ..
npm run lint
npm run test:run
npm run build
```

### 阶段 12：可观测性、性能和部署

学习：结构化日志、RED 指标、trace、profiling、容量限制、零停机与回滚。

动手：

- 日志字段至少有 requestId、userId、route、status、durationMs；不记录正文、答案、Key、密码和 token。
- 指标至少有请求量、错误率、延迟、活跃数据库连接、AI 上游延迟/错误、导入大小。
- 对 API 设置 server timeout、数据库 query timeout、AI timeout 和进程 shutdown deadline。
- Docker 使用非 root 用户、多阶段构建、固定基础镜像版本和只读文件系统（临时目录例外）。
- CI 执行格式、vet、test、race、漏洞扫描、镜像构建和 migration 检查。

验收：能从 requestId 追踪一次失败请求；能解释一次慢请求耗时在 API、数据库还是 DeepSeek；能在 staging 完成升级和回滚。

### 阶段 13：前端适配与无中断切换

后端全部通过后才修改前端数据层：

1. 进入 `/Users/a-void/Code/MedStudy`，切换到 `feat/go-api-integration`，绝不直接在前端 `main` 开发。
2. 把现有 `useDb` 的接口视为适配目标，新建 API adapter，不先重写页面。
3. 增加 `VITE_DATA_MODE=local|api`，生产初始值仍为 `local`。
4. 本地和 staging 使用 `api`，逐页完成契约测试。
5. 增加注册/登录和旧数据迁移向导。
6. 用户在旧版导出 JSON；新版先 preview，确认账号归属后再 import。
7. 对 COS 中仅有的旧数据，使用受控的管理员 CLI 按明确用户名映射导入；禁止做“输入用户名即可读取旧 COS 数据”的公开接口。
8. 全量验收、备份、回滚演练完成后，再将前端功能分支合入前端 `main` 并切换生产配置。

不要同时重做 UI。第一轮只替换数据与 AI 访问路径，这样问题更容易定位，也更接近“功能保持不变”。

## 7. 功能完成追踪矩阵

| 现有功能 | 后端责任 | 主要验收 |
| --- | --- | --- |
| 用户名数据空间 | 正式账号、会话、用户隔离 | 跨用户 UUID 访问返回 404 |
| 资料/真题文件夹 | folders CRUD | 类型校验、级联删除文档 |
| TXT/PDF/DOC/DOCX | 上传、解析或接收前端解析文本 | 大小/MIME/扫描件/损坏文件 |
| 考点生成 | AI 资料卡片接口 | 文档归属、1～100、JSON 校验 |
| 智能卡片 | 去重、CRUD、到期、SRS | 并发去重、复习事务 |
| 真题模拟天天练 | AI 模拟题接口、批量写入快刷 | 题型、来源、导出内容一致 |
| 模拟快刷 | CRUD、未复习、评分、错题/卡片 | 评分事件和幂等 |
| 考试模式 | AI 试卷、评分事件、错题/卡片 | 题型、答案、人工主观题流程 |
| 错题解析 | AI 分析、错题 CRUD、派生卡片 | 关联不重复、批量部分失败 |
| 数据看板 | 聚合、趋势、薄弱卡片 | 时区边界、SQL 性能 |
| 强化练习 | AI 薄弱点与练习接口 | 仅使用所属资料/卡片 |
| JSON 导出/恢复 | 快照、preview、事务替换 | 恢复前备份、失败不覆盖 |
| COS 冲突备份 | 服务端安全备份替代 | 保留策略、权限、可下载 |
| DeepSeek Key 配置 | 默认 Key 或用户加密 BYOK | 永不返回/记录明文 |
| 同步状态 | API 成为权威数据源 | 无全量快照冲突，失败可重试 |

## 8. Go 重点知识复盘清单

项目结束时，不看答案解释下面的问题：

### 语言

- slice 的 `len`、`cap` 和共享底层数组会造成什么问题？
- map 为什么不能并发读写？何时复制 map，何时加锁？
- interface 的动态类型和动态值是什么？“含 nil 指针的 interface”为什么可能不等于 nil？
- 值接收者与指针接收者如何影响方法集、复制和并发安全？
- `defer` 的参数何时求值？循环里使用 `defer` 有何风险？
- sentinel error、typed error、wrap 分别适合什么情况？
- 泛型能解决什么重复，何时普通接口反而更清楚？

### 并发

- context 的取消信号应该从哪里向哪里传播？为什么不能把它存进 struct 长期复用？
- channel 由谁关闭？关闭是为了什么？
- goroutine 如何退出？怎样识别泄漏？
- 什么时候使用互斥锁比 channel 更直接？
- `go test -race` 能发现什么，不能证明什么？

### HTTP 与数据库

- header 写出后为什么不能再改变状态码？
- 一个请求超时后，数据库和上游 AI 请求如何一起取消？
- 为什么每个 SQL 都要带 `user_id` 条件？
- 事务中为什么不能混用 pool 查询和 tx 查询？
- 唯一索引和“先查再插”各自解决什么？
- 如何从 `EXPLAIN (ANALYZE, BUFFERS)` 判断索引是否有效？

### 安全与运维

- 为什么前端环境变量不是秘密？
- 密码哈希与 AI Key 加密为什么用不同算法？
- access token 泄露、refresh token 重放分别如何控制影响？
- 数据库 migration 失败怎样回滚？应用版本和 schema 如何兼容？
- 备份存在不等于可恢复，怎样证明恢复有效？

## 9. 每阶段让 Codex 如何教你

在新的 Codex 终端中粘贴下面这段即可开始。它要求 Codex一次只带你完成一个阶段，避免直接生成一大堆你无法解释的代码。

```text
我正在 /Users/a-void/Code/MedStudy-Backend 独立项目中学习并实现 MedStudy 的 Go 后端。

请先完整阅读：
1. docs/GO_BACKEND_LEARNING_PATH.md
2. docs/GO_BACKEND_API.md
3. docs/openapi.yaml
4. docs/GIT_BRANCH_GUIDE.md
5. 本项目 README.md
6. 将相邻的 /Users/a-void/Code/MedStudy 视为只读参考，阅读其中 src/types.ts、src/store.tsx、src/api.ts 和当前阶段涉及的页面

先运行 pwd、git branch --show-current 和 git status。pwd 必须是 /Users/a-void/Code/MedStudy-Backend，分支必须是后端 main。不要进入相邻前端仓库写文件，不要部署生产环境，不要修改当前线上配置。后端完成后，前端改造只能在前端 feat/go-api-integration 分支进行。

你是我的 Go 导师兼结对开发者。我完全不会 Go。请从“阶段 0”开始，一次只推进一个可验收的小任务：
- 改代码前，用中文讲清本次涉及的 Go 概念，并将 TypeScript 写法和 Go 写法对照；
- 让我先复述关键点，核心练习优先让我动手；如果我明确让你实现，再由你修改文件；
- 每一步都展示要运行的命令，解释编译错误，不跳过错误处理；
- 先定义验收或测试，再实现；完成后运行 gofmt、go vet、go test，适用时运行 race/integration 测试；
- 严格遵守 API 契约，不擅自新增或改变字段；如果契约需要修改，先解释原因并同步修改 Markdown 和 OpenAPI；
- 每完成一个阶段，更新文档中的进度记录，列出我必须能独立回答的问题，并建议一个小而完整的 Git 提交；
- 发现密钥、个人数据、破坏性 Git 命令或生产操作时立即停止并提醒我。

现在只检查环境和阶段 0 的状态，然后给我第一个 20～40 分钟可完成的小任务，不要一次实现整个后端。
```

如果你更想让 Codex 主导实现，将上面“核心练习优先让我动手”替换为：

```text
你可以直接实现，但每次只做一个小任务；完成后逐文件讲解，并让我修改一个关键点、补一个测试来证明我理解了。
```

## 10. 学习记录模板

每完成一个阶段，在项目根目录的 `LEARNING_LOG.md` 追加：

```markdown
## 阶段 N：标题

- 日期：
- 本次交付：
- 我能解释的 3 个概念：
- 我遇到的错误及根因：
- 我独立补的测试：
- 还不理解的地方：
- 验收命令与结果：
- Git commit：
```

## 11. 权威学习资料

优先阅读官方材料，不要把零散博客当语言规范：

- Go 安装、教程、数据库、REST、fuzz、govulncheck：https://go.dev/doc/tutorial/
- A Tour of Go：https://go.dev/tour/
- Effective Go：https://go.dev/doc/effective_go
- Go 内存模型：https://go.dev/ref/mem
- Go 安全资料：https://go.dev/doc/security/
- 标准库文档：https://pkg.go.dev/std
- pgx 官方仓库与文档：https://github.com/jackc/pgx
- PostgreSQL 官方文档：https://www.postgresql.org/docs/
- OpenAPI 规范：https://spec.openapis.org/oas/latest.html

阅读顺序不是从头到尾背完，而是在对应阶段查阅相关章节，并立即用 MedStudy 的代码验证。

## 12. 最终毕业任务

不借助现有实现，从空目录完成一个缩小版服务：注册登录、资料夹 CRUD、卡片批量去重、复习事务、7 天统计、备份导出；为它写单元和 PostgreSQL 集成测试，放进 Docker，在 staging 做一次升级与回滚。然后对照当前 MedStudy 实现做 code review，列出至少 5 个安全或可靠性改进。

如果这个任务能独立完成、能解释设计取舍、能在故障注入时定位问题，你就不再只是“跟着教程写过 Go”，而是已经具备了可验证的 Go 后端开发能力。
