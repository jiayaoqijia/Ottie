#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SESSION_DIR="$SCRIPT_DIR/sessions"
mkdir -p "$SESSION_DIR"

source "$ROOT_DIR/.env"

TIMESTAMP=$(date -u +%Y%m%dT%H%M%SZ)

record_session() {
  local track="$1"
  local title="$2"
  local outfile="$SESSION_DIR/track${track}-session.txt"

  echo "Recording Track $track: $title..."

  {
    echo "======================================================="
    echo "  DEMO SESSION: Track $track -- $title"
    echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "  Wallet: ${E2E_WALLET_ADDRESS}"
    echo "  Network: Sepolia (11155111)"
    echo "======================================================="
    echo ""
    # Track-specific commands are called via eval below
  } > "$outfile" 2>&1

  echo "  Saved: $outfile ($(wc -l < "$outfile") lines)"
}

###############################################################################
# Track 1: Lido MCP Server
###############################################################################
echo "Recording Track 1: Lido MCP Server..."
OUTFILE="$SESSION_DIR/track1-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 1 -- Lido MCP Server"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""

  # List all 14 tools
  python3 -c "
import ast
with open('$ROOT_DIR/mcp-servers/lido-mcp/server.py') as f:
    tree = ast.parse(f.read())
funcs = sorted([n.name for n in ast.walk(tree) if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef)) and n.name.startswith('lido_')])
print('Lido MCP Server -- 14 Tools:')
print('  Read Tools:')
for f in funcs:
    if f in ('lido_apr','lido_stats','lido_balance','lido_exchange_rate','lido_rewards','lido_withdrawal_status','lido_governance_proposals'):
        print(f'    - {f}')
print('  Write Tools (dry_run supported):')
for f in funcs:
    if f in ('lido_stake','lido_wrap','lido_unwrap','lido_request_withdrawal','lido_vote'):
        print(f'    - {f}')
print('  Vault Monitor Tools:')
for f in funcs:
    if f in ('lido_vault_health','lido_alert_check'):
        print(f'    - {f}')
"

  # Fetch live APR
  echo ""
  echo "--- Live API: Lido stETH APR ---"
  curl -s -H 'User-Agent: Ottie/1.0' 'https://eth-api.lido.fi/v1/protocol/steth/apr/sma' | python3 -c "
import json, sys
d = json.load(sys.stdin)
print(f'  SMA APR: {d[\"data\"][\"smaApr\"]}%')
"

  # Exchange rate on-chain
  echo ""
  echo "--- On-Chain: wstETH/stETH Exchange Rate ---"
  python3 -c "
import urllib.request, json
payload = json.dumps({'jsonrpc':'2.0','method':'eth_call','params':[{'to':'0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0','data':'0x035faf82'},'latest'],'id':1}).encode()
req = urllib.request.Request('https://ethereum-rpc.publicnode.com', data=payload, headers={'Content-Type':'application/json','User-Agent':'Ottie/1.0'})
r = json.loads(urllib.request.urlopen(req, timeout=15).read())
rate = int(r['result'], 16) / 1e18
print(f'  1 wstETH = {rate:.6f} stETH')
"

  # Dry-run stake
  echo ""
  echo "--- Dry-Run: Simulate staking 0.01 ETH ---"
  python3 -c "
import urllib.request, json
payload = json.dumps({'jsonrpc':'2.0','method':'eth_call','params':[{'to':'0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84','data':'0xa1903eab0000000000000000000000000000000000000000000000000000000000000000','value':'0x2386F26FC10000','from':'0xe90e61edF69B2cF46b835409d87A2C0E36b641B2'},'latest'],'id':1}).encode()
req = urllib.request.Request('https://ethereum-rpc.publicnode.com', data=payload, headers={'Content-Type':'application/json','User-Agent':'Ottie/1.0'})
r = json.loads(urllib.request.urlopen(req, timeout=15).read())
if 'result' in r and r['result'] not in ('0x','0x0',None):
    shares = int(r['result'], 16) / 1e18
    print(f'  Simulation: 0.01 ETH -> {shares:.8f} stETH shares')
else:
    print(f'  Simulation sent to contract (may revert in dry-run)')
print('  Mode: dry_run=true (no real tx)')
"

  # Governance
  echo ""
  echo "--- Snapshot: Lido Governance Proposals ---"
  python3 -c "
import urllib.request, json
query = json.dumps({'query': '{proposals(first:3,where:{space_in:[\"lido-snapshot.eth\"]},orderBy:\"created\",orderDirection:desc){id title state created}}'})
req = urllib.request.Request('https://hub.snapshot.org/graphql', data=query.encode(), headers={'Content-Type':'application/json','User-Agent':'Ottie/1.0'})
resp = json.loads(urllib.request.urlopen(req, timeout=15).read())
for p in resp['data']['proposals']:
    print(f'  [{p[\"state\"].upper()}] {p[\"title\"][:70]}')
"
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Track 2: Uniswap Swap
###############################################################################
echo "Recording Track 2: Uniswap Swap..."
OUTFILE="$SESSION_DIR/track2-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 2 -- Uniswap Swap"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""
  echo "--- Executing Uniswap Swap: 0.0001 ETH -> USDC on Sepolia ---"
  cd "$ROOT_DIR" && node scripts/uniswap-swap.js --amount 0.0001 --chain sepolia 2>&1 || true
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Track 3: Venice AI
###############################################################################
echo "Recording Track 3: Venice AI..."
OUTFILE="$SESSION_DIR/track3-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 3 -- Venice AI"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""
  bash "$ROOT_DIR/scripts/venice-demo.sh" 2>&1 || true
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Track 4: Let Agent Cook
###############################################################################
echo "Recording Track 4: Let Agent Cook..."
OUTFILE="$SESSION_DIR/track4-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 4 -- Let Agent Cook"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""
  bash "$ROOT_DIR/scripts/autonomous-demo.sh" 2>&1 || true
  echo ""
  echo "--- agent_log.json (formatted) ---"
  python3 -c "
import json
with open('$ROOT_DIR/workspace/agent_log.json') as f:
    log = json.load(f)
print(json.dumps(log, indent=2)[:2000])
"
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Track 5: ERC-8004
###############################################################################
echo "Recording Track 5: ERC-8004..."
OUTFILE="$SESSION_DIR/track5-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 5 -- ERC-8004"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""
  echo "--- ERC-8004 Agent Registration ---"
  cd "$ROOT_DIR" && node scripts/erc8004-register.js 2>&1 || true
  echo ""
  echo "--- Verifying on 8004scan.io ---"
  python3 -c "
import urllib.request, json
req = urllib.request.Request('https://www.8004scan.io/api/v1/public/agents?q=Ottie&chainId=11155111', headers={'User-Agent':'Ottie/1.0'})
try:
    d = json.loads(urllib.request.urlopen(req, timeout=15).read())
    agents = [a for a in d.get('data',[]) if a.get('name') == 'Ottie']
    if agents:
        a = agents[0]
        print(f'  Found on 8004scan: {a[\"agent_id\"]}')
        print(f'  Name: {a[\"name\"]}')
        print(f'  Owner: {a[\"owner_address\"]}')
        print(f'  Created: {a[\"created_at\"]}')
    else:
        print('  Agent not found yet on 8004scan (may take time to index)')
except Exception as e:
    print(f'  8004scan query: {e}')
" 2>&1 || true
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Track 6: Agent Treasury
###############################################################################
echo "Recording Track 6: Agent Treasury..."
OUTFILE="$SESSION_DIR/track6-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 6 -- Agent Treasury"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""
  echo "--- AgentTreasury Deployment ---"
  cd "$ROOT_DIR" && node scripts/deploy-treasury.js 2>&1 || true
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Track 7: Vault Monitor
###############################################################################
echo "Recording Track 7: Vault Monitor..."
OUTFILE="$SESSION_DIR/track7-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 7 -- Vault Monitor"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""
  bash "$ROOT_DIR/scripts/vault-monitor-demo.sh" 2>&1 || true
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Track 8: Self Agent ID
###############################################################################
echo "Recording Track 8: Self Agent ID..."
OUTFILE="$SESSION_DIR/track8-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 8 -- Self Agent ID"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""
  echo "--- Self Agent ID Registration ---"
  cd "$ROOT_DIR" && node scripts/self-agent-register.js 2>&1 || true
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Track 9: ERC-8183
###############################################################################
echo "Recording Track 9: ERC-8183..."
OUTFILE="$SESSION_DIR/track9-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 9 -- ERC-8183"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""
  echo "--- ERC-8183 Agentic Commerce ---"
  cd "$ROOT_DIR" && node scripts/erc8183-job.js 2>&1 || true
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Track 10: Open Track
###############################################################################
echo "Recording Track 10: Open Track..."
OUTFILE="$SESSION_DIR/track10-session.txt"
{
  echo "======================================================="
  echo "  DEMO SESSION: Track 10 -- Open Track"
  echo "  Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "  Wallet: ${E2E_WALLET_ADDRESS}"
  echo "  Network: Sepolia (11155111)"
  echo "======================================================="
  echo ""
  echo "--- Ottie Agent Manifest ---"
  python3 -c "
import json
with open('$ROOT_DIR/workspace/agent.json') as f:
    print(json.dumps(json.load(f), indent=2))
"
  echo ""
  echo "--- On-Chain Transaction Summary ---"
  echo "  Track 2 (Uniswap): See track2-session.txt"
  echo "  Track 5 (ERC-8004): See track5-session.txt"
  echo "  Track 6 (Treasury): See track6-session.txt"
  echo ""
  echo "--- Wallet Balance ---"
  python3 -c "
import urllib.request, json
payload = json.dumps({'jsonrpc':'2.0','method':'eth_getBalance','params':['${E2E_WALLET_ADDRESS}','latest'],'id':1}).encode()
req = urllib.request.Request('${SEPOLIA_RPC}', data=payload, headers={'Content-Type':'application/json','User-Agent':'Ottie/1.0'})
r = json.loads(urllib.request.urlopen(req, timeout=15).read())
balance = int(r['result'], 16) / 1e18
print(f'  Remaining: {balance:.6f} ETH')
"
} > "$OUTFILE" 2>&1
echo "  Saved: $OUTFILE ($(wc -l < "$OUTFILE") lines)"

###############################################################################
# Summary
###############################################################################
echo ""
echo "======================================================="
echo "  All demo sessions recorded!"
echo "======================================================="
echo "Sessions:"
for f in "$SESSION_DIR"/track*-session.txt; do
  track=$(basename "$f" | sed 's/track//' | sed 's/-session.txt//')
  lines=$(wc -l < "$f")
  echo "  Track $track: $f ($lines lines)"
done
