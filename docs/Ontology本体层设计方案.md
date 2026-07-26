# Phoenix 本体层(Ontology)设计方案

> 目标版本 V1.4。**P0+P1 已实施(2026-07-26):字段类型化、reimbursement 单据类型、
> 本体加载/物化/重建、objects 端点、前端对象页与审核台联动、smoke 断言全绿。**
> **P4 专家包接入已实施(2026-07-27)**:objects.py 命令、agent MD 三选一路由、
> extract 主体字段 entity 标记、ask 命中附实体摘要。P2(人工合并队列)/P3(Action)待启动。践行 Palantir Ontology 的**模式**而非复刻 Foundry:
> 对象/链接/Action 作为语义层压在现有文档流水线之上,沿用 Phoenix 三条基因 ——
> **一切皆 YAML 配置、Postgres 承载、REST 以新增端点扩展契约**(既有 7 个端点不动)。

## 1. 背景与目标

现状是**文档中心**:一切皆 Document,字段是挂在文档上的字符串。两份单据里的
"凤凰软件服务有限公司"互不相识,跨文档的问题("和这家供应商今年合作总金额?")
只能靠 RAG 碰运气。

本体层把世界观换成**实体中心**:文档降格为**证据(evidence)**,一等公民是文档里
提到的东西 —— 公司、发票、合同。目标:

1. **跨文档实体归一**:同一家公司在所有单据中解析到同一个对象
2. **类型化关系**:发票 —销售方→ 公司;发票 —结算→ 合同;可沿关系查询与聚合
3. **确定性回答**:实体/关系/聚合类问题走图查询,RAG 只负责开放式正文问答
4. **AI 的受治理视野**:专家经对象/链接读、经 Action 写(Phase 3),而非裸操作数据

## 2. 概念模型与 Phoenix 对应

| 概念 | 定义 | Phoenix 落点 |
|------|------|------|
| Object | 业务实体实例(某家公司、某张发票) | `objects` 表,类型由 YAML 定义 |
| Object Type | 实体类型(属性 schema + 归一键) | `configs/ontology/*.yaml` |
| Property | 带类型的属性(string/number/date/enum) | 归一化后存 JSONB |
| Link | 类型化关系(发票→公司) | `links` 表 |
| Evidence | 对象/关系的文档出处 | `object_evidence` 表 → documents |
| Action | 受治理的写操作(校验+权限+审计) | Phase 3,YAML 定义 + REST 端点 |

**文档与对象的关系**:文档处理流水线(上传→识别→校验→入库)完全不变;
`save` 成功后**追加一步物化**——按映射规则从文档字段生成/更新对象与链接,
并记录证据。对象永远可以回答"你是从哪些单据来的"。

## 3. 本体定义层(configs/ontology/*.yaml)

### 3.1 对象类型示例一:公司(company.yaml)

```yaml
# 对象类型:公司(供应商/客户/甲乙方的归一实体)
name: company
title: 公司
display_property: name          # 列表/链接展示用哪个属性
properties:
  - name: name
    label: 公司名称
    type: string
    required: true
  - name: tax_no
    label: 纳税人识别号
    type: string                # 统一社会信用代码/税号
  - name: contact
    label: 联系方式
    type: string
# 实体归一键:按顺序尝试。命中即视为同一对象(upsert);
# 都未命中 → 新建;键冲突(同 tax_no 不同名)→ 进对象审核队列(Phase 2)
resolution_keys:
  - [tax_no]                    # 有税号,精确归一
  - [name]                      # 否则按归一化名称(去空格/全半角统一)
```

### 3.2 对象类型示例二:发票(invoice_object.yaml)

```yaml
name: invoice
title: 发票
display_property: invoice_no
properties:
  - name: invoice_no
    label: 发票号码
    type: string
    required: true
  - name: issue_date
    label: 开票日期
    type: date                  # 物化时归一为 ISO(2026-07-01)
  - name: total_amount
    label: 价税合计
    type: number                # 物化时去千分位、转数值
  - name: tax_amount
    label: 税额
    type: number
resolution_keys:
  - [invoice_no]                # 发票号码天然唯一
links:                          # 该类型作为起点的关系
  - name: seller
    label: 销售方
    to: company
  - name: buyer
    label: 购买方
    to: company
```

### 3.3 对象类型示例三:报销单 + 员工(本体价值最高的场景)

> **前置缺口**:专家 agent MD 与防篡改技能已在使用 `reimbursement` 类型,但
> `configs/doctypes/` 尚未配置该单据类型(现在真入库会报"类型未配置")。
> P0 需先补 `doctypes/reimbursement.yaml`(编号/报销人/事由/总金额/日期/
> 所附发票号/审批人/check_result 防篡改结论),再谈其本体。

```yaml
# employee.yaml —— 员工对象:把"单据上的人"与"登录的人"接到同一个实体
name: employee
title: 员工
display_property: name
properties:
  - { name: name,     label: 姓名,     type: string, required: true }
  - { name: username, label: 平台账号, type: string }  # 对应 users.username
resolution_keys:
  - [username]        # 有账号精确归一(uploaded_by 可自动回填)
  - [name]            # 单据上只有姓名时按姓名归一(同名风险 → 待确认口径)
```

```yaml
# reimbursement_object.yaml —— 报销单对象
name: reimbursement
title: 报销单
display_property: doc_no
properties:
  - { name: doc_no,       label: 单据编号, type: string, required: true }
  - { name: title,        label: 报销事由, type: string }
  - { name: total_amount, label: 报销总额, type: number }
  - { name: apply_date,   label: 申请日期, type: date }
  - { name: check_result, label: 自洽校验结论, type: string }  # doc-tamper-check 产出
resolution_keys:
  - [doc_no]
links:
  - { name: submitted_by, label: 报销人, to: employee }
  - { name: approved_by,  label: 审批人, to: employee }
  - { name: includes,     label: 所附发票, to: invoice }   # 一单多票
sources:
  - doc_type: reimbursement
    map:
      doc_no: doc_no
      title: title
      total_amount: total_amount
      apply_date: apply_date
      check_result: check_result
    link_map:
      applicant:   { link: submitted_by, to: employee, property: name }
      approver:    { link: approved_by,  to: employee, property: name }
      invoice_nos: { link: includes, to: invoice, property: invoice_no, multi: true }
      # multi: 值为分隔列表(如 "04431877;04431902"),逐个归一建链
```

这一组的本体红利,纯文档模型给不了:

- **重复报销检测(免费得到)**:两张报销单 `includes` 同一张发票,查发票对象的
  入链数 >1 即告警 —— 图上的一次计数,不需要任何新算法;可做成对象页提示与巡检查询
- **人企打通**:`员工 ←submitted_by— 报销单 —includes→ 发票 —seller→ 公司`,
  "杜永强今年在凤凰软件报了多少"是一条链路查询
- **防篡改结论落对象**:`check_result` 从埋在正文里的段落升级为可过滤属性
  ("列出所有自洽校验未通过的报销单")

### 3.4 抽取映射:文档 → 对象(在本体文件中按来源单据类型声明)

```yaml
# invoice_object.yaml 续 —— 从哪些单据类型物化本对象
sources:
  - doc_type: invoice           # 对应 configs/doctypes/invoice.yaml
    map:                        # 文档字段 → 对象属性
      invoice_no:   invoice_no
      issue_date:   issue_date
      total_amount: total_amount
      tax_amount:   tax_amount
    link_map:                   # 文档字段 → 关联对象(按对方 resolution_keys 归一)
      seller: { link: seller, to: company, property: name }
      buyer:  { link: buyer,  to: company, property: name }
```

> 通用单据(generic)同理可物化 Contract 对象并链接甲方/乙方公司;
> 新增一种实体或关系 = 加/改一个 YAML,与 doctypes 的哲学一致,不改代码。

### 3.5 Action 定义(Phase 3 形态预览)

```yaml
# company.yaml 续
actions:
  - name: mark_blocked
    label: 列入黑名单
    params:
      - { name: reason, label: 原因, type: string, required: true }
    sets: { blocked: true }     # 简单赋值型;复杂逻辑挂 Go 处理器注册表
    audit: true                 # 强制;操作人/参数/前后值入 audit_log
```

## 4. 存储设计(迁移 0005,幂等)

```sql
CREATE TABLE IF NOT EXISTS objects (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    object_type  TEXT NOT NULL,              -- company / invoice / ...
    display_name TEXT NOT NULL DEFAULT '',
    properties   JSONB NOT NULL DEFAULT '{}',-- 已按声明类型归一化的值
    version      INT NOT NULL DEFAULT 1,     -- 每次物化更新 +1(对象级变更史入 audit)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_objects_type ON objects (object_type);
CREATE INDEX IF NOT EXISTS idx_objects_props ON objects USING gin (properties);

-- 归一键:实体解析的核心。key_hash = 键名+归一化键值
CREATE TABLE IF NOT EXISTS object_keys (
    object_id  UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    key_hash   TEXT NOT NULL,
    UNIQUE (key_hash)
);

CREATE TABLE IF NOT EXISTS links (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    link_type   TEXT NOT NULL,               -- seller / buyer / party_a ...
    from_object UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    to_object   UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    document_id UUID NOT NULL,               -- 溯源:该关系由哪份文档产生。
                                             -- 文档修正/删除时按此精确撤销;
                                             -- 重复报销检测 = 同 to_object 的
                                             -- includes 链接来自多个 from/文档
    UNIQUE (link_type, from_object, to_object, document_id)
);
CREATE INDEX IF NOT EXISTS idx_links_doc ON links (document_id);

-- 证据:对象(或其某次更新)来自哪份文档
CREATE TABLE IF NOT EXISTS object_evidence (
    object_id   UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    document_id UUID NOT NULL,
    UNIQUE (object_id, document_id)
);
```

**类型化前置**:doctype 字段规则增加可选 `type: number|date`(默认 string),
`validate.Run` 校验通过后即归一化(金额去千分位转数值、日期转 ISO)再落
`documents.fields` 与对象属性 —— 现有字段过滤的"数值比较去逗号 hack"随之退役。

## 5. 物化流程(pipeline 内,同事务)

```
save 权威校验通过 → 落 documents(现状)
  └→ 物化:遍历命中 sources 的本体定义
       1. 属性归一化(类型转换)
       2. 按 resolution_keys 逐组求 key_hash → 查 object_keys
          命中 → 更新对象(version+1);未命中 → 新建对象+键
          键冲突(hash 命中但关键属性明显不符)→ 只记 evidence,标记待人工合并(Phase 2)
       3. link_map 解析关联对象(同样走归一)→ upsert links
       4. 写 object_evidence;audit_log 记 object_upsert(操作人沿用文档上传人)
```

失败策略:物化异常**不阻断入库**(文档层是权威),记 audit 告警,可重放
(对象层可随时由 documents 全量重建 —— 这也是"文档=证据"的架构红利)。

### 5.1 修正/删除/重建的一致性(文档层是权威,对象层永远可跟随)

- **重新 save(审核修正字段)**:物化前先按 `links.document_id` 删除该文档
  此前贡献的全部链接,再整体重放 —— 抽错的关联(错的 seller)被自然纠正;
  evidence 保留(该文档仍是对象的出处)
- **删除文档**:级联删除其 links(按 document_id)与 object_evidence;
  **对象本身保留**(历史实体真实存在过),证据数归零的对象在 UI 标记
  「无在库证据」,由人工决定清理
- **全量重建**:管理端点 `POST /api/ontology/rebuild`(X-Access-Key):清空
  objects/object_keys/links/object_evidence 后按全部 saved 文档重放,幂等。
  本体 YAML 大改(换归一键、改映射)后一键让对象层追上新配置
- **并发**:两个 save 同时新建同一实体 → `object_keys` UNIQUE 兜底,
  冲突方捕获后转为对已有对象 upsert(一次重试)
- **配置 fail-fast**:启动时校验本体 YAML(link.to 类型存在、map/link_map
  引用的字段在对应 doctype 中存在、resolution_keys 引用的属性已声明),
  配置错误直接拒绝启动 —— 与 schema.Registry 同一策略

## 6. API 设计(新增端点,既有契约不动)

| 端点 | 说明 |
|------|------|
| `GET /pub/v1/objects` | 按 `type`/`keyword`/`property_filters`(复用 field_filters 语法)查对象 |
| `GET /pub/v1/objects/{id}` | 对象详情:属性 + 出链入链(含对端 display)+ 证据文档列表 |
| `GET /pub/v1/objects/{id}/documents` | 该对象的全部证据单据(直接复用 query 视图) |
| `/api/objects*` | 管理面同形端点,供后台「对象」页使用 |
| `POST /pub/v1/objects/{id}/actions/{name}` | Phase 3:执行 Action(权限+校验+审计) |

## 7. WorkBuddy 专家包配合设计

原则:**写入侧零感知,读取侧新工具+路由规则,治理边界不变**。

### 7.1 写入侧(专家无需改动即兼容)

物化在后端 save 之后发生,专家照旧 upload→extract→识别→save。两个增强:

- **抽取提示**:extract 字段清单中主体字段带 `entity: <object_type>` 标记,
  agent MD 加规范"主体字段抄完整规范名称,不用简称"——归一质量在源头提升
- **物化摘要回传**:save 响应附物化结果("已关联公司「××」,与它的第 3 份单据"),
  专家转述,用户即时感知归一、当场发现归一错误;**报销单所附发票已被其他
  报销单引用时,save 返回重复报销警告**,专家当场提醒(防篡改从单据内自洽
  延伸到跨单据图检查)

### 7.2 读取侧(新命令 + 三选一路由)

新增 `commands/objects.py`(调 /pub/v1/objects*)。agent MD 的核心是路由决策表:

| 用户问题类型 | 工具 | 例 |
|---|---|---|
| 单据检索(字段/状态/类型) | query.py(文档层) | 上月待审核的报销单 |
| 实体/关系/聚合 | objects.py(对象层) | 凤凰软件今年所有单据与总额 |
| 开放式正文理解 | ask.py(RAG) | 合同付款周期怎么约定 |

模糊问题两段式:先 objects 锁定实体(多候选让用户选)→ 沿关系拉关联单据 →
需要正文细节再 ask。ask 命中片段附所属文档的对象摘要,回答自带实体全景。

### 7.3 治理边界(读写不对称)

- 对象查询开放;对象合并/修正/删除**不开放**给专家,引导管理后台人工(与文档删除同策略)
- Phase 3 后专家才获得写能力:仅限执行 YAML 定义的 Action,权限按登录员工,全程审计
- 身份呼应:employee 对象经 username 与登录账号打通;报销人与当前登录员工
  不一致时专家提示确认(代报销显式化)

### 7.4 专家包改动清单(对应 P4)

`commands/objects.py` · agent MD 增"实体查询"阶段+路由表 · `references/`
新增对象类型速查 · `phoenix-api-docs.md` 补 objects 端点契约。

## 8. 管理后台(前端配合改动)

前端改动集中在**新增页面**,现有页面只做轻量联动;沿用三层主题 token 与
现有组件写法(`lib/api.ts`/`types.ts` 扩展对象类型与端点封装),无新依赖。

### 8.1 新增「对象」页(P1,NavRail 第 5 项)

- 列表:对象类型 tabs(来自本体 YAML)+ display/关键属性列 + 关键词与
  property_filters 筛选(复用文档列表的筛选交互)
- 详情:属性卡(带类型渲染:金额右对齐 tabular、日期归一显示)/
  出链入链列表(对端 display 可点击跳转;关系图可视化后置,先列表)/
  证据单据列表(点击跳文档)/ 变更历史(audit 中该对象的 object_upsert 记录)
- 「无在库证据」对象的标记与筛选(见 §5.1 删除策略)

### 8.2 现有页面联动(P1,小改)

- **文档列表 & 审核台**:文档行/详情展示其物化出的对象 chips
  (如 `公司:凤凰软件` `发票:04431877`),点击跳对象详情 —— 双向可达
- **审核台提示**:修正字段重新入库时,提示"关联对象将同步更新"(§5.1 重放语义)
- 重复报销警告:审核台与对象页均呈现(发票对象被多张报销单引用)

### 8.3 合并队列页(P2)

键冲突的候选重复对象:并排属性对比 + 冲突原因 + 合并/保留操作
(合并前快照入 audit,复用审核台"队列+编辑区"交互模式)。

### 8.4 重建入口(P1,放「服务状态」页)

「重建对象层」按钮(调 /api/ontology/rebuild,二次确认+进度反馈),
供本体 YAML 大改后使用。

## 9. 分阶段实施

| 阶段 | 内容 | 预估 |
|------|------|------|
| P0 前置 | doctype 字段类型化 + 归一化落库(独立价值:字段过滤不再 hack);**补 reimbursement 单据类型**(修复专家已引用但后端未配置的缺口) | 0.5~1 天 |
| P1 | 本体 YAML 加载器 + 0005 迁移 + save 物化 + objects 查询端点 + company/invoice/contract/reimbursement/employee 五个本体 | 2~2.5 天 |
| P2 | 归一冲突人工合并队列(后端 + §8.3 前端) | 1~1.5 天 |

> P1 已含前端:「对象」页、文档/审核页联动、重建入口(§8.1/8.2/8.4,约占 P1 的 1 天)。

**各阶段验证**:P0 —— 现有测试全绿 + 字段过滤对数值/日期类型断言;
P1 —— smoke 扩展:save 后断言对象/链接/证据生成、修正重放后旧链接消失、
两张报销单引同一发票触发重复警告;P2 —— 构造键冲突样例走人工合并全流程。
| P3 | Action 框架(YAML 定义 + 执行端点 + 权限/审计) | 1~1.5 天,**建议等真实业务动作需求出现再做** |
| P4 | 专家包配合(objects.py 命令 + 路由规范 + save 物化摘要转述 + ask 对象上下文,见 §7) | 0.5~1 天 |

P0+P1 即可兑现核心价值(跨文档实体查询);P2 让它可运营;P3/P4 按需推进。

## 10. 风险与待确认

- **归一口径是客户决策**:公司以税号还是名称为准、名称清洗规则(分/子公司算不算同一实体)、员工同名如何区分——列入说明书【待确认】
- **现存缺口**:专家 agent MD/防篡改技能已使用 `reimbursement` 类型,但后端 doctypes 未配置,当前真入库会报错 —— P0 修复,与本体无关也应尽快做
- **说明书修订**:本方案属 V1.4 产品演进,§8.1 以"新增端点"方式扩展,须加修订记录
- **重复合并的破坏性**:人工合并对象要可回溯(audit 记合并前快照),不做级联删除
- **性能**:对象量 = 实体数(远小于文档数),JSONB+GIN 足够;真到图遍历深查询再考虑递归 CTE,仍不需要图数据库
- **权限口径**:沿用现状 —— 登录员工可见全部对象与关系,**不做行级/属性级权限**
  (Palantir 有,我们明确不做;客户提出数据隔离需求时再议,列入【待确认】)
- **不做的事**:不引入图数据库、不做本体版本迁移系统(YAML 改动向前兼容:新属性可空,
  破坏性改动走 rebuild)、不做跨类型继承、不做行级权限
