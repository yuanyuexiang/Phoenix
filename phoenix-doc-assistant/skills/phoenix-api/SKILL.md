---
name: phoenix-api
description: Built-in Python REST client for the document backend (/pub/v1), authenticated per-employee via Keycloak OAuth Device Flow (upload/extract/validate/save/query/ask)
---

# Phoenix API Client

本 skill 的 Python 脚本直连后端 `/pub/v1/*` REST 端点,鉴权是**每员工身份**:
每次请求携带员工本人经 Keycloak 登录得到的 Bearer token(Device Flow,自动续期),
后端据此把操作记到具体员工名下。不走 MCP 协议,不依赖 WorkBuddy 连接器。

## 架构

```
模型 → Bash 执行 commands/xxx.py → api_client.py(带 Bearer token)→ 后端 /pub/v1 → JSON → 模型作答
                                        ↑ token 由 auth.py 管理(Device Flow 登录 + 刷新)
```

## Scripts

| 脚本 | 作用 | 对应端点 |
|------|------|---------|
| `scripts/config.py` | 配置文件读写 / 脱敏展示（`--show`/`--endpoint-check`/`--logout`） | - |
| `scripts/auth.py` | **登录与 token**（`--check` / `--login`(弹浏览器登录) / `--whoami` / `--logout`） | Keycloak |
| `scripts/api_client.py` | REST HTTP 客户端封装（各命令 import,自动带 Bearer） | - |
| `scripts/setup.py` | 端点配置向导（手动终端用） | - |
| `scripts/commands/upload.py` | 上传文档归档 | POST /pub/v1/documents |
| `scripts/commands/extract_fields.py` | 取字段清单 | POST /pub/v1/documents/{id}/extract |
| `scripts/commands/validate.py` | 预校验 | POST /pub/v1/documents/{id}/validate |
| `scripts/commands/save.py` | 入库 | POST /pub/v1/documents/{id}/save |
| `scripts/commands/query.py` | 结构化查询 | GET /pub/v1/documents |
| `scripts/commands/ask.py` | 语义问答 | POST /pub/v1/ask |

## 配置文件位置

`scripts/.config.json`（已加入 .gitignore,权限 0600）:

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

端点三要素(api_base_url / oidc_issuer / client_id)是公司级常量,通常由 IT 预置;
`tokens` 由 `auth.py` 登录后写入,里面是员工个人 token(会过期、自动刷新),不是共享 key。

## 鉴权方式

- 登录:**弹浏览器登录**(Authorization Code + PKCE)——`auth.py --login` 弹出 Keycloak 登录页,
  员工在页面上输账号密码 → 浏览器跳回本机 loopback(`redirect_port`,默认 47100)拿 token。
  用户始终在 **Keycloak 自己的页面**输密码(脚本不碰密码)。
- 请求:`api_client.py` 每次自动取一个有效 access_token(过期用 refresh_token 续期),带
  `Authorization: Bearer <token>`。未登录 → 输出 `{"error":"NEEDS_LOGIN"}`。

## 新增业务命令

1. 在 `scripts/commands/` 下新建 `xxx.py`
2. `from api_client import ApiClient`(必要时 `to_field_array`)
3. argparse 接收参数
4. `client = ApiClient(); client.post('/pub/v1/...', data={...})` 或 `client.get('/pub/v1/...', params={...})`,JSON 输出到 stdout
5. 在 Agent MD 里补充调用规范
