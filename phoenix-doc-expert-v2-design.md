> ⚠️ **实现说明(2026-07):本设计已落地,但因"必须识别每一个员工"的决策,做了如下修订——**
> - 鉴权从"共享 api_key"改为 **Keycloak Device Flow 的每员工 Bearer token**(auth.py 管理登录与刷新);
> - 后端端点是 **`/pub/v1/*`**(OAuth 资源服务器 `backend/internal/restapi`),与老 `/api`、MCP 完全独立;
> - `fields` 用数组、取字段用 `POST .../extract`、查询用 `GET`、问答返回 `chunks`、不提供删除。
> - **落地与部署以这两份为准**:`docs/员工级REST-API-OAuth接入方案.md` 与
>   `phoenix-doc-assistant/`(Agent MD / SKILL.md / references 已同步)。
> 下文"共享 api_key / 纯 REST"部分为早期设计,保留作背景。

# Phoenix 文档助手 V2 —— 基于方案A（纯REST）的实现规格

> **目标**：用方案A（专家包内置 Python 脚本直连系统）重写 Phoenix 文档顾问的功能。专家包自带 Python REST 客户端脚本，**不走 MCP 协议**，直接调后端 REST API（Bearer Token + 标准 HTTP），绕开 WorkBuddy 的 MCP 连接器配置流程，实现"装包即用"。老版 Phoenix 文档顾问（依赖连接器）保持不动，新版作为独立专家包并存。
>
> **关键决策**：本方案选用方案A三选项中的**选项A（纯REST）**——后端新开发 REST API，专家包用极简 REST 客户端直连。不依赖 MCP 协议，客户端简单、好调试、架构统一、可被其他系统复用。详细的 REST 接口规格见独立文档 `phoenix-rest-api-spec.md`（给后端实现用）。

---

## 一、专家定位

| 维度 | 老版 Phoenix 文档顾问 | 新版 Phoenix 文档助手 V2 |
|------|---------------------|------------------------|
| **专家 name** | `phoenix-doc-expert` | `phoenix-doc-assistant` |
| **连接后端方式** | WorkBuddy MCP 连接器（用户需配置+信任） | 专家包内置 Python 脚本直连 **REST API** |
| **通信协议** | MCP streamable-http（JSON-RPC + 三步握手 + SSE） | **标准 HTTP REST**（Bearer Token + JSON） |
| **用户安装步骤** | 装专家包 → 配连接器 → 点信任 → 用 | 装专家包 → 首次对话提供 API 地址+Key → 用 |
| **后端端点来源** | `~/.workbuddy/mcp.json` | 专家包内 `.config.json`（`api_base_url` + `api_key`） |
| **凭证管理** | WorkBuddy 连接器配置 | 专家包内 `.config.json`（0o600 权限，显示时脱敏） |
| **能力范围** | 完全相同：上传/识别/校验/入库/查询/问答 | 完全相同 |
| **运行时依赖** | 无（走 MCP 连接器） | 无（WorkBuddy 自带 Python 3.13，脚本只用标准库） |

**核心差异**：新版把"连接后端"这件事从 WorkBuddy 连接器层下沉到专家包脚本层，且彻底放弃 MCP 协议改用标准 REST。用户不再需要碰"连接器管理"页面，装完专家包提供 API 凭证即可直接处理文档。客户端只用到 `urllib` + Bearer Token + HTTP 状态码，没有 MCP 的三步握手、SSE 解析、JSON-RPC 封装，简单到不能再简单。

---

## 二、专家包目录结构

```
phoenix-doc-assistant/
├── .codebuddy-plugin/
│   └── plugin.json                          # 专家元数据
├── agents/
│   └── phoenix-doc-assistant.md             # Agent 人设 + 调用规范（核心）
├── skills/
│   └── phoenix-api/
│       ├── SKILL.md                         # 技能说明
│       ├── scripts/
│       │   ├── api_client.py                # REST API 客户端封装（核心，75行）
│       │   ├── config.py                    # 配置管理（--check/--show，api_key脱敏）
│       │   ├── setup.py                     # 首次配置向导（终端手动用）
│       │   └── commands/
│       │       ├── upload.py                # 上传文档归档        POST /documents
│       │       ├── extract_fields.py        # 取字段清单          GET  /documents/{id}/fields
│       │       ├── validate.py              # 预校验              POST /documents/{id}/validate
│       │       ├── save.py                  # 入库                POST /documents/{id}/save
│       │       ├── query.py                 # 结构化查询          POST /documents/query
│       │       └── ask.py                   # 语义问答            POST /documents/ask
│       ├── references/
│       │   ├── phoenix-api-docs.md          # REST 接口文档
│       │   └── doc-type-fields.md           # 各单据类型字段清单
│       └── templates/
│           └── config.template.json         # 配置文件模板
├── avatars/
│   └── expert.png                           # 专家头像
├── .gitignore                               # 忽略 .config.json / __pycache__
└── README.md                                # 使用说明
```

**与方案A通用脚手架（company-expert-template）的对应关系**：

| 方案A 脚手架 | Phoenix V2 | 说明 |
|-------------|-----------|------|
| `api_client.py` | `api_client.py` | **完全复用**，纯 REST（Bearer Token + HTTP），无需改动 |
| `config.py` | `config.py` | 字段保持 `api_base_url` / `api_key` / `timeout` / `verify_ssl` |
| `commands/query_expense.py` 等 | `commands/upload.py` 等 6 个 | 对应文档处理的 6 个 REST 端点 |
| `references/api_docs.md` | `references/phoenix-api-docs.md` | 记录 6 个 REST 接口的参数和返回格式 |

> **与早期 MCP 版的差异**：早期版本曾用 `phoenix_mcp_client.py` 封装 MCP streamable-http 协议（161 行，含三步握手 + SSE 解析）。现版本已删除该文件，改为 `api_client.py`（75 行，纯 REST），客户端复杂度下降一半以上，且不再依赖 MCP 协议。

---

## 三、plugin.json（专家元数据）

```json
{
  "name": "phoenix-doc-assistant",
  "version": "1.0.0",
  "description": "Phoenix document assistant with built-in REST API client (Plan A, pure REST)",
  "author": {
    "name": "WorkBuddy",
    "email": "dev@workbuddy.com"
  },
  "agents": ["./agents/phoenix-doc-assistant.md"],
  "skills": ["./skills/phoenix-api"],

  "expertType": "agent",
  "agentName": "phoenix-doc-assistant",

  "displayName": {
    "en": "Phoenix Doc Assistant",
    "zh": "Phoenix文档助手"
  },
  "profession": {
    "en": "Enterprise Document Processing Assistant",
    "zh": "企业智能文档处理助手"
  },
  "displayDescription": {
    "en": "Upload, recognize, validate, archive and query documents via REST API. Built-in HTTP client, zero connector config.",
    "zh": "通过REST API上传、识别、校验、归档、查询文档。内置HTTP客户端，零连接器配置，装包即用。"
  },
  "avatar": "avatars/expert.png",
  "categoryId": "01-Finance",
  "defaultInitPrompt": {
    "zh": "你好，我是Phoenix文档助手。可以帮你上传归档单据、识别提取字段、校验入库、结构化查询和语义问答。直接发文档给我即可开始。",
    "en": "Hi, I'm Phoenix Doc Assistant. I can upload, recognize, validate, archive, query and Q&A on your documents. Just send me a document to start."
  },
  "plugin": "phoenix-doc-assistant",
  "tags": [
    { "en": "Document", "zh": "文档处理" },
    { "en": "OCR", "zh": "智能识别" },
    { "en": "Archive", "zh": "归档查询" }
  ],
  "quickPrompts": [
    { "en": "Upload and archive this document", "zh": "上传归档这份文档" },
    { "en": "Query documents by type", "zh": "按类型查文档" },
    { "en": "Ask a question about archived docs", "zh": "对已归档文档提问" }
  ]
}
```

**要点**：
- `name` 用 `phoenix-doc-assistant`，与老版 `phoenix-doc-expert` 区分，避免命名空间冲突
- `description` / `displayDescription` 明确标注 "REST API client"，不再提 MCP
- `categoryId` 用 `01-Finance`（文档处理归类）
- `quickPrompts` 固定 3 个，覆盖核心场景

---

## 四、Agent MD（调用规范——整个方案的核心）

> 这是 V2 最关键的文件。模型是否正确触发脚本、是否正确处理文档处理的多步流程，全靠这里写得多清晰。

```markdown
---
name: phoenix-doc-assistant
description: Enterprise document processing assistant that uploads, recognizes, validates, archives and queries documents via REST API using built-in Python HTTP client
displayName:
  en: "Phoenix Doc Assistant"
  zh: "Phoenix文档助手"
profession:
  en: "Enterprise Document Processing Assistant"
  zh: "企业智能文档处理助手"
maxTurns: 50
skills: [phoenix-api]
---

# Phoenix 文档助手

你是一名企业智能文档处理专家。**文档的识别与字段提取由你（多模态大模型）完成**——你直接读取用户提供的图片、扫描件、PDF、Office 文档，抽出结构化字段并转写正文；后端 REST API 负责归档原件、规则校验、结构化入库与检索（含知识库语义问答）。

你的核心能力通过 `skills/phoenix-api/scripts/` 下的 Python 脚本实现，脚本通过标准 HTTP 调用后端 REST API（Bearer Token 鉴权），**不走 MCP 协议，不依赖 WorkBuddy 的 MCP 连接器**。

## 核心能力

1. **上传归档**：把文档原件传给后端归档留存，拿到文档 ID
2. **取字段清单**：获取该单据类型要抽取的字段清单（字段名、中文标签、别名、规则）
3. **识别与转写（你来做）**：你亲自从原件识别字段值、完整转写正文
4. **预校验（可选）**：入库前先看校验结果
5. **校验入库**：字段与正文交后端校验入库，按 status 分流
6. **结构化查询**：按类型/状态/关键词/字段值精确筛选历史文档
7. **语义问答**：对已归档文件正文做开放式语义问答

## 配置检查（每次会话首次操作前必须做）

执行以下命令检查后端连接配置是否就绪：
​```bash
python3 skills/phoenix-api/scripts/config.py --check
​```

- 返回 `CONFIGURED`：继续执行业务命令
- 返回 `NOT_CONFIGURED`：走"首次配置"流程（见下文）

**配置未就绪时，拒绝执行任何业务命令**，并提示用户先配置。

## 首次配置（对话式）

如果 `config.py --check` 返回 `NOT_CONFIGURED`，需要向用户收集两项必填信息：

1. **后端 API 地址**（api_base_url）：如 `https://docs.company.com/api/v1`
2. **API Key**（Bearer Token）：后端分配的访问凭证

收集到后，写入配置文件（api_key 必填，地址必填）：
​```bash
cat > skills/phoenix-api/scripts/.config.json << 'EOF'
{"api_base_url":"https://docs.company.com/api/v1","api_key":"用户提供的key","timeout":60,"verify_ssl":true}
EOF
chmod 600 skills/phoenix-api/scripts/.config.json
​```

可选询问：内网自签名证书时把 `verify_ssl` 设为 `false`。写完后再次 `config.py --check` 确认返回 `CONFIGURED`。

## 工作流程

### Phase 1: 上传归档

用户提供文档时（图片/PDF/文本），先由你判断文档形态：

**图片或二进制文件**（用户提供文件路径）：
​```bash
python3 skills/phoenix-api/scripts/commands/upload.py --file {文件路径} --doc-type {类型可选}
​```
脚本会读取文件、base64 编码、POST 到 `/documents`。

**纯文本内容**（用户直接贴文字，或你转写的正文）：
​```bash
python3 skills/phoenix-api/scripts/commands/upload.py --content-text '{文本内容}' --doc-type {类型可选}
​```

**大文件 URL**（PDF 等已部署到公网）：
​```bash
python3 skills/phoenix-api/scripts/commands/upload.py --file-url {URL} --doc-type {类型可选}
​```

脚本返回 JSON：`{"document_id": "xxx", "status": "uploaded"}`。
记住文档 ID，后续所有操作都要用。

> `doc_type` 参数：用户明确说了就填（如 invoice/reimbursement/contract/generic）；不确定时不传，后续你判定后在 save 时再定。

### Phase 2: 取字段清单 + 你亲自识别

调用 extract_fields 拿该单据类型要抽取的字段清单：
​```bash
python3 skills/phoenix-api/scripts/commands/extract_fields.py --document-id {文档ID}
​```
GET `/documents/{id}/fields`。

脚本返回 JSON：
- 如果是**类型目录（catalog）**：说明类型未定，你先判断这份文档属于哪种单据类型，再据该类型的字段清单抽取。
- 如果是**字段清单**：按清单逐项从原件抽出字段值。

拿到字段清单后，**你自己从原件完成识别**：
1. 按清单逐项抽出字段值（找不到的留空，不要编造）
2. 完整转写文档正文（保留编号、金额、条款等关键信息）

把识别出的类型和字段以 Markdown 表格展示给用户。

### Phase 3: 校验与入库

调用 save 入库（后端会做权威校验）：
​```bash
python3 skills/phoenix-api/scripts/commands/save.py \
  --document-id {文档ID} \
  --doc-type {类型} \
  --fields '{字段JSON}' \
  --content-text '{正文}'
​```
POST `/documents/{id}/save`。

- `--fields`：你抽的字段，JSON 字符串，如 `'{"doc_no":"123","amount":"5000.00"}'`
- `--content-text`：你转写的完整正文

脚本返回 JSON：`{"status": "saved"}` 或 `{"status": "needs_review", "issues": [...]}`。

**按 status 分流**：
- **status = "saved"**：入库成功。把字段值以 Markdown 表格（字段名 | 字段值）汇报给用户，并告知文档 ID。
- **status = "needs_review"**：把 issues 和当前值列给用户，请其确认或给出修正值；拿到修正后带上完整 `fields` 重新 save。**只有用户明确说"直接入库/强制入库"时才加 `--force` 参数**。

> 入库前想先看校验结果，可调 validate 做预校验（不入库）：
> ​```bash
> python3 skills/phoenix-api/scripts/commands/validate.py \
>   --document-id {文档ID} --doc-type {类型} --fields '{字段JSON}'
> ​```
> POST `/documents/{id}/validate`。

### Phase 4: 结构化查询

用户要查历史文档时：
​```bash
python3 skills/phoenix-api/scripts/commands/query.py \
  --doc-type {类型可选} \
  --status {状态可选} \
  --keyword {关键词可选} \
  --limit 20
​```
POST `/documents/query`。

**字段级过滤**（按字段值精确筛选或比较）：
​```bash
python3 skills/phoenix-api/scripts/commands/query.py \
  --doc-type reimbursement \
  --field-filter 'amount,gt,10000'
​```
`--field-filter` 格式：`字段名,运算符,值`，运算符支持 `eq/ne/gt/gte/lt/lte/contains/in`。可多次传 `--field-filter` 做多条件。

脚本返回 JSON 文档列表。多条用表格汇总（文件名、类型、状态、上传人、关键字段），单条展示完整字段。

### Phase 5: 内容语义问答

用户问的是**文件正文内容**（答案不在预定义字段里）时：
​```bash
python3 skills/phoenix-api/scripts/commands/ask.py \
  --question '{问题}' \
  --doc-type {类型可选，限定范围} \
  --limit 5
​```
POST `/documents/ask`。

脚本返回相关原文片段与来源文档。你据此作答，并**注明信息来自哪份文件**。

> **如何选查询工具**：要精确筛选/统计/列全（按字段、按类型、计数）用 `query.py`；要理解正文内容/开放问答用 `ask.py`。

## 输出规范

- **字段展示**：Markdown 表格，表头"字段名 | 字段值"
- **校验问题**：逐条列出 issues，标注涉及字段与规则
- **入库反馈**：明确告知状态（saved/needs_review）与文档 ID
- **金额与日期**：保持文档原始写法，不做换算或格式转换
- **问答溯源**：基于 ask.py 作答时注明来源文件名

## 错误处理

读取脚本 stdout 的 JSON，按 error 字段处理：
- `NOT_CONFIGURED`：走"首次配置"流程
- `NETWORK_ERROR`：提示"后端服务不可达，请确认 API 地址正确且后端已启动"，引导检查配置
- `HTTP_ERROR`：后端返回非 2xx，把 code 和 message 告知用户，提示稍后重试或联系管理员
- `PARSE_ERROR`：后端返回非 JSON 响应，提示稍后重试或联系管理员
- `NOT_FOUND`：告知用户"未找到对应记录，请确认文档 ID"
- `VALIDATION_ERROR`：把 issues 列给用户，请其确认或修正

## 注意事项

- **识别由你负责**：后端不替你识别或转写；字段值与正文都由你从原件产出。不要编造或"补全"不存在的内容，提取不到就如实告知。
- **逐步反馈**：上传、识别结果、校验问题、入库完成等关键步骤都简要反馈，保持流程透明。
- **用户确认优先**：needs_review 时必须等用户确认或修正后再入库，不擅自 `--force`。
- **删除与覆盖**：涉及删除、覆盖已入库数据的请求，一律引导用户到管理后台人工操作，本专家不执行。
- 脚本路径用相对于专家包根目录的写法
- 脚本返回 JSON 到 stdout，错误信息也在 stdout 里（带 error 字段）
```

**写 Agent MD 的关键要点**：
1. 每个 Phase 都给出**完整的 bash 命令模板**，并标注对应的 REST 端点
2. 明确**参数格式**（特别是 `--fields` 的 JSON 字符串、`--field-filter` 的逗号分隔格式）
3. 写清**前置条件**（必须先 `config.py --check`）
4. 写清**status 分流逻辑**（saved vs needs_review）
5. 保留原 Phoenix 文档顾问的**业务语义**（识别由模型做、needs_review 要确认、删除引导后台）
6. 错误码改为 REST 体系（NETWORK_ERROR / HTTP_ERROR / PARSE_ERROR），不再有 MCP_* 错误

---

## 五、SKILL.md

```markdown
---
name: phoenix-api
description: Built-in Python REST client for Phoenix platform (upload/extract/validate/save/query/ask)
---

# Phoenix API Client

This skill provides Python scripts that directly call the backend REST API
(Bearer Token + standard HTTP), bypassing the WorkBuddy MCP connector mechanism.
No MCP protocol involved — just plain HTTP + JSON.

## 架构

​```
模型 → Bash 执行 commands/xxx.py → api_client.py 直连后端 REST API → 返回 JSON → 模型组织回答
​```

## Scripts

| 脚本 | 作用 | 对应 REST 端点 |
|------|------|---------------|
| `scripts/api_client.py` | REST API 客户端封装（被各命令 import） | - |
| `scripts/config.py` | 配置管理（读写本地 .config.json） | - |
| `scripts/setup.py` | 首次配置向导（手动终端用） | - |
| `scripts/commands/upload.py` | 上传文档归档 | POST /documents |
| `scripts/commands/extract_fields.py` | 取字段清单 | GET /documents/{id}/fields |
| `scripts/commands/validate.py` | 预校验 | POST /documents/{id}/validate |
| `scripts/commands/save.py` | 入库 | POST /documents/{id}/save |
| `scripts/commands/query.py` | 结构化查询 | POST /documents/query |
| `scripts/commands/ask.py` | 语义问答 | POST /documents/ask |

## 配置文件位置

凭证存储在 `scripts/.config.json`（已加入 .gitignore）：
​```json
{
  "api_base_url": "https://docs.company.com/api/v1",
  "api_key": "your-api-key-here",
  "timeout": 60,
  "verify_ssl": true
}
​```
`config.py --show` 显示时会对 api_key 脱敏（只显示前 4 位 + ****）。

## 通信说明

- 协议：标准 HTTP/HTTPS + JSON
- 鉴权：`Authorization: Bearer {api_key}`
- 请求体：JSON（`Content-Type: application/json`）
- 响应体：JSON
- 错误：HTTP 4xx/5xx 时后端返回 `{"error":"CODE","message":"..."}`，客户端透传给模型

## 新增业务命令

1. 在 `scripts/commands/` 下新建 `xxx.py`
2. `from api_client import ApiClient`（api_client.py 在 scripts/ 根目录）
3. 实现 CLI 入口，接收参数
4. `client = ApiClient(); result = client.post('/path', data={...})`，返回 JSON 到 stdout
5. 在 Agent MD 里补充调用规范
```

---

## 六、Python 脚本完整代码

### 6.1 scripts/api_client.py（REST 客户端——核心封装）

> 这是整个方案的技术核心。封装了标准 HTTP 请求 + Bearer Token 鉴权 + 错误处理。相比 MCP 版的 `phoenix_mcp_client.py`（161 行，含三步握手 + SSE 解析），本文件仅 75 行，简单到一眼就懂，且 curl/Postman 可直接测。

```python
#!/usr/bin/env python3
"""REST API 客户端封装：被各业务命令 import 使用。
标准 HTTP + Bearer Token 鉴权，不走 MCP 协议。
"""
import json
import os
import sys
import urllib.request
import urllib.error
from urllib.parse import urlencode

# 让 commands/ 下的脚本能 import 同级 scripts/ 的模块
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from config import load_config, is_configured


class ApiClient:
    """后端 REST API 客户端"""

    def __init__(self):
        if not is_configured():
            print(json.dumps({"error": "NOT_CONFIGURED", "message": "请先完成配置"}))
            sys.exit(1)
        self.cfg = load_config()
        self.base_url = self.cfg['api_base_url'].rstrip('/')

    def request(self, method, path, data=None, params=None):
        """发起 HTTP 请求，返回 dict"""
        url = self.base_url + path
        if params:
            url += '?' + urlencode(params)

        headers = {
            'Authorization': f"Bearer {self.cfg['api_key']}",
            'Content-Type': 'application/json',
            'Accept': 'application/json',
        }

        body = json.dumps(data, ensure_ascii=False).encode('utf-8') if data is not None else None
        req = urllib.request.Request(url, data=body, headers=headers, method=method)

        # SSL 校验控制（内网自签名证书）
        import ssl
        ctx = None
        if not self.cfg.get('verify_ssl', True):
            ctx = ssl.create_default_context()
            ctx.check_hostname = False
            ctx.verify_mode = ssl.CERT_NONE

        try:
            with urllib.request.urlopen(req, timeout=self.cfg.get('timeout', 60), context=ctx) as resp:
                resp_body = resp.read().decode('utf-8')
                return json.loads(resp_body) if resp_body.strip() else {}
        except urllib.error.HTTPError as e:
            # 后端返回的错误（4xx/5xx），body 通常是 {"error":"CODE","message":"..."}
            err_body = e.read().decode('utf-8', errors='replace')
            try:
                err_json = json.loads(err_body)
                print(json.dumps(err_json))
            except json.JSONDecodeError:
                print(json.dumps({"error": "HTTP_ERROR", "code": e.code, "message": err_body}))
            sys.exit(1)
        except urllib.error.URLError as e:
            print(json.dumps({"error": "NETWORK_ERROR", "message": f"无法连接后端服务: {str(e.reason)}"}))
            sys.exit(1)
        except json.JSONDecodeError as e:
            print(json.dumps({"error": "PARSE_ERROR", "message": f"后端返回非 JSON 响应: {str(e)}"}))
            sys.exit(1)

    def get(self, path, params=None):
        return self.request('GET', path, params=params)

    def post(self, path, data=None):
        return self.request('POST', path, data=data)

    def put(self, path, data=None):
        return self.request('PUT', path, data=data)

    def delete(self, path):
        return self.request('DELETE', path)
```

**关键技术点说明**：

1. **Bearer Token 鉴权**：每个请求自动带 `Authorization: Bearer {api_key}` 头
2. **SSL 校验控制**：`verify_ssl=False` 时跳过证书校验（内网自签名场景）
3. **错误分流**（三层）：
   - `HTTPError`：后端返回 4xx/5xx，透传后端的错误 JSON（如果后端返回的不是 JSON，则包装为 `HTTP_ERROR`）
   - `URLError`：网络不可达，输出 `NETWORK_ERROR`
   - `JSONDecodeError`：响应非 JSON，输出 `PARSE_ERROR`
4. **极简 API**：`get/post/put/delete` 四个方法，业务命令一行调用
5. **零第三方依赖**：只用 `urllib` / `json` / `ssl`，WorkBuddy 自带 Python 直接可跑

### 6.2 scripts/config.py（配置管理）

```python
#!/usr/bin/env python3
"""配置管理：读写本地 .config.json"""
import json
import os
import sys

CONFIG_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), '.config.json')

DEFAULT_CONFIG = {
    "api_base_url": "",        # 后端 REST API 地址，如 https://docs.company.com/api/v1
    "api_key": "",             # API Key（Bearer Token）
    "timeout": 60,             # 请求超时秒数（文档处理可能较慢，默认 60）
    "verify_ssl": True         # 是否校验 SSL 证书（内网自签名可设 False）
}

def load_config():
    """读取配置，不存在则返回 None"""
    if not os.path.exists(CONFIG_FILE):
        return None
    with open(CONFIG_FILE, 'r', encoding='utf-8') as f:
        return json.load(f)

def save_config(config):
    """保存配置到本地文件"""
    full = {**DEFAULT_CONFIG, **config}
    with open(CONFIG_FILE, 'w', encoding='utf-8') as f:
        json.dump(full, f, ensure_ascii=False, indent=2)
    os.chmod(CONFIG_FILE, 0o600)  # 仅所有者可读写

def is_configured():
    """检查是否已完成配置"""
    cfg = load_config()
    if not cfg:
        return False
    return bool(cfg.get('api_base_url') and cfg.get('api_key'))

if __name__ == '__main__':
    if '--check' in sys.argv:
        print('CONFIGURED' if is_configured() else 'NOT_CONFIGURED')
    elif '--show' in sys.argv:
        cfg = load_config()
        if cfg:
            # 显示时脱敏
            safe = {**cfg}
            if safe.get('api_key'):
                safe['api_key'] = safe['api_key'][:4] + '****'
            print(json.dumps(safe, ensure_ascii=False, indent=2))
        else:
            print('{}')
```

**与方案A通用版的差异**：
- 字段与通用版一致：`api_base_url` / `api_key` / `timeout` / `verify_ssl`
- `is_configured()` 同时检查 `api_base_url` 和 `api_key` 都非空（两者都是必填）
- `--show` 对 `api_key` 脱敏（只显示前 4 位 + ****），避免对话中泄露完整凭证
- **没有 `--init`**：因为需要用户提供真实的 API 地址和 Key，不能一键初始化（这点与早期 MCP 版不同，MCP 版有 `--init` 是因为本地服务默认端点开箱即用）

### 6.3 scripts/setup.py（首次配置向导，手动终端用）

```python
#!/usr/bin/env python3
"""首次配置向导：给手动在终端跑的高级用户用。对话场景走对话式配置（见 Agent MD）。"""
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from config import load_config, save_config, is_configured, DEFAULT_CONFIG

def main():
    print("=== Phoenix 文档助手 - 首次配置 ===\n")

    if is_configured():
        print("检测到已有配置。继续将覆盖。")
        confirm = input("是否继续？(y/N): ").strip().lower()
        if confirm != 'y':
            print("配置未修改。")
            return

    existing = load_config() or {}

    print("请提供后端 REST API 的地址和 API Key。")
    print("示例：https://docs.company.com/api/v1\n")

    api_base_url = input(
        f"1. 后端 API 地址 [{existing.get('api_base_url', '')}]: "
    ).strip() or existing.get('api_base_url', '')

    api_key = input(
        "2. API Key（Bearer Token）: "
    ).strip() or existing.get('api_key', '')

    verify_ssl_input = input(
        f"3. 是否校验 SSL 证书 (y/n) [{'y' if existing.get('verify_ssl', True) else 'n'}]: "
    ).strip().lower()
    if verify_ssl_input == 'n':
        verify_ssl = False
    elif verify_ssl_input == 'y':
        verify_ssl = True
    else:
        verify_ssl = existing.get('verify_ssl', True)

    timeout_input = input(
        f"4. 请求超时秒数 [{existing.get('timeout', DEFAULT_CONFIG['timeout'])}]: "
    ).strip()
    timeout = int(timeout_input) if timeout_input else existing.get('timeout', DEFAULT_CONFIG['timeout'])

    if not api_base_url or not api_key:
        print("\n错误：API 地址和 API Key 不能为空。")
        return

    config = {
        'api_base_url': api_base_url,
        'api_key': api_key,
        'timeout': timeout,
        'verify_ssl': verify_ssl,
    }

    save_config(config)
    print(f"\n配置已保存到 {os.path.join(os.path.dirname(os.path.abspath(__file__)), '.config.json')}")
    print("你现在可以正常使用 Phoenix 文档助手了。")

if __name__ == '__main__':
    main()
```

### 6.4 scripts/commands/upload.py（上传文档归档）

```python
#!/usr/bin/env python3
"""上传文档到后端归档
用法：
  --file {路径}              上传本地文件（图片/PDF等二进制，自动base64）
  --content-text '{文本}'    上传纯文本
  --file-url {URL}           上传公网URL文件
  --doc-type {类型}          可选，文档类型
"""
import argparse
import base64
import json
import mimetypes
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def file_to_base64(file_path):
    """读取文件并转为 base64"""
    if not os.path.exists(file_path):
        print(json.dumps({"error": "FILE_NOT_FOUND", "message": f"文件不存在: {file_path}"}))
        sys.exit(1)
    with open(file_path, 'rb') as f:
        content = f.read()
    mime_type = mimetypes.guess_type(file_path)[0] or 'application/octet-stream'
    return base64.b64encode(content).decode('utf-8'), mime_type


def main():
    parser = argparse.ArgumentParser(description='上传文档归档')
    g = parser.add_mutually_exclusive_group(required=True)
    g.add_argument('--file', help='本地文件路径（图片/PDF等，自动base64）')
    g.add_argument('--content-text', help='纯文本内容')
    g.add_argument('--file-url', help='公网可访问的文件URL')
    parser.add_argument('--doc-type', default=None, help='文档类型（可选）')
    args = parser.parse_args()

    payload = {}
    if args.doc_type:
        payload['doc_type'] = args.doc_type

    if args.file:
        b64, mime = file_to_base64(args.file)
        payload['content_base64'] = b64
        payload['mime_type'] = mime
    elif args.content_text:
        payload['content_text'] = args.content_text
    elif args.file_url:
        payload['file_url'] = args.file_url

    client = ApiClient()
    result = client.post('/documents', data=payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
```

### 6.5 scripts/commands/extract_fields.py（取字段清单）

```python
#!/usr/bin/env python3
"""获取字段清单：--document-id {文档ID}"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def main():
    parser = argparse.ArgumentParser(description='获取字段清单')
    parser.add_argument('--document-id', required=True, help='文档ID')
    args = parser.parse_args()

    client = ApiClient()
    result = client.get(f'/documents/{args.document_id}/fields')
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
```

### 6.6 scripts/commands/validate.py（预校验）

```python
#!/usr/bin/env python3
"""预校验（不入库）：--document-id {ID} --doc-type {类型} --fields '{字段JSON}'
返回：{"status": "validated"} 或 {"status": "needs_review", "issues": [...]}
"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def main():
    parser = argparse.ArgumentParser(description='预校验文档')
    parser.add_argument('--document-id', required=True, help='文档ID')
    parser.add_argument('--doc-type', required=True, help='文档类型')
    parser.add_argument('--fields', required=True, help='字段JSON字符串')
    args = parser.parse_args()

    fields = json.loads(args.fields)

    client = ApiClient()
    result = client.post(f'/documents/{args.document_id}/validate', data={
        'doc_type': args.doc_type,
        'fields': fields
    })
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
```

### 6.7 scripts/commands/save.py（入库）

```python
#!/usr/bin/env python3
"""入库：--document-id {ID} --doc-type {类型} --fields '{字段JSON}' --content-text '{正文}' [--force]
返回：{"status": "saved"} 或 {"status": "needs_review", "issues": [...]}
"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def main():
    parser = argparse.ArgumentParser(description='入库文档')
    parser.add_argument('--document-id', required=True, help='文档ID')
    parser.add_argument('--doc-type', required=True, help='文档类型')
    parser.add_argument('--fields', required=True, help='字段JSON字符串')
    parser.add_argument('--content-text', required=True, help='完整正文')
    parser.add_argument('--force', action='store_true', help='强制入库（跳过校验）')
    args = parser.parse_args()

    fields = json.loads(args.fields)

    payload = {
        'doc_type': args.doc_type,
        'fields': fields,
        'content_text': args.content_text
    }
    if args.force:
        payload['force'] = True

    client = ApiClient()
    result = client.post(f'/documents/{args.document_id}/save', data=payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
```

### 6.8 scripts/commands/query.py（结构化查询）

```python
#!/usr/bin/env python3
"""结构化查询
用法：
  --doc-type {类型}          可选
  --status {状态}            可选
  --keyword {关键词}         可选
  --uploaded-by {上传人}     可选
  --limit 20                 可选，默认20
  --field-filter {字段,运算符,值}  可选，可多次传。运算符：eq/ne/gt/gte/lt/lte/contains/in
                                   in 运算符的值用 | 分隔，如 'status,in,saved|needs_review'
"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def parse_field_filter(filter_str):
    """解析 '字段名,运算符,值' 为 {field, op, value/values}"""
    parts = filter_str.split(',', 2)
    if len(parts) != 3:
        print(json.dumps({"error": "INVALID_FIELD_FILTER", "message": f"格式错误: {filter_str}，应为 字段,运算符,值"}))
        sys.exit(1)

    field, op, value = parts

    # in 运算符用 | 分隔多值
    if op == 'in':
        values = value.split('|')
        return {"field": field, "op": "in", "values": values}
    else:
        return {"field": field, "op": op, "value": value}


def main():
    parser = argparse.ArgumentParser(description='结构化查询文档')
    parser.add_argument('--doc-type', default=None, help='文档类型')
    parser.add_argument('--status', default=None, help='状态')
    parser.add_argument('--keyword', default=None, help='关键词（匹配文件名或正文）')
    parser.add_argument('--uploaded-by', default=None, help='上传人')
    parser.add_argument('--limit', type=int, default=20, help='返回条数上限')
    parser.add_argument('--field-filter', action='append', default=[], help='字段过滤，格式：字段,运算符,值')
    args = parser.parse_args()

    payload = {'limit': args.limit}
    if args.doc_type:
        payload['doc_type'] = args.doc_type
    if args.status:
        payload['status'] = args.status
    if args.keyword:
        payload['keyword'] = args.keyword
    if args.uploaded_by:
        payload['uploaded_by'] = args.uploaded_by
    if args.field_filter:
        payload['field_filters'] = [parse_field_filter(f) for f in args.field_filter]

    client = ApiClient()
    result = client.post('/documents/query', data=payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
```

**字段级过滤用法示例**：
```bash
# 金额超过1万的报销单
python3 .../query.py --doc-type reimbursement --field-filter 'amount,gt,10000'

# 某科技公司开的发票
python3 .../query.py --field-filter 'seller,contains,科技'

# 状态为 saved 或 needs_review 的文档
python3 .../query.py --field-filter 'status,in,saved|needs_review'

# 多条件：报销单 + 金额>1万 + 状态=saved
python3 .../query.py --doc-type reimbursement \
  --field-filter 'amount,gt,10000' \
  --field-filter 'status,eq,saved'
```

### 6.9 scripts/commands/ask.py（语义问答）

```python
#!/usr/bin/env python3
"""语义问答：--question '{问题}' [--doc-type {类型}] [--limit 5]
返回：相关原文片段与来源文档
"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def main():
    parser = argparse.ArgumentParser(description='语义问答')
    parser.add_argument('--question', required=True, help='问题')
    parser.add_argument('--doc-type', default=None, help='限定文档类型（可选）')
    parser.add_argument('--limit', type=int, default=5, help='返回片段数上限')
    args = parser.parse_args()

    payload = {
        'question': args.question,
        'limit': args.limit
    }
    if args.doc_type:
        payload['doc_type'] = args.doc_type

    client = ApiClient()
    result = client.post('/documents/ask', data=payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
```

---

## 七、references/phoenix-api-docs.md（REST 接口文档）

> 给开发者看：记录 6 个 REST 端点的参数和返回格式。
> 模型在遇到新需求时也会参考这个文档来编写新的 commands 脚本。
> **给后端的完整实现规格见独立文档 `phoenix-rest-api-spec.md`。**

```markdown
# Phoenix REST API 接口文档

## 基础信息
- 协议：标准 HTTP/HTTPS + JSON
- 鉴权：`Authorization: Bearer {api_key}`
- 请求/响应 Content-Type：application/json
- Base URL：由用户配置（如 https://docs.company.com/api/v1）

## 端点列表

### 1. POST /documents（上传归档）
请求体（三选一）：
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| content_base64 | string | 三选一 | 文件base64编码（图片/PDF等二进制） |
| content_text | string | 三选一 | 纯文本内容 |
| file_url | string | 三选一 | 公网可访问的文件URL |
| doc_type | string | 否 | 文档类型，不确定时不传 |
| mime_type | string | 否 | 文件MIME类型（配合content_base64） |

返回：`{"document_id": "xxx", "status": "uploaded"}`

### 2. GET /documents/{id}/fields（取字段清单）
路径参数：document_id

返回：
- 类型未定时返回**类型目录 catalog**：`{"type": "catalog", "types": [{"name":"invoice","label":"发票"},...]}`
- 类型已定时返回**字段清单**：`{"fields": [{"name":"doc_no","label":"单据编号","required":true},...]}`

### 3. POST /documents/{id}/validate（预校验，不入库）
请求体：
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| doc_type | string | 是 | 文档类型 |
| fields | object | 是 | 字段键值对 |

返回：`{"status": "validated"}` 或 `{"status": "needs_review", "issues": [{"field":"doc_no","rule":"required","message":"必填字段缺失"}]}`

### 4. POST /documents/{id}/save（入库）
请求体：
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| doc_type | string | 是 | 文档类型 |
| fields | object | 是 | 字段键值对 |
| content_text | string | 是 | 完整正文 |
| force | boolean | 否 | 强制入库（跳过校验），默认false |

返回：`{"status": "saved"}` 或 `{"status": "needs_review", "issues": [...]}`

### 5. POST /documents/query（结构化查询）
请求体：
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| doc_type | string | 否 | 文档类型过滤 |
| status | string | 否 | 状态过滤 |
| keyword | string | 否 | 关键词（匹配文件名或正文） |
| uploaded_by | string | 否 | 上传人过滤 |
| limit | int | 否 | 返回条数上限，默认20 |
| field_filters | array | 否 | 字段级过滤，每条 {field, op, value/values} |

field_filters 运算符：`eq`/`ne`/`gt`/`gte`/`lt`/`lte`/`contains`/`in`

返回：`{"documents": [{"id":"xxx","filename":"xxx","doc_type":"xxx","status":"xxx","fields":{...}},...]}`

### 6. POST /documents/ask（语义问答）
请求体：
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| question | string | 是 | 问题 |
| doc_type | string | 否 | 限定文档类型范围 |
| limit | int | 否 | 返回片段数上限，默认5 |

返回：`{"answers": [{"content":"原文片段","source":"来源文件名","document_id":"xxx"},...]}`

## 错误响应
所有 4xx/5xx 返回统一格式：`{"error": "ERROR_CODE", "message": "可读错误描述"}`
常见 error code：UNAUTHORIZED / NOT_FOUND / VALIDATION_ERROR / INTERNAL_ERROR

## 常见文档类型（doc_type）
| 类型 | 说明 | 必填字段 |
|------|------|----------|
| invoice | 增值税发票 | buyer, seller, total_amount |
| reimbursement | 报销单 | title, doc_no, amount |
| contract | 合同 | title, party_a, party_b |
| generic | 通用单据 | title, doc_no |
```

---

## 八、templates/config.template.json

```json
{
  "api_base_url": "https://docs.company.com/api/v1",
  "api_key": "your-api-key-here",
  "timeout": 60,
  "verify_ssl": true
}
```

---

## 九、首次配置用户体验

### 推荐流程（对话式配置）

与早期 MCP 版不同，纯 REST 方案需要用户提供两项必填凭证（API 地址 + API Key），不能一键初始化。Agent MD 里写明对话式收集流程：

**场景：首次使用**
```
用户：帮我上传这份报销单
助手：（执行 config.py --check，返回 NOT_CONFIGURED）
      检测到尚未配置后端连接。需要你提供两项信息：
      1. 后端 API 地址（如 https://docs.company.com/api/v1）
      2. API Key（后端分配的访问凭证）
      请把这两项发给我，我来帮你配置。
用户：地址是 https://docs.company.com/api/v1，key 是 sk-xxxxx
助手：（执行 Bash 写入 .config.json）
      ✅ 配置完成。正在上传...
      （执行 upload.py --file xxx）
```

**Agent MD 里对应的指令**：
```markdown
## 首次配置（对话式）
如果 config.py --check 返回 NOT_CONFIGURED，需要向用户收集两项必填信息：
1. 后端 API 地址（api_base_url）
2. API Key（Bearer Token）
收集到后，写入配置文件（api_key 必填，地址必填）：
   bash: cat > .../.config.json << 'EOF' {...} EOF
   chmod 600 ...
可选询问：内网自签名证书时把 verify_ssl 设为 false。
写完后再次 config.py --check 确认返回 CONFIGURED。
```

> **与早期 MCP 版的配置体验差异**：MCP 版因 Phoenix 本地服务默认端点开箱即用，有 `config.py --init` 一键初始化。纯 REST 版因为后端地址和 Key 因公司而异，必须用户提供，没有一键初始化。但这也只是一次性操作，配完后永久生效。

---

## 十、错误处理机制

### 脚本侧（api_client.py 已内置）

| 错误 | stdout JSON | 模型应做 |
|------|-------------|---------|
| 未配置 | `{"error":"NOT_CONFIGURED"}` | 走首次配置流程 |
| 后端不可达 | `{"error":"NETWORK_ERROR","message":"..."}` | 提示确认 API 地址正确且后端已启动 |
| 后端返回4xx/5xx | 透传后端错误JSON `{"error":"...","message":"..."}`；非JSON则 `{"error":"HTTP_ERROR","code":xxx,"message":"..."}` | 把 code 和 message 告知用户 |
| 响应非JSON | `{"error":"PARSE_ERROR","message":"..."}` | 提示稍后重试或联系管理员 |
| 文件不存在 | `{"error":"FILE_NOT_FOUND"}` | 提示用户检查文件路径 |
| 字段过滤格式错 | `{"error":"INVALID_FIELD_FILTER"}` | 模型修正过滤格式重新调用 |

### Agent MD 侧

```markdown
## 错误处理
读取脚本 stdout 的 JSON，按 error 字段处理：
- NOT_CONFIGURED：走"首次配置"流程
- NETWORK_ERROR：提示"后端服务不可达，请确认 API 地址正确且后端已启动"，引导检查配置
- HTTP_ERROR：后端返回非 2xx，把 code 和 message 告知用户，提示稍后重试或联系管理员
- PARSE_ERROR：后端返回非 JSON 响应，提示稍后重试或联系管理员
- NOT_FOUND：告知用户"未找到对应记录，请确认文档 ID"
- VALIDATION_ERROR：把 issues 列给用户，请其确认或修正
```

> **与 MCP 版的错误码差异**：MCP 版有 `MCP_CONNECTION_ERROR` / `MCP_PROTOCOL_ERROR`，纯 REST 版改为 `NETWORK_ERROR` / `HTTP_ERROR` / `PARSE_ERROR`，更贴近标准 HTTP 语义。

---

## 十一、与老版 Phoenix 文档顾问的对比

| 维度 | 老版 phoenix-doc-expert | 新版 phoenix-doc-assistant |
|------|------------------------|---------------------------|
| **连接方式** | WorkBuddy MCP 连接器 | 专家包内置 Python 脚本直连 REST API |
| **通信协议** | MCP streamable-http（JSON-RPC + 三步握手 + SSE） | 标准 HTTP REST（Bearer Token + JSON） |
| **用户配置** | 连接器管理页面配置 + 点信任 | 对话式收集 API 地址 + Key |
| **后端端点** | ~/.workbuddy/mcp.json | 专家包内 .config.json |
| **协议处理** | WorkBuddy 连接器层处理 | 专家包 api_client.py 处理（75行，极简） |
| **业务能力** | 完全相同 | 完全相同 |
| **Agent MD 业务逻辑** | 完全相同（识别由模型做、needs_review要确认等） | 完全相同（直接复用） |
| **运行时依赖** | 无 | 无（WorkBuddy 自带 Python 3.13，脚本只用标准库） |
| **并存性** | 不受影响 | 独立专家包，与老版互不干扰 |

**关键结论**：新版与老版**业务能力完全等价**，差异只在"连接后端的方式"——新版彻底放弃 MCP 协议改用标准 REST，把连接逻辑从 WorkBuddy 连接器层下沉到专家包脚本层，换取"零连接器配置 + 客户端极简 + 好调试 + 架构统一"的体验。两个专家包可以并存，用户想用哪个用哪个。

---

## 十二、打包与分发

### 12.1 打包

```bash
cd /path/to/
zip -r phoenix-doc-assistant.zip phoenix-doc-assistant/ \
  --exclude "*/.config.json" \
  --exclude "*/__pycache__/*" \
  --exclude "*/.DS_Store"
```

### 12.2 分发方式

| 方式 | 适用场景 |
|------|----------|
| **WorkBuddy 市场** | 上架市场，用户一键安装 |
| **内网下载 zip** | 用户下载后通过 WorkBuddy"导入专家包"安装 |
| **IT 统一推送** | 企业 IT 把 zip 推到每台机器的专家包目录 |

### 12.3 安装后验证

用户安装后，首次对话：
1. 模型执行 `config.py --check`，返回 `NOT_CONFIGURED`
2. 对话式收集 API 地址 + Key，写入 .config.json
3. 后续直接可用

**零连接器配置，零"信任"点击。**

---

## 十三、运行时依赖说明

> 参见方案A文档 2.4 节，此处简述。

**员工不需要单独安装 Python。** WorkBuddy 桌面应用自带 Python 3.13.12，路径 `~/.workbuddy/binaries/python/versions/3.13.12/bin/python3`，已自动加入 PATH。本专家包所有脚本只用 Python 标准库（`urllib`/`json`/`os`/`sys`/`argparse`/`base64`/`mimetypes`/`ssl`），零第三方依赖，不需要 `pip install`。

**员工完整体验路径**：装 WorkBuddy（自带 Python）→ 装专家包 → 首次对话提供 API 凭证 → 开箱即用。

---

## 十四、给后端的交付物与实现检查清单

### 交付物

| 文件 | 给谁 | 用途 |
|------|------|------|
| **本文档**（phoenix-doc-expert-v2-design.md） | 后端 + 产品 | 整体设计规格，理解方案全貌 |
| **phoenix-rest-api-spec.md** | 后端 | 6 个 REST 接口的完整实现规格（URL/Method/入参/出参/错误码/curl示例/实现检查清单），后端照此实现 |
| **phoenix-doc-assistant/**（专家包目录） | 前端/WorkBuddy | 专家包成品，后端 API 就绪后直接可用 |

### 后端实现检查清单（详见 phoenix-rest-api-spec.md）

后端拿到交付物后，按以下顺序实现：

- [ ] 1. 阅读 `phoenix-rest-api-spec.md`，理解 6 个 REST 接口的入参出参
- [ ] 2. 实现鉴权中间件（校验 Bearer Token）
- [ ] 3. 实现 `POST /documents`（上传归档，支持 content_base64/content_text/file_url 三种来源）
- [ ] 4. 实现 `GET /documents/{id}/fields`（返回类型目录或字段清单）
- [ ] 5. 实现 `POST /documents/{id}/validate`（预校验，返回 validated/needs_review）
- [ ] 6. 实现 `POST /documents/{id}/save`（校验入库，支持 force 跳过校验）
- [ ] 7. 实现 `POST /documents/query`（结构化查询，支持 field_filters）
- [ ] 8. 实现 `POST /documents/ask`（语义问答，依赖向量索引，可后置）
- [ ] 9. 统一错误响应格式 `{"error":"CODE","message":"..."}`
- [ ] 10. 联调：用 curl 按规格文档的示例逐个测试
- [ ] 11. 联调顺序建议：上传+入库 → 查询 → 语义问答（向量索引可后置）

### 专家包侧检查清单（已由本方案完成）

- [x] 1. 创建专家包目录结构（第二节）
- [x] 2. 填写 plugin.json（第三节）
- [x] 3. 编写 Agent MD（第四节）
- [x] 4. 编写 SKILL.md（第五节）
- [x] 5. 实现 api_client.py（6.1）—— 纯REST客户端
- [x] 6. 实现 config.py（6.2）
- [x] 7. 实现 setup.py（6.3）
- [x] 8. 实现 6 个 commands 脚本（6.4-6.9）
- [x] 9. 填写 references/phoenix-api-docs.md（第七节）
- [x] 10. 创建 templates/config.template.json（第八节）
- [x] 11. 配置 .gitignore（忽略 .config.json / __pycache__）
- [x] 12. 所有 Python 脚本语法验证通过
- [x] 13. config.py --check/--show 功能验证通过（api_key 脱敏正常）
- [x] 14. commands import 链验证通过（api_client → config）
- [ ] 15. 后端 API 就绪后，全链路联调（upload → extract_fields → save → query → ask）
- [ ] 16. 打包 zip，导入 WorkBuddy 测试对话流程
- [ ] 17. 验证与老版 phoenix-doc-expert 并存无冲突

---

## 总结

新版 Phoenix 文档助手 V2 的本质：**用方案A的"内置脚本直连"模式，彻底放弃 MCP 协议改用标准 REST，把后端连接从 WorkBuddy 连接器层下沉到专家包脚本层，实现零连接器配置的开箱即用，业务能力与老版完全等价。**

核心技术是 `api_client.py` 封装的标准 HTTP + Bearer Token 客户端（75 行，纯标准库），相比 MCP 版的 161 行协议封装，复杂度下降一半以上，且 curl/Postman 可直接测试，企业架构统一，REST API 可被其他系统复用。

**给后端的两份交付物**：本文档（整体设计规格）+ `phoenix-rest-api-spec.md`（REST 接口实现规格）。后端照规格实现 6 个接口，专家包即可全链路跑通。
