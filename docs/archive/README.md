# 归档文档(已废止方案)

本目录存放**已废止**的历史方案文档,仅作回溯参考,不再随产品演进维护。

## MCP 连接器方案(V1.3 下线,2026-07-22)

平台曾以 MCP Server(连接器)+「phoenix-doc-expert」专家作为 WorkBuddy 接入形态。
V1.3 起整体下线,对外接入改为**员工级 REST API(/pub/v1,OAuth 2.1/Keycloak)**
+ 专家包 `phoenix-doc-assistant`(见 `docs/员工级REST-API-OAuth接入方案.md`
与说明书 V1.3 修订记录)。下线原因:WorkBuddy 连接器 OAuth 登录体验存在阻塞性
问题(UI 状态机 bug 需本地代理绕过),且 REST + Device Flow 方案已覆盖同等能力
并原生支持每员工身份。

| 文件 | 原用途 |
|------|--------|
| `WorkBuddy接入指南.md` | WorkBuddy 侧添加 phoenix MCP 连接器与专家的操作指南 |
| `WorkBuddy连接器接入方案.md` | 把 Phoenix MCP 注册为 WorkBuddy 官方连接器的实现方案与 OAuth 实测记录 |
| `MCP-OAuth鉴权方案.md` | MCP 端点 OAuth 2.1 资源服务器方案(internal/mcpauth,代码已删除) |
| `文档处理专家_发布包.md` | 老专家「phoenix-doc-expert」的 WorkBuddy 发布物料 |
| `phoenix-doc-expert-v2-design.md` | 老专家 v2 设计稿(未实施) |

## Keycloak 员工鉴权方案(V1.3 下线,2026-07-22)

`/pub/v1` 曾以 Keycloak 作为 OAuth 2.1 授权服务器(Auth Code + PKCE)。因团队不熟悉
Keycloak 运维,改为**自研账号体系 + 平台签发 JWT**(见 `docs/员工级REST-API-鉴权方案.md`),
Keycloak 容器、realm 配置与 `/auth` 路由随之删除。

| 文件 | 原用途 |
|------|--------|
| `员工级REST-API-OAuth接入方案.md` | /pub/v1 + Keycloak(Device Flow/PKCE)员工级鉴权方案(代码 restapi/oidc.go 已删除) |

对应代码(`backend/cmd/mcp`、`internal/mcpauth`、`internal/mcpserver`、
`internal/clients`、`phoenix-doc-expert/`)已于同日删除,如需查阅请检出
V1.3 之前的 git 历史(commit 68b0aae 及更早)。
