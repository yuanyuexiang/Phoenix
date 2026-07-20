#!/usr/bin/env python3
"""上传文档到后端归档  →  POST /pub/v1/documents
用法：
  --file {路径}              上传本地文件（图片/PDF等二进制，自动base64）
  --content-text '{文本}'    上传纯文本
  --file-url {URL}           上传公网URL文件
  --doc-type {类型}          可选，文档类型
  --name {文件名}            可选，归档显示用文件名(不给则从 --file/--file-url 推断)
返回：DocumentView，含 "id"(后续操作都用它)与 "status":"uploaded"
"""
import argparse
import base64
import json
import os
import sys
from urllib.parse import urlparse

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def file_to_base64(file_path):
    if not os.path.exists(file_path):
        print(json.dumps({"error": "FILE_NOT_FOUND", "message": f"文件不存在: {file_path}"}))
        sys.exit(1)
    with open(file_path, 'rb') as f:
        return base64.b64encode(f.read()).decode('utf-8')


def main():
    parser = argparse.ArgumentParser(description='上传文档归档')
    g = parser.add_mutually_exclusive_group(required=True)
    g.add_argument('--file', help='本地文件路径（图片/PDF等，自动base64）')
    g.add_argument('--content-text', help='纯文本内容')
    g.add_argument('--file-url', help='公网可访问的文件URL')
    parser.add_argument('--doc-type', default=None, help='文档类型（可选）')
    parser.add_argument('--name', default=None, help='归档文件名（可选）')
    args = parser.parse_args()

    payload = {}
    if args.doc_type:
        payload['doc_type'] = args.doc_type

    if args.file:
        payload['content_base64'] = file_to_base64(args.file)
        payload['filename'] = args.name or os.path.basename(args.file)
    elif args.content_text:
        payload['content_text'] = args.content_text
        payload['filename'] = args.name or 'untitled.txt'
    elif args.file_url:
        payload['file_url'] = args.file_url
        payload['filename'] = args.name or os.path.basename(urlparse(args.file_url).path) or 'download'

    client = ApiClient()
    result = client.post('/pub/v1/documents', data=payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
