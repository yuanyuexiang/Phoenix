# Phoenix Doc Expert(文档处理专家)

上传单据,自动识别、提取字段、校验并入库;支持查询历史文档。

## 类型

Agent 型(单个 AI 专家)

## 功能

基于 Phoenix 企业智能文档处理平台:AI 视觉模型识别扫描件/图片,AI 提取关键字段,
规则校验后写入企业数据库并归档原件。适用于行政、财务、合同、采购等单据录入场景。

- 文档上传与类型自动识别(未指明类型时平台自动分类,识别失败转人工)
- 字段智能提取(金额、日期、编号等,保留原始写法)
- 规则校验与人工审核分流(validated 直接入库 / needs_review 用户确认后入库)
- 数据入库、原件归档与多条件查询(类型/状态/关键词/上传人)

## 前置条件:phoenix 连接器

本专家依赖名为 **`phoenix`** 的 MCP 连接器(工具引用为 `mcp__phoenix__*`,
连接器名不一致则工具全部失效),需先在 WorkBuddy 中添加:

```json
{
  "mcpServers": {
    "phoenix": {
      "type": "streamable-http",
      "url": "https://phoenix.matrix-net.tech/mcp"
    }
  }
}
```

- 生产端点已启用 **OAuth 2.1 鉴权**:无需在连接器里配置密钥,首次调用会自动
  跳转 Keycloak 登录页,用管理员分配的员工账号登录一次即可,之后自动续期。
- 本机联调改用 `http://localhost:8080/mcp`(无鉴权),平台侧 `make infra-up && make run-all`。
- 详见仓库 `docs/WorkBuddy接入指南.md`。

## 使用示例

- 帮我录入这份合作确认单:(粘贴内容或附件)
- 这批发票提取一下金额和开票日期
- 查一下上个月所有待人工审核的单据

## 头像

头像已自动生成在 `avatars/` 目录下。如需替换为自定义头像,要求:
- 格式:PNG(推荐)或 JPG
- 尺寸:512×512 px
- 大小:单张不超过 500KB

## 安装

将专家包目录放到以下路径:

```
~/.workbuddy/plugins/marketplaces/my-experts/plugins/phoenix-doc-expert/
```

然后运行注册命令使其在 WorkBuddy 中可见:

```bash
python3 scripts/register_expert.py ~/.workbuddy/plugins/marketplaces/my-experts/plugins/phoenix-doc-expert/
```

## 打包分享

```bash
zip -r phoenix-doc-expert.zip phoenix-doc-expert/
```

## 变更记录

| 版本 | 日期 | 变更 |
|------|------|------|
| 1.0.1 | 2026-07-15 | 对齐平台提示词:doc_type 未指明时不传(自动分类),补 unknown 转人工、删除/覆盖引导后台原则;查询支持按上传人;README 补 OAuth 连接器前置条件 |
| 1.0.0 | 2026-07-15 | WorkBuddy 工具自动生成首版 |
