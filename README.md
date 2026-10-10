# MedStudy Backend

MedStudy 的独立 Go 后端项目。现有 React 前端位于相邻但完全独立的仓库：

```text
/Users/a-void/Code/MedStudy
```

后端实现阶段只读取该前端仓库来核对现有功能，不修改、不提交、不部署它。后端全部功能在 staging 验收完成后，再回到前端仓库创建单独的 API 适配分支。

当前 Git 策略：后端作为全新独立项目直接在自己的 `main` 开发；前端线上 `main` 保持不动，并保留 `feat/go-api-integration` 供后续对接。

## 开发文档

- [从零学习 Go 并完成 MedStudy 后端](docs/GO_BACKEND_LEARNING_PATH.md)
- [后端工程规范与结对开发分工](docs/ENGINEERING_GUIDE.md)
- [PostgreSQL 初始化与 Navicat 导入](docs/DATABASE_SETUP.md)
- [数据库详细设计：字段、关系、约束与事务](docs/DATABASE_DESIGN.md)
- [API 接口契约](docs/GO_BACKEND_API.md)
- [OpenAPI 3.1](docs/openapi.yaml)
- [Git 分支入门](docs/GIT_BRANCH_GUIDE.md)

## 计划中的技术栈

- Go 1.27.x
- `net/http`
- PostgreSQL
- pgx v5
- Docker Compose

代码将直接放在本项目根目录的 `cmd/`、`internal/`、`migrations/` 等目录中，不再创建额外的 `backend/` 子目录。
