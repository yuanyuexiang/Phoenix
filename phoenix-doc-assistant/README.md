# Phoenix 文档助手（phoenix-doc-assistant）

企业智能文档处理专家包。内置 Python 脚本直连后端 REST(`/pub/v1`),**每个操作追溯到具体员工**:
每次请求携带员工本人经 Keycloak 登录得到的 token(OAuth 2.1 Device Flow,自动续期)。
不走 MCP 连接器。与老专家 `phoenix-doc-expert`(MCP)可并存。

## 核心能力

- **上传归档**：上传文档原件到后端
- **智能识别**：模型直接从图片/PDF/文本识别字段并转写正文
- **校验入库**：后端规则校验,按 status 分流(saved / needs_review)
- **结构化查询**：按类型/状态/关键词/字段值精确筛选
- **语义问答**：对已归档文档正文做开放式问答

## 鉴权:每员工身份(Keycloak Device Flow)

- 员工首次使用登录一次:`auth.py --login-start`(给出验证地址+验证码)→ 浏览器批准 → `auth.py --login-poll`。
- 之后 refresh_token 自动续期;`auth.py --logout` 可切换账号。
- 后端 `/pub/v1` 校验 token(aud=phoenix-api)→ 落 `uploaded_by`/`reviewed_by` 与 `audit_log`。
- 详见 `docs/员工级REST-API-OAuth接入方案.md`(仓库根 docs/)。

## 运行时依赖

**无需额外安装 Python。** WorkBuddy 桌面应用自带 Python 3.13。脚本只用标准库
(urllib/json/os/sys/argparse/base64/ssl),零第三方依赖。

## 目录结构

```
phoenix-doc-assistant/
├── .codebuddy-plugin/plugin.json        # 专家元数据
├── agents/phoenix-doc-assistant.md      # Agent 人设 + 调用规范(核心)
├── skills/phoenix-api/
│   ├── SKILL.md
│   ├── scripts/
│   │   ├── config.py                    # 配置文件读写/脱敏
│   │   ├── auth.py                      # Keycloak Device Flow 登录 + token 刷新
│   │   ├── api_client.py                # REST 客户端(自动带 Bearer)
│   │   ├── setup.py                     # 端点配置向导(手动终端用)
│   │   └── commands/                    # upload/extract_fields/validate/save/query/ask
│   ├── references/                      # phoenix-api-docs.md / doc-type-fields.md
│   └── templates/config.template.json
├── avatars/expert.png                   # 专家头像(需自行放置)
├── .gitignore                           # 忽略 .config.json(内含员工 token)/__pycache__
└── README.md
```

## 安装与使用

### 1. 后端与 Keycloak 就绪
- workflow 配 `PHX_API_OIDC_ISSUER`/`PHX_API_OIDC_AUDIENCE`,启用 `/pub/v1`。
- Keycloak 新增 `phoenix-cli`(device flow)+ audience mapper `phoenix-api`。
- 均见 `docs/员工级REST-API-OAuth接入方案.md`。

### 2. 安装专家包
将本目录打包为 zip,通过 WorkBuddy"导入专家包"安装。端点三要素建议 IT 预置进
`templates/config.template.json`(员工只需登录,不填任何密钥)。

### 3. 首次使用
和专家对话,它会自动 `auth.py --check`;未登录则引导设备登录。之后即可:
- "帮我上传归档这份报销单" + 附上图片
- "查一下金额超过1万的报销单"
- "那份合同里违约金怎么约定的？"

## 调试(终端)

```bash
cd skills/phoenix-api/scripts
python3 auth.py --check              # NOT_CONFIGURED / NEEDS_LOGIN / CONFIGURED
python3 auth.py --login-start        # 发起设备登录,拿验证地址+码
python3 auth.py --login-poll         # 等待批准,落 token
python3 auth.py --whoami             # 当前登录员工
python3 config.py --show             # 查看配置(token 脱敏)
python3 commands/upload.py --content-text "测试内容" --doc-type generic
```

## 开发说明:新增业务命令

1. `scripts/commands/xxx.py`;`from api_client import ApiClient`(必要时 `to_field_array`)
2. argparse 接参 → `client.post('/pub/v1/...', data={...})` / `client.get('/pub/v1/...', params={...})`
3. 在 `agents/phoenix-doc-assistant.md` 补调用规范
