#!/usr/bin/env python3
"""Keycloak OAuth 2.1 客户端(Device Authorization Grant, RFC 8628)—— 每员工身份的核心。

为什么用 Device Flow:员工机器上的脚本无法内嵌浏览器/回调服务,Device Flow 让员工
在任意浏览器里用自己的 Keycloak 账号批准一次,脚本轮询拿到「绑定到本人」的 token,
之后用 refresh_token 自动续期。后端 /pub/v1 校验该 token → 落到具体员工。

命令(供 Agent MD 调用):
  --check         打印 NOT_CONFIGURED | NEEDS_LOGIN | CONFIGURED
  --login-start   发起设备授权,打印验证地址 + user_code,立即返回(让模型把地址转告用户)
  --login-poll    轮询直到用户批准/拒绝/超时,成功后落 token,打印 AUTHORIZED + 身份
  --whoami        打印当前登录员工身份(调用 /pub/v1/me 实测 token 有效性)
  --logout        清除本地 token

对外(被 api_client import):valid_access_token() / NeedsLogin
"""
import json
import sys
import time
import urllib.parse
import urllib.request
import urllib.error

import config as cfg_mod


class NeedsLogin(Exception):
    """本地没有可用 token(未登录或 refresh 失败),需要走 Device Flow。"""


class AuthError(Exception):
    pass


_EXPIRY_SKEW = 30  # 提前 30s 视为过期,预留刷新余量


def _post_form(url, data, cfg):
    """POST application/x-www-form-urlencoded,返回 (status_code, dict)。"""
    body = urllib.parse.urlencode(data).encode('utf-8')
    req = urllib.request.Request(
        url, data=body, method='POST',
        headers={'Content-Type': 'application/x-www-form-urlencoded', 'Accept': 'application/json'},
    )
    ctx = cfg_mod.ssl_context(cfg)
    try:
        with urllib.request.urlopen(req, timeout=cfg.get('timeout', 60), context=ctx) as resp:
            return resp.status, json.loads(resp.read().decode('utf-8'))
    except urllib.error.HTTPError as e:
        raw = e.read().decode('utf-8', errors='replace')
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {'error': 'http_error', 'error_description': raw}
    except urllib.error.URLError as e:
        raise AuthError(f"无法连接授权服务器: {e.reason}")


def _get_json(url, cfg, token=None):
    headers = {'Accept': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request(url, headers=headers, method='GET')
    ctx = cfg_mod.ssl_context(cfg)
    with urllib.request.urlopen(req, timeout=cfg.get('timeout', 60), context=ctx) as resp:
        return json.loads(resp.read().decode('utf-8'))


_disc_cache = {}


def _discovery(cfg):
    """拉取并缓存 OIDC discovery(device/token 端点)。"""
    issuer = cfg['oidc_issuer'].rstrip('/')
    if issuer in _disc_cache:
        return _disc_cache[issuer]
    d = _get_json(issuer + '/.well-known/openid-configuration', cfg)
    # 兜底:Keycloak 的 device 端点约定路径(个别版本 discovery 里可能缺字段)
    d.setdefault('device_authorization_endpoint', issuer + '/protocol/openid-connect/auth/device')
    _disc_cache[issuer] = d
    return d


def _require_cfg():
    cfg = cfg_mod.load_config()
    if not cfg or not cfg_mod.is_endpoint_configured():
        raise NeedsLogin("端点未配置")
    return cfg


def _store_tokens(cfg, tok):
    """把 token 响应落库(带上算好的过期时刻),清掉待批准的设备会话。"""
    tokens = {
        'access_token': tok['access_token'],
        'refresh_token': tok.get('refresh_token', ''),
        'access_expires_at': time.time() + int(tok.get('expires_in', 300)) - _EXPIRY_SKEW,
    }
    cfg['tokens'] = tokens
    cfg.pop('pending_device', None)
    cfg_mod.save_config(cfg)
    return tokens


def _refresh(cfg):
    """用 refresh_token 换新的 access_token;成功返回 access_token,失败抛 NeedsLogin。"""
    tokens = cfg.get('tokens') or {}
    rt = tokens.get('refresh_token')
    if not rt:
        raise NeedsLogin("无 refresh_token")
    disc = _discovery(cfg)
    status, tok = _post_form(disc['token_endpoint'], {
        'grant_type': 'refresh_token',
        'refresh_token': rt,
        'client_id': cfg['client_id'],
    }, cfg)
    if status != 200 or 'access_token' not in tok:
        # refresh 也失效了 → 需要重新登录
        raise NeedsLogin(tok.get('error_description') or tok.get('error') or 'refresh 失败')
    return _store_tokens(cfg, tok)['access_token']


def valid_access_token():
    """返回一个可用的 access_token;必要时自动刷新;都不行则抛 NeedsLogin。"""
    cfg = _require_cfg()
    tokens = cfg.get('tokens') or {}
    at = tokens.get('access_token')
    exp = tokens.get('access_expires_at', 0)
    if at and time.time() < exp:
        return at
    if tokens.get('refresh_token'):
        return _refresh(cfg)
    raise NeedsLogin("尚未登录")


# ---------------- Device Flow ----------------

def login_start():
    cfg = _require_cfg()
    disc = _discovery(cfg)
    status, r = _post_form(disc['device_authorization_endpoint'], {
        'client_id': cfg['client_id'],
        'scope': cfg.get('scope', 'openid profile email'),
    }, cfg)
    if status != 200 or 'device_code' not in r:
        raise AuthError(r.get('error_description') or r.get('error') or f"设备授权请求失败(HTTP {status})")
    cfg['pending_device'] = {
        'device_code': r['device_code'],
        'interval': int(r.get('interval', 5)),
        'expires_at': time.time() + int(r.get('expires_in', 600)),
        'token_endpoint': disc['token_endpoint'],
    }
    cfg_mod.save_config(cfg)
    return {
        'user_code': r['user_code'],
        'verification_uri': r.get('verification_uri'),
        'verification_uri_complete': r.get('verification_uri_complete'),
        'expires_in': int(r.get('expires_in', 600)),
    }


def login_poll():
    cfg = _require_cfg()
    pd = cfg.get('pending_device')
    if not pd:
        raise AuthError("没有待批准的登录会话,请先执行 --login-start")
    interval = pd['interval']
    while time.time() < pd['expires_at']:
        status, tok = _post_form(pd['token_endpoint'], {
            'grant_type': 'urn:ietf:params:oauth:grant-type:device_code',
            'device_code': pd['device_code'],
            'client_id': cfg['client_id'],
        }, cfg)
        if status == 200 and 'access_token' in tok:
            _store_tokens(cfg, tok)
            return 'AUTHORIZED'
        err = tok.get('error')
        if err == 'authorization_pending':
            time.sleep(interval)
        elif err == 'slow_down':
            interval += 5
            time.sleep(interval)
        elif err == 'access_denied':
            return 'DENIED'
        elif err == 'expired_token':
            return 'EXPIRED'
        else:
            raise AuthError(tok.get('error_description') or err or '轮询失败')
    return 'EXPIRED'


def login_status():
    if not cfg_mod.is_endpoint_configured():
        return 'NOT_CONFIGURED'
    try:
        valid_access_token()
        return 'CONFIGURED'
    except NeedsLogin:
        return 'NEEDS_LOGIN'
    except AuthError:
        # 网络/AS 暂时不可达:有 token 就先当已登录,让业务命令去暴露真实错误
        return 'CONFIGURED' if (cfg_mod.get_tokens().get('access_token')) else 'NEEDS_LOGIN'


def whoami():
    cfg = _require_cfg()
    token = valid_access_token()
    base = cfg['api_base_url'].rstrip('/')
    return _get_json(base + '/pub/v1/me', cfg, token=token)


def _emit(obj):
    print(json.dumps(obj, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    arg = sys.argv[1] if len(sys.argv) > 1 else ''
    try:
        if arg == '--check':
            print(login_status())
        elif arg == '--login-start':
            info = login_start()
            _emit({'status': 'PENDING', **info})
        elif arg == '--login-poll':
            result = login_poll()
            out = {'status': result}
            if result == 'AUTHORIZED':
                try:
                    out['user'] = whoami()
                except Exception:  # noqa: BLE001 —— 登录已成功,身份自省失败不影响结果
                    pass
            _emit(out)
        elif arg == '--whoami':
            _emit(whoami())
        elif arg == '--logout':
            cfg_mod.clear_tokens()
            print('LOGGED_OUT')
        else:
            print("用法: auth.py [--check | --login-start | --login-poll | --whoami | --logout]")
    except NeedsLogin as e:
        _emit({'error': 'NEEDS_LOGIN', 'message': str(e)})
        sys.exit(1)
    except AuthError as e:
        _emit({'error': 'AUTH_ERROR', 'message': str(e)})
        sys.exit(1)
