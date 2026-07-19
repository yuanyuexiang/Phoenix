#!/usr/bin/env python3
"""
Phoenix MCP Auth Proxy v2
=========================
Local reverse proxy: WorkBuddy -> localhost:9876/mcp -> (inject Bearer token) -> Phoenix MCP.

Workaround for WorkBuddy v5.2.6 OAuth UI state machine bug.
Uses http.client for proper SSE streaming and ThreadingHTTPServer for concurrency.

Usage:
    python3 phoenix-auth-proxy.py [--port 9876] [--user bob] [--password bob123]
"""

import http.server
import http.client
import ssl
import json
import time
import threading
import argparse
import sys
import os

# --- Config ---
PHOENIX_HOST = "phoenix.matrix-net.tech"
PHOENIX_PORT = 443
PHOENIX_PATH = "/mcp"
KEYCLOAK_TOKEN_URL = "https://phoenix.matrix-net.tech/auth/realms/phoenix/protocol/openid-connect/token"
CLIENT_ID = "phoenix-smoke"

# SSL context for upstream HTTPS
SSL_CTX = ssl.create_default_context()


class TokenManager:
    """Thread-safe token manager with auto-refresh."""

    def __init__(self, username, password):
        self.username = username
        self.password = password
        self._token = None
        self._expires_at = 0
        self._lock = threading.Lock()

    def get_token(self):
        with self._lock:
            now = time.time()
            if self._token is None or now >= self._expires_at - 60:
                self._refresh()
            return self._token

    def _refresh(self):
        import urllib.request
        import urllib.parse

        data = urllib.parse.urlencode({
            "grant_type": "password",
            "client_id": CLIENT_ID,
            "username": self.username,
            "password": self.password,
        }).encode()

        req = urllib.request.Request(
            KEYCLOAK_TOKEN_URL,
            data=data,
            headers={"Content-Type": "application/x-www-form-urlencoded"},
        )

        try:
            resp = urllib.request.urlopen(req, context=SSL_CTX, timeout=10)
            body = json.loads(resp.read())
        except Exception as e:
            print(f"[{time.strftime('%H:%M:%S')}] Token refresh FAILED: {e}", file=sys.stderr, flush=True)
            if self._token:
                print(f"[{time.strftime('%H:%M:%S')}] Using expired token as fallback", file=sys.stderr, flush=True)
                return
            raise

        self._token = body["access_token"]
        self._expires_at = time.time() + body.get("expires_in", 300)
        remaining = self._expires_at - time.time()
        print(f"[{time.strftime('%H:%M:%S')}] Token OK: expires in {remaining:.0f}s, user={self.username}", file=sys.stderr, flush=True)


class ProxyHandler(http.server.BaseHTTPRequestHandler):
    token_manager = None

    # Headers that should NOT be forwarded to upstream
    SKIP_REQUEST_HEADERS = {
        "host", "content-length", "transfer-encoding", "connection",
        "expect", "accept-encoding",  # let http.client handle encoding
    }
    SKIP_RESPONSE_HEADERS = {
        "transfer-encoding", "connection", "content-length",  # we set our own
    }

    def _forward(self):
        # Read request body
        content_length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_length) if content_length > 0 else b""

        # Build upstream headers
        upstream_headers = {}
        for k, v in self.headers.items():
            if k.lower() not in self.SKIP_REQUEST_HEADERS:
                upstream_headers[k] = v

        # Inject auth token
        token = self.token_manager.get_token()
        upstream_headers["Authorization"] = f"Bearer {token}"
        upstream_headers["Host"] = PHOENIX_HOST
        if body:
            upstream_headers["Content-Length"] = str(len(body))

        # Connect to Phoenix and send request
        try:
            conn = http.client.HTTPSConnection(PHOENIX_HOST, PHOENIX_PORT, context=SSL_CTX, timeout=600)
            conn.request(self.command, PHOENIX_PATH, body=body, headers=upstream_headers)
            resp = conn.getresponse()
        except Exception as e:
            self._send_error(502, f"Cannot connect to upstream: {e}")
            print(f"[{time.strftime('%H:%M:%S')}] Upstream connection error: {e}", file=sys.stderr, flush=True)
            return

        # Forward response status and headers
        self.send_response(resp.status)

        content_type = resp.getheader("Content-Type", "")
        is_sse = "text/event-stream" in content_type

        for k, v in resp.getheaders():
            if k.lower() not in self.SKIP_RESPONSE_HEADERS:
                self.send_header(k, v)

        # For non-SSE responses, set Content-Length; for SSE, use chunked
        if not is_sse:
            # Read full body and forward
            resp_body = resp.read()
            self.send_header("Content-Length", str(len(resp_body)))
            self.end_headers()
            self.wfile.write(resp_body)
        else:
            # SSE: stream chunks as they arrive
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            try:
                while True:
                    chunk = resp.read(4096)
                    if not chunk:
                        break
                    self.wfile.write(chunk)
                    self.wfile.flush()
            except Exception as e:
                print(f"[{time.strftime('%H:%M:%S')}] SSE stream interrupted: {e}", file=sys.stderr, flush=True)

        conn.close()

        status = resp.status
        method = self.command
        print(f"[{time.strftime('%H:%M:%S')}] {method} -> {status} ({'SSE' if is_sse else 'JSON'})", file=sys.stderr, flush=True)

    def _send_error(self, code, message):
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        body = json.dumps({"error": message}).encode()
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        self._forward()

    def do_GET(self):
        self._forward()

    def do_DELETE(self):
        self._forward()

    def do_PUT(self):
        self._forward()

    def log_message(self, fmt, *args):
        # Suppress default logging, we do our own
        pass


def main():
    parser = argparse.ArgumentParser(description="Phoenix MCP Auth Proxy v2")
    parser.add_argument("--port", type=int, default=9876)
    parser.add_argument("--user", default="bob")
    parser.add_argument("--password", default="bob123")
    args = parser.parse_args()

    ProxyHandler.token_manager = TokenManager(args.user, args.password)

    print(f"Phoenix MCP Auth Proxy v2", file=sys.stderr, flush=True)
    print(f"  Upstream: https://{PHOENIX_HOST}{PHOENIX_PATH}", file=sys.stderr, flush=True)
    print(f"  User: {args.user}", file=sys.stderr, flush=True)

    try:
        ProxyHandler.token_manager.get_token()
    except Exception as e:
        print(f"FATAL: Cannot get initial token: {e}", file=sys.stderr, flush=True)
        sys.exit(1)

    server = http.server.ThreadingHTTPServer(("127.0.0.1", args.port), ProxyHandler)
    server.daemon_threads = True

    print(f"  Listening: http://127.0.0.1:{args.port}/mcp", file=sys.stderr, flush=True)
    print(f"  Ready. Update .mcp.json url to: http://127.0.0.1:{args.port}/mcp", file=sys.stderr, flush=True)
    print(file=sys.stderr, flush=True)

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down...", file=sys.stderr, flush=True)
        server.shutdown()


if __name__ == "__main__":
    main()
