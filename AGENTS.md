# 仓库指南

## 项目结构与模块组织

Phoenix 是一个以本体（Ontology）为核心的文档智能单体仓库。`backend/` 包含 Go 工作流服务：入口位于 `cmd/`，领域包位于 `internal/`。由配置驱动的模型位于 `backend/configs/doctypes/` 和 `backend/configs/ontology/`。`frontend/` 是 Next.js 管理控制台；路由位于 `src/app/`，组件位于 `src/components/`，API 客户端位于 `src/lib/`。`phoenix-doc-assistant/` 包含 WorkBuddy 智能体、技能和 Python 客户端。共享 JSON 契约位于 `contracts/`。部署文件位于 `deploy/`，需求文档位于 `docs/`。

## 构建、测试与开发命令

- `make build`：编译所有 Go 包。
- `make test`：运行 Go 测试套件。
- `make vet`：运行 Go 静态分析。
- `make infra-up`：启动 PostgreSQL/pgvector、MinIO 和 Redis。
- `make run-workflow`：在 8081 端口运行后端。
- `make fe-install && make fe-dev`：安装前端依赖，并在 8084 端口运行 Next.js。
- `make fe-build`：生成用于生产环境的前端静态导出文件。
- `make smoke`：测试上传、校验、存储、查询及本体物化流程。

## 编码风格与命名约定

使用 `gofmt` 格式化 Go 代码；包名使用简短的小写名词，测试文件使用 `*_test.go`。注释和面向用户的文本应沿用现有的中文领域术语。TypeScript 使用两个空格缩进，组件使用 PascalCase，函数使用 camelCase，路由和组件文件名使用 kebab-case。Python 使用四个空格缩进和 snake_case，并且仅依赖标准库。文档类型和本体标识符应保持小写 snake_case，例如 `resolution_keys` 和 `warn_duplicate`。

## 测试指南

在修改过的包旁添加表驱动 Go 测试，并将测试命名为 `TestBehaviorCondition`。测试应覆盖校验边界、规范化、身份标识、实体物化和 API 兼容性。提交前运行 `make test`、`make vet`、`make fe-build` 和 `git diff --check`。修改 JSON 契约时，必须在 `contracts/examples/` 下添加或更新示例。

## 提交与拉取请求指南

提交历史使用简洁的 Conventional Commits 风格主题，例如 `feat: implement ontology layer`、`fix: persist doc_type` 和 `docs: update product guide`。每个提交应聚焦于单一事项。拉取请求应说明客户成果、受影响的本体对象/链接/操作、迁移、API 兼容性以及验证命令。UI 变更需附带截图，REST 变更需附带请求/响应 JSON 示例。

## 架构、安全与兼容性

WorkBuddy 负责识别；后端负责归档、校验、存储、本体对象物化和数据检索。将 `/pub/v1` 视为稳定的外部契约——应通过扩展来演进，而不是更改现有结构。不得恢复已归档的 MCP 组件。不得提交令牌、生产环境密码、`.env` 文件或 WorkBuddy `.config.json`。修正提取字段时，必须保留证据和可审计性。
