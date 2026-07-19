#!/bin/bash
# Phoenix MCP Token Auto-Refresh Script
# Workaround for WorkBuddy v5.2.6 OAuth UI state machine bug
# Gets a fresh Keycloak token via password grant and updates .mcp.json
#
# Setup: launchd runs this every 4 minutes (token expires in 5 min)
# Remove when WorkBuddy fixes the OAuth callback UI bug

set -euo pipefail

MCP_JSON="/Users/yuanyuexiang/.workbuddy/plugins/marketplaces/my-experts/plugins/phoenix-doc-expert/.mcp.json"
TOKEN_URL="https://phoenix.matrix-net.tech/auth/realms/phoenix/protocol/openid-connect/token"
CLIENT_ID="phoenix-smoke"
USERNAME="bob"
PASSWORD="bob123"
LOG_FILE="/tmp/phoenix-token-refresh.log"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >> "$LOG_FILE"
}

# Get fresh token
TOKEN_RESPONSE=$(curl -sf -X POST "$TOKEN_URL" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password&client_id=${CLIENT_ID}&username=${USERNAME}&password=${PASSWORD}" 2>&1) || {
    log "ERROR: Token request failed: $TOKEN_RESPONSE"
    exit 1
}

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])" 2>/dev/null) || {
    log "ERROR: Failed to parse token response"
    exit 1
}

if [ -z "$ACCESS_TOKEN" ]; then
    log "ERROR: Empty access token"
    exit 1
fi

# Update .mcp.json with new token
python3 -c "
import json
config = {
    'mcpServers': {
        'phoenix': {
            'type': 'streamable-http',
            'url': 'https://phoenix.matrix-net.tech/mcp',
            'headers': {
                'Authorization': 'Bearer ${ACCESS_TOKEN}'
            },
            'disabled': False
        }
    }
}
with open('${MCP_JSON}', 'w') as f:
    json.dump(config, f, indent=2, ensure_ascii=False)
" 2>/dev/null || {
    log "ERROR: Failed to update .mcp.json"
    exit 1
}

log "OK: Token refreshed (${#ACCESS_TOKEN} chars), .mcp.json updated"
