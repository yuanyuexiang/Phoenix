# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 产品:Phoenix —— 企业智能文档处理平台(DIP)

> **Phoenix** 是项目代号。需求的唯一来源是 `docs/产品说明书_企业智能文档处理平台_V1.0.md`
> (客户确认版,其中【待确认】项尚未定稿);同目录 `.docx` 是最初的原始说明书。
> 产品/领域术语请与说明书保持一致(中文)。

通过 OCR + AI 提取 + 规则校验,把非结构化企业文档转换为结构化数据,写入 PostgreSQL
并归档到 MinIO。**交付形态:WorkBuddy 中的「文档处理专家」**,底层是本平台暴露的
MCP Server(连接器)。

## 顶层结构(按技术栈分)

```
docs/       产品文档(说明书)
frontend/   前端管理后台 —— React 18 + Ant Design 5 + Vite(TypeScript)
backend/    Go 后端,单一 go.mod,四个服务入口在 cmd/ 下
ocr/        OCR 服务 —— Python FastAPI + PaddleOCR
deploy/     docker-compose(9 容器)
samples/    演示样例文档
```

## 常用命令(全部在仓库根目录执行)

```bash
make build / test / vet      # Go:构建 / 测试 / vet(自动 cd backend)
cd backend && go test ./internal/validate -run TestRunViolations   # 单个测试

make infra-up                # 拉起 Postgres/MinIO/Redis/OCR 容器
make run-all                 # 前台并行起 4 个 Go 服务(Ctrl-C 全停)
make fe-dev                  # 前端 dev server(8084,/api 代理到 workflow)
make smoke                   # 端到端冒烟:模拟 WorkBuddy 调用五个 MCP 工具

make fe-install / fe-build   # 前端依赖 / 生产构建
make compose-up              # 全套容器化(前端由 nginx 托管)
```

**端口约定**(本机其他项目占用了 5432/8000/9001,宿主机端口整体错开):
mcp **8080**(`/mcp`)· workflow **8081** · parser **8082** · ai **8083** ·
admin 前端 **8084** · OCR **8001** · Postgres **5433** · MinIO **9100/9101** · Redis **6380**。
`backend/internal/config` 的默认值与这些端口一致,开箱即用。

## 架构:多服务(对应说明书 §7 系统组成)

```
WorkBuddy ─MCP→ backend/cmd/mcp ──┐
                                  ├─REST→ backend/cmd/workflow ─→ backend/cmd/parser(office 文档)
浏览器 ───→ frontend(nginx/vite)─┘        │      │    └────────→ ocr/(图片,Python)
             /api 反代 workflow            │      │      └──────→ backend/cmd/ai(字段提取)
                                     PostgreSQL  MinIO
```

- `backend/cmd/mcp` —— MCP Server(官方 go-sdk,Streamable HTTP),无状态,转调 workflow
- `backend/cmd/workflow` —— **工作流引擎**,唯一持有存储的服务;REST API 见其 main.go 顶部注释;
  编排逻辑在 `internal/pipeline`(按扩展名路由:图片→OCR,office→parser;再调 ai 提取)
- `backend/cmd/parser` —— 文档解析,无状态;核心逻辑 `internal/parser`(txt/docx 已支持,**PDF/xlsx/doc 未实现**)
- `backend/cmd/ai` —— AI 字段提取,无状态;字段定义随请求下发;`internal/extract` 提供
  `Mock`("标签: 值"行匹配)与 `LLM`(OpenAI 兼容端点,设 `PHX_LLM_ENDPOINT` 自动切换,DeepSeek/Qwen 通用)
- `backend/cmd/smoke` —— 冒烟客户端(模拟 WorkBuddy),不是服务
- `frontend/` —— 管理后台:文档列表、**人工审核**(字段修改→入库);生产用 nginx 托管并反代 `/api`
- `backend/internal/api` —— 服务间 HTTP 契约 DTO;`internal/clients` —— 服务间客户端
- `backend/internal/schema` —— **可配置单据类型**:`backend/configs/doctypes/*.yaml` 定义字段与
  校验规则,加单据类型不改代码
- `backend/internal/store` —— Postgres(pgx,迁移内嵌)+ MinIO;字段存 JSONB

流水线状态机(`internal/model.Status`):`uploaded → extracted → validated|needs_review → saved`,
失败 → `failed`;状态持久化,调用方可分步驱动、断点续跑。

## 硬性约束

- **MCP 工具名是对外契约**(说明书 §8.1),不得改名:`upload_document` / `extract_fields` /
  `validate_document` / `save_database` / `query_document`
- **字段提取逻辑必须留在平台内**(说明书 §13):模型来源可配置,但提取不外包给 WorkBuddy
- 大文件走 `file_url` 上传(MCP 传 base64 会撑爆上下文);流水线耗时操作未来要改异步任务语义
  (Redis 已预留,尚未使用)
