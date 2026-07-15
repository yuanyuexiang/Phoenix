# Phoenix

> 企业智能文档处理平台(DIP)· Enterprise Intelligent Document Processing Platform
>
> **Phoenix** 是本产品的项目代号。

自动处理企业文档(扫描件、PDF、Word、Excel、图片等),通过 **AI 视觉转写 + AI 识别 + 规则校验**,
把非结构化文档转换为结构化数据,并自动写入数据库和文件服务器。可作为 **MCP 服务**接入
[WorkBuddy](#与-workbuddy-集成)。

> **项目状态:开发中。** Monorepo 多服务骨架已就绪(MCP 连接器、工作流引擎、
> 文档解析、AI 提取/转写、管理后台五个服务),端到端流程(上传→提取→校验→
> 人工审核→入库→查询)已跑通;PDF/Excel 解析、LLM 真实提取待接入客户环境后启用。

## 目标用户

企业行政、财务、档案管理、合同管理、采购、工程资料等需要大量处理文档的部门。

## 核心功能

| # | 功能 | 说明 |
|---|------|------|
| ① | 文档上传 | 支持扫描件、PDF、Word、Excel、图片等 |
| ② | 图片文字识别 | 视觉大模型转写图片/扫描件中的文字 |
| ③ | 文档解析 | 解析 PDF / Word / Excel 内容 |
| ④ | AI 字段提取 | 用大模型从文本中抽取结构化字段 |
| ⑤ | 数据校验 | 基于规则校验提取结果 |
| ⑥ | 人工审核 | 可选的人工复核环节 |
| ⑦ | 数据入库 | 结构化数据写入数据库 |
| ⑧ | 文件归档 | 原始文件归档到对象存储 |
| ⑨ | 文档查询 | 按条件检索已处理文档 |
| ⑩ | WorkBuddy 调用 | 通过 MCP 供外部自动化调用 |

## 业务流程

```
上传文档 → 文字识别/解析 → AI提取字段 → 规则校验
        → 人工审核(可选) → 写入数据库 → 文件归档 → 完成
```

## 仓库结构(Monorepo,按技术栈分)

```
docs/       产品文档(说明书、WorkBuddy 接入指南)
frontend/   前端管理后台 —— Next.js + Tailwind(人工审核、查询、服务状态)
backend/    Go 后端 —— 四个服务:workflow / parser / ai / mcp
deploy/     docker-compose
samples/    演示样例
```

## 系统架构(对应说明书 §7)

| 组件 | 位置 | 职责 |
|------|------|------|
| MCP Server | `backend/cmd/mcp` | WorkBuddy 连接器,主要使用入口(8080,`/mcp`) |
| 工作流引擎 | `backend/cmd/workflow` | 编排流水线、持有存储,REST API(8081) |
| 文档解析服务 | `backend/cmd/parser` | PDF / Word / Excel → 纯文本(8082) |
| AI 服务 | `backend/cmd/ai` | 大模型字段提取 + 图片视觉转写,模型可配置(8083) |
| 前端管理后台 | `frontend/` | Next.js + Tailwind:人工审核、查询、服务状态(8084) |
| 数据库 | — | PostgreSQL,存储结构化数据(5433) |
| 对象存储 | — | MinIO,存储原始文件(9100/9101) |
| 缓存/队列 | — | Redis,预留(6380) |

## 技术选型

Go · DeepSeek / Qwen · Qwen-VL(视觉转写) · PostgreSQL · MinIO · Redis · Docker

## 与 WorkBuddy 集成

产品以 WorkBuddy 中的**「文档处理专家」**作为客户使用入口(交付形态);专家底层通过 **MCP Server**(连接器形态)调用以下工具完成自动化文档处理:

| 工具 | 作用 |
|------|------|
| `upload_document` | 上传文档 |
| `extract_fields` | 提取字段 |
| `validate_document` | 校验文档 |
| `save_database` | 数据入库 |
| `query_document` | 查询文档 |

> 上述工具名为对外契约,实现时须保持完全一致。

## 产品价值

减少人工录入,提高数据准确率,实现文档数字化、知识沉淀与业务自动化。

## 快速开始

依赖:Go 1.26+、Node 20+、Docker。

```bash
make infra-up       # 拉起 Postgres / MinIO / Redis 容器
make run-all        # 前台并行启动 4 个 Go 服务(Ctrl-C 全停)
make fe-install && make fe-dev   # 另开终端:前端 dev server(8084)
make smoke          # 另开终端:模拟 WorkBuddy 调用五个 MCP 工具,端到端跑通流水线
```

- MCP 端点:`http://localhost:8080/mcp`;管理后台:`http://localhost:8084`(人工审核在这里)。
- 管理后台需登录,默认访问密码 `phoenix123`(环境变量 `PHX_ADMIN_PASSWORD`,生产务必修改;置空则关闭鉴权)。
- 全套容器化部署:`make compose-up`(前端打包后由 nginx 托管);单元测试:`make test`。
- **生产部署(测试阶段:push 即部署)**:推送到 master 自动触发
  [ci.yml](.github/workflows/ci.yml) 全流程——测试 → 构建 6 个服务镜像推送阿里云 ACR →
  SSH 到服务器用 [deploy/docker-compose.prod.yml](deploy/docker-compose.prod.yml) 滚动更新
  (Traefik 统一入口:域名 → 管理后台,`/mcp` → 连接器)。服务器 `.env` 参考
  [deploy/.env.prod.example](deploy/.env.prod.example);所需 Secrets 见 ci.yml 头部注释。
- 单据类型与提取字段在 `backend/configs/doctypes/*.yaml` 中配置,新增单据类型无需改代码。
- AI 服务默认使用 Mock 提取器;配置 `PHX_LLM_ENDPOINT` / `PHX_LLM_API_KEY` 后自动切换到
  真实大模型(DeepSeek / Qwen 等 OpenAI 兼容端点均可)。
- 本机宿主端口整体错开避免冲突:各服务端口见上方架构表。
