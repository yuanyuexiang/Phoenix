#!/usr/bin/env python3
"""结构化查询  →  GET /pub/v1/documents
用法：
  --doc-type {类型}          可选
  --status {状态}            可选(uploaded/saved/needs_review)
  --keyword {关键词}         可选(匹配文件名或正文)
  --uploaded-by {上传人}     可选
  --limit 20                 可选，默认20
  --field-filter {字段,运算符,值}  可选，可多次传。运算符：eq/ne/gt/gte/lt/lte/contains/in
                                   in 运算符的值用 | 分隔，如 'status,in,saved|needs_review'
返回:{"total":N,"documents":[DocumentView...]}
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
    if op == 'in':
        return {"field": field, "op": "in", "values": value.split('|')}
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

    # 后端 /pub/v1/documents 是 GET,过滤条件走 query string;field_filters 作为 JSON 字符串参数。
    params = {'limit': args.limit}
    if args.doc_type:
        params['doc_type'] = args.doc_type
    if args.status:
        params['status'] = args.status
    if args.keyword:
        params['keyword'] = args.keyword
    if args.uploaded_by:
        params['uploaded_by'] = args.uploaded_by
    if args.field_filter:
        params['field_filters'] = json.dumps([parse_field_filter(f) for f in args.field_filter], ensure_ascii=False)

    client = ApiClient()
    result = client.get('/pub/v1/documents', params=params)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
