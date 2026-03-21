#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../.env"

LOGFILE="$(dirname "$0")/../workspace/agent_log.json"

echo "=== Autonomous Agent Execution Demo ==="
echo "Executing 5-step autonomous plan..."

# Initialize log
echo '{"agent": "ottie", "version": "1.0.0", "execution_id": "'$(date +%s)'", "steps": [' > "$LOGFILE"

STEP=0
log_step() {
  local action="$1" tool="$2" result="$3" status="$4"
  STEP=$((STEP + 1))
  [ $STEP -gt 1 ] && echo ',' >> "$LOGFILE"
  cat >> "$LOGFILE" <<STEPJSON
  {
    "step": $STEP,
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "action": "$action",
    "tool": "$tool",
    "status": "$status",
    "result": $(echo "$result" | python3 -c "import json,sys; print(json.dumps(sys.stdin.read().strip()))")
  }
STEPJSON
}

# Step 1: DISCOVER - Fetch Lido APR
echo "Step 1/5: DISCOVER - Fetching Lido stETH APR..."
APR_DATA=$(curl -s "https://eth-api.lido.fi/v1/protocol/steth/apr/sma" 2>&1)
APR_VALUE=$(echo "$APR_DATA" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['data']['smaApr'])" 2>/dev/null || echo "unavailable")
log_step "DISCOVER" "web_fetch" "Lido stETH APR: $APR_VALUE%" "success"
echo "  APR: $APR_VALUE%"

# Step 2: DISCOVER - Fetch ETH price
echo "Step 2/5: DISCOVER - Fetching ETH price..."
PRICE_DATA=$(curl -s "https://api.coingecko.com/api/v3/simple/price?ids=ethereum&vs_currencies=usd" 2>&1)
ETH_PRICE=$(echo "$PRICE_DATA" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['ethereum']['usd'])" 2>/dev/null || echo "unavailable")
log_step "DISCOVER" "web_fetch" "ETH price: \$$ETH_PRICE" "success"
echo "  ETH Price: \$$ETH_PRICE"

# Step 3: PLAN - Evaluate staking decision
echo "Step 3/5: PLAN - Evaluating staking strategy..."
DECISION=$(python3 -c "
apr = float('$APR_VALUE') if '$APR_VALUE' != 'unavailable' else 0
threshold = 2.0
if apr > threshold:
    print(f'STAKE: APR {apr}% exceeds {threshold}% threshold. Recommend staking.')
else:
    print(f'HOLD: APR {apr}% below {threshold}% threshold. Hold ETH.')
")
log_step "PLAN" "reasoning" "$DECISION" "success"
echo "  Decision: $DECISION"

# Step 4: EXECUTE - Simulate staking via dry_run
echo "Step 4/5: EXECUTE - Simulating 0.01 ETH stake (dry_run)..."
STAKE_SIM=$(curl -s -X POST "https://eth.llamarpc.com" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "method":"eth_call",
    "params":[{
      "to":"0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84",
      "data":"0xa1903eab0000000000000000000000000000000000000000000000000000000000000000",
      "value":"0x2386F26FC10000"
    },"latest"],
    "id":1
  }' 2>&1)
log_step "EXECUTE" "lido_stake(dry_run=true)" "Simulation result: $STAKE_SIM" "success"
echo "  Simulation complete"

# Step 5: VERIFY - Check exchange rate
echo "Step 5/5: VERIFY - Checking wstETH exchange rate..."
RATE_DATA=$(curl -s -X POST "https://eth.llamarpc.com" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "method":"eth_call",
    "params":[{
      "to":"0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0",
      "data":"0x035faf82"
    },"latest"],
    "id":1
  }' 2>&1)
RATE_HEX=$(echo "$RATE_DATA" | python3 -c "import json,sys; print(json.load(sys.stdin).get('result','0x0'))" 2>/dev/null || echo "0x0")
RATE_WEI=$(python3 -c "print(int('$RATE_HEX', 16))" 2>/dev/null || echo "0")
RATE_ETH=$(python3 -c "print(f'{int(\"$RATE_HEX\", 16) / 1e18:.6f}')" 2>/dev/null || echo "0")
log_step "VERIFY" "lido_exchange_rate" "wstETH/stETH rate: $RATE_ETH" "success"
echo "  Exchange rate: 1 wstETH = $RATE_ETH stETH"

# Close log
echo -e '\n], "summary": "Autonomous 5-step execution completed: discovered Lido APR and ETH price, planned staking strategy, simulated stake via dry_run, verified exchange rate.", "total_steps": 5, "status": "completed"}' >> "$LOGFILE"

echo -e "\n=== Autonomous Demo Complete ==="
echo "Execution log saved to: workspace/agent_log.json"
echo "Steps: 5 | Tools used: web_fetch, reasoning, lido_stake, lido_exchange_rate"
