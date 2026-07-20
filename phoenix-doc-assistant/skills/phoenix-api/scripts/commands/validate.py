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


def main():
    parser = argparse.ArgumentParser(description='预校验文档')
    parser.add_argument('--document-id', required=True, help='文档ID')
    parser.add_argument('--doc-type', required=True, help='文档类型')
    parser.add_argument('--fields', required=True, help='字段JSON对象字符串')
    args = parser.parse_args()

    client = ApiClient()
    result = client.post(f'/pub/v1/documents/{args.document_id}/validate', data={
        'doc_type': args.doc_type,
        'fields': to_field_array(json.loads(args.fields)),
    })
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
