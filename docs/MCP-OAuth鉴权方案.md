# Phoenix MCP 端点 OAuth 2.1 鉴权与用户识别方案

> 状态:**生产已启用(测试阶段,`required` 模式)**  |  版本 V1.2  |  日期 2026-07-15
> AS = 自建 Keycloak(§3 选项 B),与平台同机 compose 部署,经 Traefik 以同域
> `/auth` 子路径对外(issuer `https://phoenix.matrix-net.tech/auth/realms/phoenix`,
> 复用平台 Postgres 的 keycloak 库)。端到端已实测:401 → 发现 → 取 token →
> 调工具 → `uploaded_by` 落库。仍待:WorkBuddy 真机验证(§5)、AS 终局选型
> (客户 IdP 联邦 or 沿用 Keycloak)与员工账号口径(§8)。
>
> 平台侧实现说明(§4 已全部落地):
> - 鉴权开关为三档 `PHX_OAUTH_MODE=off|optional|required`(默认 off,零行为变化;
>   optional 即 §7 灰度模式:有 token 记身份、无 token 放行);
> - 配套配置:`PHX_OAUTH_ISSUER`(期望 iss)、`PHX_OAUTH_AUDIENCE`(默认 `phoenix-mcp`)、
>   `PHX_OAUTH_RESOURCE`(资源标识,生产 `https://phoenix.matrix-net.tech/mcp`)、
>   `PHX_OAUTH_DISCOVERY_URL`(容器内取 JWKS 的内网地址,与 iss 不同时才需设)、
>   `PHX_OAUTH_SCOPES`(必需 scope,空=不检查);
> - 开发联调:`make oauth-up` 起 Keycloak(localhost:8180,测试用户 alice/bob),
>   `make smoke-oauth` 跑带 token 的端到端冒烟(含无 token 401 负向断言与身份落库断言)。
> 解决两个问题:① 企业多员工经 WorkBuddy 使用「文档处理专家」时,平台识别到**人**;
> ② MCP 端点(`https://phoenix.matrix-net.tech/mcp`)当前无鉴权,补上标准防护。
> 本方案采用 **MCP 官方授权规范**(OAuth 2.1),是协议标准做法,同类项目可复用。

---

## 1. 标准依据:MCP 授权规范要点

MCP 规范(2025-06-18 起)在 HTTP 传输层定义了标准授权机制,角色划分:

| 角色 | 承担者 | 职责 |
|------|--------|------|
| 资源服务器(RS) | **Phoenix mcp 服务** | 校验每个请求的 Bearer token,拒绝无效请求 |
| 授权服务器(AS) | 企业 IdP 或自建(见 §3) | 认证员工身份,签发 access token |
| 客户端 | **WorkBuddy** | 引导员工完成授权,持 token 调用工具 |

规范强制要求(MUST):

- 客户端使用 **OAuth 2.1 + PKCE(SHA-256)**,全程 HTTPS
- 资源服务器发布 **RFC 9728 受保护资源元数据**(`/.well-known/oauth-protected-resource`),
  客户端由此自动发现授权服务器,无需手工配置
- 客户端在授权与换 token 请求中携带 **RFC 8707 resource 参数**,声明 token 的目标资源
- 资源服务器**必须校验 token 的 audience** 是自己,防止 token 被挪用到其他服务

一句话:**员工首次使用专家时跳一次企业登录页,之后 WorkBuddy 自动带着"属于这个员工、
只对 Phoenix 有效"的 token 调工具;平台从 token 里拿到可信的用户身份。**

## 2. 端到端流程

```
员工                WorkBuddy(客户端)          Phoenix mcp(资源服务器)      授权服务器(AS)
 │  @文档处理专家        │                             │                        │
 │──────────────────────▶│  调用工具(无 token)         │                        │
 │                        │────────────────────────────▶│                        │
 │                        │◀── 401 + 资源元数据地址 ────│                        │
 │                        │  发现 AS、注册客户端、发起 PKCE 授权 ────────────────▶│
 │◀── 跳转企业登录页(仅首次)─────────────────────────────────────────────────│
 │── 登录/授权 ──────────────────────────────────────────────────────────────▶│
 │                        │◀──────────── access token(含用户身份、aud=Phoenix)─│
 │                        │  携 token 重新调用工具 ─────▶│ 验签/验 aud/取身份     │
 │                        │◀── 工具结果 ────────────────│                        │
```

首次之后 token 静默续期,员工无感。

## 3. 授权服务器(AS)选型 —— 需客户拍板

| 选项 | 说明 | 优 | 劣 |
|------|------|----|----|
| **A. 客户已有企业 IdP** | 钉钉/企业微信/AD FS/Azure AD 等,若支持标准 OIDC | 员工账号即登录账号,零新增系统 | 是否完整支持 PKCE/元数据发现需逐一验证 |
| **B. 自建 Keycloak(推荐兜底)** | 开源 IdP,与平台同机 compose 部署 | 完全可控;可与选项 A 做身份联邦(员工仍用企业账号登录) | 多维护一个组件;RFC 8707 需 26.5+ 版本(旧版用 Audience Mapper 变通,已验证可行) |
| **C. WorkBuddy 平台自带 IdP** | 若 WorkBuddy 有用户体系并能当 AS | 与 WorkBuddy 账号天然一致 | 完全依赖 WorkBuddy 能力,待确认 |

**建议**:优先确认 A/C;都不具备则上 B(Keycloak + 企业账号联邦),不阻塞项目。

## 4. Phoenix 侧改造点(工作量:中,约 3~5 人日)

平台使用的 MCP 官方 Go SDK **已内置资源服务器全套组件**,无需自研协议:

| 改造 | 位置 | 说明 |
|------|------|------|
| Bearer token 校验中间件 | `backend/cmd/mcp` | SDK `auth.RequireBearerToken` + JWKS 验签(iss/exp/aud/scope) |
| 资源元数据端点 | 同上 | SDK `auth.ProtectedResourceMetadataHandler`,指向所选 AS |
| 身份透传 | mcp → workflow | 从 `TokenInfo` 取用户(sub/姓名),经内部请求头传给 workflow |
| 身份落库 | `documents` 表 | 新增 `uploaded_by` / `reviewed_by`;审计日志表 |
| 按人查询/统计 | workflow + 管理后台 | `query_document` 支持按人过滤;后台列表显示操作人 |
| 配置项 | `.env` | `PHX_OAUTH_ISSUER` / `PHX_OAUTH_AUDIENCE`,置空=不启用(向后兼容) |

若选 Keycloak:compose 增加一个容器 + 领域/客户端初始化脚本,约 +1~2 人日。

## 5. 对 WorkBuddy 的硬性要求(客户确认核心)

WorkBuddy 作为 MCP 客户端,必须满足以下四项,方案才能成立:

- [ ] 支持 MCP 授权规范的 **401 → 元数据发现 → OAuth 2.1 授权码 + PKCE** 流程
- [ ] 支持 **RFC 8707 resource 参数**(2025-06-18 及以后规范的客户端要求)
- [ ] 能为**每个员工**独立完成授权并保存各自 token(而非组织级共享一个)
- [ ] 支持 token 过期后的静默刷新

> 主流 MCP 客户端(Claude、Cursor 等)均已实现该流程;WorkBuddy 若基于标准 MCP SDK
> 开发,大概率原生支持——请 WorkBuddy 团队按上表逐项书面确认。

## 6. 风险与降级路径

| 风险 | 应对 |
|------|------|
| WorkBuddy 不支持 OAuth 流程 | 降级到「平台签发每员工 token」方案(Authorization 头静态填入,识别能力等价、少了 SSO 体验),平台侧改造 80% 可复用 |
| 客户 IdP 不支持 PKCE/元数据发现 | 上 Keycloak 做中间层,与 IdP 联邦 |
| 首次授权跳转体验被质疑 | 仅首次一跳,与钉钉/企微内免登类似;可演示后再定 |
| DCR(动态客户端注册)管理问题 | 可关闭 DCR,改为在 AS 预注册 WorkBuddy 为固定客户端 |

## 7. 实施计划(客户确认后启动)

1. **联调验证(1 周)**:WorkBuddy 按 §5 确认能力;搭 Keycloak 测试环境,
   用 MCP Inspector 先跑通标准流程,再接 WorkBuddy 真机
2. **平台改造(1 周)**:§4 清单全部落地,冒烟客户端增加带 token 用例,CI 覆盖
3. **灰度**:生产开启 `PHX_OAUTH_ISSUER`,先允许"有 token 记身份、无 token 放行"的
   过渡模式观察一周,再切强制
4. **收尾**:管理后台上线操作人展示与审计查询;更新专家发布包与接入指南

## 8. 待客户确认问题清单

1. 企业现有 IdP 是什么?是否支持标准 OIDC(授权码 + PKCE)?
2. WorkBuddy 对 §5 四项能力的书面确认
3. AS 选型(§3 的 A/B/C)
4. 员工身份的落库口径:工号 / 邮箱 / IdP sub?
5. 是否需要在此基础上做**权限分级**(如:普通员工只能查自己上传的文档)——影响
   `query_document` 默认过滤策略

## 参考资料

- [MCP Authorization 规范(官方)](https://modelcontextprotocol.io/specification/draft/basic/authorization)
- [Descope:Diving Into the MCP Authorization Specification](https://www.descope.com/blog/post/mcp-auth-spec)
- [Go + Keycloak 保护 MCP 服务器实践](https://medium.com/@wadahiro/protecting-mcp-server-with-oauth-2-1-a-practical-guide-using-go-and-keycloak-7544eb5379d3)
- [MCP OAuth 2.1 与 PKCE 综述(Aembit)](https://aembit.io/blog/mcp-oauth-2-1-pkce-and-the-future-of-ai-authorization/)
- 官方 Go SDK `auth` 包(本仓库依赖 v1.6.1,已含 RS 组件)
