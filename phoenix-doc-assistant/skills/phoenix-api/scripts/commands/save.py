#!/usr/bin/env python3
"""入库  →  POST /pub/v1/documents/{id}/save
用法：--document-id {ID} --doc-type {类型} --fields '{字段JSON对象}' --content-text '{正文}' [--force]
  --fields 传对象即可,如 '{"doc_no":"123","amount":"5000.00"}';脚本会转成后端要求的数组。
返回:DocumentView(status=saved / needs_review,后者带 issues)。
"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient, to_field_array
from request_input import load_json_input


def main():
    parser = argparse.ArgumentParser(description='入库文档')
    parser.add_argument('--input', help='请求 JSON 文件路径；传 - 从 stdin 读取（推荐，避免正文进入命令行）')
    parser.add_argument('--document-id', help='文档ID')
    parser.add_argument('--doc-type', help='文档类型')
    parser.add_argument('--fields', help='字段JSON对象字符串（兼容旧用法）')
    parser.add_argument('--content-text', help='完整正文（兼容旧用法，长正文请用 --input）')
    parser.add_argument('--force', action='store_true', help='强制入库（跳过校验）')
    args = parser.parse_args()

    source = load_json_input(args.input) if args.input else {}
    document_id = source.pop('document_id', None) or args.document_id
    doc_type = source.get('doc_type') or args.doc_type
    fields = source.get('fields') if 'fields' in source else (json.loads(args.fields) if args.fields else None)
    content_text = source.get('content_text') if 'content_text' in source else args.content_text
    if not document_id or not doc_type or fields is None or content_text is None:
        parser.error('需要 --input，或完整提供 --document-id/--doc-type/--fields/--content-text')
    payload = dict(source)
    payload.update({'doc_type': doc_type, 'fields': to_field_array(fields), 'content_text': content_text})
    if args.force:
        payload['force'] = True

    client = ApiClient()
    result = client.post(f'/pub/v1/documents/{document_id}/save', data=payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
