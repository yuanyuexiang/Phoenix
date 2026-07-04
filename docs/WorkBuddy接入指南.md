# WorkBuddy 接入指南(连接器 + 文档处理专家)

> 对应产品说明书 §8「与 WorkBuddy 集成」与 §11 交付物「文档处理专家配置」。
> 本指南面向在 WorkBuddy 侧做集成配置的同事。

## 1. 前置条件

Phoenix 平台已启动(本机开发环境):

```bash
make infra-up     # Postgres / MinIO / Redis / OCR
make run-all      # 4 个 Go 服务
```

MCP 端点:`http://localhost:8080/mcp`(传输协议:**Streamable HTTP**;当前开发版无鉴权,
生产部署前需补充 —— 见 §5)。

## 2. 添加连接器

在 WorkBuddy 的「连接器 / MCP 服务器」配置中添加:

```json
{
  "mcpServers": {
    "phoenix": {
      "type": "streamable-http",
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

> 不同客户端对 `type` 字段的叫法可能是 `streamable-http` / `http` / `url` 直连,
> 以 WorkBuddy 实际支持为准。

若 WorkBuddy 仅支持 stdio 方式的 MCP 服务器,用 `mcp-remote` 桥接:

```json
{
  "mcpServers": {
    "phoenix": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "http://localhost:8080/mcp"]
    }
  }
}
```

连接成功的标志:WorkBuddy 能列出以下五个工具(对外契约,名称固定):

| 工具 | 作用 |
|------|------|
| `upload_document` | 上传文档(`content_text` / `content_base64` / `file_url` 三选一) |
| `extract_fields` | OCR/解析 + AI 字段提取 |
| `validate_document` | 规则校验(通过 → validated;不通过 → needs_review) |
| `save_database` | 确认入库(可携带人工修正后的 fields,或 force) |
| `query_document` | 按类型/状态/关键词查询 |

## 3. 配置「文档处理专家」

在 WorkBuddy 的专家/Agent 创建入口:

- **名称**:文档处理专家
- **绑定工具**:phoenix 连接器的全部五个工具
- **系统提示词**(可直接粘贴):

```
你是「文档处理专家」,通过 Phoenix 企业智能文档处理平台的工具处理企业文档。

工作流程:
1. 用户提供文档时,调用 upload_document 上传。单据类型(doc_type)优先按用户说明选择;
   未说明时用 generic。文件内容小的用 content_base64/content_text,大文件让用户提供
   可访问的 URL 走 file_url。
2. 上传成功后依次调用 extract_fields、validate_document。
3. 校验结果处理:
   - status 为 validated:直接调用 save_database 入库,然后把提取出的字段值
     以表格形式汇报给用户。
   - status 为 needs_review:把 issues(校验问题)和当前字段值列给用户,
     请用户确认或给出修正值;拿到修正后,把完整的 fields 数组传入 save_database。
     用户明确表示"直接入库"时才使用 force=true。
4. 用户查询历史文档时,调用 query_document(支持 doc_type/status/keyword/limit)。

原则:
- 不要编造或"补全"文档中不存在的字段值;提取不到就如实告知。
- 金额、日期等保持文档原始写法,不做换算。
- 每个关键步骤(上传成功、提取结果、校验问题、入库完成)都简要反馈给用户。
```

- **开场白(可选)**:「请把需要处理的单据发给我(文本/扫描件/PDF),或告诉我要查询什么。」

### 使用方式

在 WorkBuddy 对话中召唤该专家(@文档处理专家 或其入口),示例指令:

- “帮我录入这份合作确认单:(粘贴内容/附件)”
- “这批发票提取一下金额和开票日期”
- “查一下上个月所有待审核的单据”

## 4. 没有 WorkBuddy 时的联调方式

- **MCP Inspector**(官方调试界面):
  `npx @modelcontextprotocol/inspector`,连接 `http://localhost:8080/mcp`,可视化调用工具。
- **Claude Code 模拟**:
  `claude mcp add -t http phoenix http://localhost:8080/mcp`,对话式驱动,
  可用来预先调优专家提示词。
- **仓库内冒烟客户端**:`make smoke`(backend/cmd/smoke,固定顺序调用五个工具)。

## 5. 生产部署前的待办(与说明书 §14 对应)

- [ ] MCP 端点鉴权方式(API Key / OAuth,以 WorkBuddy 支持为准)【待确认】
- [ ] WorkBuddy 是否支持 MCP sampling(决定 AI 模型能否复用 WorkBuddy 侧)【待确认】
- [ ] 大文件上传通道:file_url 的来源约定(WorkBuddy 文件存储 or MinIO 预签名)【待确认】
- [ ] 耗时文档的异步语义(任务 ID + 轮询)——当前为同步调用,大图/长文档会阻塞
- [ ] 按客户单据类型在 `backend/configs/doctypes/` 增加正式 schema,并同步更新专家提示词
