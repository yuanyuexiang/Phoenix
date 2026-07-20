#!/usr/bin/env python3
"""获取字段清单（下发抽取指令，后端不识别）  →  POST /pub/v1/documents/{id}/extract
返回 FieldBrief:
  - 类型已定:{"doc_type","title","fields":[{"name","label","aliases","required","pattern","enum"}...]}
  - 类型未定:{"doc_type":"auto","catalog":[{"name","title","description"}...]} 供你判定类型
"""
import argparse
import json
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from api_client import ApiClient


def main():
    parser = argparse.ArgumentParser(description='获取字段清单')
    parser.add_argument('--document-id', required=True, help='文档ID')
    args = parser.parse_args()

    client = ApiClient()
    result = client.post(f'/pub/v1/documents/{args.document_id}/extract')
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
