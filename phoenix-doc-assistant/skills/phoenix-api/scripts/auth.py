#!/usr/bin/env python3
"""Keycloak 登录客户端 —— 每员工身份的核心。

浏览器直接登录(OAuth 2.1 Authorization Code + PKCE):弹出公司 Keycloak 登录页,员工
输账号密码登录后,浏览器跳回本机 loopback(127.0.0.1:redirect_port),脚本拿到「绑定到本人」
的 token;之后 refresh_token 自动续期。后端 /pub/v1 校验该 token → 落到具体员工。

命令(供 Agent MD 调用):
  --check     打印 NOT_CONFIGURED | NEEDS_LOGIN | CONFIGURED
  --login     弹浏览器登录,成功后落 token 并打印 AUTHORIZED + 身份(可选 --wait N,默认 120s)
  --whoami    打印当前登录员工身份(调用 /pub/v1/me 实测 token 有效性)
  --logout    清除本地 token

对外(被 api_client import):valid_access_token() / NeedsLogin
"""
import base64
import hashlib
import http.server
import json
import secrets
import sys
import time
import urllib.parse
import urllib.request
import urllib.error
import webbrowser

import config as cfg_mod


class NeedsLogin(Exception):
    """本地没有可用 token(未登录或 refresh 失败),需要重新 --login。"""


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
    """拉取并缓存 OIDC discovery(authorization/token 端点)。"""
    issuer = cfg['oidc_issuer'].rstrip('/')
    if issuer not in _disc_cache:
        _disc_cache[issuer] = _get_json(issuer + '/.well-known/openid-configuration', cfg)
    return _disc_cache[issuer]


def _require_cfg():
    cfg = cfg_mod.load_config()
    if not cfg or not cfg_mod.is_endpoint_configured():
        raise NeedsLogin("端点未配置")
    return cfg


def _store_tokens(cfg, tok):
    """把 token 响应落库(带上算好的过期时刻)。"""
    tokens = {
        'access_token': tok['access_token'],
        'refresh_token': tok.get('refresh_token', ''),
        'access_expires_at': time.time() + int(tok.get('expires_in', 300)) - _EXPIRY_SKEW,
    }
    cfg['tokens'] = tokens
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


# ---------------- 浏览器登录(Authorization Code + PKCE) ----------------

def _pkce():
    verifier = base64.urlsafe_b64encode(secrets.token_bytes(40)).rstrip(b'=').decode()
    challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b'=').decode()
    return verifier, challenge


def login(wait=120):
    """弹出 Keycloak 登录页(用户输账号密码)→ 浏览器跳回本机 loopback → 交换 token。
    返回 {"status":"AUTHORIZED"} 或 {"status":"PENDING",...}(wait 秒内没等到登录,可再次 --login)。"""
    cfg = _require_cfg()
    disc = _discovery(cfg)
    if 'authorization_endpoint' not in disc:
        raise AuthError("授权服务器未提供 authorization_endpoint")
    port = int(cfg.get('redirect_port', 47100))
    redirect_uri = f"http://127.0.0.1:{port}/callback"
    verifier, challenge = _pkce()
    state = secrets.token_urlsafe(16)
    auth_url = disc['authorization_endpoint'] + '?' + urllib.parse.urlencode({
        'response_type': 'code',
        'client_id': cfg['client_id'],
        'redirect_uri': redirect_uri,
        'scope': cfg.get('scope', 'openid profile email'),
        'state': state,
        'code_challenge': challenge,
        'code_challenge_method': 'S256',
    })

    result = {}

    class _Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            parsed = urllib.parse.urlparse(self.path)
            if parsed.path != '/callback':
                self.send_response(404)
                self.end_headers()
                return
            qs = urllib.parse.parse_qs(parsed.query)
            result['code'] = qs.get('code', [None])[0]
            result['state'] = qs.get('state', [None])[0]
            result['error'] = qs.get('error', [None])[0]
            self.send_response(200)
            self.send_header('Content-Type', 'text/html; charset=utf-8')
            self.end_headers()
            self.wfile.write('<h3>登录完成,可以关闭本页面,回到 WorkBuddy。</h3>'.encode('utf-8'))

        def log_message(self, *args):
            pass

    try:
        srv = http.server.HTTPServer(('127.0.0.1', port), _Handler)
    except OSError as e:
        raise AuthError(f"无法在本机 {port} 端口起回调服务(端口被占用): {e}")
    srv.timeout = 2
    # 浏览器没弹出时,这行 URL 是手动兜底(打到 stderr,不污染 stdout 的 JSON 结果)
    print(f"[auth] 若浏览器未自动打开,请手动访问登录: {auth_url}", file=sys.stderr)
    try:
        webbrowser.open(auth_url, new=2)
    except Exception:  # noqa: BLE001
        pass
    try:
        deadline = time.time() + wait
        while time.time() < deadline and 'code' not in result and 'error' not in result:
            srv.handle_request()  # 每次最多等 2s;处理掉 favicon 等杂请求后继续
    finally:
        srv.server_close()

    if 'code' not in result and 'error' not in result:
        return {'status': 'PENDING', 'auth_url': auth_url,
                'message': '还没等到登录,请在浏览器完成登录后再次执行 --login'}
    if result.get('error'):
        raise AuthError(f"授权失败: {result['error']}")
    if result.get('state') != state:
        raise AuthError("state 不匹配,已中止(疑似会话串扰),请重试")
    if not result.get('code'):
        raise AuthError("未拿到授权码")
    status, tok = _post_form(disc['token_endpoint'], {
        'grant_type': 'authorization_code',
        'code': result['code'],
        'redirect_uri': redirect_uri,
        'client_id': cfg['client_id'],
        'code_verifier': verifier,
    }, cfg)
    if status != 200 or 'access_token' not in tok:
        raise AuthError(tok.get('error_description') or tok.get('error') or '授权码换 token 失败')
    _store_tokens(cfg, tok)
    return {'status': 'AUTHORIZED'}


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
        elif arg == '--login':
            wait = 120  # 可选 --wait N 覆盖等待窗口
            if '--wait' in sys.argv:
                try:
                    wait = int(sys.argv[sys.argv.index('--wait') + 1])
                except (ValueError, IndexError):
                    pass
            out = login(wait=wait)
            if out.get('status') == 'AUTHORIZED':
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
            print("用法: auth.py [--check | --login | --whoami | --logout]")
    except NeedsLogin as e:
        _emit({'error': 'NEEDS_LOGIN', 'message': str(e)})
        sys.exit(1)
    except AuthError as e:
        _emit({'error': 'AUTH_ERROR', 'message': str(e)})
        sys.exit(1)
