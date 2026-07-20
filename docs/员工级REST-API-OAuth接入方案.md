# 员工级 REST API(/pub/v1)+ Keycloak Device Flow 接入方案

> 面向新专家 **phoenix-doc-assistant**(WorkBuddy 内置 Python 脚本直连后端 REST,不走 MCP 连接器)。
> 目标:**每个操作都能追溯到具体员工**——每次请求携带员工本人经 Keycloak 登录得到的 token,
> 后端校验后落 `documents.uploaded_by/reviewed_by` 与 `audit_log`。
>
> 与既有的 MCP 端点(`/mcp` + `internal/mcpauth`,老专家 phoenix-doc-expert 用)**完全独立、互不影响**;
> 与管理后台的 `X-Access-Key`(`/api/*`)也各走各的。本方案只新增,不改动上述任何一条链路。

---

## 一、整体链路

```
员工浏览器 ──(Device Flow 批准一次)──> Keycloak(realm=phoenix, client=phoenix-cli)
   │                                          │ 签发 access_token(aud=phoenix-api, 带 sub/email)
   ▼                                          ▼
WorkBuddy 专家 ── Python 脚本(Bearer token)──> workflow /pub/v1/*  ──> pipeline ──> PG / MinIO
                                               (internal/restapi 校验 token → 取身份 → 落审计)
```

- **协议**:标准 HTTP + JSON,`Authorization: Bearer <access_token>`。端点在 `/pub/v1/*`。
- **登录**:OAuth 2.1 **Device Authorization Grant (RFC 8628)**——脚本无需内嵌浏览器/回调服务,
  员工在任意浏览器批准一次,之后 refresh_token 自动续期。
- **身份来源**:后端只信 token 里校验出来的身份,**不采信任何客户端请求头**(公网面防伪造)。

## 二、后端(已实现)

- 新包 `backend/internal/restapi`:OAuth 资源服务器 + `/pub/v1` 路由,业务全部复用 `pipeline`。
- 装配在 `cmd/workflow/main.go`:仅当配置了 issuer 才挂载 `/pub/v1`(否则老部署零变化)。
- 端点(全部要求有效 Bearer,**不含删除**——删除仍走管理后台):

  | 方法 | 路径 | 说明 |
  |---|---|---|
  | GET | `/pub/v1/me` | 当前 token 对应员工身份(客户端确认登录) |
  | POST | `/pub/v1/documents` | 上传归档(content_text/content_base64/file_url 三选一,可带 filename/doc_type) |
  | POST | `/pub/v1/documents/{id}/extract` | 返回该类型字段清单 FieldBrief(catalog 或 fields) |
  | POST | `/pub/v1/documents/{id}/validate` | 预校验(不入库) |
  | POST | `/pub/v1/documents/{id}/save` | 落字段+正文入库(权威校验) |
  | GET | `/pub/v1/documents` | 结构化查询(doc_type/status/keyword/uploaded_by/field_filters/limit) |
  | POST | `/pub/v1/ask` | 知识库语义问答 |

  > `fields` 一律是数组 `[{"name","value"}]`(与 `/api` 一致);客户端脚本把便于书写的对象自动转数组。

### 后端环境变量(新增,加到 workflow)

```bash
PHX_API_OIDC_ISSUER=https://phoenix.matrix-net.tech/auth/realms/phoenix   # 空 = /pub/v1 不启用
PHX_API_OIDC_AUDIENCE=phoenix-api                                          # 默认 phoenix-api
# PHX_API_OIDC_DISCOVERY_URL=http://keycloak:8080/realms/phoenix          # 仅当容器内网取 JWKS 的地址与 issuer 不同才设
```

## 三、Keycloak 配置(生产照抄)

在 **phoenix** realm 里**新增**一个客户端 `phoenix-cli`(不要动 workbuddy / phoenix-smoke 这两个 MCP 客户端)。

### 方式 A：管理控制台

1. Clients → Create client
   - Client type: **OpenID Connect**;Client ID: `phoenix-cli`
2. Capability config:
   - **Client authentication: Off**(公共客户端)
   - Authorization: Off
   - Authentication flow:**只勾 Standard flow**(浏览器登录 = 授权码);
     **Direct access grants / Device / Service accounts 都不勾**(生产别开密码授权)
3. Access settings(授权码回调):
   - **Valid redirect URIs**:`http://127.0.0.1:47100/callback` 和 `http://localhost:47100/callback`
     (端口须与客户端 `.config.json` 的 `redirect_port` 一致;loopback 回调走 http 是 RFC 8252 允许的)
   - **Web origins**:`http://127.0.0.1:47100`、`http://localhost:47100`
   - **Proof Key for Code Exchange (PKCE)**:S256
4. Save 后 → Clients → phoenix-cli → **Client scopes** → `phoenix-cli-dedicated` → Add mapper → By configuration → **Audience**
   - Name: `phoenix-api-audience`;Included Custom Audience: `phoenix-api`;Add to access token: **On**
5.(可选)确认员工账号能登录 realm(生产一般对接企业 OA/LDAP)。

### 方式 B：kcadm 命令行

```bash
# 进入 keycloak 容器(或本地 kcadm),先登录 admin
kcadm.sh config credentials --server http://localhost:8080 --realm master --user admin --password "$KC_ADMIN_PW"

# 新增公共客户端:开 standard flow(授权码),注册 loopback 回调,S256
CID=$(kcadm.sh create clients -r phoenix -s clientId=phoenix-cli -s publicClient=true \
  -s standardFlowEnabled=true -s directAccessGrantsEnabled=false -s serviceAccountsEnabled=false \
  -s 'redirectUris=["http://127.0.0.1:47100/callback","http://localhost:47100/callback"]' \
  -s 'webOrigins=["http://127.0.0.1:47100","http://localhost:47100"]' \
  -s 'attributes."pkce.code.challenge.method"=S256' -i)

# 加 audience mapper → phoenix-api
kcadm.sh create clients/$CID/protocol-mappers/models -r phoenix \
  -s name=phoenix-api-audience -s protocol=openid-connect -s protocolMapper=oidc-audience-mapper \
  -s 'config."included.custom.audience"=phoenix-api' -s 'config."access.token.claim"=true' \
  -s 'config."id.token.claim"=false'
```

> **登录只有一种**:浏览器登录(授权码+PKCE,`auth.py --login`,需 Standard flow + 上面的 redirect URI)。
> **开发联调**:`deploy/keycloak/phoenix-realm.json` 已内置 `phoenix-cli`(`make oauth-up` 自动导入)。
> 它额外开了 `directAccessGrantsEnabled` 仅为非交互冒烟取 token 验 aud;**生产不要开 direct access grant**。

### 验证 token 的 audience 正确

```bash
# 生产用 device flow;这里用 dev 的密码授权快速验 aud
curl -s -X POST $ISSUER/protocol/openid-connect/token \
  -d grant_type=password -d client_id=phoenix-cli -d username=alice -d password=alice123 \
  -d scope='openid profile email' | jq -r .access_token | cut -d. -f2 | base64 -d | jq .aud
# 期望输出包含 "phoenix-api"
```

## 四、部署(Traefik 路由,生产)

让 `/pub/v1` 直达 workflow 服务(:8081)。给 prod compose 的 workflow 服务加一条 Traefik 路由(**新增,不动 /mcp 与 /api 现有路由**):

```yaml
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.phx-pub.rule=Host(`phoenix.matrix-net.tech`) && PathPrefix(`/pub/v1`)"
  - "traefik.http.routers.phx-pub.entrypoints=websecure"
  - "traefik.http.routers.phx-pub.tls.certresolver=le"
  - "traefik.http.services.phx-pub.loadbalancer.server.port=8081"
```

并在 `/opt/phoenix/.env` 补上 `PHX_API_OIDC_ISSUER` / `PHX_API_OIDC_AUDIENCE`(见 §二)。

## 五、客户端(专家包)配置

端点三要素是公司级常量,建议 IT 预置进 `phoenix-doc-assistant/skills/phoenix-api/templates/config.template.json`:

```json
{
  "api_base_url": "https://phoenix.matrix-net.tech",
  "oidc_issuer": "https://phoenix.matrix-net.tech/auth/realms/phoenix",
  "client_id": "phoenix-cli",
  "scope": "openid profile email",
  "timeout": 60,
  "verify_ssl": true,
  "tokens": {}
}
```

员工首次使用只需登录:专家会自动跑 `auth.py --login-start`(给出验证地址+验证码)→ 员工浏览器批准 →
`auth.py --login-poll`(拿到 token)。之后自动续期。

## 六、安全要点

- **每员工身份**:token 带 `sub`/`preferred_username`/`email`,后端落 `uploaded_by`/`reviewed_by` 与 `audit_log.detail`。
- **面隔离**:`/pub/v1` 只认 `aud=phoenix-api` 的 token;MCP 的 `aud=phoenix-mcp` token 会被拒(反之亦然)。
  管理后台的 `X-Access-Key` 与二者都不通用。
- **不给删除**:`/pub/v1` 不暴露删除/覆盖;破坏性操作走管理后台人工。
- **`.config.json` 权限 0600**;里面存的是员工个人 token(会过期),不是共享长期密钥。
- 生产 `phoenix-cli` 关闭 direct access grant(只留 device flow),避免密码授权这种已被 OAuth 2.1 淘汰的方式。

## 七、本地端到端验证(已跑通)

`make infra-up && make oauth-up`,workflow 带 `PHX_API_OIDC_ISSUER=http://localhost:8180/realms/phoenix` 启动后:

- 无 token → `/pub/v1/me` 返回 401 `AUTH_FAILED`;alice token → 返回 alice 身份。
- 客户端脚本 upload → extract_fields → save → query 全链路通过,`uploaded_by`/`reviewed_by` = alice。
- `audit_log`:`actor=alice, actor_source=oauth`,`detail` 含 sub/email。
- `auth.py --login-start` 能从 Keycloak 拿到 user_code(device flow 就绪)。
- `aud=phoenix-mcp` 的 token 访问 `/pub/v1` 被拒(面隔离生效);老 `/api`(X-Access-Key)不受影响。
