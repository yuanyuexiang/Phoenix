# Phoenix 文档助手 - 后端 REST API 规格文档

> ⚠️ **本文件是早期草案,已被实现取代。真实契约以下面两处为准:**
> - **接口规格**:`phoenix-doc-assistant/skills/phoenix-api/references/phoenix-api-docs.md`(已实现的 `/pub/v1`)
> - **鉴权/部署**:`docs/员工级REST-API-OAuth接入方案.md`
>
> **实现时相对本草案的关键修订(因为"每员工身份"这个决策):**
> 1. 鉴权不是共享 `api_key`,而是 **Keycloak Device Flow 的每员工 Bearer token**(aud=phoenix-api);
> 2. 端点前缀是 **`/pub/v1/*`**(不是 `/documents/*`);取字段是 `POST .../extract`、查询是 `GET /pub/v1/documents`;
> 3. `fields` 是**数组** `[{name,value}]`(不是对象);客户端脚本自动把对象转数组;
> 4. 问答返回 `{"total","chunks":[...]}`(不是 `{"answers":[...]}`);
> 5. **不提供删除**端点(破坏性操作走管理后台)。
>
> 下文保留原草案内容仅作背景参考。

> **给后端团队**：本文档定义了 `phoenix-doc-assistant` 专家包需要的 6 个 REST 接口。
> 后端照此实现，专家包即可开箱即用。接口风格为标准 RESTful，无 MCP 协议依赖。

---

## 一、概述

### 背景
专家包（`phoenix-doc-assistant`）是运行在 WorkBuddy 桌面应用中的 AI 文档处理助手。它通过内置 Python 脚本调用后端 REST API，完成文档上传、字段抽取、校验、入库、查询、语义问答。

### 调用方
- **客户端**：专家包内置 Python 脚本（`api_client.py`），用标准库 `urllib` 发 HTTP 请求
- **鉴权**：Bearer Token
- **数据格式**：JSON（`Content-Type: application/json`）

### Base URL
由用户配置，示例：`https://docs.company.com/api/v1`

所有接口路径以下文为准，拼接在 Base URL 之后。

---

## 二、通用约定

### 2.1 请求头
```
Authorization: Bearer {api_key}
Content-Type: application/json
Accept: application/json
```

### 2.2 成功响应
HTTP 2xx，body 为 JSON 对象。各接口的具体返回结构见下文。

### 2.3 错误响应
HTTP 非 2xx，body 统一格式：
```json
{
  "error": "ERROR_CODE",
  "message": "人类可读的错误描述",
  "issues": []   // 仅校验类错误带此字段
}
```

| HTTP 状态码 | error 值 | 触发场景 |
|------------|---------|---------|
| 400 | `VALIDATION_ERROR` | 字段校验失败（body 带 `issues` 数组） |
| 400 | `INVALID_FIELD_FILTER` | 查询的 field_filters 格式错误 |
| 400 | `BAD_REQUEST` | 请求体格式错误、缺少必填字段 |
| 401 | `AUTH_FAILED` | API Key 无效或过期 |
| 404 | `NOT_FOUND` | 文档 ID 不存在 |
| 413 | `PAYLOAD_TOO_LARGE` | 上传内容超限 |
| 500 | `INTERNAL_ERROR` | 后端内部错误 |

### 2.4 issues 结构（校验类错误）
```json
{
  "field": "doc_no",
  "rule": "required",
  "message": "必填字段缺失"
}
```
常见 rule：`required`（必填）、`format`（格式不符）、`duplicate`（重复入库）、`range`（值越界）。

---

## 三、接口详细规格

### 3.1 上传归档 - `POST /documents`

把文档原件传给后端归档，拿到文档 ID。后续所有操作都用这个 ID。

**请求 body**（content_base64 / content_text / file_url 三选一）：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| content_base64 | string | 三选一 | 文件 base64 编码（图片/PDF等二进制文件） |
| mime_type | string | 条件必填 | 当 content_base64 时提供，如 `image/jpeg`、`application/pdf` |
| content_text | string | 三选一 | 纯文本内容（如转写的正文、法条文本） |
| file_url | string | 三选一 | 公网可访问的文件 URL（后端去拉取） |
| doc_type | string | 否 | 文档类型。不确定时不传，后续 save 时再定 |

**成功响应** `200 OK`：
```json
{
  "document_id": "d8f3a1b2-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "status": "uploaded"
}
```

**错误响应**：
- `400 BAD_REQUEST`：三种内容字段一个都没传
- `413 PAYLOAD_TOO_LARGE`：文件过大（建议限制 50MB）

**实现建议**：
- 二进制文件存对象存储（OSS/COS/S3），document_id 用 UUID
- file_url 模式：后端异步拉取，或要求客户端先下载再 base64 上传（取决于网络环境）
- 归档后文档状态为 `uploaded`，待 save 后才变为 `saved`

---

### 3.2 取字段清单 - `GET /documents/{id}/fields`

获取该文档要抽取的字段清单。如果文档类型未定，返回类型目录让调用方选择。

**路径参数**：`id` = 文档 ID

**成功响应** `200 OK`（两种情况）：

**情况 A - 类型未定，返回类型目录**：
```json
{
  "type": "catalog",
  "types": [
    {"name": "invoice", "label": "增值税发票"},
    {"name": "reimbursement", "label": "报销单"},
    {"name": "contract", "label": "合同"},
    {"name": "generic", "label": "通用单据"}
  ]
}
```

**情况 B - 类型已定，返回字段清单**：
```json
{
  "fields": [
    {"name": "doc_no", "label": "单据编号", "required": true, "aliases": ["发票号码", "票据号"]},
    {"name": "amount", "label": "金额", "required": true, "aliases": ["价税合计"]},
    {"name": "date", "label": "日期", "required": false, "aliases": ["开票日期"]}
  ]
}
```

**实现建议**：
- 文档类型在上传时已指定 → 直接返回该类型字段清单
- 文档类型未指定 → 返回 catalog，调用方（AI 模型）会判断类型后据该类型字段清单抽取
- 字段清单按 doc_type 配置，建议后端可配置化（不同公司字段不同）

---

### 3.3 预校验 - `POST /documents/{id}/validate`

入库前先校验字段，不入库。返回校验结果。

**路径参数**：`id` = 文档 ID

**请求 body**：
```json
{
  "doc_type": "invoice",
  "fields": {
    "buyer": "某某公司",
    "seller": "某某科技公司",
    "total_amount": "5000.00"
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| doc_type | string | 是 | 文档类型 |
| fields | object | 是 | 字段键值对，key 为字段名，value 为字符串 |

**成功响应** `200 OK`：

校验通过：
```json
{"status": "validated"}
```

校验有问题（需人工确认）：
```json
{
  "status": "needs_review",
  "issues": [
    {"field": "total_amount", "rule": "format", "message": "金额格式不符，应为数字"},
    {"field": "doc_no", "rule": "duplicate", "message": "该单据编号已存在"}
  ]
}
```

**实现建议**：
- 校验规则：必填检查、格式检查（金额数字、日期格式）、重复检查
- `needs_review` 不阻断流程，调用方可据 issues 修正后重新 validate 或直接 save（带 force）

---

### 3.4 入库 - `POST /documents/{id}/save`

把字段和正文写入数据库，完成入库。后端做权威校验。

**路径参数**：`id` = 文档 ID

**请求 body**：
```json
{
  "doc_type": "invoice",
  "fields": {
    "buyer": "某某公司",
    "seller": "某某科技公司",
    "total_amount": "5000.00"
  },
  "content_text": "完整正文内容...",
  "force": false
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| doc_type | string | 是 | 文档类型 |
| fields | object | 是 | 字段键值对 |
| content_text | string | 是 | 完整正文（用于语义检索索引） |
| force | boolean | 否 | 强制入库（跳过校验），默认 false |

**成功响应** `200 OK`：

入库成功：
```json
{"status": "saved"}
```

需人工确认：
```json
{
  "status": "needs_review",
  "issues": [...]
}
```

**实现建议**：
- `force=true` 时跳过校验直接入库
- `content_text` 要建全文索引 + 向量索引（供 ask 接口语义检索）
- 入库后文档状态变为 `saved`
- 字段值建议存为 JSON（不同 doc_type 字段不同，用 NoSQL 或 JSON 列）

---

### 3.5 结构化查询 - `POST /documents/query`

按条件查询已归档文档。支持类型/状态/关键词/上传人/字段级过滤。

> 用 POST 而非 GET：因为 `field_filters` 是结构化数组，放 body 比拼 query string 清晰。

**请求 body**：
```json
{
  "doc_type": "reimbursement",
  "status": "saved",
  "keyword": "差旅",
  "uploaded_by": "zhangsan",
  "limit": 20,
  "field_filters": [
    {"field": "amount", "op": "gt", "value": "10000"},
    {"field": "status", "op": "in", "values": ["saved", "needs_review"]}
  ]
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| doc_type | string | 否 | 文档类型过滤 |
| status | string | 否 | 状态过滤（uploaded/saved/needs_review） |
| keyword | string | 否 | 关键词，匹配文件名或正文 |
| uploaded_by | string | 否 | 上传人过滤 |
| limit | int | 否 | 返回条数上限，默认 20 |
| field_filters | array | 否 | 字段级过滤 |

**field_filters 元素结构**：

| 运算符 op | 字段 | 说明 |
|-----------|------|------|
| eq / ne | value | 等于 / 不等于 |
| gt / gte | value | 大于 / 大于等于 |
| lt / lte | value | 小于 / 小于等于 |
| contains | value | 包含（字符串子串匹配） |
| in | values（数组） | 在指定集合中 |

**成功响应** `200 OK`：
```json
{
  "documents": [
    {
      "id": "d8f3a1b2-xxxx",
      "filename": "发票_20260301.pdf",
      "doc_type": "invoice",
      "status": "saved",
      "uploaded_by": "zhangsan",
      "uploaded_at": "2026-03-01T10:30:00Z",
      "fields": {
        "buyer": "某某公司",
        "seller": "某某科技公司",
        "total_amount": "5000.00"
      }
    }
  ],
  "total": 1
}
```

**实现建议**：
- field_filters 里的字段比较：数字类做数值比较，字符串类做字典序比较
- keyword 用全文索引匹配
- 返回的 fields 是入库时存的原样字段，不裁剪

---

### 3.6 语义问答 - `POST /documents/ask`

对已归档文档正文做语义检索问答。返回相关原文片段。

**请求 body**：
```json
{
  "question": "违约金怎么约定的？",
  "doc_type": "contract",
  "limit": 5
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| question | string | 是 | 问题 |
| doc_type | string | 否 | 限定文档类型范围 |
| limit | int | 否 | 返回片段数上限，默认 5 |

**成功响应** `200 OK`：
```json
{
  "answers": [
    {
      "content": "第十二条 违约金：任何一方违反本合同约定，应向守约方支付违约金人民币伍万元整...",
      "source": "采购合同_2026.pdf",
      "document_id": "d8f3a1b2-xxxx",
      "score": 0.92
    }
  ]
}
```

**实现建议**：
- 基于 content_text 建向量索引（如用 embedding + 向量数据库）
- doc_type 限定时只在对应类型文档里检索
- score 是相似度评分（0~1），可选返回
- 如果向量索引尚未就绪（异步构建中），可降级为关键词检索，但要返回结果（不能空）

---

## 四、数据模型

### 4.1 文档状态（status）

| 状态 | 说明 |
|------|------|
| uploaded | 已上传归档，未入库 |
| saved | 已入库 |
| needs_review | 入库时校验有问题，待人工确认 |

### 4.2 文档类型（doc_type）

| 类型 | 说明 | 必填字段 |
|------|------|----------|
| invoice | 增值税发票 | buyer, seller, total_amount |
| reimbursement | 报销单 | title, doc_no, amount |
| contract | 合同 | title, party_a, party_b |
| generic | 通用单据 | title, doc_no |

> 字段清单可配置化。后端应支持管理员通过配置文件或管理后台增减字段。

### 4.3 各类型字段清单参考

详见专家包内 `skills/phoenix-api/references/doc-type-fields.md`。

---

## 五、实现检查清单

后端实现时逐项确认：

- [ ] `POST /documents` 支持 content_base64 / content_text / file_url 三种模式
- [ ] `GET /documents/{id}/fields` 能区分返回 catalog 或 fields
- [ ] `POST /documents/{id}/validate` 返回 validated 或 needs_review + issues
- [ ] `POST /documents/{id}/save` 支持 force 参数跳过校验
- [ ] `POST /documents/{id}/save` 的 content_text 建立全文索引 + 向量索引
- [ ] `POST /documents/query` 支持 field_filters 的 8 种运算符
- [ ] `POST /documents/ask` 基于向量检索返回原文片段
- [ ] 所有接口鉴权：`Authorization: Bearer {api_key}`
- [ ] 错误响应统一格式：`{"error":"CODE","message":"..."}`
- [ ] 文档 ID 用 UUID
- [ ] 字段值存储支持灵活 schema（JSON 列或 NoSQL）
- [ ] 大文件上传限制（建议 50MB）

---

## 六、与专家包的对接

### 6.1 专家包调用方式
专家包内置 `api_client.py`，调用示例：
```python
from api_client import ApiClient
client = ApiClient()
result = client.post('/documents', data={'content_text': '...', 'doc_type': 'invoice'})
```

### 6.2 配置
用户在首次对话时提供 `api_base_url` 和 `api_key`，存入专家包本地配置文件 `.config.json`：
```json
{
  "api_base_url": "https://docs.company.com/api/v1",
  "api_key": "xxx",
  "timeout": 60,
  "verify_ssl": true
}
```

### 6.3 联调顺序建议
1. 先实现 `POST /documents`（上传）+ `POST /documents/{id}/save`（入库）
2. 再实现 `POST /documents/query`（查询）
3. 最后实现 `POST /documents/ask`（语义问答，依赖向量索引，可后置）
4. `GET /documents/{id}/fields` 和 `POST /documents/{id}/validate` 可并行实现

---

## 七、附录：curl 测试示例

```bash
# 上传纯文本文档
curl -X POST https://docs.company.com/api/v1/documents \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"content_text":"测试文档内容","doc_type":"generic"}'

# 取字段清单
curl https://docs.company.com/api/v1/documents/d8f3a1b2/fields \
  -H "Authorization: Bearer YOUR_API_KEY"

# 入库
curl -X POST https://docs.company.com/api/v1/documents/d8f3a1b2/save \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"doc_type":"generic","fields":{"title":"测试","doc_no":"001"},"content_text":"正文内容","force":false}'

# 查询
curl -X POST https://docs.company.com/api/v1/documents/query \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"doc_type":"generic","limit":20}'

# 语义问答
curl -X POST https://docs.company.com/api/v1/documents/ask \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"question":"测试问题","limit":5}'
```
