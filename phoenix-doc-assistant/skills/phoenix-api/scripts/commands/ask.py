#!/usr/bin/env python3
"""语义问答（知识库检索）  →  POST /pub/v1/ask
用法：--question '{问题}' [--doc-type {类型}] [--limit 5]
返回:{"total":N,"chunks":[{"document_id","filename","doc_type","content","score"}...]}
据 chunks 作答并注明来源文件名(filename)。
"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def main():
    parser = argparse.ArgumentParser(description='语义问答')
    parser.add_argument('--question', required=True, help='问题')
    parser.add_argument('--doc-type', default=None, help='限定文档类型（可选）')
    parser.add_argument('--limit', type=int, default=5, help='返回片段数上限')
    args = parser.parse_args()

    payload = {'question': args.question, 'limit': args.limit}
    if args.doc_type:
        payload['doc_type'] = args.doc_type

    client = ApiClient()
    result = client.post('/pub/v1/ask', data=payload)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
