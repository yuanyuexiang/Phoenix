# Phoenix

> 企业智能文档处理平台(DIP)· Enterprise Intelligent Document Processing Platform
>
> **Phoenix** 是本产品的项目代号。

自动处理企业文档(扫描件、PDF、Word、Excel、图片等),通过 **OCR + AI 识别 + 规则校验**,
把非结构化文档转换为结构化数据,并自动写入数据库和文件服务器。可作为 **MCP 服务**接入
[WorkBuddy](#与-workbuddy-集成)。

> ⚠️ **项目状态:规划中。** 当前仓库仅包含产品说明书,尚未开始编码。本 README 依据
> 《企业智能文档处理平台_产品说明书_V1.0》整理。

## 目标用户

企业行政、财务、档案管理、合同管理、采购、工程资料等需要大量处理文档的部门。

## 核心功能

| # | 功能 | 说明 |
|---|------|------|
| ① | 文档上传 | 支持扫描件、PDF、Word、Excel、图片等 |
| ② | OCR 识别 | 提取图片/扫描件中的文字 |
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
上传文档 → OCR识别 → 文档解析 → AI提取字段 → 规则校验
        → 人工审核(可选) → 写入数据库 → 文件归档 → 完成
```

## 系统架构

| 组件 | 职责 |
|------|------|
| 前端管理后台 | 管理控制台 |
| OCR 服务 | 图片/扫描件文字识别(PaddleOCR) |
| 文档解析服务 | PDF / Word / Excel 内容提取 |
| AI 服务 | 基于 DeepSeek / Qwen 的字段提取 |
| 工作流引擎 | 编排流水线各阶段,含可选人工审核 |
| MCP Server | WorkBuddy 集成入口 |
| 数据库 | PostgreSQL,存储结构化数据 |
| 对象存储 | MinIO,存储原始文件 |
| 缓存/队列 | Redis |

## 技术选型

Go · PaddleOCR · DeepSeek / Qwen · PostgreSQL · MinIO · Redis · Docker

## 与 WorkBuddy 集成

平台对外提供一个 **MCP Server**,WorkBuddy 通过调用以下 MCP 工具完成自动化文档处理:

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

> 项目尚未搭建,构建与运行方式待补充。完成 Go 项目初始化后,请在此处补充
> 环境依赖、`docker compose` 启动、构建与测试命令。
