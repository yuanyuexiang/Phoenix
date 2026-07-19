#!/bin/bash
# Phoenix MCP Auth Proxy - Auto-restart wrapper
# Keeps the proxy running even if it crashes

PROXY_SCRIPT="/Users/yuanyuexiang/Desktop/workspace/Phoenix/scripts/phoenix-auth-proxy.py"
PYTHON="/Users/yuanyuexiang/.workbuddy/binaries/python/versions/3.13.12/bin/python3"
LOG_FILE="/tmp/phoenix-proxy.log"
PID_FILE="/tmp/phoenix-proxy.pid"

# Kill existing proxy if running
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE")
    kill "$OLD_PID" 2>/dev/null
    sleep 1
fi

# Start proxy with auto-restart
while true; do
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Starting Phoenix MCP Auth Proxy..." >> "$LOG_FILE"
    "$PYTHON" "$PROXY_SCRIPT" --port 9876 --user bob --password bob123 >> "$LOG_FILE" 2>&1 &
    PROXY_PID=$!
    echo "$PROXY_PID" > "$PID_FILE"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Proxy started (PID: $PROXY_PID)" >> "$LOG_FILE"
    wait "$PROXY_PID"
    EXIT_CODE=$?
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Proxy exited (code: $EXIT_CODE), restarting in 3s..." >> "$LOG_FILE"
    sleep 3
done
