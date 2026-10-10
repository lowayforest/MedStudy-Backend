# MedStudy 数据库详细设计

本文解释当前 [初始化 SQL](../migrations/000001_initial_schema.sql) 的实际结构，供学习、开发和评审使用。适用 PostgreSQL 18+、UTF8，数据库结构版本为 `1 / initial_schema`。

**建表完成不等于后端业务完成。** 下文会区分“数据库已保证”和“需要 Go 实现”；事务流程是后续实现要求，不表示对应接口已经写好。

如果要用 Navicat 导入，请看 [数据库初始化与导入指南](DATABASE_SETUP.md)。HTTP 字段和返回行为以 [API 契约](GO_BACKEND_API.md) 与 [OpenAPI](openapi.yaml) 为准。本文与 SQL 不一致时，先核对 SQL，再同步修正文档，不能依靠说明文字改变数据库行为。

## 1. 建议怎么读

第一次不必记住全部表：

1. 先读第 2～4 节，理解账号隔离、类型和表之间的关系。
2. 再读 `users`、`folders`、`documents` 三张表；它们最接近你接下来要写的资料夹接口。
3. 写卡片、错题接口前，读第 6～8 节：来源、去重和删除行为。
4. 做认证、评分、恢复时，再读对应表和第 9 节事务要求。
5. 第 10～12 节用于写 Go repository、检查边界和维护迁移。

逐表字段说明在第 5 节，可以作为字典查阅。

## 2. 先分清数据库账号与业务用户

你在 Navicat / psql 中使用的 `medstudy` 是 **PostgreSQL 登录角色**，用于建立数据库连接；它不是网站里的某个注册用户。

网站的用户存放在 `public.users` 表。例如“小明”和“小红”是两条业务数据，后端通常复用同一个应用数据库连接身份来访问它们。

```text
HTTP 登录用户 → Go 认证得到 user_id → 查询该用户的数据
                                ↓
                  应用数据库连接 → medstudy 数据库
                                      └─ public schema
                                          ├─ users
                                          ├─ folders
                                          └─ 其他表
```

这里 `public` 是数据库内部组织表、函数等对象的 schema，不是“允许互联网公开访问”的意思。

### 2.1 为什么业务表几乎都有 user_id

这是用户数据隔离的基础。查询单条资料夹也应同时检查资源 ID 和用户 ID：

```sql
SELECT id, name, type, created_at, updated_at
FROM public.folders
WHERE user_id = $1 AND id = $2;
```

`$1`、`$2` 是 PostgreSQL 查询参数占位符，之后由 pgx 绑定真实值，不是字符串拼接。`$1` 应来自服务端认证结果，不能信任请求体自行传入的用户身份。

只写 `WHERE id = $1`，即使表中有 `user_id`，也不会自动帮你检查当前登录者是否有权限。

本版 **未启用行级安全策略 RLS**，也没有创建独立生产应用角色。同用户外键限制的是关系合法性，不是所有读取权限。后续生产应用应使用受限角色，不能沿用开发管理员身份。RLS 是可追加的防线，需要单独设计连接身份与策略。[PostgreSQL 官方：行级安全](https://www.postgresql.org/docs/18/ddl-rowsecurity.html)

### 2.2 为什么 id 是主键，还要 UNIQUE(user_id, id)

`id` 本身全表唯一，额外的复合唯一约束主要是为了让其他表建立这样的外键：

```sql
FOREIGN KEY (user_id, folder_id)
REFERENCES public.folders(user_id, id)
```

它要求“资料夹 ID 存在，且属于这一行的用户”，而不仅是“某个用户的资料夹存在”。例如小明的文档不能挂到小红的资料夹。

这会增加一个唯一索引，但换来了数据库对跨用户关联的兜底保护。[PostgreSQL 官方：复合约束与外键](https://www.postgresql.org/docs/18/ddl-constraints.html)

## 3. 数据类型与字段字典读法

### 3.1 类型

| SQL 类型 | 在本项目中的含义 | Go 边界注意事项 |
| --- | --- | --- |
| `uuid` | 用户、资源、事件的 ID | 合法 UUID；不是自增整数，也不是权限凭证 |
| `text` | 名称、题干、正文等文字 | 长度限制要看 CHECK 和业务规则，`text` 不代表不限制输入 |
| `bigint` | revision、字节大小、结构版本等整数 | 通常对应 `int64`，边界校验不能省略 |
| `integer` | 字符数、间隔天数、次数等 | 数据库为 32 位整数；不能假定无限大 |
| `double precision` | 卡片易度因子 | 对应浮点数，注意特殊值和范围 |
| `boolean` | 是否已复习 | `true` / `false` |
| `timestamptz` | 一个明确的时间点 | 通常读取为 `time.Time`，HTTP 按契约转 epoch 毫秒 |
| `bytea` | 哈希、密文、nonce 的原始字节 | 不是“已经加密的字符串”；用途取决于字段 |
| `jsonb` | 选项、快照、摘要、缓存响应 | 只在适合的结构化数据处使用，不替代全部关系表 |

`timestamptz` 按时间点存储并按数据库会话时区显示，不保留输入时原有的时区名称。账号的 `timezone` 单独保存，用于日统计等业务规则。[PostgreSQL 官方：日期时间类型](https://www.postgresql.org/docs/18/datatype-datetime.html)

字符数与字节数不同。例如“医学”是两个 Unicode 字符，在 UTF8 中占六个字节。`char_length` 不计算 UTF8 字节数，也不保证等于用户视觉上的字形数量（组合字符、部分 emoji 可能由多个字符组成）。

### 3.2 字段表的标记

| 标记 | 含义 |
| --- | --- |
| 必填 | SQL 声明 `NOT NULL`，或因主键而不能为 NULL；是否能省略还要看默认值 |
| 可空 | 允许 SQL NULL，表示没有值，不等于空字符串 `''` |
| 生成 | `GENERATED ALWAYS AS ... STORED`：数据库计算并存储，不由 Go 直接赋值 |
| `现在` | 默认 `CURRENT_TIMESTAMP`，是当前事务的开始时间，不是每次调用都重新取墙钟时间 |
| `随机 UUID` | 默认 `gen_random_uuid()`；需要恢复旧 ID 时可显式提供合法 UUID |
| 无 | 没有默认值；必填字段必须由应用提供 |

生成列在本脚本中没有另外声明 `NOT NULL`。例如 `content_hash` 的非空输入会得到非空结果，而 `origin_hash`、复习记录的 `flashcard_id` 可以生成 NULL。表格会注明这种区别。

默认值不是校验规则。例如 `repetitions DEFAULT 0` 只表示没填时用 0；禁止负数靠的是 CHECK。生成列一般不能直接赋值，写 INSERT/UPDATE 时应省略它们。[PostgreSQL 官方：生成列](https://www.postgresql.org/docs/18/ddl-generated-columns.html)

SQL NULL、JSON `null`、字符串 `"null"` 是三种不同的值，不能混用。Go 读取可空列时也需要明确选择指针、pgx 可空类型等表示方式，不能把 NULL 全部当空字符串。

## 4. 整体表关系

当前共 **14 张表、7 个自定义函数、10 个触发器、35 个显式索引**。主键和表级 UNIQUE 还会自动建立索引，不包含在这 35 个中。

| 类别 | 表 | 主要职责 |
| --- | --- | --- |
| 结构管理 | `schema_migrations` | 记录数据库结构版本 |
| 账号与认证 | `users`、`sessions`、`ai_credentials` | 业务账号、refresh 会话、个人 AI Key |
| 学习资料 | `folders`、`documents` | 资料夹与解析后的文档文本 |
| 学习内容 | `flashcards`、`quick_quizzes`、`error_questions` | 卡片、持久快刷、错题 |
| 关系与事件 | `error_flashcards`、`review_history` | 错题关联卡片、复习事件 |
| 恢复与并发 | `safety_backups`、`idempotency_keys`、`import_previews` | 恢复前备份、请求幂等、一次性导入 |

| 关系 | 基数 | 说明 |
| --- | --- | --- |
| 用户 → 资料夹 / 文档 / 卡片等 | 一对多 | 所有记录归属于一个用户 |
| 用户 → AI 凭据 | 每 provider 至多一条 | 当前 provider 仅允许 `deepseek` |
| 资料夹 → 文档 | 一对多 | 文档可因兼容旧快照而没有资料夹；新建接口不允许省略 |
| 文档 / 资料夹 → 卡片、快刷、错题 | 一对多的可选存活引用 | 来源删除后学习内容仍保留 |
| 错题 ↔ 卡片 | 多对多 | 用 `error_flashcards` 关联，不存 ID 数组 |
| 卡片 → 卡片复习事件 | 一对多 | 删除卡片会删除其复习事件 |
| 旧 session → 新 session | 可选自关联 | 表示 refresh token 轮换，不是自动轮换机制 |

临时考试题、强化练习题不另建持久题目表；需要时保存复习事件或生成卡片。看板通过现有表聚合，不额外维护一份可能失真的统计表。

## 5. 逐表字段、约束与索引

### 5.1 schema_migrations：数据库结构版本

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `version` | `bigint` | 必填；无 | 结构版本号，主键，必须大于 0 |
| `name` | `text` | 必填；无 | 迁移名称，本次为 `initial_schema` |
| `applied_at` | `timestamptz` | 必填；现在 | 迁移记录写入时间 |

初始脚本最后写入版本 `1`。它不是用户业务数据版本 `users.revision`，也不是备份 JSON 的 `schemaVersion`；三者职责不同。

只有主键自动索引，没有额外显式索引。目前没有 Go migration runner、迁移 checksum 和自动执行机制，不能把这张表的存在理解成迁移系统已经完工。

### 5.2 users：业务用户

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 主键 |
| `username` | `text` | 必填；无 | 用于显示的用户名 |
| `username_key` | `text`，`C` 排序规则 | 生成；非空输入生成非空结果 | 标准化用户名，登录匹配和唯一性判断使用 |
| `password_hash` | `text` | 必填；无 | 完整 Argon2id 编码，不是明文密码 |
| `timezone` | `text` | 必填；`Asia/Shanghai` | 用户时区名称 |
| `revision` | `bigint` | 必填；`0` | 当前用户业务数据版本 |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间 |
| `updated_at` | `timestamptz` | 必填；现在，更新时触发器维护 | 更新时间 |

数据库约束：

- `username` 已去首尾 Unicode 空白，长度 3～32 字符；`username_key` 标准化后不能为空且唯一。
- `password_hash` 以 `$argon2id$` 开头。**这只检查前缀**，不证明参数、盐、哈希内容合法，也不执行密码验证。
- `timezone` 长度 1～255，不验证是否为有效 IANA 时区。
- `revision >= 0`；`created_at` 不早于 Unix epoch。

用户名允许保留显示大小写，但按 NFKC、trim、小写、空白合并后的键判断同名。`C` 排序规则用于比较最终键，不再依赖部署机器的默认文本排序规则。

索引来自主键和 `users_username_key_unique`。没有额外显式索引。

Go 要负责注册密码限制、生成和验证 Argon2id、时区有效性，以及在规定的业务写入事务中递增 `revision`。触发器不会自动加 revision，认证等非业务数据写入也不能未经约定随意递增。

### 5.3 sessions：refresh token 会话

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 主键 |
| `user_id` | `uuid` | 必填；无 | 所属用户，删除用户时级联删除 |
| `token_hash` | `bytea` | 必填；无 | refresh token 的 SHA-256，恰好 32 字节，全表唯一 |
| `family_id` | `uuid` | 必填；随机 UUID | 同一条轮换链的 family |
| `expires_at` | `timestamptz` | 必填；无 | 到期时间，必须晚于创建时间 |
| `consumed_at` | `timestamptz` | 可空；无 | 旧 token 被轮换消费的时间 |
| `revoked_at` | `timestamptz` | 可空；无 | 被吊销的时间 |
| `replaced_by_id` | `uuid` | 可空；无 | 替代本会话的新 session ID |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间 |

除主键外，`UNIQUE(user_id, id)` 支持同用户自关联，`token_hash` 唯一约束支持通过 token 哈希找会话。

如果 `replaced_by_id` 非空，必须不是自己的 ID，且 `consumed_at` 已填写。复合外键要求新 session 属于同一用户；删除新 session 时只清空旧行的 `replaced_by_id`，不会清空 `user_id`。

| 显式索引 | 列 / 条件 | 用途 |
| --- | --- | --- |
| `sessions_family_idx` | `(user_id, family_id)` | 查找或吊销整条 token family |
| `sessions_expiry_idx` | `(expires_at)` | 清理到期记录 |
| `sessions_replacement_idx` | `(user_id, replaced_by_id)`，仅非 NULL | 替代关系查询和外键维护 |

首次登录可以用默认的新 family；轮换时 Go 必须把旧 `family_id` 显式传给新行，否则默认值会生成另一条 family。数据库不检查两行 family 相同、不自动吊销、不自动判断 token 到期，也不自动执行轮换。

已消费的旧行不能在轮换后立刻删除，否则无法识别旧 token 重放。保留与清理期限需由认证业务规定。

### 5.4 ai_credentials：个人 AI Key

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `user_id` | `uuid` | 必填；无 | 所属用户 |
| `provider` | `text` | 必填；`deepseek` | 提供商，当前仅允许 `deepseek` |
| `ciphertext` | `bytea` | 必填；无 | AES-GCM 密文，至少 16 字节 |
| `nonce` | `bytea` | 必填；无 | AES-GCM nonce，恰好 12 字节 |
| `key_version` | `integer` | 必填；无 | 加密主密钥版本，大于 0 |
| `last4` | `text` | 必填；无 | Key 末尾四字符，仅用于状态展示 |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间 |
| `updated_at` | `timestamptz` | 必填；现在，触发器维护 | 更新时间 |

主键为 `(user_id, provider)`：同一用户同一提供商只能配置一条。没有额外显式索引；删除用户时级联删除。

数据库长度约束不等于密文有效性验证。加解密、认证标签验证、同一密钥下 nonce 不重复以及主密钥轮换由 Go 和部署配置保证。**解密主密钥不能存进这张表。** 普通 API 不返回密文、nonce、主密钥或 Key 原文。

### 5.5 folders：资料夹

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 主键 |
| `user_id` | `uuid` | 必填；无 | 所属用户 |
| `name` | `text` | 必填；无 | 资料夹名，已 trim，1～100 字符 |
| `type` | `text` | 必填；无 | `material` 或 `exam` |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间，不早于 epoch |
| `updated_at` | `timestamptz` | 必填；现在，触发器维护 | 更新时间 |

主键和 `UNIQUE(user_id, id)` 用于资源定位及同用户引用。同名资料夹允许存在，没有 `UNIQUE(user_id, name)`。类型合法性有 CHECK，但“修改接口不允许更改类型”属于 API 规则，数据库没有额外的类型不可变触发器。

| 显式索引 | 列 | 用途 |
| --- | --- | --- |
| `folders_list_idx` | `(user_id, type, created_at, id)` | 按用户、类型列出资料夹，支持时间与 ID 的顺序 |
| `folders_all_list_idx` | `(user_id, created_at, id)` | 不筛选类型的用户资料夹列表 |

删除资料夹会级联删除它的文档，但不删除由它产生的卡片、快刷、错题。详细删除结果见第 8 节。

这张表可以先理解成 Go 的资料夹结构体加上两件事：`user_id` 表示归属，约束和索引保证存储合法、查询有效率。数据库表不等于 HTTP 请求结构体，创建请求不应允许客户端决定所有字段。

### 5.6 documents：文档文本

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 主键 |
| `user_id` | `uuid` | 必填；无 | 所属用户 |
| `folder_id` | `uuid` | 可空；无 | 所属资料夹，NULL 用于兼容旧快照 |
| `name` | `text` | 必填；无 | 已 trim 的名称，1～255 字符 |
| `content` | `text` | 必填；无 | 解析后的正文，至少 10 字符 |
| `character_count` | `integer` | 生成；非空输入生成非空结果 | `char_length(content)`，不用 Go 重复写入 |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间，不早于 epoch |
| `updated_at` | `timestamptz` | 必填；现在，触发器维护 | 更新时间 |

有 `UNIQUE(user_id, id)`，以及 `(user_id, folder_id)` 同用户外键。默认外键匹配规则允许 `folder_id` 为 NULL；非空时必须引用当前用户的资料夹。删除资料夹会删除这些文档。

| 显式索引 | 列 | 用途 |
| --- | --- | --- |
| `documents_folder_list_idx` | `(user_id, folder_id, created_at DESC, id DESC)` | 某个资料夹内的最新文档列表 |
| `documents_all_list_idx` | `(user_id, created_at DESC, id DESC)` | 用户全部文档的最新列表 |

新建接口仍要求有效资料夹，不因数据库允许 NULL 就放宽。数据库没有写死 5 MiB 等可配置上限，由 HTTP/service 按契约限制。正文 CHECK 只计算长度，不执行正文 trim，也不验证文档所属资料夹的业务类型。

目前保存的是解析文本，不是原始 PDF/图片二进制，也没有单独的原文件上传表。

### 5.7 flashcards：智能卡片与 SRS 状态

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 主键 |
| `user_id` | `uuid` | 必填；无 | 所属用户 |
| `front` | `text` | 必填；无 | 卡片正面，标准化后不能为空 |
| `back` | `text` | 必填；无 | 卡片背面，标准化后不能为空 |
| `content_hash` | `bytea` | 生成；非空输入生成非空结果 | 标准化 front/back 的 SHA-256 去重键 |
| `origin_type` | `text` | 可空；无 | `quick-quiz`、`error`、`exam`、`reinforcement` |
| `origin_id` | `text`，`C` 排序规则 | 可空；无 | 来源题目标识；非空时已 trim 且至少一字符 |
| `origin_hash` | `bytea` | 生成；可生成 NULL | `origin_id` 原文的 SHA-256，不对 ID 做内容标准化 |
| `source_doc_id` | `uuid` | 可空；无 | 历史来源文档 ID，不直接做外键 |
| `source_doc_name` | `text` | 可空；无 | 历史来源文档名 |
| `source_folder_id` | `uuid` | 可空；无 | 历史来源资料夹 ID，不直接做外键 |
| `source_folder_name` | `text` | 可空；无 | 历史来源资料夹名 |
| `source_text` | `text` | 可空；无 | 生成卡片时保存的来源片段 |
| `source_doc_ref_id` | `uuid` | 可空；触发器绑定 | 当前仍存在的同用户文档引用，内部字段 |
| `source_folder_ref_id` | `uuid` | 可空；触发器绑定 | 当前仍存在的同用户资料夹引用，内部字段 |
| `next_review_at` | `timestamptz` | 必填；现在 | 下次复习时间，不早于 epoch |
| `interval_days` | `integer` | 必填；`0` | 复习间隔天数，非负 |
| `ease_factor` | `double precision` | 必填；`2.5` | 易度因子，至少 1.3，且不能为 Infinity/NaN |
| `repetitions` | `integer` | 必填；`0` | 连续复习次数，非负 |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间，不早于 epoch |
| `updated_at` | `timestamptz` | 必填；现在，触发器维护 | 更新时间 |

有 `UNIQUE(user_id, id)`。`origin_id` 是字符串而不是强制 UUID：来源可能是临时考试或强化题，所以没有外键强行指向持久题表。

SQL 允许来源键不完整，例如有 `origin_type` 但没有 `origin_id`；只有两项都有值时才参与来源唯一索引。HTTP 对缺省、空白 originId 的处理必须由 Go 与契约对齐，空白应规范为 NULL。

非空存活 ref 必须等于对应历史 ID，并通过同用户外键；来源删除时只清空 ref。第 6 节专门解释为什么要两组 ID。

| 显式索引 | 列 / 条件 | 用途 |
| --- | --- | --- |
| `flashcards_content_unique` | UNIQUE `(user_id, content_hash)` | 用户内内容始终去重 |
| `flashcards_origin_unique` | UNIQUE `(user_id, origin_type, origin_hash)`，来源两项都非 NULL | 用户内完整来源键去重 |
| `flashcards_due_idx` | `(user_id, next_review_at, id)` | 到期复习队列 |
| `flashcards_list_idx` | `(user_id, created_at DESC, id DESC)` | 全部卡片的最新列表 |
| `flashcards_folder_idx` | `(user_id, source_folder_id, created_at DESC, id DESC)` | 按历史资料夹筛选 |
| `flashcards_origin_list_idx` | `(user_id, origin_type, created_at DESC, id DESC)` | 按来源类型筛选 |
| `flashcards_weak_idx` | `(user_id, next_review_at, id)`，仅薄弱卡片 | 薄弱卡片复习队列 |
| `flashcards_doc_ref_idx` | `(user_id, source_doc_ref_id)`，仅非 NULL | 存活文档外键维护 |
| `flashcards_folder_ref_idx` | `(user_id, source_folder_ref_id)`，仅非 NULL | 存活资料夹外键维护 |

薄弱条件为 `ease_factor < 2.5 OR (interval_days <= 3 AND repetitions > 0)`，与 API 保持一致。该部分索引按复习时间排序，不保证按创建时间列出的薄弱结果无需另外排序。

SRS 的评分计算由 Go service 实现，数据库只保护基础范围；不能直接把任意客户端传入的 SRS 数值当作可信评分结果。

### 5.8 quick_quizzes：持久快刷题

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 主键 |
| `user_id` | `uuid` | 必填；无 | 所属用户 |
| `type` | `text` | 必填；无 | 题型自由文本，trim 后非空，如“选择题” |
| `question` | `text` | 必填；无 | 题干，trim 后非空 |
| `options` | `jsonb` | 可空；无 | 字符串键到字符串值的对象，如 `{"A":"甲","B":"乙"}` |
| `answer` | `text` | 必填；无 | 答案，trim 后非空 |
| `analysis` | `text` | 必填；空字符串 | 解析，允许为空字符串 |
| `reviewed` | `boolean` | 必填；`false` | 是否已复习 |
| `source_doc_id` | `uuid` | 可空；无 | 历史来源文档 ID |
| `source_doc_name` | `text` | 可空；无 | 历史来源文档名 |
| `source_folder_id` | `uuid` | 可空；无 | 历史来源资料夹 ID |
| `source_folder_name` | `text` | 可空；无 | 历史来源资料夹名 |
| `source_doc_ref_id` | `uuid` | 可空；触发器绑定 | 同用户存活文档引用 |
| `source_folder_ref_id` | `uuid` | 可空；触发器绑定 | 同用户存活资料夹引用 |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间，不早于 epoch |
| `updated_at` | `timestamptz` | 必填；现在，触发器维护 | 更新时间 |

有 `UNIQUE(user_id, id)`。题型没有枚举限制；题干、答案只要求 trim 后非空，不像名称字段那样强制原文已经 trim。选项允许 SQL NULL 和空对象 `{}`，但拒绝 JSON `null`、数组及非字符串选项值。数据库不验证答案一定对应某个选项字母。

来源外键、快照和触发器行为同卡片。快刷题没有卡片那样的内容唯一索引，不应擅自把同题干视为数据库重复。

| 显式索引 | 列 / 条件 | 用途 |
| --- | --- | --- |
| `quick_quizzes_list_idx` | `(user_id, created_at DESC, id DESC)` | 最新快刷列表 |
| `quick_quizzes_reviewed_idx` | `(user_id, reviewed, created_at DESC, id DESC)` | 已复习 / 未复习列表 |
| `quick_quizzes_folder_idx` | `(user_id, source_folder_id, created_at DESC, id DESC)` | 历史资料夹筛选 |
| `quick_quizzes_doc_ref_idx` | `(user_id, source_doc_ref_id)`，仅非 NULL | 文档引用维护 |
| `quick_quizzes_folder_ref_idx` | `(user_id, source_folder_ref_id)`，仅非 NULL | 资料夹引用维护 |

按契约，标记 reviewed 但没有评分时，不新增复习事件；同时提交评分时，状态与事件应在同一事务写入。这些行为不会由表自动执行。

### 5.9 error_questions：错题

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 主键 |
| `user_id` | `uuid` | 必填；无 | 所属用户 |
| `question` | `text` | 必填；无 | 题干，trim 后非空 |
| `question_hash` | `bytea` | 生成；非空输入生成非空结果 | `SHA-256(trim(question))` |
| `options` | `jsonb` | 可空；无 | 字符串选项对象，规则同快刷 |
| `wrong_answer` | `text` | 必填；无 | 用户错误答案，trim 后非空 |
| `correct_answer` | `text` | 可空；无 | 正确答案；SQL 也允许空字符串 |
| `analysis` | `text` | 必填；空字符串 | 解析 |
| `source_doc_id` | `uuid` | 可空；无 | 历史来源文档 ID |
| `source_doc_name` | `text` | 可空；无 | 历史来源文档名 |
| `source_folder_id` | `uuid` | 可空；无 | 历史来源资料夹 ID |
| `source_folder_name` | `text` | 可空；无 | 历史来源资料夹名 |
| `source_doc_ref_id` | `uuid` | 可空；触发器绑定 | 同用户存活文档引用 |
| `source_folder_ref_id` | `uuid` | 可空；触发器绑定 | 同用户存活资料夹引用 |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间，不早于 epoch |
| `updated_at` | `timestamptz` | 必填；现在，触发器维护 | 更新时间 |

有 `UNIQUE(user_id, id)`；来源行为同卡片。错题去重规则与卡片不同：只 trim 题干，不做 NFKC、小写或内部空白合并；相同题干不同资料夹可以同时存在。

| 显式索引 | 列 / 条件 | 用途 |
| --- | --- | --- |
| `error_questions_dedup_unique` | UNIQUE `(user_id, question_hash, source_folder_id) NULLS NOT DISTINCT` | 用户内题干 + 历史资料夹去重，无资料夹也去重 |
| `error_questions_list_idx` | `(user_id, created_at DESC, id DESC)` | 最新错题列表 |
| `error_questions_folder_idx` | `(user_id, source_folder_id, created_at DESC, id DESC)` | 历史资料夹筛选 |
| `error_questions_doc_ref_idx` | `(user_id, source_doc_ref_id)`，仅非 NULL | 文档引用维护 |
| `error_questions_folder_ref_idx` | `(user_id, source_folder_ref_id)`，仅非 NULL | 资料夹引用维护 |

HTTP 的 `derivedFlashcardIds` 通过关联表查询得到，不是这张表里的 UUID 数组。哈希冲突后仍需比对实际 trim 后的题干，不应只凭哈希判断重复。

### 5.10 error_flashcards：错题与卡片的关联

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `user_id` | `uuid` | 必填；无 | 两端共同的所属用户 |
| `error_id` | `uuid` | 必填；无 | 错题 ID |
| `flashcard_id` | `uuid` | 必填；无 | 卡片 ID |
| `created_at` | `timestamptz` | 必填；现在 | 建立关联的时间 |

主键为 `(user_id, error_id, flashcard_id)`，同一关联不能重复。两组复合外键分别引用错题和卡片，要求两端都属于 `user_id`。

一个错题可以关联多个卡片，一个卡片也可以被多个错题关联，尤其是内容去重后复用已有卡片的情况。

主键索引支持按用户、错题查卡片；显式 `error_flashcards_card_idx` 的列为 `(user_id, flashcard_id)`，支持反过来查询及删除卡片时维护关联。删除任何一端只删除关联行，不通过关联表删除另一端资源。

### 5.11 review_history：复习事件

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 事件主键 |
| `user_id` | `uuid` | 必填；无 | 所属用户 |
| `subject_id` | `uuid` | 必填；无 | 复习对象 ID，HTTP 名称为 `cardId` |
| `subject_type` | `text` | 必填；无 | `flashcard`、`quickQuiz`、`examQuestion`、`reinforcement` |
| `flashcard_id` | `uuid` | 生成；可生成 NULL | 类型为 flashcard 时等于 subject_id，否则为 NULL |
| `rating` | `text` | 必填；无 | `forgot`、`hard`、`good` |
| `reviewed_at` | `timestamptz` | 必填；现在 | 复习时间，不早于 epoch，HTTP 名称为 `timestamp` |
| `created_at` | `timestamptz` | 必填；现在 | 事件入库时间，可与复习时间不同 |

只有持久卡片事件有 `(user_id, flashcard_id)` 外键。这样既保护真实卡片关系，又允许临时考试/强化题的历史 UUID 存在。

quickQuiz 事件不建立快刷外键，以便快刷题删除后仍保留事件；写入前的题目存在性和归属由 Go 检查。普通评分接口不能允许任意客户端伪装成另一种类型来绕过这项检查。

注意事件枚举的 `quickQuiz` / `examQuestion` 与卡片来源枚举 `quick-quiz` / `exam` 不同，需要明确映射。

| 显式索引 | 列 / 条件 | 用途 |
| --- | --- | --- |
| `review_history_time_idx` | `(user_id, reviewed_at DESC, id DESC)` | 时间范围与最新复习事件 |
| `review_history_rating_time_idx` | `(user_id, rating, reviewed_at DESC, id DESC)` | 按评分筛选 |
| `review_history_subject_idx` | `(user_id, subject_type, subject_id, reviewed_at DESC, id DESC)` | 某个对象的复习轨迹 |
| `review_history_flashcard_idx` | `(user_id, flashcard_id)`，仅非 NULL | 卡片外键与级联删除维护 |

这不是不可删除的审计日志：删除卡片会级联删除该卡片的事件，删除用户会删除所有用户事件。本版没有单独的合规审计表。

### 5.12 safety_backups：恢复前安全备份

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 备份主键 |
| `user_id` | `uuid` | 必填；无 | 所属用户 |
| `reason` | `text` | 必填；无 | `conflict` 或 `before-restore` |
| `schema_version` | `integer` | 必填；`1` | 业务快照格式版本，大于 0 |
| `revision` | `bigint` | 必填；无 | 备份时的用户业务 revision，非负 |
| `snapshot` | `jsonb` | 可空；无 | 完整业务快照，非空时必须是对象 |
| `object_key` | `text` | 可空；无 | 受控对象存储 key，非空时 trim 后不能为空 |
| `summary` | `jsonb` | 必填；无 | 备份摘要对象 |
| `size_bytes` | `bigint` | 必填；无 | 大小，单位字节，非负 |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间 |

CHECK 要求 `snapshot` 和 `object_key` **恰好一个非 NULL**，不能两者都没有，也不能两者都有。`object_key` 不应是客户端可随意指定的任意 URL；Go 需保证其属于受控存储和当前用户。

`safety_backups_list_idx` 的列为 `(user_id, created_at DESC, id DESC)`，用于最新备份列表。主键定位备份时仍需查询用户归属。

数据库只检查 JSON 顶层类型，不验证完整快照格式、摘要字段和记录数量，`size_bytes` 也不是生成列。API 返回的 `source: server` 是响应构造，不是表中字段。

至少保留最近三份恢复前备份由 service 实现。该表用于单用户业务恢复，不等于 PostgreSQL 全库灾备；数据库和本表一起丢失时，仍需要独立备份恢复。

### 5.13 idempotency_keys：请求幂等

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `user_id` | `uuid` | 必填；无 | 请求所属用户 |
| `route` | `text`，`C` 排序规则 | 必填；无 | HTTP 方法 + 规范实际资源路径，1～255 字符 |
| `key` | `text`，`C` 排序规则 | 必填；无 | `Idempotency-Key`，8～128 字符 |
| `request_hash` | `bytea` | 必填；无 | 规范请求体 SHA-256，32 字节 |
| `status` | `text` | 必填；`processing` | `processing` 或 `completed` |
| `response_status` | `integer` | 可空；无 | 已完成 HTTP 状态码，非空时 100～599 |
| `response_headers` | `jsonb` | 可空；无 | 可重放的安全响应头对象 |
| `response` | `jsonb` | 可空；无 | 可重放业务 JSON，不限于对象 |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间 |
| `expires_at` | `timestamptz` | 必填；现在 + 24 小时 | 到期时间，必须晚于创建时间 |

主键 `(user_id, route, key)` 保证作用域内唯一。例如 `POST /api/v1/flashcards/实际UUID/reviews`，路径必须含实际资源 ID，不能把不同卡片都存成同一个 `{id}` 模板。

状态 CHECK：

- `processing` 时，状态码、响应头、响应都必须为 SQL NULL。
- `completed` 时，三项都必须非 SQL NULL，响应头必须为对象。

对于无响应体的已完成请求，可按服务约定存 JSON `null` 作为内部标记，它不是 SQL NULL；重放时仍应根据状态码正确构造 HTTP 响应，不能给 204 强行输出 `null`。

显式 `idempotency_keys_expiry_idx` 的列为 `(expires_at)`，支持过期清理。到期不代表行自动消失；在清理或受锁替换前，原主键仍占用。

Go 需要检查同 key 的请求哈希是否一致，不一致按契约返回 409；业务写入与 completed 响应必须在同一事务提交。不能只先提交“处理中”就宣称幂等已经保证，异常状态和超时策略也要实现。

这里只缓存允许重放的业务结果，不存密码、认证 token、AI Key 或 `Set-Cookie` 等敏感响应。

### 5.14 import_previews：一次性导入预览

| 字段 | 类型 | 可空 / 默认 | 含义 |
| --- | --- | --- | --- |
| `id` | `uuid` | 必填；随机 UUID | 预览主键 |
| `user_id` | `uuid` | 必填；无 | 发起预览的用户 |
| `purpose` | `text` | 必填；无 | `backup_import` 或 `legacy_import` |
| `token_hash` | `bytea` | 必填；无 | 一次性 token 的 SHA-256，32 字节，全表唯一 |
| `payload_hash` | `bytea` | 必填；无 | 被预览数据的 SHA-256，32 字节 |
| `ciphertext` | `bytea` | 必填；无 | 预览数据的 AES-GCM 密文，至少 16 字节 |
| `nonce` | `bytea` | 必填；无 | 12 字节 nonce |
| `key_version` | `integer` | 必填；无 | 主密钥版本，大于 0 |
| `summary` | `jsonb` | 必填；无 | 预览摘要对象 |
| `warnings` | `jsonb` | 必填；空数组 `[]` | 警告数组；SQL 不检查每个元素的业务格式 |
| `expires_at` | `timestamptz` | 必填；无 | 到期时间，必须晚于创建时间 |
| `used_at` | `timestamptz` | 可空；无 | 一次性导入完成消费的时间 |
| `created_at` | `timestamptz` | 必填；现在 | 创建时间 |

这张表补充 preview / confirm 协议所需的内部持久状态，不向 HTTP 暴露密文结构，也不替代安全备份。

| 显式索引 | 列 | 用途 |
| --- | --- | --- |
| `import_previews_expiry_idx` | `(expires_at)` | 清理过期预览 |
| `import_previews_user_idx` | `(user_id, created_at DESC, id DESC)` | 用户预览查询与管理 |

Go 必须联合校验 token、用户、purpose、payload_hash、有效期和 used_at，再在导入事务中锁定并消费；不能仅看 token_hash 匹配就放行。数据库不会自动拒绝已过期/已使用的记录，也不会自己解密或删除它们。

## 6. 为什么来源要分“快照”与“存活引用”

卡片、快刷、错题同时有两组来源字段：

| 字段组 | 例子 | 作用 | 来源删除后 |
| --- | --- | --- | --- |
| 历史快照 | `source_folder_id`、`source_folder_name` | 记住创建时来自哪里，用于展示、历史筛选和错题去重 | 保留 |
| 存活引用 | `source_folder_ref_id` | 内部同用户外键，表示那个资料夹当前还存在 | 置为 NULL |

文档来源同理。举例：卡片来自“生理学资料”，随后删除该资料夹，你仍应能看到这张卡片和当时的来源名称，而不是因为外键级联被全部删除。

如果只保留一个强外键并删除来源时清空 ID，不同历史资料夹的相同错题可能变成相同的 NULL 去重键。拆分字段可以同时保留历史身份和当前关系。

### 6.1 来源触发器实际做什么

`medstudy_bind_source_refs` 由 `flashcards_bind_sources`、`quick_quizzes_bind_sources`、`error_questions_bind_sources` 三个 BEFORE INSERT/UPDATE 触发器调用：

1. 插入，或对应历史来源 ID 改变时，清空 ref，再查询对应资源。
2. 当前用户的资源存在：绑定 ref。
3. 相同 ID 的资源存在但属于其他用户：拒绝。
4. 该 ID 当前完全不存在：允许作为历史快照保存，ref 为 NULL。
5. 更新时禁止把这三类记录的 `user_id` 转移到另一用户。

对应 CHECK 还保证非空 ref 与历史 ID 相等，复合外键保证 ref 对象属于同一用户。

### 6.2 它不做什么

- 不自动获取或更新来源名称，名称快照由 Go 写入；资料夹改名不会自动改所有历史快照。
- 不证明来源文档一定在声明的来源资料夹里，两项关系各自检查归属，业务一致性仍需 Go 校验。
- 不自动给已存的历史记录重新绑定之后恢复出来的资源。导入时应先恢复资料夹、文档，再写学习内容。
- 历史 ID 未改变的普通 UPDATE 不重新绑定 ref；来源删除导致的 ref 清空也不会被重新填回。
- 不是新建接口的来源存在性检查。普通创建指定来源时，Go 必须要求资源当前存在且归属正确；旧备份恢复才有“来源已不存在也可保留历史 ID”的需求。

另外，“禁止转移 user_id”的触发器只覆盖上述三张表，不能推断所有表都具有相同的不可变保护。其他资源归属同样不应由普通 API 任意修改。

## 7. 标准化、哈希与去重

### 7.1 数据库函数

| 函数 | 输入 / 输出 | 职责 |
| --- | --- | --- |
| `medstudy_trim_text` | text → text | 去首尾空白，显式使用与 Go `strings.TrimSpace` 对齐的 Unicode White_Space 集合 |
| `medstudy_normalize_text` | text → text | NFKC → trim → Unicode 小写 → 连续空白变一个普通空格 |
| `medstudy_text_hash` | text → bytea | UTF8 文本的 SHA-256，不自行 trim 或小写 |
| `medstudy_flashcard_content_hash` | front/back → bytea | 分别标准化，用字节长度前缀编码两段内容，再 SHA-256 |
| `medstudy_valid_options` | jsonb → boolean | 选项是否为字符串值对象 |
| `medstudy_touch_updated_at` | trigger → trigger | 更新行的 `updated_at` |
| `medstudy_bind_source_refs` | trigger → trigger | 绑定当前来源引用，保护同用户关系 |

前五个函数声明为 `IMMUTABLE`、`STRICT`、`PARALLEL SAFE`；STRICT 表示有 SQL NULL 输入时直接返回 NULL。固定 UTF8 的哈希包装用于生成列表达式，之后修改这类函数必须考虑已有生成值和唯一索引，不能当成普通工具函数随意替换。

`medstudy_normalize_text` 使用 `pg_unicode_fast` 小写规则，不依赖机器默认语言环境。Go 仍需通过中英文、全角字符、特殊大小写、Unicode 空白等对齐测试，不能假定不同语言库的转换完全一致。

NFKC 的意义不只是英文大小写：例如全角 `Ａ` 与普通 `A` 可得到一致形式。汉字本身通常没有大小写变化，不会因此变成另一种语言。数据库保存的 front/back 原文没有被函数自动替换，标准化主要用于判断重复。

### 7.2 卡片：来源优先匹配，内容始终兜底

这是本项目按当前前端选择的规则，而不是“有来源时就不查内容”：

1. 有完整 `origin_type + origin_id` 时，先按该来源寻找已有卡片。
2. 没命中来源，或来源不完整时，再按标准化 `front + back` 寻找已有卡片。
3. 不同来源但相同内容，仍视为重复。
4. 不同用户之间互不去重。

两个唯一索引同时生效，用于保护并发。**索引不会自动执行“先来源、再内容”的返回逻辑**，查找顺序、重复计数、返回哪个已有对象仍由 service 实现。

两个键在历史数据或编辑后可能分别命中不同记录；实现时必须按接口规则显式处理，不能依赖数据库先报哪个索引冲突。如果写入或编辑会破坏唯一性，应给出受控冲突结果，不能静默覆盖另一张卡片。

front/back 使用长度前缀编码，避免简单拼接的歧义，例如 `ab + c` 和 `a + bc` 不能算同一内容。长度按 UTF8 字节计算，两段内容分别参与编码。

SHA-256 用于把长文本变成固定 32 字节索引键，避免把大段正文直接放进 B-tree。它不是加密，也不能替代密码哈希。遇到键冲突需要复核标准化原文；来源哈希也应复核实际 originId，不能把理论上的哈希碰撞直接当重复。

### 7.3 错题：另一套规则

错题键为用户范围内的 `trim(question) + sourceFolderId`，不包含答案、解析、文档 ID，也不额外做大小写/NFKC 转换。

唯一索引声明 `NULLS NOT DISTINCT`，所以两个“都无资料夹”的相同题干也冲突；不能借助 NULL 绕过去重。历史来源 ID 在删除资料夹后保留，因此原来属于不同资料夹的错题不会被合并。[PostgreSQL 官方：唯一索引](https://www.postgresql.org/docs/18/indexes-unique.html)

### 7.4 自动更新时间

以下七张表具有 `{表名}_touch_updated_at` 触发器：

- `users`、`ai_credentials`、`folders`、`documents`。
- `flashcards`、`quick_quizzes`、`error_questions`。

所有 UPDATE 都会设置 `updated_at = CURRENT_TIMESTAMP`，即使只是清空存活 ref 或更新值与原值相同。它不记录更新操作者，也不保证相邻事务的时间严格递增；revision 是另一项业务机制。

其余表没有 `updated_at`，也没有这个触发器。恢复旧数据时，INSERT 可以显式提供历史时间；后续 UPDATE 仍会触发当前更新时间。

## 8. 删除规则：删一个资源，哪些数据会跟着变

| 操作 | 自动删除 | 自动修改 / 保留 |
| --- | --- | --- |
| 删除业务用户 | 该用户的会话、AI 凭据、资料夹、文档、学习内容、关联、事件、备份、幂等、预览 | 结构版本表保留；对象存储文件不会因 SQL 自动删除 |
| 删除资料夹 | 该资料夹下的文档 | 来源学习内容保留；folder ref 清空；被删除文档的 doc ref 也清空；历史 ID/名称保留 |
| 删除文档 | 不自动删除卡片、错题、快刷 | 对应 doc ref 清空；历史 ID/名称、卡片 source_text 保留 |
| 删除卡片 | 关联表中该卡片的行、该卡片的复习事件 | 错题本身保留；其他类型事件不因该卡片删除而清除 |
| 删除错题 | 关联表中该错题的行 | 关联卡片保留；卡片 origin 字符串不自动清空 |
| 删除快刷题 | 不通过外键删除其复习事件 | 快刷事件历史 ID、相关卡片 origin 字符串保留 |
| 删除被旧 session 引用的新 session | 只删除目标会话行 | 旧行 replaced_by_id 清空，其余旧状态保留 |
| 删除安全备份 / 预览 / 幂等行 | 只删除该行 | 对象存储清理、客户端状态不是数据库自动行为 |

`CASCADE` 是删除依赖行；`SET NULL (某列)` 是只清空指定引用列。这些动作发生在数据库写入事务中，不是后台定时任务。

以上是数据库能力，不表示当前已经提供用户注销、所有资源删除等 HTTP 接口；是否开放操作以契约为准。本版主要采用物理删除，没有通用 `deleted_at` 软删除字段。

## 9. 写业务时需要怎样的事务

事务保证一组数据库变化共同成功或共同回滚。下面是设计要求，不是已经落地的 service 代码；仅靠表结构无法完成。

### 9.1 统一用户写入与 revision

如果需要让 revision 和备份快照准确代表用户业务状态，规定范围内的写入应采用一致的用户级锁和事务顺序：

```text
开始事务
  → 按认证 user_id 锁定 users 行
  → 校验归属、来源、请求规则
  → 写业务表及相关关联/事件
  → revision = revision + 1（按接口规定的成功变更粒度）
  → 写幂等完成响应（如果该接口要求）
提交事务
```

例如创建资料夹时，插入资料夹与增加用户 revision 要在同一事务。没有实际变化的重复请求如何处理 revision，需要与接口语义统一，不能在重放时重复增加。

锁定 users 不是 SQL 自动施加的策略，所有相关写入路径都必须遵守，才能与导入/导出保持一致。锁顺序应统一，避免不同功能相反顺序锁多行造成死锁；后续结合实际吞吐量评估粒度。

### 9.2 卡片评分

同一事务中校验卡片归属、读取并锁定当前 SRS 状态、在 Go 中计算新状态、更新卡片、插入 `review_history`、递增 revision，必要时保存幂等结果。

不能先提交 SRS，再单独插事件：第二步失败会出现“卡片已经变了，统计却没有记录”。不能仅靠普通读取计算并发评分，否则可能丢失更新。

### 9.3 refresh token 轮换

在认证事务中锁定旧 session，检查过期、吊销和消费状态，创建同 family 新 session，标记旧 session 已消费并绑定替代行。识别旧 token 重放时按认证契约吊销整个 family。

token 的原文只出现在适当的认证边界，数据库存 SHA-256；不能把数据库中记录的哈希直接当作登录凭证。认证业务不能沿用“不检查状态、查到就接受”的写法。

### 9.4 请求幂等

同一用户 + 路由 + key 的竞争需要靠唯一约束、锁和受控冲突处理。相同 key 且不同规范请求体返回 409；已完成的相同请求重放原结果，不再执行业务写入或增加 revision。

业务写入与 completed 响应一起提交，避免“业务成功但重试又执行一次”。普通 SQL 错误会使事务进入失败状态，若需要冲突后继续查询，应采用合适的 `ON CONFLICT` 或 savepoint 等策略，不能捕获错误后直接继续在失败事务中执行。

具体锁竞争、处理超时、过期 key 替换、异常 processing 状态恢复，需要实现和集成测试，不是这张表已经保证的流程。

### 9.5 导入与恢复

按当前契约，在同一数据库事务中执行：

1. 锁定当前用户的导入/业务数据修改。
2. 生成并记录当前业务数据安全备份。
3. 校验并锁定一次性 token，确认用户、purpose、数据哈希、有效期和未消费状态。
4. 按依赖顺序替换该用户的业务数据：先资料夹、文档，再学习内容，最后关联与事件。
5. 更新 revision，标记 token 已使用，保存幂等完成响应。
6. 一起提交；任何一步失败，数据库中的上述变化一起回滚。

恢复的是规定的学习业务数据，不是删除用户重建账号，也不应把密码、会话、个人 AI Key 混进普通学习快照。具体恢复范围以备份契约为准。

若安全备份或文件使用外部对象存储，它不属于 PostgreSQL 事务；需先准备可靠对象并设计失败清理、重试或补偿，不能假设数据库 ROLLBACK 会删除已上传文件。快照读取同样需要一致性方案，不能逐表在不同时间无锁读取后拼成一个“完整备份”。

## 10. HTTP 字段与数据库字段的对应

数据库使用 `snake_case`，API 使用契约里的 camelCase。两者之间由 Go DTO / repository 明确映射，不应直接 `SELECT *` 后把整行暴露给前端。

| API / JSON | 数据库 | 说明 |
| --- | --- | --- |
| `id` | 对应表的 `id` | UUID |
| `createdAt` | `created_at` | 时间点转 epoch 毫秒 |
| `updatedAt` | `updated_at` | 时间点转 epoch 毫秒，只有契约需要时返回 |
| `folderId` | `documents.folder_id` | 可空历史值与新建必填规则分开 |
| `characterCount` | `documents.character_count` | 数据库生成的字符数 |
| `originType` / `originId` | `flashcards.origin_type` / `origin_id` | 完整来源键用于匹配 |
| `sourceDocId` / `sourceDocName` | `source_doc_id` / `source_doc_name` | 返回历史快照，而不是内部 ref |
| `sourceFolderId` / `sourceFolderName` | `source_folder_id` / `source_folder_name` | 返回历史快照 |
| `sourceText` | `flashcards.source_text` | 来源片段 |
| `nextReviewDate` | `flashcards.next_review_at` | 时间点转 epoch 毫秒，不是原样整数存储 |
| `interval` | `flashcards.interval_days` | 天数 |
| `easeFactor` | `flashcards.ease_factor` | 浮点数 |
| `repetitions` | `flashcards.repetitions` | 次数 |
| `wrongAnswer` / `correctAnswer` | `error_questions.wrong_answer` / `correct_answer` | 保留正确答案可空语义 |
| `derivedFlashcardIds` | 查询 `error_flashcards.flashcard_id` | 关联查询或聚合，不是错题列 |
| `cardId` | `review_history.subject_id` | 兼容 API 名称，不保证对象都是卡片 |
| `subjectType` | `review_history.subject_type` | 事件类型，第一版事件 DTO 可省略；临时评分请求使用此字段 |
| `timestamp` | `review_history.reviewed_at` | 事件发生时间，不是 created_at |
| `schemaVersion` | `safety_backups.schema_version` 或快照格式字段 | 不是 schema_migrations.version |
| `revision` | 用户当前版本 / 备份时版本 | 按响应上下文选择 |
| AI 状态的 `last4` | `ai_credentials.last4` | configured、source 等由服务组装 |
| 备份的 `source: server` | 没有对应列 | 服务端响应约定 |

内部 `user_id` 用于归属校验；`source_*_ref_id`、去重 hash、密码 hash、token hash、密文和 nonce 不应随着普通资源响应泄露。

## 11. 索引选择、边界与尚未实现事项

### 11.1 为什么索引大多以 user_id 开头

主要查询先限定用户，再限定类型、来源、时间等。例如 `(user_id, folder_id, created_at DESC, id DESC)` 面向“某用户某资料夹的文档列表”。不带用户条件的全库搜索并不是同一个目标查询。[PostgreSQL 官方：多列索引](https://www.postgresql.org/docs/18/indexes-multicolumn.html)

`id` 作为时间排序的第二键，可在多个对象时间相同时形成稳定顺序；repository 的游标条件必须与 `ORDER BY` 的字段和方向一致。

部分索引只收录满足条件的行，如非 NULL ref 和薄弱卡片。SQL 谓词需要让优化器能够判断条件满足，并不是所有加了筛选条件的查询都一定用该索引。

索引增加写入和存储成本；35 个显式索引是当前查询与约束的初始设计，不是生产性能已经验证。后续用真实查询、数据规模和执行计划评估，避免无依据地继续叠加索引。

### 11.2 数据库与 Go 的职责边界

| 事项 | SQL 当前保证 | Go / 运维仍需实现 |
| --- | --- | --- |
| 数据归属 | 关键关联的同用户复合外键 | 所有读取、写入、导出、AI 上下文查询都限制 user_id |
| 名称合法 | 指定字段的 trim 与长度 CHECK | 清洗输入、返回清晰字段错误 |
| 密码安全 | Argon2id 前缀 CHECK | 参数和编码校验、哈希、验证、登录防护 |
| 时区 | 字符长度 | IANA 时区校验及按用户时区聚合 |
| 文本与请求大小 | 文档最低字符数等基础检查 | 可配置字节上限、HTTP body 限额、导入限额 |
| 来源历史 | 快照与存活引用分离 | 新建存在性、归属、文档与资料夹一致性、名称快照 |
| 内容重复 | 生成哈希和唯一索引 | 查找优先级、冲突后原文复核、计数与响应 |
| SRS | 数值基础范围 | 评分算法、时间计算、并发与事件事务 |
| 会话 / token | 哈希长度、唯一性及基础关系 | 随机 token、状态和到期判断、轮换、重放吊销 |
| 加密数据 | nonce / 密文长度、密钥版本范围 | AEAD、nonce 安全、主密钥管理与轮换 |
| 幂等 | 作用域唯一、状态结构 | 请求规范化、并发处理、重放、清理 |
| 预览与恢复 | token 哈希唯一、存储结构 | 一次性校验消费、安全备份、替换事务 |
| 时间与 revision | 部分字段时间下限、自动 updated_at、revision 非负 | HTTP 单位转换、业务时间限制、revision 递增 |
| 备份 | 每行存储二选一及基本类型 | 最近三份保留、格式验证、全库灾备与恢复演练 |

不是所有时间列都有 epoch 下限；也没有对所有时间关系、所有文本空白、所有 JSON 内部字段写 CHECK。复习时间是否允许未来值、备份摘要是否准确等，要按契约在应用层检查，不能从字段类型推导出额外保护。

### 11.3 当前明确没有的能力

- 没有独立生产数据库角色、RLS、自动权限体系。
- 没有通用软删除、完整审计日志、原文件存储表。
- 没有自动清理 session / 幂等 / preview 的任务。
- 没有 SQL 内置的 SRS 算法、revision 自动递增或导入执行器。
- 没有生产索引压测、分区、读写分离等额外部署能力；不为“企业级”名义预先堆复杂度。
- 没有把建表、Docker 数据卷或用户安全快照当作完整灾备方案。

## 12. 初始化、迁移与验证

### 12.1 初始脚本的执行边界

初始化 SQL 在一个事务里执行，检查 PostgreSQL 版本与数据库编码，设置当前事务的 UTF8、5 秒锁等待超时、120 秒语句超时，并取得事务级 advisory lock 协调并发导入。

它创建 `public` 下的表、函数、触发器和索引，最后记录结构版本。**不创建数据库、不修改登录密码、不创建业务用户、不导入种子数据，也不执行 DROP / TRUNCATE。**

重复导入会因对象已存在报错，而不是跳过或覆盖。失败事务需要回滚；不要跳过错误继续执行部分 DDL。advisory lock 不是未来迁移版本检查器，语句超时也不是整个脚本的总时长限制。

### 12.2 以后怎么改表

尚未导入的初稿可以继续评审；一旦已有环境应用了这版，就用新的版本化 migration 修改结构，不要反复改 `000001` 再导入，更不要只在 Navicat 手改后忘记记录。

后续迁移应包括兼容性分析、已有数据处理、验证、备份及失败恢复方案。增加非空或唯一约束前要检查旧数据；修改标准化规则前要处理重新生成哈希和可能的新重复。

目前还需实现迁移执行器、checksum 检查和部署流程。`schema_migrations` 只是为这些工作提供结构基础。

### 12.3 已有验证材料怎么用

- [SQL 约束测试](../tests/sql/000001_initial_schema_test.sql)：验证关键合法/非法写入、跨用户关联、去重、来源删除、事件删除等规则。
- [隔离验证脚本](../scripts/verify_schema.sh)：在它自己创建的临时 PostgreSQL 容器验证初始化、约束和重复导入保护；不连接现有 Compose 开发库。
- [初始化指南](DATABASE_SETUP.md)：连接参数、Navicat 导入和导入后只读检查。

运行隔离验证的命令是 `bash scripts/verify_schema.sh`；执行前可先阅读脚本行为。本次文档整理不会替你导入、删表或重置数据库。

数据库测试不能替代未来的 Go 测试：HTTP 校验、鉴权、事务回滚、并发评分、幂等重试、token 消费等仍要分别验证。写接口时，我们会围绕这一接口的完整调用链补测试，而不是让你一次记住全部 14 张表。
