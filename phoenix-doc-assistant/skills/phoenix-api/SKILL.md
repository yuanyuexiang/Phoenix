---
name: phoenix-api
description: Built-in Python REST client for the document backend (/pub/v1), authenticated per-employee with platform-issued JWT (upload/extract/validate/save/query/ask)
---

# Phoenix API Client

本 skill 的 Python 脚本直连后端 `/pub/v1/*` REST 端点,鉴权是**每员工身份**:
员工用本人账号口令登录(`/pub/v1/auth/login`),平台签发 Bearer token(自动续期),
后端据此把操作记到具体员工名下。账号由管理员在 Phoenix 管理后台「员工」页维护。
不走 MCP 协议,不依赖 Keycloak 等第三方身份组件(V1.3 起)。

## 架构

```
模型 → Bash 执行 commands/xxx.py → api_client.py(带 Bearer token)→ 后端 /pub/v1 → JSON → 模型作答
                                        ↑ token 由 auth.py 管理(账号登录 + 自动续期)
```

## Scripts

| 脚本 | 作用 | 对应端点 |
|------|------|---------|
| `scripts/config.py` | 配置文件读写 / 脱敏展示（`--show`/`--endpoint-check`/`--logout`） | - |
| `scripts/auth.py` | **登录与 token**（`--check` / `--login`(弹浏览器;`--password` 终端后备) / `--whoami` / `--logout`） | /pub/v1/auth/authorize、/auth/token、/auth/login、/auth/refresh |
| `scripts/api_client.py` | REST HTTP 客户端封装（各命令 import,自动带 Bearer） | - |
| `scripts/setup.py` | 端点配置向导（手动终端用） | - |
| `scripts/commands/upload.py` | 上传文档归档 | POST /pub/v1/documents |
| `scripts/commands/extract_fields.py` | 取字段清单 | POST /pub/v1/documents/{id}/extract |
| `scripts/commands/validate.py` | 预校验 | POST /pub/v1/documents/{id}/validate |
| `scripts/commands/save.py` | 入库 | POST /pub/v1/documents/{id}/save |
| `scripts/commands/query.py` | 结构化查询 | GET /pub/v1/documents |
| `scripts/commands/ask.py` | 语义问答(命中片段附实体摘要) | POST /pub/v1/ask |
| `scripts/commands/objects.py` | **实体查询**(跨文档归一的对象/关系/证据;类型速查见 references/ontology-objects.md) | GET /pub/v1/objects、/objects/{id} |

## 配置文件位置

`scripts/.config.json`（已加入 .gitignore,权限 0600）:

```json
{
  "api_base_url": "https://phoenix.matrix-net.tech",
  "timeout": 60,
  "verify_ssl": true,
  "tokens": {}
}
```

端点(api_base_url)是公司级常量,通常由 IT 预置;
`tokens` 由 `auth.py` 登录后写入,里面是员工个人 token(会过期、自动续期),不是共享 key。

## 鉴权方式

- 登录:**弹浏览器登录**(默认)——`auth.py --login` 打开平台自己的登录页
  (`GET /pub/v1/auth/authorize`,授权码 + PKCE),员工在页面输入账号口令,
  成功页提示返回 WorkBuddy(浏览器允许时自动关闭);口令不经过终端与对话。
  无浏览器环境的后备:`--login --password` 终端交互输入(getpass 不回显)。
- 请求:`api_client.py` 每次自动取一个有效 access_token(过期用 refresh_token 续期),带
  `Authorization: Bearer <token>`。未登录 → 输出 `{"error":"NEEDS_LOGIN"}`。
- 管理员改密/禁用账号后旧 token 立即失效,客户端会提示重新登录。

## 查询路由(三选一)

单据检索 → query.py;**实体/关系/聚合 → objects.py**;开放式正文理解 → ask.py。
对象的合并/修正不由脚本执行(引导管理后台)。

## 新增业务命令

1. 在 `scripts/commands/` 下新建 `xxx.py`
2. `from api_client import ApiClient`(必要时 `to_field_array`)
3. argparse 接收参数
4. `client = ApiClient(); client.post('/pub/v1/...', data={...})` 或 `client.get('/pub/v1/...', params={...})`,JSON 输出到 stdout
5. 在 Agent MD 里补充调用规范
