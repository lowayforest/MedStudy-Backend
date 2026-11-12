# 在 Apifox 查看和调试 MedStudy API

导入文件：[openapi.yaml](openapi.yaml)。它是项目的 OpenAPI 3.1 契约，也是 Apifox 的导入文件，包含全部规划接口、请求与响应模型、鉴权方式和示例。后续接口变更先更新这份文件，再重新导入 Apifox，避免维护两份不同的接口定义。

## 导入

在 Apifox 的「团队项目」页面选择「导入项目」→「OpenAPI/Swagger」，上传本仓库的 `docs/openapi.yaml`。想在界面里查看接口树、调试请求时，选择「以可视化界面维护接口」。如果已经建好项目，在「项目设置」→「导入数据」中选择同一文件。

导入选项中的「导入 Servers 为环境」可将规范中的地址转为环境。测试本机服务时选择 `http://127.0.0.1:11345`。测试环境和生产环境地址目前只在契约中规划，尚未部署。

Apifox 支持直接导入 OpenAPI 3.1 的 YAML 文件：[官方导入说明](https://docs.apifox.com/doc-5174534)、[环境导入设置](https://docs.apifox.com/import-settings)。接口上的 `x-apifox-status` 是 Apifox 支持的状态字段：[状态说明](https://docs.apifox.com/x-apifox-status)。

## 当前实现状态

| Apifox 状态 | 接口 | 当前含义 |
| --- | --- | --- |
| `tested` | `GET /health/live` | 已注册到本地路由，可实际请求 |
| `developing` | 注册、登录、刷新与创建资料夹 | 部分业务代码已实现，HTTP 路由尚未接通，当前不能按契约调用 |
| `designing` | 其余接口 | 目标契约，尚未实现或尚未接入 HTTP 路由 |

导入后能看到全部接口，不表示它们已经可以请求。未接入的路径目前通常返回 `404`。如果状态与实际代码不一致，应在实现接口的同一需求中更新 `openapi.yaml`。

## 先测试存活检查

先启动本地 PostgreSQL 与 Go 服务，保持运行；`go run` 会读取你本地未提交的 `.env`：

```bash
docker compose up -d db
go run ./cmd/api
```

在 Apifox 选择本地环境，发送 `GET /health/live`。预期 HTTP `200`、JSON 响应：

```json
{"status":"ok"}
```

数据库端口 `13445` 用于 PostgreSQL 客户端连接，API 调试使用的是 Go 服务端口 `11345`。

## 登录与创建资料夹接通后的测试顺序

登录请求示例和创建资料夹请求示例已经写在 `openapi.yaml`，导入后可以在对应接口中查看。等路由接通并有可登录的业务用户后，先发送 `POST /api/v1/auth/login`，再把响应中的 `data.accessToken` 用作业务接口的 Bearer token，发送 `POST /api/v1/folders`。refresh token 只应出现在登录响应的 `Set-Cookie`，不能出现在 JSON 中。

示例密码只是接口文档的占位数据。不要把真实密码、JWT 签名密钥、access token 或 refresh token 写入仓库中的文档或测试文件。
