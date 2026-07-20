# 后端 REST API 接口文档（/pub/v1）

> 给开发者/模型看：记录专家包调用的后端 `/pub/v1` 端点。**这是已实现的真实契约**
> (后端 `backend/internal/restapi`),与老的 `/api/*`(X-Access-Key,前端/mcp 用)独立。

## 基础信息
- 协议：标准 HTTP REST（JSON over HTTPS）
- 鉴权：`Authorization: Bearer {access_token}`(Keycloak 签发,aud=phoenix-api;由 auth.py 管理)
- Base URL：配置文件 `api_base_url` + `/pub/v1`(如 `https://phoenix.matrix-net.tech`)
- Content-Type：`application/json`

## 错误响应格式

HTTP 非 2xx,body 为 `{"error":"CODE","message":"..."}`:

| HTTP | error | 说明 |
|------|-------|------|
| 401 | AUTH_FAILED | 缺 token / token 无效或过期 → 重新登录 |
| 400 | BAD_REQUEST | 请求体格式错误 / 三种内容字段未恰好给一个 |
| 400 | INVALID_FIELD_FILTER | 查询过滤格式错误 |
| 422 | UNPROCESSABLE | 文档不存在 / 类型未配置等 |

> 注:字段校验不通过不是 HTTP 错误,而是正常 200 返回 `status=needs_review` + `issues`(见 save/validate)。

## 接口列表

### 0. GET /pub/v1/me（当前员工身份）
响应:`{"sub","username","email","name","display"}`

### 1. POST /pub/v1/documents（上传归档）
请求 body（content_base64 / content_text / file_url 三选一）:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| content_base64 | string | 三选一 | 文件 base64（图片/PDF等二进制） |
| content_text | string | 三选一 | 纯文本内容 |
| file_url | string | 三选一 | 公网可访问的文件 URL（后端下载） |
| filename | string | 否 | 归档显示文件名 |
| doc_type | string | 否 | 文档类型,不确定时不传 |

响应:DocumentView,含 `"id"`(后续操作用它)、`"status":"uploaded"`、`"uploaded_by"`。

### 2. POST /pub/v1/documents/{id}/extract（取字段清单）
无 body。响应 FieldBrief:
- 类型未定:`{"doc_type":"auto","catalog":[{"name","title","description"}...]}`
- 类型已定:`{"doc_type","title","fields":[{"name","label","aliases","required","pattern","enum"}...]}`

### 3. POST /pub/v1/documents/{id}/validate（预校验,不入库）
请求 body:`{"doc_type": "...", "fields": [{"name","value"}...]}`
响应:DocumentView,`status=validated` 或 `needs_review`+`issues`。不落库。

### 4. POST /pub/v1/documents/{id}/save（入库）
请求 body:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| doc_type | string | 是 | 文档类型 |
| fields | array | 是 | `[{"name","value"}...]`(客户端脚本把对象自动转数组) |
| content_text | string | 是 | 完整正文(供检索/知识库) |
| force | boolean | 否 | 强制入库(跳过校验),默认 false |

响应:DocumentView,`status=saved` 或 `needs_review`+`issues`;含 `reviewed_by`。

### 5. GET /pub/v1/documents（结构化查询）
query string 参数:`doc_type` / `status` / `keyword` / `uploaded_by` / `limit` /
`field_filters`(URL 编码的 JSON 数组,每条 `{field, op, value/values}`)。
运算符:`eq`/`ne`/`gt`/`gte`/`lt`/`lte`/`contains`/`in`。
响应:`{"total":N,"documents":[DocumentView...]}`。

### 6. POST /pub/v1/ask（语义问答）
请求 body:`{"question","doc_type"?,"limit"?}`
响应:`{"total":N,"chunks":[{"document_id","filename","doc_type","content","score"}...]}`。
(知识库未配置 embedding 时返回错误提示"未启用"。)

## 常见文档类型（doc_type,以后端 configs/doctypes/*.yaml 为准）
| 类型 | 说明 |
|------|------|
| generic | 通用单据 |
| invoice | 发票 |

> 字段清单以 extract 返回的 FieldBrief 为准(后端可配置化,不同公司字段不同)。
