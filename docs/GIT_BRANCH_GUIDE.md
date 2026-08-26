# MedStudy 双项目 Git 使用指南

## 当前采用的方案

```text
/Users/a-void/Code/MedStudy-Backend
  └─ main                    # Go 后端直接开发、提交和推送

/Users/a-void/Code/MedStudy
  ├─ main                    # 当前线上前端，后端完成前不修改
  └─ feat/go-api-integration # 将来连接 Go API 的前端开发分支
```

两个目录是两个完全独立的 Git 仓库。后端提交到自己的 `main` 不会修改或部署前端仓库；前端只有把 `feat/go-api-integration` 合入前端 `main` 并触发部署后，线上页面才会变化。

## 分支是什么

分支是同一个 Git 项目中的平行开发线。前端仓库使用分支隔离改造：

```text
前端 main:                     A --- B --- C    （线上继续运行 C）
                                           \
前端 feat/go-api-integration:               D --- E    （API 适配）
```

在功能分支提交 `D`、`E` 不会改变 `main` 指向的 `C`。全部测试通过后，才通过 Pull Request 将功能分支合入 `main`。

后端现在是尚未上线的新项目，因此按你的决定直接使用自己的 `main`，无需再建立后端功能分支。

## 如何确认自己没有进错项目

后端开发前：

```bash
cd /Users/a-void/Code/MedStudy-Backend
pwd
git branch --show-current
git status
```

应看到后端目录和 `main`。

以后改前端时：

```bash
cd /Users/a-void/Code/MedStudy
pwd
git branch --show-current
git status
```

必须看到 `feat/go-api-integration`，不能直接在前端 `main` 修改。

## 每天最常用的 Git 命令

### 查看状态和改动

```bash
git status
git diff
git diff --staged
```

- `git status`：现在在哪个分支、哪些文件变化了。
- `git diff`：尚未暂存的具体改动。
- `git diff --staged`：下一次提交将包含什么。

### 暂存明确的文件

```bash
git add internal/auth/service.go internal/auth/service_test.go
```

初学时不要习惯性使用 `git add .`，先确认本次提交应该包含哪些文件。

### 创建提交

```bash
git commit -m "feat(auth): add user login"
```

推荐前缀：

| 前缀 | 用途 | 示例 |
| --- | --- | --- |
| `feat` | 新功能 | `feat(cards): add review endpoint` |
| `fix` | 修复缺陷 | `fix(auth): rotate refresh token safely` |
| `test` | 测试 | `test(cards): cover SRS ratings` |
| `docs` | 文档 | `docs: clarify backup contract` |
| `refactor` | 不改变行为的重构 | `refactor(db): extract transaction helper` |
| `chore` | 依赖与维护 | `chore: add CI checks` |

### 查看历史

```bash
git log --oneline --decorate --graph --all -20
```

### 推送后端 main

先在 GitHub 创建空的后端仓库，并配置一次远端：

```bash
git remote add origin <后端 GitHub 仓库地址>
git push -u origin main
```

以后在后端 `main`：

```bash
git push
```

不要把前端仓库的 `origin` 地址误加到后端项目。

## 后端的推荐提交循环

每完成一个小功能：

```bash
cd /Users/a-void/Code/MedStudy-Backend
git branch --show-current
git status
git diff
go test ./...
git add <本次相关文件>
git diff --staged
git commit -m "feat(api): describe the completed behavior"
git push
```

即使直接开发 `main`，也应保持“小而完整”的提交：代码、对应测试和必要文档一起提交，不要等整个后端完成后只做一次巨大提交。

## 前端 API 适配分支怎么使用

后端全部功能完成前，只保留该分支，不在前端写 API 适配代码。开始适配时：

```bash
cd /Users/a-void/Code/MedStudy
git switch feat/go-api-integration
git status
```

开发期间如果线上 `main` 有新修复，需要同步：

```bash
git switch main
git pull --ff-only origin main
git switch feat/go-api-integration
git merge main
```

遇到冲突时：

1. 用 `git status` 找到冲突文件。
2. 理解并处理 `<<<<<<<`、`=======`、`>>>>>>>`。
3. 运行前端类型检查、测试和构建。
4. `git add <已解决文件>`。
5. `git commit` 完成合并。

不要在不理解冲突内容时选择“全部接受当前”或“全部接受传入”。

## 如何保证现有前端一直在线

1. 后端代码只写入 `/Users/a-void/Code/MedStudy-Backend`。
2. 后端实现阶段，将 `/Users/a-void/Code/MedStudy` 当作只读功能参考。
3. 后端 API 先部署到独立 staging 地址。
4. 后端接口、迁移、备份和回滚全部验证完，再修改前端功能分支。
5. 前端功能分支使用 `VITE_DATA_MODE=local|api`，生产默认仍为 `local`。
6. 新前端在 staging 完成完整用户旅程和旧数据迁移测试。
7. 最后才合并前端 `main`；上线前保留数据库备份、Git 标签和上一版镜像。

## 安全撤销

尚未暂存且确认不要某个文件的改动：

```bash
git diff -- path/to/file
git restore path/to/file
```

已暂存但不想放进本次提交：

```bash
git restore --staged path/to/file
```

已经推送的提交需要撤销时，优先创建反向提交：

```bash
git revert <commit-id>
```

不要在共享历史上随意执行 `git reset --hard` 或 `git push --force`。

## 前端合并上线检查单

- [ ] 后端功能追踪矩阵全部验收。
- [ ] 后端 main 已推送并在 staging 稳定运行。
- [ ] 当前目录是前端 `/Users/a-void/Code/MedStudy`。
- [ ] 当前分支是 `feat/go-api-integration`，不是前端 `main`。
- [ ] 没有提交 `.env`、密码、DeepSeek Key、COS Secret 或真实用户数据。
- [ ] 旧 IndexedDB/COS 数据迁移与失败回滚已经演练。
- [ ] 前端类型检查、测试、构建和关键端到端流程通过。
- [ ] CORS、Cookie、HTTPS 和生产 API 地址正确。
- [ ] 合并前端 main 前已创建发布标签并保留上一版镜像。

## 最重要的习惯

每次操作前先确认 `pwd`、`git branch --show-current` 和 `git status`。两个项目最危险的错误不是不会写命令，而是在错误目录或错误分支执行了正确命令。
