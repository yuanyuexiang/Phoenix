#!/usr/bin/env python3
"""安全读取业务命令的 JSON 输入，避免长正文出现在命令行和进程列表中。"""
import json
import sys


def load_json_input(path):
    """从 --input 文件或 stdin(-)读取 JSON 对象。"""
    stream = sys.stdin if path == '-' else open(path, 'r', encoding='utf-8')
    try:
        value = json.load(stream)
    finally:
        if stream is not sys.stdin:
            stream.close()
    if not isinstance(value, dict):
        raise ValueError('输入必须是 JSON 对象')
    return value
