#!/usr/bin/env python3
"""员工登录与 token 管理 —— 每员工身份的核心(V1.3 自研账号体系,取代 Keycloak)。

登录:员工输入本人账号口令 → POST /pub/v1/auth/login → 平台签发
access_token(短期)+ refresh_token(长期),存本地 .config.json(0600,仅本人可读)。
之后每次调用自动用 refresh_token 续期,通常很久才需再登。账号由管理员在
Phoenix 管理后台「员工」页创建;改密/禁用后旧 token 立即失效。

命令(供 Agent MD 调用):
  --check              打印 NOT_CONFIGURED | NEEDS_LOGIN | CONFIGURED user=xxx
  --login [--user X]   交互式登录:提示输用户名与口令(getpass 不回显、不进 shell 历史)
  --whoami             打印当前登录员工身份(调用 /pub/v1/me 实测 token 有效性)
  --logout             清除本地 token(切换账号用)

非交互场景可用环境变量 PHX_USERNAME / PHX_PASSWORD(仅限受控环境)。

对外(被 api_client import):valid_access_token() / NeedsLogin / AuthError
"""
import getpass
import json
import os
import sys
import time
import urllib.error
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import config as cfg_mod


class NeedsLogin(Exception):
    """本地没有可用 token(未登录或续期失败),需要重新 --login。"""


class AuthError(Exception):
    """网络/服务器错误(非凭证问题)。"""


_EXPIRY_SKEW = 60  # access token 提前 60s 视为过期,预留续期余量


def _post_json(cfg, path, payload, bearer=None):
    url = cfg['api_base_url'].rstrip('/') + path
    headers = {'Content-Type': 'application/json', 'Accept': 'application/json'}
    if bearer:
        headers['Authorization'] = 'Bearer ' + bearer
    body = json.dumps(payload, ensure_ascii=False).encode('utf-8') if payload is not None else None
    req = urllib.request.Request(url, data=body, headers=headers,
                                 method='POST' if payload is not None else 'GET')
    try:
        with urllib.request.urlopen(req, timeout=cfg.get('timeout', 60), context=cfg_mod.ssl_context(cfg)) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except urllib.error.HTTPError as e:
        err = e.read().decode('utf-8', errors='replace')
        try:
            msg = json.loads(err).get('message', err)
        except json.JSONDecodeError:
            msg = err
        if e.code == 401:
            raise NeedsLogin(msg)
        raise AuthError(f"HTTP {e.code}: {msg}")
    except urllib.error.URLError as e:
        raise AuthError(f"后端不可达: {e.reason}")


def _store_tokens(resp):
    cfg_mod.set_tokens({
        'access_token': resp['access_token'],
        'refresh_token': resp.get('refresh_token', ''),
        'access_expires_at': int(time.time()) + int(resp.get('expires_in', 3600)),
        'username': (resp.get('user') or {}).get('username', ''),
    })


def login(username, password):
    """账号口令登录,成功后 token 落盘,返回用户名。"""
    cfg = cfg_mod.load_config()
    if not cfg or not cfg_mod.is_endpoint_configured():
        raise AuthError("端点未配置(api_base_url),请先运行 setup.py")
    resp = _post_json(cfg, '/pub/v1/auth/login', {'username': username, 'password': password})
    _store_tokens(resp)
    return (resp.get('user') or {}).get('username', username)


def valid_access_token():
    """返回一个有效的 access_token:未过期直接用;过期用 refresh_token 续期;
    都不行则抛 NeedsLogin,由上层引导登录。"""
    cfg = cfg_mod.load_config()
    if not cfg or not cfg_mod.is_endpoint_configured():
        raise NeedsLogin("端点未配置")
    tokens = cfg_mod.get_tokens()
    if not tokens.get('access_token'):
        raise NeedsLogin("尚未登录")
    if time.time() < tokens.get('access_expires_at', 0) - _EXPIRY_SKEW:
        return tokens['access_token']
    if not tokens.get('refresh_token'):
        raise NeedsLogin("登录已过期")
    try:
        resp = _post_json(cfg, '/pub/v1/auth/refresh', {'refresh_token': tokens['refresh_token']})
    except NeedsLogin:
        cfg_mod.clear_tokens()
        raise NeedsLogin("登录已过期(可能口令已修改或账号被禁用),请重新登录")
    _store_tokens(resp)
    return resp['access_token']


def _cli_check():
    if not cfg_mod.is_endpoint_configured():
        print("NOT_CONFIGURED")
        return
    try:
        valid_access_token()
        print(f"CONFIGURED user={cfg_mod.get_tokens().get('username', '?')}")
    except NeedsLogin as e:
        print(f"NEEDS_LOGIN {e}")
    except AuthError as e:
        print(f"NETWORK_ERROR {e}")


def _cli_login(argv):
    username = os.environ.get('PHX_USERNAME', '')
    password = os.environ.get('PHX_PASSWORD', '')
    if '--user' in argv and argv.index('--user') + 1 < len(argv):
        username = argv[argv.index('--user') + 1]
    if not username:
        username = input("用户名: ").strip()
    if not password:
        password = getpass.getpass("口令(输入不回显): ")
    try:
        who = login(username, password)
        print(f"AUTHORIZED user={who}")
    except (NeedsLogin, AuthError) as e:
        print(f"LOGIN_FAILED {e}")
        sys.exit(1)


def _cli_whoami():
    cfg = cfg_mod.load_config()
    try:
        token = valid_access_token()
        me = _post_json(cfg, '/pub/v1/me', None, bearer=token)
        print(json.dumps(me, ensure_ascii=False))
    except NeedsLogin as e:
        print(f"NEEDS_LOGIN {e}")
        sys.exit(1)
    except AuthError as e:
        print(f"NETWORK_ERROR {e}")
        sys.exit(1)


if __name__ == '__main__':
    if '--check' in sys.argv:
        _cli_check()
    elif '--login' in sys.argv:
        _cli_login(sys.argv)
    elif '--whoami' in sys.argv:
        _cli_whoami()
    elif '--logout' in sys.argv:
        cfg_mod.clear_tokens()
        print("LOGGED_OUT")
    else:
        print("用法: auth.py [--check | --login [--user 用户名] | --whoami | --logout]")
