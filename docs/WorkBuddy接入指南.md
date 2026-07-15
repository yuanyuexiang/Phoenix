# WorkBuddy 接入指南(连接器 + 文档处理专家)

> 对应产品说明书 §8「与 WorkBuddy 集成」与 §11 交付物「文档处理专家配置」。
> 本指南面向在 WorkBuddy 侧做集成配置的同事。
> 专家的完整发布物料见 [文档处理专家_发布包.md](文档处理专家_发布包.md)。

## 1. 前置条件

**生产环境**(已部署):MCP 端点 `https://phoenix.matrix-net.tech/mcp`
(传输协议 **Streamable HTTP**;平台已支持 OAuth 2.1 鉴权,生产由 `PHX_OAUTH_MODE`
控制,⚠️ 当前默认 off,对外发布专家前必须开启,见 §5 与 `docs/MCP-OAuth鉴权方案.md`)。

**本机开发环境**:

```bash
make infra-up     # Postgres / MinIO / Redis
make run-all      # 4 个 Go 服务
```

本地 MCP 端点:`http://localhost:8080/mcp`。

## 2. 添加连接器

在 WorkBuddy 的「连接器 / MCP 服务器」配置中添加:

```json
{
  "mcpServers": {
    "phoenix": {
      "type": "streamable-http",
      "url": "https://phoenix.matrix-net.tech/mcp"
    }
  }
}
```

(本机联调时把 `url` 换成 `http://localhost:8080/mcp`。)

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
| `extract_fields` | 文字识别/解析 + AI 字段提取 |
| `validate_document` | 规则校验(通过 → validated;不通过 → needs_review) |
| `save_database` | 确认入库(可携带人工修正后的 fields,或 force) |
| `query_document` | 按类型/状态/关键词查询 |

## 3. 配置「文档处理专家」

> **连接器已内嵌专家**:服务器同时暴露 MCP server instructions 与 `document-expert`
> prompt——WorkBuddy 若支持任一能力,添加连接器即得专家,可跳过本节的提示词粘贴,
> 只需补元信息(名称/简介/开场白,见发布包 §1)。

在 WorkBuddy 的专家/Agent 创建入口:

- **名称**:文档处理专家
- **绑定工具**:phoenix 连接器的全部五个工具
- **系统提示词**(WorkBuddy 不支持 instructions/prompts 时手工粘贴):

```
你是「文档处理专家」,通过 Phoenix 企业智能文档处理平台的工具处理企业文档。

工作流程:
1. 用户提供文档时,调用 upload_document 上传。单据类型(doc_type)按用户说明选择;
   用户未说明时不传该参数,平台会自动识别类型(识别不出会转人工审核定类型)。
   文件内容小的用 content_base64/content_text,大文件让用户提供可访问的 URL 走 file_url。
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

## 4.5 发布专家(分发给团队/其他用户)

专家调通后,把它从"自己可用"变成"别人可用"的步骤:

1. **物料**:按 [文档处理专家_发布包.md](文档处理专家_发布包.md) 的字段在 WorkBuddy
   专家编辑页补全元信息(简介、开场白、示例指令)——发布后这些就是用户看到的门面。
2. **发布动作**:在 WorkBuddy 专家页寻找「发布 / 共享 / 上架」入口,按平台支持的
   范围选择:团队内共享 → 全组织 → 专家市场。各平台叫法不同,以 WorkBuddy 文档为准。
3. **发布前硬性检查**:过一遍发布包 §5 的检查清单——尤其是 **MCP 端点鉴权**
   (发布 = 端点 URL 扩散,当前生产端点无鉴权,先补再发)和**正式单据类型 schema**
   (别让用户拿着 generic 演示配置干真活)。
4. **版本管理**:提示词/工具/schema 任何一项变更,同步更新发布包的版本号与变更记录,
   再在 WorkBuddy 重新发布。

## 5. 生产部署前的待办(与说明书 §14 对应)

- [x] ~~MCP 端点鉴权方式~~ 平台侧 OAuth 2.1 资源服务器已实现(`docs/MCP-OAuth鉴权方案.md`,
      三档开关 `PHX_OAUTH_MODE`,当前生产为 off);仍待:AS 选型拍板(方案 §3)、
      WorkBuddy 对方案 §5 四项能力的书面确认(不支持则降级静态 token,方案 §6)【待确认】
- [ ] WorkBuddy 是否支持 MCP sampling(决定 AI 模型能否复用 WorkBuddy 侧)【待确认】
- [ ] 大文件上传通道:file_url 的来源约定(WorkBuddy 文件存储 or MinIO 预签名)【待确认】
- [ ] 耗时文档的异步语义(任务 ID + 轮询)——当前为同步调用,大图/长文档会阻塞
- [ ] 按客户单据类型在 `backend/configs/doctypes/` 增加正式 schema,并同步更新专家提示词
