# MedStudy Go 后端 API 接口文档

> 状态：实现前契约（contract-first）  
> 版本：v1  
> 目标：覆盖当前 React 项目的全部数据、AI、学习和备份功能，同时允许前端渐进迁移。

机器可读版本见 [openapi.yaml](openapi.yaml)。如果本文与 OpenAPI 不一致，实现前必须先同时修正文档，不能自行猜测。

## 1. 设计边界

### 1.1 后端负责

- 正式账号、登录会话、用户隔离和个人 AI Key 安全保存。
- 文件夹、文档、卡片、复习记录、错题、快刷题的权威数据。
- 卡片去重、SRS、统计等业务规则和数据库事务。
- 调用 DeepSeek、维护业务提示词、校验结构化输出和映射错误。
- 备份导出、恢复前安全备份、旧 IndexedDB/COS JSON 快照迁移。
- 上传限制、权限、限流、日志、健康检查与可观测性。

### 1.2 前端继续负责

- 页面展示、弹窗、键盘快捷键、翻卡动画和全屏。
- 选择题即时交互和主观题自我评估。
- 将已生成模拟题导出为 TXT（纯展示转换，无需服务端状态）。
- 后端切换前继续运行旧的 IndexedDB/COS 数据层。

### 1.3 当前行为有意改进之处

- 原“输入用户名即进入”改为账号密码；否则服务端无法安全区分用户。
- PostgreSQL 成为唯一权威数据源，不再让浏览器上传全量快照做日常同步。
- DeepSeek 与 COS 长期密钥不再出现在浏览器包中。
- 多设备并发通过资源事务和约束处理，不再要求用户选择整库覆盖来源。

## 2. 通用约定

### 2.1 地址与版本

```text
生产：https://api.medstudy.a-void.cn/api/v1
测试：https://api-staging.medstudy.a-void.cn/api/v1
本地：http://localhost:8080/api/v1
```

健康检查位于版本前缀外：`/health/live`、`/health/ready`。

### 2.2 Content-Type

- 普通请求与响应：`application/json; charset=utf-8`
- 文件上传：`multipart/form-data`
- 快照下载：`application/json` + `Content-Disposition: attachment`

### 2.3 命名与时间

- JSON 字段使用 `camelCase`，与现有 TypeScript 类型一致。
- ID 是 UUID 字符串。旧快照中的合法 UUID 保持不变。
- 为减少第一轮前端改造，API 时间均为 Unix epoch 毫秒整数，例如 `1787712000000`。
- 数据库存储使用 `timestamptz`；HTTP DTO 边界负责毫秒转换。
- 用户时区存为 IANA 名称，默认 `Asia/Shanghai`。

### 2.4 认证

注册、登录成功返回有效期 15 分钟的 JWT access token：

```http
Authorization: Bearer <access-token>
```

access token 使用 HS256 签名，`sub` 为用户 ID，`iss` 为 `medstudy-backend`，`aud` 为 `medstudy-api`，并包含 `iat` 和 `exp`。服务端必须验证签名算法、签名、签发方、接收方和过期时间；token 不包含密码等敏感信息。签名密钥来自运行时配置，至少为 32 字节的随机值，不写入仓库。

refresh token 仍为随机 token，只存在 `HttpOnly; Secure; SameSite=Strict` Cookie 中，数据库只保存 token 哈希。前端不得把 access/refresh token 写入 localStorage。退出登录会吊销 refresh session；已签发的 JWT 在自身过期前仍可能有效。

除健康检查、注册、登录、刷新外，本文接口默认需要认证。

### 2.5 成功响应

单对象：

```json
{
  "data": { "id": "..." },
  "meta": { "requestId": "01J..." }
}
```

列表：

```json
{
  "data": [],
  "meta": {
    "requestId": "01J...",
    "nextCursor": null,
    "hasMore": false
  }
}
```

创建一般返回 `201`，读取/修改返回 `200`，无响应体的删除和退出返回 `204`。

### 2.6 错误响应

```json
{
  "error": {
    "code": "validation_failed",
    "message": "请求参数不符合要求",
    "fields": {
      "questionCount": "必须在 1 到 100 之间"
    },
    "retryable": false
  },
  "meta": { "requestId": "01J..." }
}
```

通用状态码：

| 状态 | 含义 |
| --- | --- |
| `400` | JSON、查询参数或业务输入无效 |
| `401` | 未登录、access token 无效或过期 |
| `403` | 已认证但无权执行操作；普通资源越权优先返回 404 |
| `404` | 当前用户范围内资源不存在 |
| `409` | 唯一约束、版本或状态冲突 |
| `413` | 上传文件、JSON body 或 AI 上下文过大 |
| `415` | 不支持的 Content-Type/文档格式 |
| `422` | 格式可读但内容无法处理，例如扫描 PDF 没有文字层 |
| `429` | 请求频率超过限制；返回 `Retry-After` |
| `500` | 未预期服务端错误 |
| `502` | DeepSeek 等上游返回不可用/非法响应 |
| `503` | 数据库或必要依赖未就绪 |
| `504` | 上游超时 |

`error.code` 是应用层稳定标识，由本项目约定；HTTP 状态码仍放在响应状态行中。创建资料夹接口当前使用以下错误码，`message` 是给用户看的安全说明，不用于客户端分支判断：

| HTTP 状态 | `error.code` | 场景 |
| --- | --- | --- |
| `400` | `invalid_json` | JSON 语法、字段或单值要求不符合请求格式 |
| `400` | `validation_failed` | 资料夹名称或类型不符合要求，`fields` 指出字段 |
| `401` | `unauthorized` | 缺少可信的登录身份 |
| `413` | `request_too_large` | JSON 请求体超限 |
| `415` | `unsupported_media_type` | 请求不是 `application/json` |
| `500` | `internal_error` | 未预期的服务端错误，响应不包含内部错误详情 |

AI 错误的 `error.code` 保留前端现有分类：

```text
missing_key
authentication
insufficient_balance
rate_limit
invalid_model
request_too_large
invalid_request
service_unavailable
network
timeout
invalid_response
unknown
```

### 2.7 分页、排序和过滤

- 列表默认 `limit=50`，最大 `100`。
- 使用不透明 cursor，不使用大 offset。
- 服务端规定稳定的第二排序键 `id`，避免翻页重复。
- 未识别的查询参数返回 `400`，防止拼写错误被静默忽略。

### 2.8 幂等

以下可能因网络重试而重复创建/执行的请求必须支持 `Idempotency-Key`：

- 批量创建卡片或快刷题。
- 复习评分。
- AI 生成后直接落库的卡片。
- 备份恢复和旧快照迁移。

同一用户、同一路由、同一 key、相同 body 在 24 小时内返回第一次结果；相同 key 搭配不同 body 返回 `409 idempotency_key_reused`。

### 2.9 限额（第一版默认值）

| 项目 | 限额 |
| --- | --- |
| 普通 JSON body | 1 MiB |
| 兼容 bootstrap/备份 body | 50 MiB |
| 单个上传文件 | 20 MiB |
| 单用户解析后文档文本 | 5 MiB/文档 |
| AI 生成数量 | 1～100 |
| AI 同次选择文档 | 20 |
| DeepSeek 请求超时 | 120 秒 |
| 登录 | 每 IP 和用户名组合限流 |
| AI | 每用户并发 2，速率由配置决定 |

限额通过配置调整，但接口错误码不变。

## 3. 核心数据模型

所有资源都隐含 `userId`，但业务响应不返回它。

### 3.1 Folder

```json
{
  "id": "2cdf5977-ee65-47c8-af76-9b38a9a14201",
  "name": "生理学资料",
  "type": "material",
  "createdAt": 1787712000000
}
```

`type` 仅允许 `material`、`exam`。名称 trim 后长度 1～100。

### 3.2 Document

```json
{
  "id": "b5a2985e-129b-4054-b1ca-c62abfb76501",
  "name": "循环系统.txt",
  "folderId": "2cdf5977-ee65-47c8-af76-9b38a9a14201",
  "content": "解析后的正文……",
  "characterCount": 15320,
  "createdAt": 1787712000000
}
```

列表默认省略 `content`；详情和兼容 bootstrap 包含。第一版保持现有行为，只要求保存解析文本和文件名；是否保留原文件是可选扩展。

### 3.3 Flashcard

```json
{
  "id": "4d76f75e-0fcb-4b6f-b063-c60a9a54a601",
  "front": "心输出量的定义是什么？",
  "back": "每分钟一侧心室射出的血量。",
  "originType": "quick-quiz",
  "originId": "499d33cb-6d14-4ef9-a4b3-304093440601",
  "sourceDocId": null,
  "sourceDocName": null,
  "sourceFolderId": "2cdf5977-ee65-47c8-af76-9b38a9a14201",
  "sourceFolderName": "生理学资料",
  "sourceText": null,
  "nextReviewDate": 1787712000000,
  "interval": 0,
  "easeFactor": 2.5,
  "repetitions": 0,
  "createdAt": 1787712000000
}
```

`originType`：`quick-quiz | error | exam | reinforcement`。创建时由服务端设置默认 SRS 字段，客户端不得指定。

去重顺序：

1. 如果 `originType+originId` 均存在，优先匹配用户内来源键。
2. 来源键缺失或未命中时，按用户内 `NFKC → trim → lower-case → 合并空白` 后的 `front+back` 去重；不同来源但相同规范内容也不能重复。
3. 批次内部也执行相同去重。

### 3.4 ReviewHistory

```json
{
  "id": "9b3c38b2-4f86-408b-ab0f-3116f869b001",
  "cardId": "4d76f75e-0fcb-4b6f-b063-c60a9a54a601",
  "rating": "hard",
  "timestamp": 1787712000000
}
```

为兼容现有前端，字段仍叫 `cardId`。它可以是持久卡片 UUID，也可以是一次临时考试/强化题 UUID。数据库额外保存 `subjectType=flashcard|quickQuiz|examQuestion|reinforcement`，DTO 第一版可省略。

### 3.5 ErrorQuestion

```json
{
  "id": "01a42986-363c-4619-a652-f16c83efb001",
  "question": "关于……下列哪项错误？",
  "options": { "A": "...", "B": "..." },
  "wrongAnswer": "B",
  "correctAnswer": "A",
  "analysis": "错因与考点……",
  "derivedFlashcardIds": [],
  "createdAt": 1787712000000,
  "sourceDocId": null,
  "sourceDocName": null,
  "sourceFolderId": null,
  "sourceFolderName": null
}
```

`derivedFlashcardIds` 来自关联表，删除卡片后自动消失，不保存悬空 ID。

### 3.6 QuickQuiz / ExamQuestion

```json
{
  "id": "499d33cb-6d14-4ef9-a4b3-304093440601",
  "type": "选择题",
  "question": "题干",
  "options": { "A": "选项 A", "B": "选项 B" },
  "answer": "A",
  "analysis": "解析",
  "reviewed": false,
  "sourceDocId": null,
  "sourceDocName": null,
  "sourceFolderId": null,
  "sourceFolderName": null,
  "createdAt": 1787712000000
}
```

AI 生成的 ExamQuestion 未入快刷库前没有 `reviewed`、`createdAt`，其余字段相同。

### 3.7 DatabaseSnapshot

与当前备份兼容：

```json
{
  "format": "medstudy-backup",
  "schemaVersion": 1,
  "revision": 42,
  "exportedAt": 1787712000000,
  "username": "demo",
  "data": {
    "folders": [],
    "documents": [],
    "flashcards": [],
    "reviewHistory": [],
    "errors": [],
    "quickQuizzes": []
  }
}
```

导入时 `username` 只作为旧数据提示，绝不能决定目标账号；目标用户始终来自已认证会话。

## 4. 健康与版本

### `GET /health/live`

只证明进程和 HTTP loop 存活，不访问数据库。

```json
{ "status": "ok" }
```

### `GET /health/ready`

在短超时内检查 PostgreSQL 和必要配置。就绪返回 `200`，否则 `503`。

### `GET /api/v1/version`

返回构建版本、commit、构建时间和 API 版本；不返回环境变量。

## 5. 认证与账号

### `POST /auth/register`

```json
{
  "username": "demo",
  "password": "Example!Password2026",
  "timezone": "Asia/Shanghai"
}
```

规则：username 3～32 个字符，规范化后唯一；密码为 8～128 个可见 ASCII 字符（英文字母、数字、英文标点；不接受空格、汉字和全角符号），且至少包含一个大写字母、一个小写字母和一个英文标点符号（如 `!`、`@`、`#`）。数字不是必填；密码原样校验，不裁剪空白。返回 `201`、用户和 access token，并设置 refresh Cookie。

### `POST /auth/login`

```json
{ "username": "demo", "password": "Example!Password2026" }
```

用户名不存在和密码错误统一返回 `401 invalid_credentials`，避免枚举账号。

### `POST /auth/refresh`

无 JSON body，读取 refresh Cookie、轮换 session，返回新的 access token并设置新 Cookie。

### `POST /auth/logout`

吊销当前 refresh session、清 Cookie，返回 `204`。接口需 refresh Cookie，access token 可选。

### `GET /me`

```json
{
  "data": {
    "id": "...",
    "username": "demo",
    "timezone": "Asia/Shanghai",
    "aiCredential": { "configured": true, "source": "personal", "last4": "7xK2" },
    "createdAt": 1787712000000
  },
  "meta": { "requestId": "..." }
}
```

### `PATCH /me`

第一版只允许修改 `timezone`。改用户名和密码应使用独立流程，避免对旧数据迁移产生歧义。

### `PUT /me/ai-credential`

```json
{ "provider": "deepseek", "apiKey": "sk-..." }
```

服务端先做格式和可选的轻量验证，再用 AES-GCM 加密保存。返回配置状态和 `last4`，绝不返回完整 Key。

### `DELETE /me/ai-credential`

删除个人 Key。若服务端有默认 Key，之后 AI 请求使用默认 Key；否则返回 `missing_key`。

## 6. 渐进迁移兼容入口

### `GET /bootstrap`

一次返回当前用户完整的 folders、documents（含 content）、flashcards、reviewHistory、errors、quickQuizzes：

```json
{
  "data": {
    "revision": 42,
    "folders": [],
    "documents": [],
    "flashcards": [],
    "reviewHistory": [],
    "errors": [],
    "quickQuizzes": []
  },
  "meta": { "requestId": "..." }
}
```

它专门帮助第一轮将现有 `useDb` 接到 API，不应成为大型账号的长期读模型。响应支持 gzip，后续前端应迁移到分页资源接口。

## 7. 文件夹接口

| 方法与路径 | 作用 | 关键输入/行为 |
| --- | --- | --- |
| `GET /folders?type=material|exam` | 列表 | `createdAt,id` 稳定排序 |
| `POST /folders` | 创建 | `{name,type}`；返回 `201` |
| `GET /folders/{folderId}` | 详情 | 仅当前用户资源 |
| `PATCH /folders/{folderId}` | 重命名 | `{name}`；type 第一版不可变 |
| `DELETE /folders/{folderId}` | 删除 | 同一事务删除其 documents；返回 `204` |

创建示例：

```http
POST /api/v1/folders
Authorization: Bearer ...
Content-Type: application/json

{"name":"口腔解剖","type":"material"}
```

删除文件夹不删除曾经由其资料生成的卡片/错题；这些资源保留创建时的 `sourceFolderName` 快照，与当前前端一致。

## 8. 文档接口

### `GET /documents`

查询：`folderId`、`type`、`cursor`、`limit`、`includeContent=false`。默认不返回 `content`。

### `POST /documents`

兼容当前浏览器解析器，直接提交解析文本：

```json
{
  "folderId": "2cdf5977-ee65-47c8-af76-9b38a9a14201",
  "name": "生理学.txt",
  "content": "解析后的正文"
}
```

验证文件夹归属、名称、字符数和总大小，返回 `201`。

### `POST /documents/import`

multipart 字段：

- `folderId`：必填 UUID。
- `files`：1～10 个文件；允许 `.txt`、`.pdf`、`.docx`，`.doc` 尽力支持或明确拒绝。

响应允许部分成功：

```json
{
  "data": {
    "imported": [{ "clientFilename": "a.txt", "document": {} }],
    "failed": [{ "clientFilename": "scan.pdf", "code": "document_text_too_short", "message": "疑似扫描件，请先 OCR" }]
  },
  "meta": { "requestId": "..." }
}
```

文件名仅作为显示名称，不能拼接成本地存储路径。

### 其余文档接口

| 方法与路径 | 作用 |
| --- | --- |
| `GET /documents/{documentId}` | 返回详情和完整 content |
| `PATCH /documents/{documentId}` | `{name}` 重命名；不通过此接口改正文/归属 |
| `DELETE /documents/{documentId}` | 删除，返回 `204` |

## 9. 智能卡片与复习

### `GET /flashcards`

查询参数：

- `due=true`：`nextReviewDate <= now`，按到期时间升序。
- `weak=true`：`easeFactor < 2.5 OR (interval <= 3 AND repetitions > 0)`。
- `folderId`、`originType`、`cursor`、`limit`。

`due` 与 `weak` 可以组合。

### `POST /flashcards/bulk`

```json
{
  "cards": [
    {
      "front": "问题",
      "back": "答案",
      "originType": "quick-quiz",
      "originId": "499d33cb-6d14-4ef9-a4b3-304093440601",
      "sourceFolderId": "2cdf5977-ee65-47c8-af76-9b38a9a14201",
      "sourceFolderName": "生理学资料"
    }
  ]
}
```

响应必须与现有 `AddFlashcardsResult` 对齐：

```json
{
  "data": {
    "requestedCount": 1,
    "addedCards": [],
    "duplicateCards": [{ "id": "...", "front": "问题", "back": "答案" }]
  },
  "meta": { "requestId": "..." }
}
```

### `POST /flashcards/{cardId}/reviews`

```json
{ "rating": "forgot" }
```

在一个数据库事务中锁定卡片、应用 SRS、更新卡片并插入 ReviewHistory。响应返回更新后卡片和 history。必须支持 `Idempotency-Key`。

### 其余卡片接口

| 方法与路径 | 作用 |
| --- | --- |
| `GET /flashcards/{cardId}` | 详情 |
| `PATCH /flashcards/{cardId}` | 只允许 `{front,back}`，至少一项 |
| `DELETE /flashcards/{cardId}` | 删除卡片、卡片自身复习记录和错题关联 |

### `GET /review-history`

查询：`from`、`to`（epoch ms）、`rating`、`cursor`、`limit`。普通前端主要使用 dashboard 聚合，导出和审计时使用此接口。

### `POST /review-history`

记录临时考试题或强化题的评分：

```json
{
  "cardId": "临时题目 UUID",
  "subjectType": "examQuestion",
  "rating": "hard",
  "timestamp": 1787712000000
}
```

持久智能卡片必须使用 `/flashcards/{id}/reviews`，不能绕过 SRS。

## 10. 快刷接口

| 方法与路径 | 作用 |
| --- | --- |
| `GET /quick-quizzes?reviewed=false` | 未复习题按 createdAt 降序；支持 folderId/分页 |
| `POST /quick-quizzes/bulk` | 批量导入 AI 模拟题，服务端生成 ID/createdAt |
| `GET /quick-quizzes/{quizId}` | 详情 |
| `PATCH /quick-quizzes/{quizId}` | 修改 question/options/answer/analysis |
| `DELETE /quick-quizzes/{quizId}` | 删除 |
| `POST /quick-quizzes/{quizId}/reviews` | 标为 reviewed，可选写评分和生成卡片 |

评分请求：

```json
{
  "rating": "hard",
  "createFlashcard": true
}
```

- `good` 默认不生成卡片。
- `forgot`/`hard` 且 `createFlashcard=true` 时调用 DeepSeek 生成并去重保存卡片，origin 为 `quick-quiz`。
- 即使 AI 生成失败，也要明确返回 `reviewSaved=true` 和嵌套 `flashcardError`，与当前“题目仍标记已复习”的行为一致。
- 选择题答对但用户未评级时可发送 `{}`，只设置 `reviewed=true`，不写 review history。

## 11. 错题接口

| 方法与路径 | 作用 |
| --- | --- |
| `GET /errors` | 支持 folderId、hasFlashcard、分页；createdAt 降序 |
| `POST /errors` | 手动或从快刷/考试收录；返回 `201` |
| `GET /errors/{errorId}` | 详情，含有效 derivedFlashcardIds |
| `PATCH /errors/{errorId}` | 修改题干、选项、答案、解析等允许字段 |
| `DELETE /errors/{errorId}` | 删除错题与关联，不删除卡片 |
| `POST /errors/{errorId}/flashcard` | AI 生成、去重保存并关联一张卡片 |
| `POST /errors/actions/generate-missing-flashcards` | 为尚无有效关联卡片的错题批量生成 |

创建错题示例：

```json
{
  "question": "完整题干",
  "options": { "A": "...", "B": "..." },
  "wrongAnswer": "B",
  "correctAnswer": "A",
  "analysis": "详细解析",
  "sourceFolderId": "...",
  "sourceFolderName": "生理学资料"
}
```

用户内以 `trim(question)+sourceFolderId` 判定从快刷/考试重复收录，重复返回现有资源和 `200`，不创建第二条。

批量生成请求允许 `concurrency` 1～3，服务端上限优先。响应逐项报告 `created`、`linkedExisting`、`failed`，一项失败不回滚已成功项。

## 12. AI 能力接口

AI 请求只接收业务参数，不开放“任意 messages 代理”。所有 documentId/cardId 必须属于当前用户。

允许模型：`deepseek-chat`、`deepseek-reasoner`。前端当前遗留的 Gemini 选项不得发送；服务端返回 `invalid_model`。

### `POST /ai/study-cards`

从学习资料生成卡片草稿，不自动保存：

```json
{
  "documentIds": ["..."],
  "questionCount": 10,
  "scope": "仅提取心脏解剖相关重点",
  "model": "deepseek-chat"
}
```

```json
{
  "data": {
    "cards": [{ "front": "...", "back": "..." }],
    "sourceFolderId": "...",
    "sourceFolderName": "资料夹 A, 资料夹 B"
  },
  "meta": { "requestId": "..." }
}
```

前端确认后调用 `POST /flashcards/bulk`。

### `POST /ai/simulated-questions`

对应“真题模拟天天练”：

```json
{
  "materialDocumentIds": ["..."],
  "examDocumentIds": ["..."],
  "types": ["选择题", "名词解释题", "简答题", "病例分析题"],
  "questionCount": 10,
  "scope": "重点考察临床表现",
  "model": "deepseek-chat"
}
```

返回 `questions: ExamQuestion[]` 和来源资料夹摘要，不自动写入。用户点击“一键导入”后调用 `/quick-quizzes/bulk`；点击下载由前端生成 TXT。

### `POST /ai/exams`

对应考试模式：

```json
{
  "materialDocumentIds": ["..."],
  "examDocumentIds": ["..."],
  "types": ["单选题", "简答题"],
  "questionCount": 10,
  "model": "deepseek-chat"
}
```

返回临时 `questions`，每项由服务端生成 UUID。第一版不保存整场考试，答案状态继续由前端维护；错题、卡片、评分分别调用对应持久接口。

### `POST /ai/errors/analyze`

```json
{ "question": "完整题干和选项", "wrongAnswer": "B", "model": "deepseek-chat" }
```

返回 `{analysis:"..."}`，用户确认后调用 `POST /errors` 保存。

### `POST /ai/question-card`

为临时考试题或强化题生成并直接去重保存卡片：

```json
{
  "originType": "exam",
  "originId": "...",
  "question": "...",
  "options": { "A": "..." },
  "answer": "...",
  "analysis": "...",
  "sourceFolderId": "...",
  "sourceFolderName": "...",
  "model": "deepseek-chat"
}
```

响应是单卡版 `AddFlashcardsResult`。`sourceFolderId` 非空时必须校验归属。

### `POST /ai/weak-points`

```json
{
  "cardIds": ["..."],
  "model": "deepseek-chat"
}
```

若省略 cardIds，服务端使用当前 `weak=true` 规则选择。返回：

```json
{
  "data": {
    "points": [
      { "id": "stable-generated-id", "point": "某药适应症", "count": 3, "cardIds": ["..."] }
    ]
  },
  "meta": { "requestId": "..." }
}
```

服务端校验 DeepSeek 返回的索引，丢弃越界项。

### `POST /ai/reinforcement-exercises`

```json
{
  "points": [
    { "point": "某药适应症", "cardIds": ["..."] }
  ],
  "count": 5,
  "model": "deepseek-chat"
}
```

返回 3～5（或明确指定 1～10）道变式练习。资料上下文优先来自卡片 `sourceDocId` 所属文档，没有时回退到卡片 front/back。结果不自动保存；评分通过 `/review-history`，生成卡片通过 `/ai/question-card`，origin 为 `reinforcement`。

## 13. 数据看板

### `GET /dashboard?days=7&timezone=Asia%2FShanghai`

服务端以账号时区为默认值；timezone 参数只能是有效 IANA 时区。

```json
{
  "data": {
    "summary": {
      "totalCards": 120,
      "masteredCards": 34,
      "todayReviews": 18,
      "errorCount": 12
    },
    "trend": [
      { "date": "08/20", "forgot": 2, "hard": 4, "good": 8, "total": 14 }
    ],
    "weakCardIds": ["..."]
  },
  "meta": { "requestId": "..." }
}
```

`masteredCards = interval > 21`。薄弱规则与 `GET /flashcards?weak=true` 完全一致，避免前后端定义漂移。

卡片/错题详情和按资料夹分组由各自列表接口完成，不在 dashboard 重复返回全部正文。

## 14. 备份与恢复

### `GET /backups/export`

下载当前用户的 `DatabaseSnapshot`，文件名：

```text
medstudy_backup_YYYY-MM-DD.json
```

### `POST /backups/import/preview`

请求 body 是原始 `DatabaseSnapshot` JSON，最大 50 MiB。只验证，不写业务表：

```json
{
  "data": {
    "importToken": "短期、一次性、不透明 token",
    "expiresAt": 1787712300000,
    "summary": {
      "folders": 4,
      "documents": 10,
      "documentCharacters": 320000,
      "flashcards": 100,
      "reviewHistory": 500,
      "errors": 20,
      "quickQuizzes": 30
    },
    "warnings": ["快照 username 与当前账号名不同；将导入当前账号"]
  },
  "meta": { "requestId": "..." }
}
```

服务端保存短期加密暂存或内容哈希绑定的安全引用，不能把整个快照塞入客户端可篡改 token。

### `POST /backups/import`

```json
{ "importToken": "..." }
```

必须携带 `Idempotency-Key`。执行顺序：锁定用户导入 → 生成当前数据安全备份 → 校验 token 未过期/未使用 → 单事务替换所有业务数据 → 更新 revision → 标记 token 已使用。

### 安全备份

| 方法与路径 | 作用 |
| --- | --- |
| `GET /backups/safety` | 列出当前用户备份，按 createdAt 降序 |
| `GET /backups/safety/{backupId}` | 返回摘要与元数据，不默认返回完整正文 |
| `GET /backups/safety/{backupId}/download` | 下载完整快照 |
| `POST /backups/safety/{backupId}/restore` | 先再备份当前数据，然后恢复指定快照 |

`reason`：`conflict`（旧系统导入兼容）或 `before-restore`。服务端第一版至少保留最近 3 份恢复前备份；生产数据库本身仍需独立的自动全库备份。

## 15. 旧 IndexedDB/COS 数据迁移

迁移接口与日常备份导入分开，便于审计一次性迁移。

### `POST /migrations/legacy/preview`

请求为旧的包装快照或未包装 `Database` JSON。执行：

- 规范化缺失的 `quickQuizzes=[]`。
- 将旧资料夹缺失的 `type` 规范化为 `material`，将快刷题缺失的 `reviewed` 规范化为 `false`。
- 验证所有数组、ID、类型、大小和引用。
- 报告重复卡片、悬空关联、非法时间、未知字段和目标账号映射。
- 只返回摘要、警告、错误和一次性 `migrationToken`，不写数据。

### `POST /migrations/legacy/import`

```json
{
  "migrationToken": "...",
  "mode": "replace"
}
```

第一版只允许明确的 `replace`，执行前建立安全备份。未来可以增加经过充分定义的 `merge`，不能用模糊规则自动合并医疗学习数据。

COS 旧数据由管理员 CLI 执行：管理员提供旧对象 key 与目标 user UUID 的显式映射，CLI 使用服务端凭据读取对象，再调用同一 legacy service。公开 API 不接受“用户名 → COS 对象”读取，避免未认证旧数据泄露。

## 16. 数据库表与关键约束

| 表 | 关键字段/约束 |
| --- | --- |
| `users` | id、规范化 username 唯一、password_hash、timezone |
| `sessions` | user_id、token_hash 唯一、family_id、expires_at、revoked_at |
| `ai_credentials` | user_id+provider 唯一、ciphertext、nonce、key_version、last4 |
| `folders` | user_id、name、type check、created_at；索引 `(user_id,type,created_at,id)` |
| `documents` | user_id、folder_id 外键、name、content、created_at；校验 folder 同用户 |
| `flashcards` | user_id、内容、来源、SRS；来源唯一和规范内容唯一索引 |
| `review_history` | user_id、subject_id、subject_type、rating check、reviewed_at |
| `error_questions` | user_id、options jsonb、答案、解析、来源 |
| `error_flashcards` | user_id、error_id、flashcard_id，复合主键，级联删关联 |
| `quick_quizzes` | user_id、options jsonb、reviewed、来源、created_at |
| `safety_backups` | user_id、reason、schema_version、snapshot/对象 key、created_at |
| `idempotency_keys` | user_id、route、key、request_hash、status、response、expires_at |
| `import_previews` | user_id、purpose、token_hash 唯一、加密暂存、payload_hash、expires_at、used_at |
| `schema_migrations` | 数据库结构版本、名称、applied_at；与备份 schemaVersion 独立 |

用户隔离必须由每条 repository 查询的 `user_id` 条件保证。若使用复合外键，应让 `(user_id,id)` 具备唯一约束，避免错误关联到另一用户的资源。

初始结构和导入方式见 [DATABASE_SETUP.md](DATABASE_SETUP.md)。来源 ID/名称按历史快照保留；内部 ref 字段提供存活资源的同用户复合外键，删除来源只清空 ref，不删除卡片/错题或抹掉历史来源。新建接口仍需验证来源当前存在，历史恢复允许已删除来源。

## 17. CORS 与浏览器配置

测试和生产分别维护精确 Origin 白名单，不能使用 `*` 搭配 credentials：

```text
http://localhost:3000
https://staging.medstudy.a-void.cn
https://medstudy.a-void.cn
```

允许方法按实际路由设置，允许 header 至少包含：

```text
Authorization
Content-Type
Idempotency-Key
X-Request-ID
```

refresh 请求使用 `credentials: "include"`；业务接口使用内存 access token。预检响应和实际响应都必须正确设置 CORS header。

## 18. 前端功能到接口的映射

| 页面/动作 | 接口 |
| --- | --- |
| 首次加载兼容数据 | `GET /bootstrap` |
| 新建/重命名/删除资料夹 | `/folders` |
| 导入浏览器已解析文本 | `POST /documents` |
| 服务端解析文件 | `POST /documents/import` |
| 考点生成 | `POST /ai/study-cards` → `POST /flashcards/bulk` |
| 到期卡片 | `GET /flashcards?due=true` |
| 卡片评分 | `POST /flashcards/{id}/reviews` |
| 真题模拟天天练 | `POST /ai/simulated-questions` |
| 导入模拟快刷 | `POST /quick-quizzes/bulk` |
| 快刷编辑/删除/评分 | `/quick-quizzes/{id}`、`/reviews` |
| 快刷加入错题 | `POST /errors` |
| 考试模式生成 | `POST /ai/exams` |
| 考试题评分/卡片 | `POST /review-history`、`POST /ai/question-card` |
| 手动错题分析/保存 | `POST /ai/errors/analyze` → `POST /errors` |
| 错题生成卡片 | `POST /errors/{id}/flashcard` |
| 全部错题生成卡片 | `POST /errors/actions/generate-missing-flashcards` |
| 看板 | `GET /dashboard` + 卡片/错题分页接口 |
| 薄弱点归纳 | `POST /ai/weak-points` |
| 强化练习 | `POST /ai/reinforcement-exercises` |
| 完整导出 | `GET /backups/export` |
| 恢复导入 | `/backups/import/preview` → `/backups/import` |
| 查看/下载/恢复安全备份 | `/backups/safety` |
| 旧数据一次性迁移 | `/migrations/legacy/preview` → `/migrations/legacy/import` |
| 配置 DeepSeek Key | `PUT /me/ai-credential` |
| 切换用户 | `POST /auth/logout` → 另一个账号登录 |

## 19. 实现完成定义

一个接口只有满足以下条件才算完成：

- OpenAPI、本文、handler 行为和前端调用一致。
- 输入严格校验，用户隔离在 repository 查询中生效。
- 成功、验证失败、未登录、越权、冲突、依赖失败都有测试。
- 涉及多表状态时使用事务，并测试中途失败。
- 涉及创建/评分/恢复时处理幂等和并发。
- 日志含 requestId 且不含正文、答案、密码、token、DeepSeek Key。
- 指标能区分应用错误与上游错误。
- 不依赖真实 DeepSeek 做自动化测试。
- `go test -race ./...` 与 PostgreSQL 集成测试通过。

## 20. API 版本策略

- 在 v1 内增加可选响应字段属于兼容变更。
- 删除字段、改字段类型、改变业务语义必须进入 v2 或提供明确迁移期。
- 新增必填请求字段必须先提供服务端默认值和弃用周期。
- 错误 `code` 是契约的一部分，前端按 code 处理，不能依赖 message 文本。
- 每次契约变更先修改 `docs/openapi.yaml` 与本文，再改后端和前端测试。
