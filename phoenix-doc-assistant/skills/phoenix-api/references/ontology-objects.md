# 本体对象类型速查(objects.py 用)

> 对象 = 跨文档归一后的业务实体;文档是对象的**证据**。对象由单据入库自动物化,
> 同一实体在所有单据中解析为同一对象(公司按税号/名称、发票按发票号、员工按账号/姓名)。
> 完整定义在后端 `configs/ontology/*.yaml`,新增类型后本速查需同步。

## 对象类型

| type | 名称 | 关键属性 | 出链(links_out) |
|------|------|----------|------------------|
| `company` | 公司 | name, tax_no | — |
| `employee` | 员工 | name, username(平台账号) | — |
| `invoice` | 发票 | invoice_no, issue_date, total_amount, tax_amount | seller/buyer → company |
| `contract` | 合同 | doc_no, title, amount, issue_date | party_a/party_b → company |
| `reimbursement` | 报销单 | doc_no, title, total_amount, apply_date, check_result | submitted_by/approved_by → employee;includes → invoice |
| `settlement` | 费用结算单 | doc_no, title, settlement_amount, form_date | submitted_by → employee |
| `loan` | 借款单 | doc_no, title, total_amount, doc_date | submitted_by → employee |

## 常用问法 → 查询

| 用户问题 | 调用 |
|----------|------|
| "中石化开过哪些发票" | `--type company --keyword 中石化` 拿 ID → `--id {ID}` 看 links_in(seller) |
| "金额超 1 万的报销单" | `--type reimbursement --property-filter 'total_amount,gt,10000'` |
| "黄智超报了几笔" | `--type employee --keyword 黄智超` → 详情 links_in(submitted_by) |
| "自洽校验没过的报销单" | `--type reimbursement --property-filter 'check_result,contains,差'` |

## 读详情要点

- `links_out`/`links_in`:关系两端带 display 名,`document_id` 是产生该关系的单据
- `documents`:证据单据列表(该对象出现在哪些单据里)
- 同一发票被多张报销单 `includes` 引用 = 疑似重复报销,应提醒用户
- 数值属性已归一(无千分位),日期为 ISO(YYYY-MM-DD),过滤直接比较
