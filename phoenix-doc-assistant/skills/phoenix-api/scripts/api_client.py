#!/usr/bin/env python3
"""REST API 客户端封装：被各业务命令 import 使用。

标准 HTTP + Keycloak Bearer Token(每员工身份)。token 由 auth.py 管理:
每次请求自动取一个有效的 access_token(过期自动刷新);未登录则输出 NEEDS_LOGIN 让模型引导登录。
所有业务端点在后端的 /pub/v1/* 下(见 backend/internal/restapi)。
"""
import json
import os
import sys
import urllib.request
import urllib.error
from urllib.parse import urlencode

# 让 commands/ 下的脚本能 import 同级 scripts/ 的模块
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import config as cfg_mod
from auth import valid_access_token, NeedsLogin, AuthError


class ApiClient:
    """后端 /pub/v1 REST 客户端(每请求携带员工本人的 Bearer token)。"""

    def __init__(self):
        cfg = cfg_mod.load_config()
        if not cfg or not cfg_mod.is_endpoint_configured():
            print(json.dumps({"error": "NOT_CONFIGURED", "message": "端点未配置(api_base_url/oidc_issuer/client_id)"}))
            sys.exit(1)
        self.cfg = cfg
        self.base_url = cfg['api_base_url'].rstrip('/')

    def _token(self):
        try:
            return valid_access_token()
        except NeedsLogin as e:
            print(json.dumps({"error": "NEEDS_LOGIN", "message": f"未登录或登录已失效: {e}。请先执行 Keycloak 设备登录"}))
            sys.exit(1)
        except AuthError as e:
            print(json.dumps({"error": "NETWORK_ERROR", "message": f"授权服务器不可达: {e}"}))
            sys.exit(1)

    def request(self, method, path, data=None, params=None):
        """发起 HTTP 请求，返回 dict。"""
        url = self.base_url + path
        if params:
            url += '?' + urlencode(params)

        headers = {
            'Authorization': 'Bearer ' + self._token(),
            'Content-Type': 'application/json',
            'Accept': 'application/json',
        }
        body = json.dumps(data, ensure_ascii=False).encode('utf-8') if data is not None else None
        req = urllib.request.Request(url, data=body, headers=headers, method=method)
        ctx = cfg_mod.ssl_context(self.cfg)

        try:
            with urllib.request.urlopen(req, timeout=self.cfg.get('timeout', 60), context=ctx) as resp:
                resp_body = resp.read().decode('utf-8')
                return json.loads(resp_body) if resp_body.strip() else {}
        except urllib.error.HTTPError as e:
            # 后端错误(4xx/5xx),body 通常是 {"error":"CODE","message":"..."};401 提示重新登录
            err_body = e.read().decode('utf-8', errors='replace')
            try:
                err_json = json.loads(err_body)
            except json.JSONDecodeError:
                err_json = {"error": "HTTP_ERROR", "code": e.code, "message": err_body}
            if e.code == 401:
                err_json.setdefault("error", "NEEDS_LOGIN")
                err_json["message"] = "登录已失效,请重新执行设备登录。" + err_json.get("message", "")
            print(json.dumps(err_json, ensure_ascii=False))
            sys.exit(1)
        except urllib.error.URLError as e:
            print(json.dumps({"error": "NETWORK_ERROR", "message": f"无法连接后端服务: {e.reason}"}))
            sys.exit(1)
        except json.JSONDecodeError as e:
            print(json.dumps({"error": "PARSE_ERROR", "message": f"后端返回非 JSON 响应: {e}"}))
            sys.exit(1)

    def get(self, path, params=None):
        return self.request('GET', path, params=params)

    def post(self, path, data=None):
        return self.request('POST', path, data=data)


def to_field_array(obj):
    """把便于模型书写的字段对象 {"doc_no":"123","amount":"5000"} 转成后端要求的数组
    [{"name":"doc_no","value":"123"}, ...]。不带 confidence(缺省=0,后端校验跳过该维度,
    避免用假的 1.0 冒充置信度)。已是数组则原样返回。"""
    if isinstance(obj, list):
        return obj
    return [{"name": k, "value": "" if v is None else str(v)} for k, v in obj.items()]
