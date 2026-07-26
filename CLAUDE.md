# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 产品:Phoenix —— 企业智能文档处理平台(DIP)

> **Phoenix** 是项目代号。需求的唯一来源是 `docs/产品说明书_企业智能文档处理平台_V1.0.md`
> (客户确认版,当前修订到 V1.3,其中【待确认】项尚未定稿);同目录 `.docx` 是最初的原始说明书。
> 产品/领域术语请与说明书保持一致(中文)。已废止的历史方案(MCP 连接器等)在 `docs/archive/`。

**内容识别/字段提取由 WorkBuddy 客户端多模态大模型完成**(说明书 §13 V1.2 修订);后端
只负责归档(MinIO)、规则校验(doctype schema)、结构化存储与检索(Postgres + pgvector 知识库)。
WorkBuddy 识别出字段+正文后回传入库。**交付形态:WorkBuddy 专家包「Phoenix 文档助手
(phoenix-doc-assistant)」**,内置脚本直连本平台员工级 REST API(`/pub/v1`)。
(V1.3 起 MCP Server/连接器已整体下线,勿再新增 MCP 相关代码。)

## 顶层结构(按技术栈分)

```
docs/                  产品文档(说明书;archive/ 为已废止的 MCP 时代方案)
frontend/              前端管理后台 —— Next.js 16 + React 19 + Tailwind v4(TypeScript,无组件库)
backend/               Go 后端,单一 go.mod;服务入口只有 cmd/workflow(smoke 为冒烟客户端)
phoenix-doc-assistant/ WorkBuddy 专家包(提示词 + Python 脚本,调 /pub/v1)
deploy/                docker-compose.yml(本机开发)/ docker-compose.prod.yml(生产,Traefik+预构建镜像)
samples/               演示样例文档
```

CI/CD:单文件 `.github/workflows/ci.yml` —— push master 全流程(测试→构建推送 2 镜像
workflow/admin→SSH 部署→健康检查,测试阶段策略);PR 只跑测试不部署。生产用 `phoenix`
前缀命名。正式运营后建议把部署改回 `v*` 标签触发(git 历史有拆分版 deploy.yml)。

前端设计:三层主题 token(raw palette → `@theme inline` 语义色 → 组件),
`<html data-theme>` 驱动 light/dark,企业蓝调性;NavRail 左侧图标导航
(文档 `/`、审核 `/review`、单据类型 `/doctypes`、服务状态 `/status`),
审核页为「队列列 + 编辑区」三列式。生产走 `BUILD_STATIC=1` 静态导出 + nginx
反代 `/api` 与 `/pub/v1`;开发用 next rewrites 代理(见 `frontend/next.config.ts`)。

鉴权分两面,互不干扰:
- **管理面 `/api`**(前端用):除 `/healthz`、`/api/auth/*` 外均要求请求头 `X-Access-Key`
  等于 `PHX_ADMIN_PASSWORD`(默认 `phoenix123`,置空关闭鉴权)。前端登录页 `/login`
  把密钥存 localStorage 并随请求携带,401 统一跳回登录。共享密码,操作人只能记 'admin'。
- **员工面 `/pub/v1`**(WorkBuddy 专家用,`internal/restapi`):OAuth 2.1 资源服务器,
  校验 Keycloak Bearer token(`docs/员工级REST-API-OAuth接入方案.md`);身份从 token
  取得(不信任请求头),落 `documents.uploaded_by/reviewed_by` 与 `audit_log`,
  每个操作追溯到具体员工。`PHX_API_OIDC_ISSUER` 为空则 `/pub/v1` 不挂载(默认关闭)。
  联调:`make oauth-up`(Keycloak,localhost:8180,alice/bob)+ `make smoke-oauth`。
  生产 AS 选型仍【待确认】;`store` 迁移每次启动全量重放,新迁移必须幂等。

## 常用命令(全部在仓库根目录执行)

```bash
make build / test / vet      # Go:构建 / 测试 / vet(自动 cd backend)
cd backend && go test ./internal/validate -run TestRunViolations   # 单个测试

make infra-up                # 拉起 Postgres/MinIO/Redis 容器
make run-workflow            # 前台起 workflow(唯一后端服务)
make fe-dev                  # 前端 dev server(8084,/api 代理到 workflow)
make smoke                   # 端到端冒烟(走管理面 /api):上传归档→回传字段+正文入库→检索
make smoke-oauth             # 员工面 /pub/v1 冒烟(需 oauth-up + workflow 配 PHX_API_OIDC_ISSUER)

make fe-install / fe-build   # 前端依赖 / 生产构建
make compose-up              # 全套容器化(前端由 nginx 托管)
```

**端口约定**(本机其他项目占用了 5432/8000/9001,宿主机端口整体错开):
workflow **8081** · admin 前端 **8084** · Postgres **5433** · MinIO **9100/9101** ·
Redis **6380** · Keycloak **8180**(联调时)。
`backend/internal/config` 的默认值与这些端口一致,开箱即用。
(mcp 8080 已随 V1.3 下线;parser 8082 / ai 8083 已随识别移至 WorkBuddy 而下线。)

## 架构(对应说明书 §7 系统组成)

识别/字段提取在 WorkBuddy 客户端完成;后端 = 归档 + 校验 + 存储 + 检索。

```
WorkBuddy 专家 ─REST(/pub/v1,Bearer)→ backend/cmd/workflow
浏览器 ─→ frontend(nginx)─REST(/api)──┘      │        │
          /api、/pub/v1 反代 workflow    PostgreSQL   MinIO
                                        (+pgvector 知识库)
```

- `backend/cmd/workflow` —— **唯一后端服务**,持有存储;cmd 只做装配。
  管理面 REST(handler/鉴权/健康聚合)在 `internal/workflowapi`;
  员工面 REST(`/pub/v1`,OAuth 资源服务器 + 审计)在 `internal/restapi`;
  编排逻辑在 `internal/pipeline`:后端不识别,`Upload` 归档 → WorkBuddy 回传字段+正文 →
  `Save` 跑 `validate.Run` 服务端校验(通过=saved,不通过且未 force=needs_review 转人工)。
  `FieldBrief` 下发"该抽哪些字段"给 WorkBuddy;`doc_type` 由 WorkBuddy 判定
- `internal/httpx` —— HTTP 启动封装(优雅退出 + ReadHeaderTimeout),
  **新服务入口一律用 `httpx.Serve`,不要裸用 `http.ListenAndServe`**
- `backend/cmd/smoke` —— 冒烟客户端(模拟专家调 REST),不是服务
- `frontend/` —— 管理后台:文档列表、**人工审核**(字段修改→入库);生产用 nginx 托管并反代
- `backend/internal/api` —— REST 契约 DTO(管理面与员工面共用);`internal/identity` —— 操作人身份(ctx 载体)
- `backend/internal/schema` —— **可配置单据类型**:`backend/configs/doctypes/*.yaml` 定义字段与
  校验规则,加单据类型不改代码
- `backend/internal/store` —— Postgres(pgx,迁移内嵌)+ MinIO;字段存 JSONB

状态机(`internal/model.Status`):`uploaded →(WorkBuddy 回传字段+正文)→ save`;
save 内跑校验,通过 → `saved`,不通过且未 force → `needs_review`(转人工);失败 → `failed`。

## 员工面 REST 端点(说明书 §8.1,对外契约;能力由原 MCP 工具迁移而来)

- `GET /pub/v1/me`:当前 token 对应的员工身份
- `POST /pub/v1/documents`:归档到 MinIO、登记 uploaded、返回 id(text/base64/file_url 三选一)
- `POST /pub/v1/documents/{id}/extract`:**返回该类型要抽的字段清单**(下发抽取指令,后端不识别);类型未定时返回目录供选型
- `POST /pub/v1/documents/{id}/validate`:对 WorkBuddy 回传字段做 schema 预校验(不入库)
- `POST /pub/v1/documents/{id}/save`:落 `fields` + `content_text`(正文)入库,服务端权威校验;入库后切片+embedding 存知识库(best-effort)
- `GET /pub/v1/documents`:结构化查询——按类型/状态/关键词/上传人,加 `field_filters` 按字段值精确/比较筛选
  (`{field, op: eq|ne|contains|gt|gte|lt|lte|in, value/values}`,数值比较自动去千分位逗号,走 `fields` JSONB)
- `POST /pub/v1/ask`:知识库语义问答——问题向量化 → pgvector 余弦 top-K → 返回原文片段+来源
  (`internal/embed` 调可配置 embedding 端点,`PHX_EMBED_ENDPOINT` 空则知识库关闭)

数据组织:结构化(JSONB 字段,精确/可聚合/按字段查)是主干,RAG(content_text 切片向量)是补充;
异构单据结构用 JSONB + doctype YAML 吸收,不用 RAG。

## 硬性约束

- **`/pub/v1` REST 端点是对外契约**(说明书 §8.1 V1.3):路径与请求/响应形状不得随意变更,
  新能力以新增端点方式扩展;`phoenix-doc-assistant` 专家包内的 API 文档须同步
- **内容识别/字段提取在 WorkBuddy 客户端完成**(说明书 §13 V1.2 修订);
  后端只做归档、校验、存储、检索。字段真实性靠 WorkBuddy,后端防线 = schema 校验 + needs_review + audit_log
- 大文件走 `file_url` 上传(base64 会撑爆专家上下文);耗时的 embedding 入库未来可改 Redis 异步(已预留)
- **MCP 已废止**(V1.3):不要恢复 `cmd/mcp`/`internal/mcpauth` 或引入 MCP SDK;历史方案见 `docs/archive/`
