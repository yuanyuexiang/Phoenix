# 文档类型字段清单

> 记录各单据类型的字段定义，供模型识别时参考。

## invoice（增值税发票）

| 字段名 | 中文标签 | 必填 | 说明 |
|--------|----------|------|------|
| invoice_code | 发票代码 | 否 | 10位数字 |
| invoice_number | 发票号码 | 否 | 8位数字 |
| issue_date | 开票日期 | 否 | YYYY-MM-DD |
| buyer | 购买方名称 | 是 | |
| buyer_tax_id | 购买方税号 | 否 | |
| seller | 销售方名称 | 是 | |
| seller_tax_id | 销售方税号 | 否 | |
| total_amount | 价税合计 | 是 | 数字，保留两位小数 |
| amount_excluding_tax | 金额（不含税） | 否 | |
| tax_amount | 税额 | 否 | |
| items | 明细 | 否 | 数组，每项含商品名称、数量、单价、金额 |

## reimbursement（报销单）

| 字段名 | 中文标签 | 必填 | 说明 |
|--------|----------|------|------|
| title | 标题 | 是 | |
| doc_no | 单据编号 | 是 | |
| applicant | 报销人 | 否 | |
| department | 部门 | 否 | |
| amount | 金额 | 是 | 数字，保留两位小数 |
| expense_date | 报销日期 | 否 | YYYY-MM-DD |
| category | 费用类别 | 否 | 如差旅、办公、招待 |
| status | 审批状态 | 否 | |

## contract（合同）

| 字段名 | 中文标签 | 必填 | 说明 |
|--------|----------|------|------|
| title | 合同名称 | 是 | |
| contract_no | 合同编号 | 否 | |
| party_a | 甲方 | 是 | |
| party_b | 乙方 | 是 | |
| sign_date | 签订日期 | 否 | YYYY-MM-DD |
| effective_date | 生效日期 | 否 | YYYY-MM-DD |
| expire_date | 到期日期 | 否 | YYYY-MM-DD |
| total_value | 合同金额 | 否 | |
| currency | 币种 | 否 | 默认CNY |

## generic（通用单据）

| 字段名 | 中文标签 | 必填 | 说明 |
|--------|----------|------|------|
| title | 标题 | 是 | |
| doc_no | 单据编号 | 是 | |
| issue_date | 日期 | 否 | YYYY-MM-DD |
| issuer | 出具方 | 否 | |
| amount | 金额 | 否 | |
| notes | 备注 | 否 | |
