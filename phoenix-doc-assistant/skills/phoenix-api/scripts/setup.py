#!/usr/bin/env python3
"""首次「端点」配置向导：给手动在终端跑的高级用户用。

只配公司级端点三要素(api_base_url / oidc_issuer / client_id);员工个人登录用 auth.py
(Device Flow),不在这里填任何密码或 key。对话场景走对话式配置(见 Agent MD)。
通常这三要素由 IT 预置进 templates/config.template.json,员工只需登录。
"""
import sys
import os

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from config import load_config, save_config, is_endpoint_configured, DEFAULT_CONFIG


def main():
    print("=== Phoenix 文档助手 - 端点配置 ===\n")

    if is_endpoint_configured():
        print("检测到已有端点配置。继续将覆盖。")
        if input("是否继续？(y/N): ").strip().lower() != 'y':
            print("配置未修改。")
            return

    existing = load_config() or {}
    print("请提供后端与 Keycloak 的地址(公司级常量,一次配好):\n")

    api_base_url = input(
        f"1. 后端主机根地址(如 https://phoenix.matrix-net.tech) [{existing.get('api_base_url', '')}]: "
    ).strip() or existing.get('api_base_url', '')

    oidc_issuer = input(
        f"2. Keycloak issuer(如 https://phoenix.matrix-net.tech/auth/realms/phoenix) [{existing.get('oidc_issuer', '')}]: "
    ).strip() or existing.get('oidc_issuer', '')

    client_id = input(
        f"3. Device Flow 客户端 id(如 phoenix-cli) [{existing.get('client_id', 'phoenix-cli')}]: "
    ).strip() or existing.get('client_id', 'phoenix-cli')

    verify_ssl_input = input(
        f"4. 是否校验 SSL 证书 (y/n) [{'y' if existing.get('verify_ssl', True) else 'n'}]: "
    ).strip().lower()
    if verify_ssl_input == 'n':
        verify_ssl = False
    elif verify_ssl_input == 'y':
        verify_ssl = True
    else:
        verify_ssl = existing.get('verify_ssl', True)

    timeout_input = input(
        f"5. 请求超时秒数 [{existing.get('timeout', DEFAULT_CONFIG['timeout'])}]: "
    ).strip()
    timeout = int(timeout_input) if timeout_input else existing.get('timeout', DEFAULT_CONFIG['timeout'])

    if not api_base_url or not oidc_issuer or not client_id:
        print("\n错误：api_base_url / oidc_issuer / client_id 都不能为空。")
        return

    save_config({
        'api_base_url': api_base_url,
        'oidc_issuer': oidc_issuer,
        'client_id': client_id,
        'scope': existing.get('scope', DEFAULT_CONFIG['scope']),
        'timeout': timeout,
        'verify_ssl': verify_ssl,
        'tokens': existing.get('tokens', {}),
    })
    print(f"\n端点配置已保存到 {os.path.join(os.path.dirname(os.path.abspath(__file__)), '.config.json')}")
    print("下一步:执行 `python3 auth.py --login` 用你的 Keycloak 账号登录。")


if __name__ == '__main__':
    main()
