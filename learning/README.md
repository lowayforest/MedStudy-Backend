# Go 学习练习

这里只存放独立的语言练习，不是 MedStudy 业务实现。练习保留在仓库中用于复习，不混入 `cmd/api` 或 `internal`，也不由 API 服务导入。

## 当前分类

| 目录 / 文件 | 学习内容 | 测试方式 |
| --- | --- | --- |
| `basics/struct_loop_test.go` | 结构体、切片、range 与三段式 for | 日志演示，不作为业务验收 |
| `basics/slice_append_test.go` | append 返回值、切片长度和新增元素 | 日志与结果断言 |
| `basics/range_update_test.go` | 值副本与索引修改切片元素 | 日志与年龄断言 |

以后按实际练习主题增加目录，不预先建立大量空目录。

## 运行命令

运行全部练习：

```bash
go test -v ./learning/...
```

只运行 range 修改练习：

```bash
go test -v ./learning/basics -run '^TestPracticeRangeUpdate$'
```

`go test ./...` 仍包含这些练习，方便发现编译或断言失败；它们不是业务测试，不能用练习 PASS 证明接口完成。完成的练习保持可通过；新增待完成练习需要明确标记，不能破坏已完成任务的提交检查。

## 哪些测试不应该移到这里

健康检查的 HTTP 测试留在 `cmd/api`；资料夹校验、卡片标准化等正式业务规则测试留在对应 `internal` 包；数据库约束测试留在 `tests/sql`。

这些测试保护项目功能，并不是因为你学习时写过就变成可丢弃的练习。后续调整业务目录时，它们随被测代码一起移动。
