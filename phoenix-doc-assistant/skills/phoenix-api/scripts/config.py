#!/usr/bin/env python3
"""配置管理：读写本地 .config.json。

「每员工身份」方案(V1.3 起为自研账号体系,不再依赖 Keycloak):
  - 端点配置(api_base_url)是公司级常量,通常由 IT 预置进模板;
  - 凭证不是一把共享 key,而是每个员工用本人账号口令登录(/pub/v1/auth/login)后
    拿到的、绑定到本人的 token(存在 tokens 字段,自动续期)。见 auth.py。

本模块只管配置文件的读写与脱敏展示;登录/刷新/校验在 auth.py。
"""
import json
import os
import ssl
import sys

CONFIG_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), '.config.json')

DEFAULT_CONFIG = {
    "api_base_url": "",        # 后端主机根地址,如 https://phoenix.matrix-net.tech(端点为 /pub/v1/...)
    "timeout": 60,             # 请求超时秒数(文档处理可能较慢)
    "verify_ssl": True,        # 是否校验 SSL 证书(内网自签名可设 False)
    "return_scheme": "workbuddy://",  # 登录成功页跳回应用的 URL scheme;须与 WorkBuddy 注册的一致,
                                      # 未注册时浏览器静默忽略(无副作用),置空 "" 则不尝试跳转
    "tokens": {}               # 登录后由 auth.py 写入:access_token/refresh_token/access_expires_at/username
}


def load_config():
    """读取配置并补齐新版默认项；旧专家包配置无需重新安装即可获得 return_scheme。"""
    if not os.path.exists(CONFIG_FILE):
        return None
    with open(CONFIG_FILE, 'r', encoding='utf-8') as f:
        return {**DEFAULT_CONFIG, **json.load(f)}


def save_config(config):
    """保存配置到本地文件(0600,仅所有者可读写)。"""
    full = {**DEFAULT_CONFIG, **config}
    with open(CONFIG_FILE, 'w', encoding='utf-8') as f:
        json.dump(full, f, ensure_ascii=False, indent=2)
    os.chmod(CONFIG_FILE, 0o600)


def is_endpoint_configured():
    """端点(api_base_url)是否已配置。"""
    cfg = load_config()
    return bool(cfg and cfg.get('api_base_url'))


def get_tokens():
    cfg = load_config() or {}
    return cfg.get('tokens') or {}


def set_tokens(tokens):
    """把 tokens 合并写回配置(登录/刷新后调用)。"""
    cfg = load_config() or {**DEFAULT_CONFIG}
    cfg['tokens'] = tokens
    save_config(cfg)


def clear_tokens():
    cfg = load_config()
    if cfg:
        cfg['tokens'] = {}
        save_config(cfg)


def ssl_context(cfg):
    """verify_ssl=False 时返回跳过证书校验的 context(内网自签名场景),否则返回 None。"""
    if cfg.get('verify_ssl', True):
        return None
    ctx = ssl.create_default_context()
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    return ctx


def _masked(cfg):
    safe = {**cfg}
    tk = dict(safe.get('tokens') or {})
    for k in ('access_token', 'refresh_token'):
        if tk.get(k):
            tk[k] = str(tk[k])[:6] + '****'
    safe['tokens'] = tk
    return safe


if __name__ == '__main__':
    if '--show' in sys.argv:
        cfg = load_config()
        print(json.dumps(_masked(cfg), ensure_ascii=False, indent=2) if cfg else '{}')
    elif '--endpoint-check' in sys.argv:
        print('ENDPOINT_OK' if is_endpoint_configured() else 'ENDPOINT_MISSING')
    elif '--logout' in sys.argv:
        clear_tokens()
        print('LOGGED_OUT')
    else:
        print("用法: config.py [--show | --endpoint-check | --logout]")
