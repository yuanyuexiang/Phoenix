# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 当前状态

**这是一个尚未开始编码的仓库。** 目前只包含一份产品说明书:
`企业智能文档处理平台_产品说明书_V1.0.docx`。没有任何源代码、构建系统、依赖清单或测试,
也还不是一个 git 仓库。

在开始实现时,请依据下方的架构从零搭建项目,不要假设已有目录结构。这份 `.docx` 是需求的
唯一来源——在做设计决策前先阅读它(解压后提取 `word/document.xml`,再去掉 XML 标签即可得到正文)。

## 产品:Phoenix —— 企业智能文档处理平台(DIP)

> **Phoenix** 是本产品的项目代号(与仓库目录名一致)。

通过 OCR + AI 提取 + 规则校验,把非结构化的企业文档(扫描件、PDF、Word、Excel、图片)
转换为结构化数据,再写入数据库和文件/对象存储。平台设计为通过 **MCP Server** 接口供
**WorkBuddy** 调用。

### 处理流水线

核心业务流程是一条流水线,每个阶段都是天然的服务/模块边界:

```
上传 → OCR识别 → 文档解析 → AI字段提取 → 规则校验
     → 人工审核(可选) → 写入数据库 → 文件归档 → 完成
```

### 规划中的架构(来自说明书,尚未实现)

- **前端管理后台** —— 管理控制台
- **OCR 服务** —— PaddleOCR
- **文档解析服务** —— PDF / Word / Excel 内容提取
- **AI 服务** —— 使用 DeepSeek / Qwen 大模型做字段提取
- **工作流引擎** —— 编排流水线各阶段,包含可选的人工审核环节
- **MCP Server** —— WorkBuddy 的集成入口(见下)
- **数据存储** —— PostgreSQL(结构化数据)、MinIO(对象/文件存储)、Redis(缓存/队列)

### 规划中的技术选型

Go · PaddleOCR · DeepSeek/Qwen · PostgreSQL · MinIO · Redis · Docker

### MCP 集成(WorkBuddy)

平台对外暴露一个 MCP Server。WorkBuddy 通过调用以下 MCP 工具来完成自动化处理——
把这些工具名/契约当作对外集成规范:

- `upload_document`
- `extract_fields`
- `validate_document`
- `save_database`
- `query_document`

## 开发注意事项

- 说明书为中文,产品/领域术语请与其保持一致。MCP 工具名必须与上面完全一致(它们是对外契约)。
- 目前没有构建、lint 或测试命令。等 Go 项目搭建好后,请在此文件补充实际的
  `go build` / `go test` / lint 命令,以及如何运行单个测试。
