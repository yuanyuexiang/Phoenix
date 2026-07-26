#!/usr/bin/env python3
"""实体(对象)查询  →  GET /pub/v1/objects | /pub/v1/objects/{id}

对象是跨文档归一后的业务实体(公司/发票/合同/报销单/员工),回答
"这家供应商所有单据/总额""某员工报了几笔"这类实体/关系/聚合问题
比文档查询更准确。文档是对象的证据,详情里可拿到关联关系与证据单据。

用法(列表):
  --type {对象类型}              可选:company/invoice/contract/reimbursement/employee/settlement/loan
  --keyword {关键词}             可选,匹配对象展示名(如公司名)
  --property-filter {属性,运算符,值}  可选,可多次传。运算符 eq/ne/gt/gte/lt/lte/contains/in(值用 | 分隔)
  --limit 50                     可选
返回:{"total":N,"objects":[{id,object_type,display_name,properties,...}]}

用法(详情):
  --id {对象ID}
返回:{"object":{...},"type_title":"公司","links_out":[...],"links_in":[...],"documents":[证据单据...]}
"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def parse_property_filter(filter_str):
    parts = filter_str.split(',', 2)
    if len(parts) != 3:
        print(json.dumps({"error": "INVALID_FIELD_FILTER", "message": f"格式错误: {filter_str}，应为 属性,运算符,值"}))
        sys.exit(1)
    field, op, value = parts
    if op == 'in':
        return {"field": field, "op": "in", "values": value.split('|')}
    return {"field": field, "op": op, "value": value}


def main():
    parser = argparse.ArgumentParser(description='查询本体对象(跨文档归一的业务实体)')
    parser.add_argument('--id', default=None, help='对象 ID(详情模式,含关联关系与证据单据)')
    parser.add_argument('--type', default=None, help='对象类型')
    parser.add_argument('--keyword', default=None, help='关键词(匹配对象展示名)')
    parser.add_argument('--limit', type=int, default=50, help='返回条数上限')
    parser.add_argument('--property-filter', action='append', default=[], help='属性过滤,格式:属性,运算符,值')
    args = parser.parse_args()

    client = ApiClient()
    if args.id:
        result = client.get(f'/pub/v1/objects/{args.id}')
    else:
        params = {'limit': args.limit}
        if args.type:
            params['type'] = args.type
        if args.keyword:
            params['keyword'] = args.keyword
        if args.property_filter:
            params['property_filters'] = json.dumps(
                [parse_property_filter(f) for f in args.property_filter], ensure_ascii=False)
        result = client.get('/pub/v1/objects', params=params)
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    main()
