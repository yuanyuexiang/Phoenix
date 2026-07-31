#!/usr/bin/env python3
"""员工登录与 token 管理 —— 每员工身份的核心(V1.3 自研账号体系,取代 Keycloak)。

默认登录方式是**弹浏览器**(复刻 OAuth 授权码 + PKCE 的体验,但登录页由 Phoenix
平台自己出,无第三方身份组件):脚本起本机回调服务 → 打开平台登录页 → 员工在
页面输入账号口令 → 平台签发一次性授权码回调本机 → 换 access/refresh token。
登录成功页会提示"正在返回 WorkBuddy"并自动关闭。

token 存本地 .config.json(0600,仅本人可读);之后每次调用自动续期,
通常很久才需再登。账号由管理员在 Phoenix 管理后台「员工」页创建;
改密/禁用后旧 token 立即失效。

命令行:
  auth.py --check                 NOT_CONFIGURED / NEEDS_LOGIN / CONFIGURED user=xxx
  auth.py --login [--wait N]      弹浏览器登录(默认;等待回调最长 N 秒,默认 180)
  auth.py --login --password      终端交互输入账号口令(无浏览器环境的后备,getpass 不回显)
  auth.py --whoami                当前登录员工(调 /pub/v1/me 实测 token)
  auth.py --logout                清除本地 token(切换账号用)

非交互场景可用环境变量 PHX_USERNAME / PHX_PASSWORD 配合 --password(仅限受控环境)。

对外(被 api_client import):valid_access_token() / NeedsLogin / AuthError
"""
import base64
import getpass
import hashlib
import html
import http.server
import json
import os
import platform
import secrets
import shutil
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import webbrowser

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import config as cfg_mod


class NeedsLogin(Exception):
    """本地没有可用 token(未登录或续期失败),需要重新 --login。"""


class AuthError(Exception):
    """网络/服务器错误(非凭证问题)。"""


_EXPIRY_SKEW = 60          # access token 提前 60s 视为过期,预留续期余量
_PREFERRED_PORT = 47100    # 回调端口:先试固定值,被占用则退到随机端口
_DEFAULT_WAIT = 180        # 等待浏览器回调的默认秒数


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


def _require_endpoint():
    cfg = cfg_mod.load_config()
    if not cfg or not cfg_mod.is_endpoint_configured():
        raise AuthError("端点未配置(api_base_url),请先运行 setup.py")
    return cfg


"""---------------- 浏览器登录(授权码 + PKCE,登录页由平台自己出) ----------------"""

_SUCCESS_HTML = """<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<title>登录成功</title><style>
body{min-height:96vh;display:grid;place-items:center;font-family:-apple-system,"PingFang SC",sans-serif;background:#F6F8FB;color:#2B3A52}
@media (prefers-color-scheme:dark){body{background:#0E141F;color:#CBD6E5}}
.box{text-align:center}.ok{font-size:44px;color:#059669}p{margin-top:10px;font-size:15px}
small{display:block;margin-top:8px;color:#8B98AC;line-height:1.8}
a.back{display:inline-block;margin-top:16px;padding:9px 22px;border-radius:8px;background:#2563EB;color:#fff;
text-decoration:none;font-size:14px}
@media (prefers-color-scheme:dark){a.back{background:#5B8DEF}}
</style></head><body><div class="box"><div class="ok">✓</div>
<p>登录成功</p>
<small id="tip">请关闭本页,返回 WorkBuddy 继续</small>
<a class="back" id="back" href="__SCHEME__" style="display:none">返回 WorkBuddy</a></div>
<script>
/* 浏览器安全策略:脚本只能关闭由脚本打开的窗口,外部程序唤起的标签页
   window.close() 会被忽略 —— 仍尽力一试,失败则保留手动提示。
   __SCHEME__ 为 WorkBuddy 的 URL scheme(客户端配置 return_scheme):
   已注册时自动跳回应用;未注册时浏览器静默忽略,按钮点了也无副作用。 */
try{window.close()}catch(e){}
var s="__SCHEME__";
if(s){
  document.getElementById("back").style.display="inline-block";
  document.getElementById("tip").textContent="正在返回 WorkBuddy…(若未自动跳转,点击下方按钮或手动关闭本页)";
  setTimeout(function(){try{location.href=s}catch(e){}},200);
}
setTimeout(function(){try{window.close()}catch(e){}},600)
</script></body></html>"""

_FAILED_HTML = """<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<title>登录未完成</title><style>
body{min-height:96vh;display:grid;place-items:center;font-family:-apple-system,"PingFang SC",sans-serif;background:#F6F8FB;color:#2B3A52}
.box{text-align:center;max-width:520px}.bad{font-size:40px;color:#DC2626}p{font-size:16px}small{color:#8B98AC;line-height:1.8}
</style></head><body><div class="box"><div class="bad">×</div><p>登录未完成</p>
<small>__ERROR__<br>请返回 WorkBuddy 后重新登录。</small></div></body></html>"""


def _success_html(cfg):
    scheme = (cfg.get('return_scheme') if cfg else None)
    if scheme is None:
        scheme = ''
    return _SUCCESS_HTML.replace('__SCHEME__', scheme)


def _activate_workbuddy(cfg):
    """Token 落盘后主动把 WorkBuddy 切到前台；URL scheme 同时用于跨平台唤起。"""
    scheme = (cfg or {}).get('return_scheme') or 'workbuddy://'
    if not scheme.lower().startswith('workbuddy:'):
        return False
    try:
        system = platform.system()
        if system == 'Darwin':
            subprocess.Popen(['open', scheme], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        elif system == 'Windows':
            os.startfile(scheme)  # type: ignore[attr-defined]
        else:
            opener = shutil.which('xdg-open')
            if not opener:
                return False
            subprocess.Popen([opener, scheme], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        return True
    except (OSError, subprocess.SubprocessError):
        return False


class _CallbackHandler(http.server.BaseHTTPRequestHandler):
    result = {}
    done = threading.Event()
    success_html = _SUCCESS_HTML  # browser_login 启动前按配置渲染(return_scheme)
    token_ready = threading.Event()  # 主线程完成授权码兑换和 token 落盘后才展示成功
    token_error = ''

    def do_GET(self):
        u = urllib.parse.urlparse(self.path)
        if u.path != '/callback':
            self.send_response(404)
            self.end_headers()
            return
        q = urllib.parse.parse_qs(u.query)
        _CallbackHandler.result = {'code': (q.get('code') or [''])[0], 'state': (q.get('state') or [''])[0]}
        _CallbackHandler.done.set()
        ready = _CallbackHandler.token_ready.wait(timeout=65)
        error = _CallbackHandler.token_error if ready else 'Token 兑换超时'
        page = (_FAILED_HTML.replace('__ERROR__', html.escape(error)) if error else _CallbackHandler.success_html)
        self.send_response(200)
        self.send_header('Content-Type', 'text/html; charset=utf-8')
        self.end_headers()
        self.wfile.write(page.encode('utf-8'))

    def log_message(self, *args):  # 静默访问日志
        pass


def browser_login(wait_seconds=_DEFAULT_WAIT):
    """弹浏览器登录:本机回调 → 授权码 + PKCE 换 token。返回用户名。"""
    cfg = _require_endpoint()

    verifier = base64.urlsafe_b64encode(secrets.token_bytes(32)).rstrip(b'=').decode()
    challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b'=').decode()
    state = secrets.token_urlsafe(16)

    _CallbackHandler.result = {}
    _CallbackHandler.done.clear()
    _CallbackHandler.token_ready.clear()
    _CallbackHandler.token_error = ''
    _CallbackHandler.success_html = _success_html(cfg)
    try:
        srv = http.server.HTTPServer(('127.0.0.1', _PREFERRED_PORT), _CallbackHandler)
    except OSError:
        srv = http.server.HTTPServer(('127.0.0.1', 0), _CallbackHandler)
    port = srv.server_address[1]
    threading.Thread(target=srv.serve_forever, daemon=True).start()

    params = urllib.parse.urlencode({
        'redirect_uri': f'http://127.0.0.1:{port}/callback',
        'state': state,
        'code_challenge': challenge,
        'code_challenge_method': 'S256',
    })
    login_url = cfg['api_base_url'].rstrip('/') + '/pub/v1/auth/authorize?' + params
    print(f"[auth] 已打开浏览器;若未弹出,请手动访问登录: {login_url}", file=sys.stderr)
    webbrowser.open(login_url)

    try:
        if not _CallbackHandler.done.wait(timeout=wait_seconds):
            raise AuthError(f"等待浏览器登录超时({wait_seconds}s),请重试 --login")
        res = _CallbackHandler.result
        if res.get('state') != state:
            raise AuthError("state 不匹配,已丢弃本次回调(可能被篡改),请重试 --login")
        if not res.get('code'):
            raise AuthError("回调缺少授权码,请重试 --login")

        resp = _post_json(cfg, '/pub/v1/auth/token', {
            'grant_type': 'authorization_code', 'code': res['code'], 'code_verifier': verifier,
        })
        _store_tokens(resp)
        _activate_workbuddy(cfg)
        return (resp.get('user') or {}).get('username', '')
    except (NeedsLogin, AuthError) as exc:
        _CallbackHandler.token_error = str(exc)
        raise
    finally:
        _CallbackHandler.token_ready.set()
        srv.shutdown()


"""---------------- 口令登录(终端后备)与 token 维护 ----------------"""

def password_login(username, password):
    """账号口令直连登录(终端后备/冒烟用),成功后 token 落盘,返回用户名。"""
    cfg = _require_endpoint()
    resp = _post_json(cfg, '/pub/v1/auth/login', {'username': username, 'password': password})
    _store_tokens(resp)
    return (resp.get('user') or {}).get('username', username)


def valid_access_token():
    """返回一个有效的 access_token:未过期直接用;过期用 refresh_token 换新;
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


"""---------------- CLI ----------------"""

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
    try:
        if '--password' in argv:
            username = os.environ.get('PHX_USERNAME', '')
            password = os.environ.get('PHX_PASSWORD', '')
            if '--user' in argv and argv.index('--user') + 1 < len(argv):
                username = argv[argv.index('--user') + 1]
            if not username:
                username = input("用户名: ").strip()
            if not password:
                password = getpass.getpass("口令(输入不回显): ")
            who = password_login(username, password)
        else:
            wait = _DEFAULT_WAIT
            if '--wait' in argv and argv.index('--wait') + 1 < len(argv):
                wait = int(argv[argv.index('--wait') + 1])
            who = browser_login(wait)
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
        print("用法: auth.py [--check | --login [--wait N] [--password [--user 用户名]] | --whoami | --logout]")
