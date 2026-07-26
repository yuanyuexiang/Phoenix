# 员工级 REST API(/pub/v1)鉴权方案 —— 自研账号 + JWT

> V1.3 现行方案。取代此前的 Keycloak OAuth 方案(历史见 `docs/archive/员工级REST-API-OAuth接入方案.md`);
> 替换动机:去掉需要专门学习运维的第三方身份组件,整条鉴权链路都在自己代码里。
> 目标不变:**每个操作都能追溯到具体员工** —— 身份落 `documents.uploaded_by/reviewed_by` 与 `audit_log`。

## 总览

```
员工(WorkBuddy 专家) ── 账号口令 ──▶ POST /pub/v1/auth/login ──▶ access(1h) + refresh(30d)
        │                                                            │
        └── 之后每请求带 Authorization: Bearer <access> ◀── 过期自动 POST /auth/refresh 续期
管理员(管理后台「员工」页 /api/users) ──▶ 建号 / 重置口令 / 禁用(旧 token 立即失效)
```

- **账号**:`users` 表(`store/migrations/0004_users.sql`),口令 PBKDF2-SHA256 哈希存储,
  管理端点 `/api/users`(X-Access-Key,前端「员工」页)。
- **token**:平台自签 HS256 JWT(`internal/userauth`,零第三方依赖)。
  claims 含 `sub`(username)、`typ`(access|refresh)、`ver`(users.token_version)。
- **撤销**:改密/禁用时 `token_version+1`;每次请求校验 token 内 `ver` 与库中一致,
  不一致即 401 —— 无需维护黑名单。
- **开关**:`PHX_AUTH_SECRET` 非空才挂载 `/pub/v1`(空 = 关闭,老部署零影响)。
  生产用强随机长串(`openssl rand -hex 32`);更换密钥会使全部登录失效。

## 端点

| 端点 | 鉴权 | 说明 |
|------|------|------|
| `POST /pub/v1/auth/login` | 开放 | `{username,password}` → `{access_token,refresh_token,expires_in,user}`;失败统一 401 不区分原因(防账号枚举);登录写 audit_log |
| `POST /pub/v1/auth/refresh` | 开放 | `{refresh_token}` → 新的一对 token(轮换) |
| 其余 `/pub/v1/*` | Bearer access token | 校验签名/有效期/typ + 对库核账号状态与 `ver` |

## 客户端(phoenix-doc-assistant)

- `auth.py --login`:交互式输用户名 + getpass 输口令(不回显、不进 shell 历史),
  token 存 `.config.json`(0600);`--check` / `--whoami` / `--logout`。
- `api_client.py` 每次请求自动取有效 access token,过期用 refresh 续期;
  续期也失败 → 输出 `NEEDS_LOGIN` 引导重登。
- 配置只剩 `api_base_url` 一项(不再有 issuer/client_id/回调端口)。

## 联调与冒烟

```bash
make infra-up
PHX_AUTH_SECRET=dev-secret make run-workflow
make smoke-auth     # 自动:建号(经 /api/users)→ 负向验证 → 登录 → 全流程 → 断言 uploaded_by
```

## 安全边界(与实施取舍)

- 口令哈希 PBKDF2-SHA256 12 万轮 + 随机盐,恒定时间比较;登录失败信息不区分原因。
- access token 短期(1h)限制泄露窗口;refresh 长期(30d)但受 `ver` 撤销约束。
- 员工侧不暴露删除等破坏性操作(与 OAuth 版一致)。
- 未做(接受的取舍):登录限速/锁定、密码复杂度策略、自助改密 —— 内部受控环境暂缓,
  需要时在 login handler 与「员工」页上加。
- 将来若客户要求接企业 SSO:`/pub/v1` 业务层不动,把鉴权中间件换回 OIDC 验证器即可
  (git 历史里有完整实现)。
