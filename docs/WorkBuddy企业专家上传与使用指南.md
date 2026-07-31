# WorkBuddy 企业专家上传与使用指南

本文用于将 Phoenix 文档助手作为“企业自建专家”发布给本企业成员。管理员负责上传、启用和配置权限；员工无需手工导入 ZIP，只需在 WorkBuddy 专家中心找到并召唤专家。

参考：[WorkBuddy Enterprise 专家管理](https://cloud.tencent.com/document/product/1831/134421)、[WorkBuddy 专家使用说明](https://cloud.tencent.com/document/product/1831/134393)。页面名称可能随客户端版本略有变化。

## 1. 发布前准备

### 1.1 Phoenix 服务端

确认以下条件：

- `https://phoenix.matrix-net.tech/healthz` 可访问，或已将专家包模板中的 `api_base_url` 改为本企业 Phoenix 地址。
- workflow 已设置强随机 `PHX_AUTH_SECRET`，`/pub/v1` 已启用。
- 管理员已在 Phoenix 管理后台“员工”页面创建测试账号。
- HTTPS 证书有效；生产环境不要关闭证书校验。

### 1.2 专家基本信息

建议填写：

| 项目 | 推荐值 |
|---|---|
| 专家标识 | `phoenix-doc-assistant` |
| 显示名称 | Phoenix 文档助手 |
| 分类 | 行业顾问 / 企业效率 / 财务与档案 |
| 版本 | 与 `.codebuddy-plugin/plugin.json` 一致，如 `2.0.1` |
| 描述 | 识别、校验、归档和查询企业文档，并基于 Ontology 关联业务对象 |
| 头像 | `avatars/expert.png` |

> 专家标识创建后不可修改。正式环境首次上传前应确认命名。

## 2. 生成安全的专家 ZIP

不要直接压缩当前工作目录。`.config.json` 可能含员工 Token，必须排除。

在仓库根目录执行：

```bash
mkdir -p dist
zip -r "dist/phoenix-doc-assistant-2.0.1.zip" phoenix-doc-assistant \
  -x "*/.config.json" "*/__pycache__/*" "*.pyc" "*/.DS_Store" "*/test_*.py"
shasum -a 256 "dist/phoenix-doc-assistant-2.0.1.zip"
```

上传前检查内容：

```bash
unzip -l "dist/phoenix-doc-assistant-2.0.1.zip"
```

必须包含：

```text
phoenix-doc-assistant/.codebuddy-plugin/plugin.json
phoenix-doc-assistant/agents/phoenix-doc-assistant.md
phoenix-doc-assistant/skills/phoenix-api/SKILL.md
phoenix-doc-assistant/skills/doc-tamper-check/SKILL.md
phoenix-doc-assistant/avatars/expert.png
```

不得包含 `.config.json`、Token、`.env`、`__pycache__`、`.pyc` 或 `.DS_Store`。

## 3. 管理员上传企业专家

1. 使用企业管理员账号登录 WorkBuddy Enterprise 管理端。
2. 进入 **AI 资源管理 → 专家管理**。
3. 单击 **+ 上传企业专家**。
4. 填写头像、专家标识、显示名称、分类、版本和描述。
5. 在“上传专家包”区域选择生成的 `.zip` 文件。
6. 配置可见范围：
   - 试点阶段：选择“部分成员”，仅开放给管理员、财务审核员和测试员工。
   - 验收完成后：再扩大到指定部门或所有成员。
7. 首次建议先“仅保存草稿”，检查信息无误后再单击“保存并启用”。
8. 回到专家列表，确认状态为已启用、版本正确、可见范围符合预期。

涉及企业文档和外部 Phoenix 服务调用，建议使用白名单策略。若同时配置黑名单和白名单，WorkBuddy 以黑名单优先。

## 4. 员工在 WorkBuddy 中找到并使用

员工必须使用已加入同一企业的账号登录 WorkBuddy。

1. 打开 WorkBuddy。
2. 在左侧边栏进入 **专家·技能·连接器**（部分版本显示为“专家中心”）。
3. 切换到企业专家或企业市场分类。
4. 搜索 **Phoenix 文档助手** 或专家标识 `phoenix-doc-assistant`。
5. 打开专家卡片，查看能力说明和推荐问题。
6. 单击 **召唤**，进入使用该专家的新对话。
7. 首次操作时，专家会打开浏览器要求登录 Phoenix 员工账号。
8. 登录成功后会自动切回 WorkBuddy；若浏览器阻止跳转，单击“返回 WorkBuddy”。
9. 可从以下任务开始验证：
   - “上传归档这份发票”并附加图片或 PDF。
   - “查询金额超过一万元的报销单”。
   - “查看某公司的相关合同和发票”。

企业专家由管理员下发，通常不需要员工再次上传或安装 ZIP；员工看到专家卡片后直接召唤即可。

## 5. 验收清单

- 测试成员能搜索到专家，非授权成员看不到。
- 专家头像、名称、版本和简介显示正确。
- 首次登录能自动返回 WorkBuddy。
- 上传、字段提取、校验、入库、查询和对象查询均成功。
- Phoenix 管理后台能看到正确的 `uploaded_by`、`reviewed_by` 和审计记录。
- 禁用员工或重置口令后，旧 Token 立即失效。
- ZIP 中不包含任何测试账号、Token 或生产密钥。

## 6. 更新、停用与排障

更新专家时先递增版本号，重新生成干净 ZIP，再在企业管理端上传新版本。建议先对白名单测试成员发布，验证通过后扩大范围。紧急情况下可关闭专家；关闭状态独立于下发策略，关闭后所有成员都无法使用。

员工找不到专家时依次检查：

1. 是否登录了正确的企业账号。
2. 专家是否已“保存并启用”，而非仅保存草稿。
3. 员工是否在可见范围或白名单中，是否命中黑名单。
4. WorkBuddy 是否已刷新专家列表；必要时重启客户端。
5. 专家标识、分类和搜索关键词是否正确。

登录或调用失败时检查 Phoenix `/healthz`、HTTPS 证书、`PHX_AUTH_SECRET`、员工账号状态以及专家包内的 `api_base_url`。不要通过聊天、截图或日志发送口令和 Token。
