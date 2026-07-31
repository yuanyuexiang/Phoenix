#!/usr/bin/env python3
"""预校验（不入库）  →  POST /pub/v1/documents/{id}/validate
用法：--document-id {ID} --doc-type {类型} --fields '{字段JSON对象}'
  --fields 传对象即可,如 '{"doc_no":"123","amount":"5000.00"}';脚本会转成后端要求的数组。
返回:DocumentView(status=validated / needs_review,后者带 issues)。不落库。
"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient, to_field_array
from request_input import load_json_input


def main():
    parser = argparse.ArgumentParser(description='预校验文档')
    parser.add_argument('--input', help='请求 JSON 文件路径；传 - 从 stdin 读取（推荐）')
    parser.add_argument('--document-id', help='文档ID')
    parser.add_argument('--doc-type', help='文档类型')
    parser.add_argument('--fields', help='字段JSON对象字符串（兼容旧用法）')
    args = parser.parse_args()

    source = load_json_input(args.input) if args.input else {}
    document_id = source.pop('document_id', None) or args.document_id
    doc_type = source.get('doc_type') or args.doc_type
    fields = source.get('fields') if 'fields' in source else (json.loads(args.fields) if args.fields else None)
    if not document_id or not doc_type or fields is None:
        parser.error('需要 --input，或完整提供 --document-id/--doc-type/--fields')
    client = ApiClient()
    result = client.post(f'/pub/v1/documents/{document_id}/validate', data={
        'doc_type': doc_type,
        'fields': to_field_array(fields),
    })
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
