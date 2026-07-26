# Phoenix

> 企业智能文档处理平台(DIP)· Enterprise Intelligent Document Processing Platform
>
> **Phoenix** 是本产品的项目代号。

自动处理企业文档(扫描件、PDF、Word、Excel、图片等):识别与字段提取由
**WorkBuddy 客户端多模态大模型**完成,平台负责**归档、规则校验、结构化存储与
检索问答**,把非结构化文档转换为随取随用的结构化数据。交付形态为 WorkBuddy
专家包 **「Phoenix 文档助手(phoenix-doc-assistant)」**,直连平台员工级 REST API。

> **项目状态:开发中。** 端到端流程(上传归档→识别回传→校验→人工审核→入库→
> 查询/问答)已跑通;单据字段清单、验收指标等【待确认】项以
> [产品说明书](docs/产品说明书_企业智能文档处理平台_V1.0.md)(当前 V1.3)为准。
> V1.3 起 MCP 连接器方案已下线,历史方案见 [docs/archive/](docs/archive/)。

## 目标用户

企业行政、财务、档案管理、合同管理、采购、工程资料等需要大量处理文档的部门。
员工通过 WorkBuddy 使用;管理员/审核员使用 Web 管理后台。

## 核心功能

| # | 功能 | 说明 |
|---|------|------|
| ① | 文档上传归档 | 扫描件、PDF、Word、Excel、图片等,原件存 MinIO |
| ② | 智能识别与字段提取 | WorkBuddy 多模态大模型按平台下发的字段清单识别(客户端完成) |
| ③ | 数据校验 | 服务端按单据类型 schema 校验(必填/格式/枚举) |
| ④ | 人工审核 | 校验不通过自动转 needs_review,管理后台修正入库 |
| ⑤ | 数据入库 | 字段 JSONB 落库,正文切片向量化进知识库 |
| ⑥ | 结构化查询 | 按类型/状态/关键词/上传人 + 字段级过滤(如金额>1万) |
| ⑦ | 知识库语义问答 | 自然语言提问,返回原文片段与来源(pgvector) |
| ⑧ | 员工级可追溯 | 每员工账号 + 平台签发 token,uploaded_by/reviewed_by + 审计日志 |
| ⑨ | 单据类型配置化 | 一类单据 = 一份 YAML,无需改代码 |

## 业务流程

```
[WorkBuddy 专家发起] → 上传归档 → 客户端识别(字段+正文) → 规则校验
                    → 通过=入库 / 不通过=人工审核 → 查询与语义问答
```

## 仓库结构(Monorepo,按技术栈分)

```
docs/                  产品文档(说明书;archive/ 为已废止方案)
frontend/              前端管理后台 —— Next.js 16 + React 19 + Tailwind v4
backend/               Go 后端 —— 唯一服务 cmd/workflow(smoke 为冒烟客户端)
phoenix-doc-assistant/ WorkBuddy 专家包(提示词 + Python 脚本,调 /pub/v1)
deploy/                docker-compose(开发/生产)
samples/               演示样例
```

## 系统架构(对应说明书 §7)

| 组件 | 位置 | 职责 |
|------|------|------|
| 工作流引擎 | `backend/cmd/workflow` | 唯一后端服务(8081):编排、存储、管理面 `/api` + 员工面 `/pub/v1` |
| 员工级 REST API | `backend/internal/restapi` | `/pub/v1`,自研账号 + JWT(`internal/userauth`),操作追溯到人 |
| 前端管理后台 | `frontend/` | 文档查询、人工审核、单据类型、服务状态(8084) |
| 专家包 | `phoenix-doc-assistant/` | WorkBuddy 内「Phoenix 文档助手」,内置脚本直连 `/pub/v1` |
| 数据库 | — | PostgreSQL + pgvector,结构化数据 + 知识库向量(5433) |
| 对象存储 | — | MinIO,原始文件(9100/9101) |
| 缓存/队列 | — | Redis,预留(6380) |

## 技术选型

Go · PostgreSQL + pgvector · MinIO · Redis · 自研员工账号体系 + JWT ·
Next.js 16 + React 19 + Tailwind v4 · Docker · 识别/提取:WorkBuddy 多模态大模型(客户端)

## 与 WorkBuddy 集成

产品以 WorkBuddy 中的**「文档处理专家」**作为客户使用入口(交付形态);专家包
`phoenix-doc-assistant` 内置脚本调用平台**员工级 REST API**完成自动化文档处理:

| 端点 | 作用 |
|------|------|
| `GET /pub/v1/me` | 员工身份自省 |
| `POST /pub/v1/documents` | 上传归档 |
| `POST /pub/v1/documents/{id}/extract` | 下发字段抽取清单 |
| `POST /pub/v1/documents/{id}/validate` | schema 预校验 |
| `POST /pub/v1/documents/{id}/save` | 权威校验并入库 |
| `GET /pub/v1/documents` | 结构化查询(含字段级过滤) |
| `POST /pub/v1/ask` | 知识库语义问答 |

> 上述端点为对外契约(说明书 §8.1 V1.3),路径与请求/响应形状保持稳定,新能力以新增端点扩展。
> 员工用本人账号登录(账号在管理后台「员工」页维护),每次请求携带平台签发的 token,操作追溯到具体员工。

## 产品价值

减少人工录入,提高数据准确率,实现文档数字化、知识沉淀与业务自动化。

## 快速开始

依赖:Go 1.26+、Node 20+、Docker。

```bash
make infra-up       # 拉起 Postgres / MinIO / Redis 容器
make run-workflow   # 前台启动 workflow(唯一后端服务,8081)
make fe-install && make fe-dev   # 另开终端:前端 dev server(8084)
make smoke          # 另开终端:端到端冒烟(走管理面 /api 跑通全流水线)
```

- 管理后台:`http://localhost:8084`(人工审核在这里),默认访问密码 `phoenix123`
  (环境变量 `PHX_ADMIN_PASSWORD`,生产务必修改;置空则关闭鉴权)。
- 员工面 `/pub/v1` 联调:workflow 以 `PHX_AUTH_SECRET=dev-secret` 启动,
  再 `make smoke-auth`(自动建号 + 登录 + 负向验证 + 身份落库断言)。
- 全套容器化部署:`make compose-up`(前端打包后由 nginx 托管);单元测试:`make test`。
- **生产部署(测试阶段:push 即部署)**:推送到 master 自动触发
  [ci.yml](.github/workflows/ci.yml) 全流程——测试 → 构建 workflow/admin 两个镜像推送阿里云 ACR →
  SSH 到服务器用 [deploy/docker-compose.prod.yml](deploy/docker-compose.prod.yml) 滚动更新
  (Traefik 统一入口:域名 → 管理后台 nginx,`/api`、`/pub/v1` 反代 workflow)。
  服务器 `.env` 参考 [deploy/.env.prod.example](deploy/.env.prod.example);所需 Secrets 见 ci.yml 头部注释。
- 单据类型与提取字段在 `backend/configs/doctypes/*.yaml` 中配置,新增单据类型无需改代码。
- 知识库问答需配置 embedding 端点(`PHX_EMBED_ENDPOINT` 等,OpenAI 兼容均可);不配则知识库关闭。
- 本机宿主端口整体错开避免冲突:各服务端口见上方架构表。

