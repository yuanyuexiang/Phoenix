# Phoenix MCP 连接器接入方案（WorkBuddy 连接器系统）

> 状态：**OAuth 全流程验证通过，token 已落盘，卡在 WorkBuddy UI 状态刷新** | 版本 V1.3 | 日期 2026-07-16
>
> 实测进展（2026-07-16，WorkBuddy v5.2.6，自定义 MCP 方式）：
> 401 → 元数据发现 → DCR 注册 → Keycloak 登录 → 授权同意页 → 浏览器跳 `workbuddy://` 回调 → **token 交换成功** → **accessToken/refreshToken 已写入 `.credentials.v3.json`**，
> 全部成功。**唯一卡点**：WorkBuddy 连接器/MCP 设置页 UI 一直停在「授权中…」，
> 尽管 token 已保存且日志显示 `success=true`（19 次成功 / 2 次失败）。
> 服务端另经冒烟客户端端到端验证通过（password grant 取 token → 调五工具 → uploaded_by 落库）。
> 结论：**服务端 + OAuth 协议层完全无误**；卡点在 WorkBuddy 客户端 UI 状态机——token 已落盘但状态未从 "authorizing" 翻转为 "connected"。
> 关联文档：[MCP-OAuth鉴权方案.md](MCP-OAuth鉴权方案.md) | [WorkBuddy接入指南.md](WorkBuddy接入指南.md)
>
> 本文档面向**开发人员**，说明如何将 Phoenix MCP 注册为 WorkBuddy 连接器（Connector），
> 实现「安装连接器 → 点连接 → OAuth 登录 → 开箱即用」的企业交付体验。

---

## 1. 为什么用连接器系统而不是专家包内嵌 .mcp.json

| 维度 | 专家包 .mcp.json | 连接器系统（本文方案） |
|------|------------------|----------------------|
| MCP 配置位置 | 专家包根目录 `.mcp.json` | `connectors/phoenix/mcp.json` |
| 安装时 MCP 自动注册 | ✅ 自动写入 mcp.json | ✅ 自动注册 |
| OAuth 动态发现（401 → PKCE） | ✅ **已验证通过**（自定义 MCP 路径） | ✅ 已有 WSO2+Keycloak 实际案例 |
| 用户授权交互（登录页跳转） | ❌ 无 UI 入口 | ✅ 连接器页面「连接」按钮触发 |
| Token 管理（存储/刷新/过期） | ❌ 无机制 | ✅ WorkBuddy 自动管理 |
| 多用户独立身份 | ❌ 共享或无 | ✅ 每员工独立授权 |
| 市场分发 | ✅ 专家市场 | ✅ 连接器市场 |
| 官方文档支持 | ❌ 无公开文档 | ✅ 连接器系统有文档 |

**结论**：Phoenix 的 OAuth 2.1 + PKCE 方案必须走连接器系统，专家包只负责 AI 角色人设。

---

## 2. WorkBuddy 连接器系统架构

### 2.1 目录结构

连接器市场位于 `~/.workbuddy/connectors-marketplace/`：

```
connectors-marketplace/
├── .codebuddy-connector/
│   └── connectors.json          ← 连接器注册表（索引）
├── connectors/
│   ├── phoenix/                 ← ← ← 我们要创建的
│   │   ├── connector-meta.json  ← 连接器元信息（名称、描述、类型、示例）
│   │   ├── mcp.json             ← MCP 端点配置
│   │   ├── skills/
│   │   │   └── SKILL.md         ← （可选）连接成功后的 Skill 指引
│   │   └── icon.png             ← （可选）连接器图标
│   ├── tyc-mcp/                 ← 参考：天眼查（token 模式）
│   ├── westock-mcp/             ← 参考：腾讯自选股（无鉴权）
│   ├── tencent-docs/            ← 参考：腾讯文档（平台级 OAuth）
│   └── ...
├── icons/                       ← 连接器图标缓存
└── dist/                        ← 构建产物
```

### 2.2 连接器注册表（connectors.json）

注册表位于 `.codebuddy-connector/connectors.json`，格式：

```json
{
  "name": "codebuddy-connectors-official",
  "description": "CodeBuddy 官方连接器配置索引",
  "owner": { "name": "...", "email": "..." },
  "auth_injection_rules": [ ... ],     ← 平台级 token 注入规则（可选）
  "connectors": [
    {
      "id": "phoenix",
      "name": "Phoenix 文档处理",
      "name_en": "Phoenix Document Processing",
      "description": "...",
      "description_zh": "...",
      "source": "phoenix",
      "type": "mcp",
      "examples_zh": ["帮我上传一份发票", "查一下上周的所有单据"],
      "examples_en": ["..."]
    }
  ]
}
```

> **注意**：`connectors.json` 中的条目和 `connector-meta.json` 的内容会同步。
> `connector-meta.json` 是连接器目录内的详细配置，`connectors.json` 是市场级索引。

### 2.3 已知的认证模式

通过分析全部 35 个现有连接器，WorkBuddy 支持两种 `auth_mode`：

| auth_mode | 数量 | 说明 | 代表连接器 |
|-----------|------|------|-----------|
| `"token"` | 8 个 | 用户手动输入 API Key，通过 `token-schema.json` 定义表单 | tyc-mcp, qingflow, tencent-map |
| 无（不设） | 27 个 | 无鉴权 **或** 平台级 OAuth（由 `auth_injection_rules` 注入） | westock-mcp, tencent-docs, github |

**关键发现**：当前**没有** `auth_mode: "oauth"` 的自定义连接器。OAuth 流程通过两种方式触发：
1. **平台级**：如 `tencent-docs`，通过 `auth_injection_rules` 注入 `mcp-oauth` 类型 token（需平台侧硬编码支持）
2. **MCP 协议级**：连接器不设 `auth_mode`、`mcp.json` 不设 headers，MCP 客户端收到 401 后自动走 RFC 9728 发现 → OAuth 2.1 + PKCE 流程

**Phoenix 方案**：走第 2 种——连接器不设 `auth_mode`、`mcp.json` 不设 headers，依赖 WorkBuddy MCP 客户端收到 401 后自动走 RFC 9728 发现 → OAuth 2.1 + PKCE 流程。

### 2.4 实测验证结果（2026-07-16，WorkBuddy v5.2.6）

**测试方式**：通过专家包根目录 `.mcp.json` 注册自定义 MCP（configId = `custom-mcp:phoenix`），非连接器系统路径。

| 环节 | 状态 | 证据 |
|------|------|------|
| 401 + 资源元数据发现 | ✅ 通过 | 浏览器成功弹出 Keycloak 登录页 |
| DCR 动态客户端注册 | ✅ 通过 | `.credentials.v3.json` 中有 `client_id: c2718c59-...` |
| Keycloak 登录 + 授权同意 | ✅ 通过 | 用户截图确认 consent 页出现，点 Yes 后回调 |
| `workbuddy://` 回调 | ✅ 通过 | 日志 `[DeepLink] handled as MCP OAuth callback` |
| Token 交换 | ✅ 通过 | 日志 `[MCP OAuth] Result: success=true`（19 次成功） |
| Token 持久化 | ✅ 通过 | `.credentials.v3.json` 含 accessToken + refreshToken（AES-256-GCM 加密） |
| **UI 状态刷新** | ❌ **卡住** | 连接器/MCP 设置页一直显示「授权中…」 |
| Token 自动刷新 | ⏳ 未验证 | token 已有 `expiresAt`，但 UI 卡住无法触发后续调用 |

**回调 URL 格式（已确认）**：
```
workbuddy://workbuddy/mcp/custom-mcp:phoenix/oauth/callback
```
> URL 编码后为 `workbuddy://workbuddy/mcp/custom-mcp%3Aphoenix/oauth/callback`，`%3A` = `:`。
> 此格式来自 `.credentials.v3.json` 的 `mcpClientInfo.redirect_uris` 字段，**已确认无误**。

**Token 存储结构（已确认）**：
```json
{
  "mcpOAuth": {
    "phoenix|fa3239f0be1c911b": {
      "serverName": "phoenix",
      "serverUrl": "https://phoenix.matrix-net.tech/mcp",
      "tokenType": "Bearer",
      "expiresAt": 1784187391069,
      "scope": "profile email",
      "accessToken": { "iv": "...", "tag": "...", "ct": "..." },
      "refreshToken": { "iv": "...", "tag": "...", "ct": "..." }
    }
  },
  "mcpClientInfo": {
    "phoenix|fa3239f0be1c911b": {
      "client_id": "c2718c59-d892-4091-b19e-4de6f7d6f7a0",
      "redirect_uris": ["workbuddy://workbuddy/mcp/custom-mcp%3Aphoenix/oauth/callback"]
    }
  }
}
```

**卡点根因分析（2026-07-16 深入排查）**：

WorkBuddy 的 MCP 架构并非 Sidecar → Phoenix 直连，而是三层：

```
Sidecar (CLI, port 63432) → connector-proxy (port 63727, Electron 主进程内) → Phoenix MCP
```

`~/.workbuddy/.mcp.json` 确认了 proxy 的存在：
```json
{
  "mcpServers": {
    "connector-proxy": {
      "type": "http",
      "url": "http://127.0.0.1:63727/mcp",
      "description": "Aggregated proxy containing MCP servers: phoenix, zsxq"
    }
  }
}
```

**问题出在 proxy 层**：proxy 尝试初始化与 Phoenix 的 MCP 会话时收到 401 → 触发 OAuth → token 交换成功并写入 `.credentials.v3.json`。
但 **proxy 未用新获取的 token 重试 MCP `initialize` 握手**——会话停留在「未初始化」状态。

直接 curl proxy 验证：
```bash
$ curl http://127.0.0.1:63727/mcp -H "Auth: Bearer <internal>" -d '{"method":"tools/list"}'
{"error":"Server not initialized"}  ← proxy 的 Phoenix 会话从未建立
```

**额外发现**：accessToken 已于 2026-07-16 15:36 过期（`expiresAt: 1784187391069`），
refreshToken 存在但从未被使用（因为 proxy 会话从未初始化，无法触发刷新）。

> ⚠️ **根因**：WorkBuddy v5.2.6 connector-proxy 的 OAuth 回调处理器在 `success=true` 后，
> 未自动用新 token 重试 MCP `initialize` 握手。这是一个客户端状态机 bug。
>
> **解决方案**（按优先级）：
> 1. **重启 WorkBuddy**（推荐）：proxy 重启后读取已保存的 refreshToken → 刷新 accessToken → 用新 token 初始化 MCP 会话 → UI 应显示「已连接」
> 2. **清除凭据重新授权**：删除 `.credentials.v3.json` 中 `mcpOAuth.phoenix|*` 条目 → 重启 → 重新走 OAuth
> 3. **反馈 WorkBuddy 团队**：OAuth 回调 `success=true` 后应自动重试 `initialize`，无需重启

---

## 3. 实现方案

动态 OAuth 发现：连接器不配置任何静态密钥。WorkBuddy 的 MCP 客户端连接 Phoenix 时收到 401，自动通过 RFC 9728 发现 Keycloak，发起 OAuth 2.1 + PKCE 授权。

**验证状态**：✅ **已通过实测**（2026-07-16，WorkBuddy v5.2.6）。从 401 到 token 交换全链路成功，token 已持久化。仅剩 UI 状态刷新问题（§2.4）。

> ⚠️ 此方案为唯一实现路径。OAuth 协议层已验证通过，不设降级方案。

---

## 4. 完整文件模板

### 4.1 连接器目录结构

```
connectors/phoenix/
├── connector-meta.json
├── mcp.json
└── skills/
    └── SKILL.md
```

### 4.2 connector-meta.json

```json
{
  "id": "phoenix",
  "name": "Phoenix 文档处理",
  "name_zh": "Phoenix 文档处理",
  "name_en": "Phoenix Document Processing",
  "description": "Enterprise document processing via Phoenix MCP. Upload, extract fields, validate, and query business documents.",
  "description_zh": "通过 Phoenix MCP 平台处理企业文档，支持单据上传、字段提取、校验入库与历史文档查询全流程。",
  "description_en": "Enterprise document processing via Phoenix MCP. Upload, extract fields, validate, and query business documents.",
  "source": "phoenix",
  "type": "mcp",
  "version": "1.0.0",
  "minWorkbuddyVersion": "4.24.0",
  "examples_zh": [
    "帮我上传这份发票并提取字段",
    "查一下上周所有待审核的单据",
    "把这份合作确认单录入系统"
  ],
  "examples_en": [
    "Upload this invoice and extract fields",
    "Query all pending documents from last week",
    "Enter this confirmation letter into the system"
  ]
}
```

**字段说明**：

| 字段 | 必填 | 说明 |
|------|:----:|------|
| `id` | ✅ | 连接器唯一标识，与目录名一致 |
| `name` / `name_zh` / `name_en` | ✅ | 显示名称 |
| `description` / `description_zh` / `description_en` | ✅ | 描述（市场搜索用） |
| `source` | ✅ | 连接器源标识，与目录名一致 |
| `type` | ✅ | 固定 `"mcp"` |
| `version` | ✅ | 连接器版本 |
| `minWorkbuddyVersion` | - | 最低 WorkBuddy 版本要求 |
| `examples_zh` / `examples_en` | - | 示例指令，显示在连接器卡片上 |
| `auth_mode` | ❌ | **不设此字段**（触发动态 OAuth 发现） |

### 4.3 mcp.json

```json
{
  "mcpServers": {
    "phoenix": {
      "type": "streamable-http",
      "url": "https://phoenix.matrix-net.tech/mcp",
      "disabled": false
    }
  }
}
```

**关键点**：
- **不设 `headers`** —— 不预置任何 Authorization 头，让 MCP 客户端通过 401 响应触发 OAuth 流程
- `type` 用 `streamable-http`（MCP 规范标准命名，实测可用；⚠️ 不是 `streamableHttp` 驼峰式）
- `disabled: false` 确保连接器加载时启用
- `timeout` 可选（文档处理可能耗时较长，建议设 600000ms）

> **本地联调**时把 `url` 改为 `http://localhost:8080/mcp`。
> **实测使用的 .mcp.json**（专家包根目录，自动加载）：与上方模板一致。

### 4.4 skills/SKILL.md

```markdown
---
name: phoenix-doc-skill
description: Phoenix 企业智能文档处理平台 MCP 能力
version: "1.0.0"
---

# Phoenix 文档处理 Skill

本 Skill 在 Phoenix 连接器激活后自动加载，提供文档处理全流程能力。

## 可用工具

### upload_document — 上传文档
参数：doc_type（可选）、content_text / content_base64 / file_url（三选一）

### extract_fields — 字段提取
参数：document_id

### validate_document — 文档校验
参数：document_id

### save_database — 入库
参数：document_id、fields（可选，人工修正值）、force（可选）

### query_document — 查询历史文档
参数：doc_type / status / keyword / uploaded_by / limit（均可选）

## 使用规范
- 严格遵循 upload → extract → validate → save 顺序
- 不编造字段值，提取不到如实告知
- 金额日期保持原始写法不做换算
- needs_review 状态必须等用户确认后再入库
```

### 4.5 注册到 connectors.json

在 `~/.workbuddy/connectors-marketplace/.codebuddy-connector/connectors.json` 的
`connectors` 数组末尾添加：

```json
{
  "id": "phoenix",
  "name": "Phoenix 文档处理",
  "name_en": "Phoenix Document Processing",
  "description": "Enterprise document processing via Phoenix MCP.",
  "description_zh": "通过 Phoenix MCP 平台处理企业文档，支持上传、提取、校验、查询全流程。",
  "description_en": "Enterprise document processing via Phoenix MCP.",
  "source": "phoenix",
  "type": "mcp",
  "examples_zh": [
    "帮我上传这份发票并提取字段",
    "查一下上周所有待审核的单据"
  ],
  "examples_en": [
    "Upload this invoice and extract fields",
    "Query all pending documents from last week"
  ]
}
```

---

## 5. Keycloak 配置要求

### 5.1 必须配置的项

| 配置项 | 值 | 说明 |
|--------|-----|------|
| Realm | `phoenix` | 已有 |
| 签发者 URL | `https://phoenix.matrix-net.tech/auth/realms/phoenix` | 已有 |
| Audience | `phoenix-mcp` | token 的 aud 声明，Phoenix RS 侧校验 |
| Resource 标识 | `https://phoenix.matrix-net.tech/mcp` | RFC 8707 resource 参数值 |

### 5.2 OAuth 配置

**Redirect URI（回调地址）**

WorkBuddy 的 OAuth 回调地址格式**已通过实测确认**：

```
workbuddy://workbuddy/mcp/custom-mcp:phoenix/oauth/callback
```

> ✅ 此格式来自 `.credentials.v3.json` 的 `mcpClientInfo.redirect_uris` 字段，实测匹配。
> URL 编码形式：`workbuddy://workbuddy/mcp/custom-mcp%3Aphoenix/oauth/callback`（`%3A` = `:`）。
> 如果走连接器系统路径（非自定义 MCP），格式可能是 `workbuddy://workbuddy/mcp/connector:phoenix/oauth/callback`，待连接器路径验证后确认。

在 Keycloak 中配置：
1. 创建或编辑客户端（如果用 DCR 则跳过此步）
2. **Valid Redirect URIs** 添加：`workbuddy://workbuddy/mcp/custom-mcp:phoenix/oauth/callback`
3. 保险起见添加通配：`workbuddy://*`

**DCR（动态客户端注册）**

WorkBuddy 每次点「连接」时会向 Keycloak 匿名发起 DCR 请求（自己注册一个 public 客户端，
带上自己的 `workbuddy://` 回调地址）。因此**无需手工建客户端、无需手工配 Redirect URI**
（上面"编辑客户端"那步在 DCR 模式下不适用，redirect_uri 由 WorkBuddy 注册时自带，天然匹配）。

Keycloak 侧要做的（★ 为实测中真正踩过的坑）：

1. **★ 删除匿名注册的 Trusted Hosts 策略**（本次最大阻塞点）：
   phoenix realm → Clients → Client registration → **Anonymous access policies** →
   删除 **Trusted Hosts**。不删则 DCR 直接被拒（`Host not trusted`），WorkBuddy 卡在「授权中…」
   连登录页都弹不出来。注意**务必在 phoenix realm 操作**，不是 master。
2. 匿名注册默认开启，删掉 Trusted Hosts 后即可用（无需额外配 Initial Access Token）。
3. （可选加固，实测不配也能跑通）DCR 客户端过期时间、PKCE 强制 Client Policy、
   删除 "Consent Required" 匿名策略以跳过授权同意页。
   - 说明：不删 Consent Required 时，员工首次授权会多一个「Grant Access」同意页，点 Yes 继续即可。

> 安全提醒：删除 Trusted Hosts = 任何人都能在你的 Keycloak 注册客户端（但**拿 token 仍需有效员工账号**，
> 数据门未开）。测试阶段可接受；转正式改用 Initial Access Token 模式或重新收紧 Trusted Hosts。

**PKCE 强制配置**

即使 WorkBuddy 在 DCR 请求中声明了 PKCE，也应在 Keycloak 侧通过 Client Policy 强制：

```
Realm → Client Policies → Policies → 新建 "PKCE Enforcer"
  → 条件：client-types = [public, dynamic]
  → 执行器：require-pkce
```

---

## 6. Phoenix 侧要求

### 6.1 MCP 端点（已有）

- URL：`https://phoenix.matrix-net.tech/mcp`
- 协议：Streamable HTTP
- 五个工具：`upload_document`、`extract_fields`、`validate_document`、`save_database`、`query_document`

### 6.2 OAuth 资源服务器（已有）

根据 [MCP-OAuth鉴权方案.md](MCP-OAuth鉴权方案.md) §4，Phoenix 已实现：

| 组件 | 状态 | 说明 |
|------|------|------|
| Bearer token 校验中间件 | ✅ 已落地 | `auth.RequireBearerToken` + JWKS 验签 |
| 资源元数据端点 | ✅ 已落地 | `/.well-known/oauth-protected-resource` |
| 身份透传与落库 | ✅ 已落地 | `uploaded_by` / `reviewed_by` |
| 三档开关 | ✅ 已落地 | `PHX_OAUTH_MODE=off|optional|required` |

### 6.3 well-known 路由（★ 实测踩过的坑，Traefik 侧必配）

MCP 客户端做 OAuth 发现时，除了 `/.well-known/oauth-protected-resource`，还会在**资源域名根部**
探测授权服务器元数据：`/.well-known/oauth-authorization-server` 与 `/.well-known/openid-configuration`
（旧版 2025-03-26 规范的客户端行为）。这些路径**不在 `/mcp` 前缀内**，默认会落到 admin 前端 SPA、
返回 HTML，客户端报 `Unexpected token '<', "<!DOCTYPE"... is not valid JSON`（本次实测原始报错）。

解决：`deploy/docker-compose.prod.yml` 的 mcp 服务已加两组 Traefik 路由（生产已生效）：
- `/.well-known/oauth-protected-resource*` → 路由到 mcp（SDK 输出 RFC 9728 资源元数据）
- `/.well-known/oauth-authorization-server` 与 `/.well-known/openid-configuration` → 302 重定向到
  Keycloak 的 `.../auth/realms/phoenix/.well-known/openid-configuration`

> 验证：`curl -I https://phoenix.matrix-net.tech/.well-known/oauth-authorization-server` 应 302 到 Keycloak，
> 而不是返回 HTML。

### 6.4 已确认 / 待确认

1. **401 响应格式**：✅ 已确认。含 `WWW-Authenticate: Bearer resource_metadata="…/.well-known/oauth-protected-resource/mcp"`。
2. **Token audience 校验**：✅ 已确认。realm 用 audience mapper 硬编码 `aud=phoenix-mcp`，与 `PHX_OAUTH_AUDIENCE` 一致。
3. **CORS**：WorkBuddy 是桌面 app、回调走 `workbuddy://` 自定义协议，非浏览器同源请求，实测未遇 CORS 拦截。

---

## 7. 专家包配合

连接器负责 MCP 端点和 OAuth 流程，专家包负责 AI 角色人设。两者配合：

### 7.1 专家包保持不变

专家包 `phoenix-doc-expert` 的结构不需要改动：

```
phoenix-doc-expert/
├── .codebuddy-plugin/plugin.json
├── agents/phoenix-doc-expert.md    ← 引用 mcp__phoenix__* 工具
├── avatars/expert.png
└── README.md
```

> **移除专家包根目录的 `.mcp.json`**——MCP 配置由连接器负责，专家包不再内嵌。

### 7.2 安装顺序

```
1. 安装连接器（phoenix connector）
   → 连接器市场搜索 "Phoenix" → 安装 → 点「连接」→ OAuth 登录

2. 安装专家（phoenix-doc-expert）
   → 专家市场搜索 "文档处理专家" → 安装

3. 使用
   → 召唤专家 → 上传文档 → 自动调用已授权的 phoenix MCP 工具
```

### 7.3 工具名映射

专家 Agent MD 中引用的工具名格式：`mcp__phoenix__{工具名}`

| Agent MD 中的工具名 | Phoenix MCP 工具 |
|---------------------|------------------|
| `mcp__phoenix__upload_document` | `upload_document` |
| `mcp__phoenix__extract_fields` | `extract_fields` |
| `mcp__phoenix__validate_document` | `validate_document` |
| `mcp__phoenix__save_database` | `save_database` |
| `mcp__phoenix__query_document` | `query_document` |

> 连接器的 `mcp.json` 中 `mcpServers` 的 key（`"phoenix"`）决定了工具名前缀。

---

## 8. 安装与验证流程

### 8.1 本地验证（开发环境）

**Step 1：创建连接器文件**

```bash
# 在 WorkBuddy 连接器市场目录下创建 phoenix 连接器
mkdir -p ~/.workbuddy/connectors-marketplace/connectors/phoenix/skills

# 创建文件（内容见 §4.2 ~ §4.4）
# - connector-meta.json
# - mcp.json
# - skills/SKILL.md
```

**Step 2：注册到 connectors.json**

编辑 `~/.workbuddy/connectors-marketplace/.codebuddy-connector/connectors.json`，
在 `connectors` 数组末尾添加 phoenix 条目（见 §4.5）。

**Step 3：重启 WorkBuddy**

关闭并重新打开 WorkBuddy，使其重新加载连接器市场。

**Step 4：验证连接器出现**

打开 WorkBuddy → 侧边栏「连接器」→ 搜索 "Phoenix" → 应能看到 Phoenix 连接器卡片。

**Step 5：本地联调（无 OAuth）**

本地开发环境 `PHX_OAUTH_MODE=off`，先验证基础连通性：

1. 修改 `mcp.json` 的 url 为 `http://localhost:8080/mcp`
2. 点击「连接」→ 应直接成功（无 401）
3. 在对话中测试：`帮我上传一份测试文档`

**Step 6：生产 OAuth 验证（核心）**

1. 恢复 `mcp.json` 的 url 为 `https://phoenix.matrix-net.tech/mcp`
2. 确保 Phoenix 生产环境 `PHX_OAUTH_MODE=required`
3. 点击「连接」→ **观察是否弹出 Keycloak 登录页**
4. 用测试账号（alice/bob）登录
5. 授权后回到 WorkBuddy → 连接状态应显示「已连接」
6. 在对话中测试：`帮我上传一份测试文档` → 检查 Phoenix 日志中 `uploaded_by` 是否正确

**Step 6 实测结果（2026-07-16）**：

| 环节 | 结果 | 说明 |
|------|------|------|
| 弹出 Keycloak 登录页 | ✅ 成功 | 401 → 发现 → DCR → 登录页全链路通 |
| 登录 + 授权同意 | ✅ 成功 | consent 页出现，点 Yes 后浏览器跳 `workbuddy://` |
| Token 交换 | ✅ 成功 | 日志 `[MCP OAuth] Result: success=true`（19 次成功 / 2 次失败） |
| Token 持久化 | ✅ 成功 | `.credentials.v3.json` 含 accessToken + refreshToken |
| **UI 状态 → 「已连接」** | ❌ **卡住** | 一直显示「授权中…」 |

> **「授权中…」卡住——已排除的原因**：
> - ❌ 不是 Redirect URI 不匹配（回调已成功跳到 app 并被处理）
> - ❌ 不是 token 交换失败（日志 `success=true`，token 已落盘）
> - ❌ 不是 Keycloak/Phoenix 服务端问题（冒烟客户端端到端通过）
>
> **已确认的根因**：WorkBuddy v5.2.6 的 MCP OAuth 回调处理器成功交换 token 并写入凭据文件，
> 但**未触发 UI 状态机从 "authorizing" 翻转到 "connected"**。`connector-states.v3.json` 的 `enabled`
> 数组中未加入 phoenix，说明状态未传播。
>
> **待尝试的解法**（按优先级）：
> 1. **重启 WorkBuddy**：让 MCP 客户端启动时读取已保存的 token，直接带 token 重连（跳过 OAuth 流程）
> 2. **断开重连**：在 MCP 设置页先断开 phoenix，再重新连接——此时 token 可能已缓存，直接走 token 验证路径
> 3. **MCP Inspector 隔离验证**：`npx @modelcontextprotocol/inspector` 连同一端点走完整 OAuth，
>    Inspector 成功 = 坐实 WorkBuddy 客户端 UI bug，带日志证据反馈 WorkBuddy 团队
> 4. **直接在对话中调用工具**：即使 UI 显示「授权中」，token 可能已可用——试试在对话中直接说
>    `帮我上传一份测试文档`，看是否能成功调用（token 在凭据文件中，MCP 客户端可能自动带上）

### 8.2 企业分发验证

验证通过后，将连接器分发到企业：

**方式 1：连接器市场发布（推荐）**

如果企业有 WorkBuddy 企业管理后台：
1. 将 `connectors/phoenix/` 目录打包为 zip
2. 上传到企业管理后台的连接器管理
3. 配置可见范围（所有成员 / 部分成员）
4. 同事在客户端连接器市场搜索安装

**方式 2：Git 仓库市场**

将连接器市场推到 Git 仓库：
```
team-connectors/
├── .codebuddy-connector/
│   └── connectors.json
└── connectors/
    └── phoenix/
        ├── connector-meta.json
        ├── mcp.json
        └── skills/SKILL.md
```
同事在 WorkBuddy 中添加 Git URL 订阅市场。

**方式 3：手动安装**

将 `phoenix/` 目录复制到同事的 `~/.workbuddy/connectors-marketplace/connectors/` 下，
并手动编辑 `connectors.json` 添加注册条目。

---

## 9. 已知风险与限制

### 9.1 风险

| 风险 | 级别 | 说明 | 应对 |
|------|------|------|------|
| ~~WorkBuddy MCP 客户端不支持 401→OAuth~~ | ~~高~~ | ✅ **已排除**：2026-07-16 实测全链路通过 | 无需应对 |
| WorkBuddy UI 状态刷新 bug | **高** | OAuth 回调 `success=true`、token 已落盘，但 UI 一直「授权中…」，`connector-states.v3.json` 未更新 | 重启 WorkBuddy / 断开重连 / 反馈 WorkBuddy 团队 |
| 连接器路径 vs 自定义 MCP 路径差异 | 中 | 实测走的是 `custom-mcp:phoenix`（.mcp.json），连接器系统路径（`connector:phoenix`）未单独验证 | 连接器路径验证后补充 |
| DCR 客户端堆积 | 低 | WorkBuddy 每次连接会重新 DCR | Keycloak 设置 DCR 客户端过期时间 |
| PKCE 未强制 | 低 | DCR 请求可能不声明 PKCE | Keycloak Client Policy 强制 PKCE |
| CORS 阻断 | 低 | 浏览器 OAuth 流程可能被 CORS 拦截 | Keycloak 和 Phoenix 配置 CORS |

### 9.2 官方文档支持情况

| 内容 | 官方文档 | 来源 |
|------|---------|------|
| 连接器系统概念 | ✅ 有 | WorkBuddy 文档 |
| `connector-meta.json` 格式 | ⚠️ 部分 | 本地连接器实例反推 |
| `mcp.json` 格式 | ✅ 有 | MCP 指南 + 本地实例 |
| 动态 OAuth 发现（401→PKCE） | ❌ 无公开文档 | ✅ **实测验证通过**（2026-07-16，WorkBuddy v5.2.6） |
| `auth_injection_rules` | ❌ 无公开文档 | 本地 connectors.json 反推 |
| 企业市场上架 | ✅ 有 | 企业管理-专家管理文档 |
| 回调 URL 格式 | ❌ 无公开文档 | ✅ **实测确认**：`workbuddy://workbuddy/mcp/custom-mcp:phoenix/oauth/callback` |
| Token 存储格式 | ❌ 无公开文档 | ✅ **实测确认**：`.credentials.v3.json` 的 `mcpOAuth` 字段，AES-256-GCM 加密 |

---

## 10. 文件清单速查

### 需要创建的文件

| 文件 | 路径 | 说明 |
|------|------|------|
| connector-meta.json | `connectors/phoenix/connector-meta.json` | 连接器元信息（§4.2） |
| mcp.json | `connectors/phoenix/mcp.json` | MCP 端点配置，无 headers（§4.3） |
| SKILL.md | `connectors/phoenix/skills/SKILL.md` | 连接后的 Skill 指引（§4.4） |
| 注册条目 | `connectors.json` 的 connectors 数组 | 市场注册（§4.5） |

### 需要从专家包移除的文件

| 文件 | 原因 |
|------|------|
| `phoenix-doc-expert/.mcp.json` | MCP 配置由连接器负责，避免冲突 |

---

## 附录 A：完整文件内容（可直接复制）

### connectors/phoenix/connector-meta.json

```json
{
  "id": "phoenix",
  "name": "Phoenix 文档处理",
  "name_zh": "Phoenix 文档处理",
  "name_en": "Phoenix Document Processing",
  "description": "Enterprise document processing via Phoenix MCP.",
  "description_zh": "通过 Phoenix MCP 平台处理企业文档，支持单据上传、字段提取、校验入库与历史文档查询全流程。",
  "description_en": "Enterprise document processing via Phoenix MCP.",
  "source": "phoenix",
  "type": "mcp",
  "version": "1.0.0",
  "minWorkbuddyVersion": "4.24.0",
  "examples_zh": [
    "帮我上传这份发票并提取字段",
    "查一下上周所有待审核的单据",
    "把这份合作确认单录入系统"
  ],
  "examples_en": [
    "Upload this invoice and extract fields",
    "Query all pending documents from last week",
    "Enter this confirmation letter into the system"
  ]
}
```

### connectors/phoenix/mcp.json

```json
{
  "mcpServers": {
    "phoenix": {
      "type": "streamable-http",
      "url": "https://phoenix.matrix-net.tech/mcp",
      "disabled": false
    }
  }
}
```

### connectors/phoenix/skills/SKILL.md

```markdown
---
name: phoenix-doc-skill
description: Phoenix 企业智能文档处理平台 MCP 能力
version: "1.0.0"
---

# Phoenix 文档处理 Skill

本 Skill 在 Phoenix 连接器激活后自动加载，提供文档处理全流程能力。

## 可用工具

### upload_document — 上传文档
参数：doc_type（可选）、content_text / content_base64 / file_url（三选一）

### extract_fields — 字段提取
参数：document_id

### validate_document — 文档校验
参数：document_id

### save_database — 入库
参数：document_id、fields（可选，人工修正值）、force（可选）

### query_document — 查询历史文档
参数：doc_type / status / keyword / uploaded_by / limit（均可选）

## 使用规范
- 严格遵循 upload → extract → validate → save 顺序
- 不编造字段值，提取不到如实告知
- 金额日期保持原始写法不做换算
- needs_review 状态必须等用户确认后再入库
```

### connectors.json 注册条目（追加到 connectors 数组）

```json
{
  "id": "phoenix",
  "name": "Phoenix 文档处理",
  "name_en": "Phoenix Document Processing",
  "description": "Enterprise document processing via Phoenix MCP.",
  "description_zh": "通过 Phoenix MCP 平台处理企业文档，支持上传、提取、校验、查询全流程。",
  "description_en": "Enterprise document processing via Phoenix MCP.",
  "source": "phoenix",
  "type": "mcp",
  "examples_zh": [
    "帮我上传这份发票并提取字段",
    "查一下上周所有待审核的单据"
  ],
  "examples_en": [
    "Upload this invoice and extract fields",
    "Query all pending documents from last week"
  ]
}
```

---

## 附录 B：验证检查清单

- [x] Phoenix 生产环境 `PHX_OAUTH_MODE=required` 已启用
- [x] `/.well-known/oauth-protected-resource` 端点可访问，返回正确的 AS issuer URL
- [x] Keycloak DCR 已启用，允许匿名注册
- [ ] Keycloak Client Policy 强制 PKCE（可选，实测不配也能跑通）
- [x] Keycloak Redirect URI 已配置（`workbuddy://*` 通配 + DCR 自带精确匹配）
- [x] 连接器/MCP 文件已创建（.mcp.json 在专家包根目录，实测可用）
- [x] WorkBuddy 重启后 MCP 设置能看到 phoenix
- [x] 点击「连接」后弹出 Keycloak 登录页
- [x] 登录 + 授权同意通过，回调 `success=true`，token 已落盘
- [ ] **连接状态变为「已连接」** ← ❌ 卡在这里（§2.4）
- [ ] 对话中调用工具成功，Phoenix 日志中 uploaded_by 正确 ← 待 UI 状态修复后验证
