---
name: steth-treasury
description: >
  Build and interact with stETH Agent Treasury contracts. Yield-bearing operating budgets
  backed by wstETH where agents spend only yield, never touching principal. Use when asked
  about agent treasury, yield-funded budgets, stETH yield spending, or principal-protected vaults.
---

# stETH Agent Treasury

A contract primitive where humans give AI agents yield-bearing operating budgets backed by wstETH. Only yield flows to the agent — principal is structurally inaccessible.

## Core Concept

1. Human deposits wstETH into treasury contract
2. Contract records deposit exchange rate (`stEthPerToken()` at deposit time)
3. wstETH value grows as staking rewards accrue (exchange rate increases)
4. Agent can only withdraw the **yield** (difference between current and deposit value)
5. Principal (original wstETH amount) is permanently locked from agent access

## How wstETH Yield Works

wstETH is non-rebasing — its balance stays constant, but its value in stETH increases:

```
depositValue = wstETHAmount * depositExchangeRate
currentValue = wstETHAmount * currentExchangeRate
accruedYield = currentValue - depositValue
```

**Get current exchange rate**:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0\",\"data\":\"0x035faf82\"},\"latest\"],\"id\":1}'"
```
> `0x035faf82` = `stEthPerToken()`. Divide by 1e18. At ~3.2% APR, 100 wstETH earns ~3.2 stETH/year.

## Contract Architecture

### Key State Variables
```solidity
mapping(address => uint256) public depositRate;       // exchange rate at deposit
mapping(address => uint256) public depositAmount;      // wstETH deposited
mapping(address => address) public agentAddress;       // authorized agent
mapping(address => address[]) public allowedRecipients; // spending whitelist
mapping(address => uint256) public perTxCap;           // max per transaction
mapping(address => uint256) public timeWindow;         // spending window
```

### Core Functions
```solidity
function deposit(uint256 wstETHAmount, address agent) external
function queryYield(address depositor) view returns (uint256)
function spendYield(address recipient, uint256 amount) external  // agent only
function withdrawPrincipal() external                             // depositor only
function setRecipientWhitelist(address[] recipients) external     // depositor only
function setPerTxCap(uint256 cap) external                        // depositor only
```

### Security Invariants
- `spendYield()` callable only by `agentAddress`
- `spendYield()` amount <= `accruedYield`
- `spendYield()` recipient must be in `allowedRecipients`
- `spendYield()` amount <= `perTxCap`
- `withdrawPrincipal()` callable only by depositor
- Agent can never call `withdrawPrincipal()`

## Agent Interaction Pattern

**1. Query available yield**:
```
exec command="curl -s -X POST https://eth.llamarpc.com -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"TREASURY_ADDRESS\",\"data\":\"0xQUERY_YIELD_SELECTOR+DEPOSITOR_ADDRESS\"},\"latest\"],\"id\":1}'"
```

**2. Spend yield** (agent pays for API call from yield balance):
- Agent calls `spendYield(apiProvider, amount)`
- Contract verifies: amount <= yield, recipient whitelisted, amount <= per-tx cap
- wstETH transferred to recipient; deposit tracking updated

**3. Check permissions**:
- Query `allowedRecipients` — who can the agent pay?
- Query `perTxCap` — max spend per transaction
- Query `timeWindow` — spending rate limits

## Use Cases

- **API compute budget**: agent pays for LLM inference from staking yield
- **Sub-agent allocation**: parent agent allocates yield budgets to child agents
- **Monthly operating budget**: team funds agent operations purely from staking rewards
- **Self-sustaining agent**: at ~3.2% APR, 1000 wstETH generates ~32 stETH/year for operations

## Supported Chains

Deploy on any chain with wstETH:
- Ethereum Mainnet (native staking + yield)
- Arbitrum, Optimism, Base (bridged wstETH, yield via exchange rate)
- See `lido-mcp` skill for full L2 wstETH address list

## Integration with Ottie

Ottie can interact with treasury contracts via:
1. `crypto-wallet` skill — query wstETH balances and approvals
2. `lido-mcp` skill — exchange rate queries, wrap/unwrap operations
3. `lido-vault-monitor` skill — track yield accrual and alert on changes
