"""Lido MCP Server — 14 tools for Lido staking operations and vault monitoring."""

import json

import aiohttp
from fastmcp import FastMCP

mcp = FastMCP("Lido-MCP")

# Contract addresses
STETH_ADDRESS = "0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84"
WSTETH_ADDRESS = "0x7f39C581F595B53c5cb19bD0b3f8dA6c935E2Ca0"
WITHDRAWAL_QUEUE_ADDRESS = "0x889edC2eDab5f40e902b864aD4d7AdE8E412F9B1"

# RPC endpoint
ETH_RPC = "https://eth.llamarpc.com"

# Baseline APR for alert checks
BASELINE_APR = 3.0


def _to_wei(amount_str: str) -> int:
    """Convert ETH/stETH amount string to wei."""
    return int(float(amount_str) * 10**18)


def _pad_address(address: str) -> str:
    """Pad an Ethereum address to 32 bytes (64 hex chars)."""
    addr = address.lower().replace("0x", "")
    return addr.zfill(64)


def _pad_uint256(value: int) -> str:
    """Pad a uint256 value to 32 bytes (64 hex chars)."""
    return hex(value)[2:].zfill(64)


async def _eth_call(to: str, data: str, value: str = "0x0") -> str:
    """Execute an eth_call via JSON-RPC."""
    payload = {
        "jsonrpc": "2.0",
        "method": "eth_call",
        "params": [{"to": to, "data": data, "value": value}, "latest"],
        "id": 1,
    }
    async with aiohttp.ClientSession() as session:
        async with session.post(ETH_RPC, json=payload) as resp:
            result = await resp.json()
            if "error" in result:
                raise Exception(f"RPC error: {result['error']}")
            return result["result"]


async def _eth_call_with_from(
    to: str, data: str, from_addr: str, value: str = "0x0"
) -> str:
    """Execute an eth_call with from address via JSON-RPC."""
    payload = {
        "jsonrpc": "2.0",
        "method": "eth_call",
        "params": [
            {"from": from_addr, "to": to, "data": data, "value": value},
            "latest",
        ],
        "id": 1,
    }
    async with aiohttp.ClientSession() as session:
        async with session.post(ETH_RPC, json=payload) as resp:
            result = await resp.json()
            if "error" in result:
                raise Exception(f"RPC error: {result['error']}")
            return result["result"]


# ---------------------------------------------------------------------------
# Read Tools (7)
# ---------------------------------------------------------------------------


@mcp.tool()
async def lido_apr() -> str:
    """Get Lido stETH APR (simple moving average)."""
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(
                "https://eth-api.lido.fi/v1/protocol/steth/apr/sma"
            ) as resp:
                data = await resp.json()
                return json.dumps(data, indent=2)
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_stats() -> str:
    """Get Lido protocol stats for stETH."""
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(
                "https://eth-api.lido.fi/v1/protocol/steth/stats"
            ) as resp:
                data = await resp.json()
                return json.dumps(data, indent=2)
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_balance(address: str) -> str:
    """Get stETH balance for an Ethereum address."""
    try:
        # balanceOf(address) selector: 0x70a08231
        data = "0x70a08231" + _pad_address(address)
        result = await _eth_call(STETH_ADDRESS, data)
        balance_wei = int(result, 16)
        balance_eth = balance_wei / 10**18
        return json.dumps(
            {
                "address": address,
                "balance_wei": str(balance_wei),
                "balance_steth": f"{balance_eth:.6f}",
            },
            indent=2,
        )
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_exchange_rate() -> str:
    """Get wstETH to stETH exchange rate."""
    try:
        # stEthPerToken() selector: 0x035faf82
        result = await _eth_call(WSTETH_ADDRESS, "0x035faf82")
        rate_wei = int(result, 16)
        rate = rate_wei / 10**18
        return json.dumps(
            {
                "steth_per_wsteth_wei": str(rate_wei),
                "steth_per_wsteth": f"{rate:.6f}",
            },
            indent=2,
        )
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_rewards(address: str) -> str:
    """Get recent stETH reward history for an address (last 10 entries)."""
    try:
        url = f"https://reward-history-backend.lido.fi/v1/rewards?address={address}&limit=10"
        async with aiohttp.ClientSession() as session:
            async with session.get(url) as resp:
                data = await resp.json()
                return json.dumps(data, indent=2)
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_withdrawal_status() -> str:
    """Get estimated Lido withdrawal request processing time."""
    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(
                "https://wq-api.lido.fi/v2/request-time/calculate"
            ) as resp:
                data = await resp.json()
                return json.dumps(data, indent=2)
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_governance_proposals(limit: int = 10) -> str:
    """Get recent Lido governance proposals from Snapshot."""
    try:
        query = """
        query Proposals($space: String!, $first: Int!) {
          proposals(
            first: $first,
            skip: 0,
            where: { space_in: [$space] },
            orderBy: "created",
            orderDirection: desc
          ) {
            id
            title
            state
            start
            end
            scores_total
            choices
          }
        }
        """
        variables = {"space": "lido-snapshot.eth", "first": limit}
        async with aiohttp.ClientSession() as session:
            async with session.post(
                "https://hub.snapshot.org/graphql",
                json={"query": query, "variables": variables},
            ) as resp:
                data = await resp.json()
                return json.dumps(data, indent=2)
    except Exception as e:
        return json.dumps({"error": str(e)})


# ---------------------------------------------------------------------------
# Write Tools with dry_run (5)
# ---------------------------------------------------------------------------


@mcp.tool()
async def lido_stake(
    amount_eth: str, from_address: str, dry_run: bool = True
) -> str:
    """Stake ETH with Lido to receive stETH. Use dry_run=True to simulate."""
    try:
        value_wei = _to_wei(amount_eth)
        value_hex = hex(value_wei)
        # submit(address _referral) selector: 0xa1903eab
        # Use zero address as referral
        calldata = "0xa1903eab" + _pad_address("0x0000000000000000000000000000000000000000")

        if dry_run:
            result = await _eth_call_with_from(
                STETH_ADDRESS, calldata, from_address, value_hex
            )
            return json.dumps(
                {
                    "dry_run": True,
                    "action": "stake",
                    "amount_eth": amount_eth,
                    "amount_wei": str(value_wei),
                    "simulation_result": result,
                    "status": "simulation_success",
                },
                indent=2,
            )
        else:
            tx = {
                "to": STETH_ADDRESS,
                "data": calldata,
                "value": value_hex,
                "chainId": 1,
                "from": from_address,
            }
            return json.dumps(
                {
                    "dry_run": False,
                    "action": "stake",
                    "amount_eth": amount_eth,
                    "unsigned_tx": tx,
                },
                indent=2,
            )
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_wrap(
    amount: str, from_address: str, dry_run: bool = True
) -> str:
    """Wrap stETH to wstETH. Amount is in stETH. Use dry_run=True to simulate."""
    try:
        value_wei = _to_wei(amount)
        # wrap(uint256) selector: 0xea598cb0
        calldata = "0xea598cb0" + _pad_uint256(value_wei)

        if dry_run:
            result = await _eth_call_with_from(
                WSTETH_ADDRESS, calldata, from_address
            )
            return json.dumps(
                {
                    "dry_run": True,
                    "action": "wrap",
                    "amount_steth": amount,
                    "amount_wei": str(value_wei),
                    "simulation_result": result,
                    "status": "simulation_success",
                },
                indent=2,
            )
        else:
            tx = {
                "to": WSTETH_ADDRESS,
                "data": calldata,
                "value": "0x0",
                "chainId": 1,
                "from": from_address,
            }
            return json.dumps(
                {
                    "dry_run": False,
                    "action": "wrap",
                    "amount_steth": amount,
                    "unsigned_tx": tx,
                },
                indent=2,
            )
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_unwrap(
    amount: str, from_address: str, dry_run: bool = True
) -> str:
    """Unwrap wstETH to stETH. Amount is in wstETH. Use dry_run=True to simulate."""
    try:
        value_wei = _to_wei(amount)
        # unwrap(uint256) selector: 0xde0e9a3e
        calldata = "0xde0e9a3e" + _pad_uint256(value_wei)

        if dry_run:
            result = await _eth_call_with_from(
                WSTETH_ADDRESS, calldata, from_address
            )
            return json.dumps(
                {
                    "dry_run": True,
                    "action": "unwrap",
                    "amount_wsteth": amount,
                    "amount_wei": str(value_wei),
                    "simulation_result": result,
                    "status": "simulation_success",
                },
                indent=2,
            )
        else:
            tx = {
                "to": WSTETH_ADDRESS,
                "data": calldata,
                "value": "0x0",
                "chainId": 1,
                "from": from_address,
            }
            return json.dumps(
                {
                    "dry_run": False,
                    "action": "unwrap",
                    "amount_wsteth": amount,
                    "unsigned_tx": tx,
                },
                indent=2,
            )
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_request_withdrawal(
    amounts: str, owner_address: str, dry_run: bool = True
) -> str:
    """Request stETH withdrawal from Lido. Amounts is a comma-separated list of stETH amounts. Use dry_run=True to simulate."""
    try:
        amount_list = [a.strip() for a in amounts.split(",")]
        wei_amounts = [_to_wei(a) for a in amount_list]

        # requestWithdrawals(uint256[],address)
        # selector: 0xd6681042 (keccak256 of "requestWithdrawals(uint256[],address)")
        # ABI encode: offset to array (64 bytes = 0x40), then address, then array length, then elements
        offset = _pad_uint256(64)  # offset to dynamic array
        addr_padded = _pad_address(owner_address)
        array_len = _pad_uint256(len(wei_amounts))
        array_elements = "".join(_pad_uint256(w) for w in wei_amounts)
        calldata = "0xd6681042" + offset + addr_padded + array_len + array_elements

        if dry_run:
            result = await _eth_call_with_from(
                WITHDRAWAL_QUEUE_ADDRESS, calldata, owner_address
            )
            return json.dumps(
                {
                    "dry_run": True,
                    "action": "request_withdrawal",
                    "amounts_steth": amount_list,
                    "amounts_wei": [str(w) for w in wei_amounts],
                    "simulation_result": result,
                    "status": "simulation_success",
                },
                indent=2,
            )
        else:
            tx = {
                "to": WITHDRAWAL_QUEUE_ADDRESS,
                "data": calldata,
                "value": "0x0",
                "chainId": 1,
                "from": owner_address,
            }
            return json.dumps(
                {
                    "dry_run": False,
                    "action": "request_withdrawal",
                    "amounts_steth": amount_list,
                    "unsigned_tx": tx,
                },
                indent=2,
            )
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_vote(
    proposal_id: str, choice: int, voter_address: str, dry_run: bool = True
) -> str:
    """Vote on a Lido Snapshot governance proposal. Returns EIP-712 typed data for signing. Use dry_run=True to preview."""
    try:
        vote_payload = {
            "space": "lido-snapshot.eth",
            "proposal": proposal_id,
            "type": "single-choice",
            "choice": choice,
            "from": voter_address,
            "timestamp": None,
        }

        # EIP-712 typed data for Snapshot voting
        eip712_data = {
            "domain": {
                "name": "snapshot",
                "version": "0.1.4",
            },
            "types": {
                "Vote": [
                    {"name": "from", "type": "address"},
                    {"name": "space", "type": "string"},
                    {"name": "proposal", "type": "bytes32"},
                    {"name": "choice", "type": "uint32"},
                ],
            },
            "message": {
                "from": voter_address,
                "space": "lido-snapshot.eth",
                "proposal": proposal_id,
                "choice": choice,
            },
        }

        if dry_run:
            return json.dumps(
                {
                    "dry_run": True,
                    "action": "vote",
                    "proposal_id": proposal_id,
                    "choice": choice,
                    "voter": voter_address,
                    "preview": vote_payload,
                    "status": "preview_only",
                },
                indent=2,
            )
        else:
            return json.dumps(
                {
                    "dry_run": False,
                    "action": "vote",
                    "proposal_id": proposal_id,
                    "choice": choice,
                    "voter": voter_address,
                    "eip712_typed_data": eip712_data,
                    "instructions": "Sign this EIP-712 typed data with the voter's private key, then POST to https://seq.snapshot.org",
                },
                indent=2,
            )
    except Exception as e:
        return json.dumps({"error": str(e)})


# ---------------------------------------------------------------------------
# Vault Monitor Tools — Track 7 (2)
# ---------------------------------------------------------------------------


@mcp.tool()
async def lido_vault_health() -> str:
    """Composite health check: Lido stETH APR vs Aave WETH supply rate, plus TVL."""
    try:
        async with aiohttp.ClientSession() as session:
            # Fetch Lido APR
            async with session.get(
                "https://eth-api.lido.fi/v1/protocol/steth/apr/sma"
            ) as resp:
                apr_data = await resp.json()

            # Fetch Lido stats for TVL
            async with session.get(
                "https://eth-api.lido.fi/v1/protocol/steth/stats"
            ) as resp:
                stats_data = await resp.json()

            # Fetch Aave WETH supply rate from DefiLlama
            aave_rate = None
            async with session.get("https://yields.llama.fi/pools") as resp:
                pools_data = await resp.json()
                if "data" in pools_data:
                    for pool in pools_data["data"]:
                        if (
                            pool.get("project") == "aave-v3"
                            and pool.get("symbol") == "WETH"
                            and pool.get("chain") == "Ethereum"
                        ):
                            aave_rate = pool.get("apy")
                            break

            # Extract Lido APR
            lido_apr_value = None
            if isinstance(apr_data, dict) and "data" in apr_data:
                data_inner = apr_data["data"]
                if isinstance(data_inner, dict):
                    lido_apr_value = data_inner.get("smaApr") or data_inner.get("apr")
                elif isinstance(data_inner, list) and len(data_inner) > 0:
                    lido_apr_value = data_inner[0].get("smaApr") or data_inner[0].get("apr")
            elif isinstance(apr_data, dict):
                lido_apr_value = apr_data.get("smaApr") or apr_data.get("apr")

            health_report = {
                "lido_apr_data": apr_data,
                "lido_apr_pct": lido_apr_value,
                "lido_stats": stats_data,
                "aave_weth_supply_apy_pct": aave_rate,
                "comparison": None,
            }

            if lido_apr_value is not None and aave_rate is not None:
                try:
                    lido_val = float(lido_apr_value)
                    aave_val = float(aave_rate)
                    spread = lido_val - aave_val
                    health_report["comparison"] = {
                        "lido_apr": lido_val,
                        "aave_weth_apy": aave_val,
                        "spread": round(spread, 4),
                        "lido_advantage": spread > 0,
                    }
                except (ValueError, TypeError):
                    health_report["comparison"] = "Could not compare rates"

            return json.dumps(health_report, indent=2)
    except Exception as e:
        return json.dumps({"error": str(e)})


@mcp.tool()
async def lido_alert_check() -> str:
    """Check for Lido protocol alert conditions: APR drop, withdrawal queue depth, exchange rate anomaly."""
    try:
        alerts = []

        async with aiohttp.ClientSession() as session:
            # Check 1: APR drop > 20% from baseline
            try:
                async with session.get(
                    "https://eth-api.lido.fi/v1/protocol/steth/apr/sma"
                ) as resp:
                    apr_data = await resp.json()

                current_apr = None
                if isinstance(apr_data, dict) and "data" in apr_data:
                    data_inner = apr_data["data"]
                    if isinstance(data_inner, dict):
                        current_apr = data_inner.get("smaApr") or data_inner.get("apr")
                    elif isinstance(data_inner, list) and len(data_inner) > 0:
                        current_apr = data_inner[0].get("smaApr") or data_inner[0].get("apr")
                elif isinstance(apr_data, dict):
                    current_apr = apr_data.get("smaApr") or apr_data.get("apr")

                if current_apr is not None:
                    current_apr = float(current_apr)
                    drop_pct = ((BASELINE_APR - current_apr) / BASELINE_APR) * 100
                    if drop_pct > 20:
                        alerts.append(
                            {
                                "type": "apr_drop",
                                "severity": "high",
                                "message": f"APR dropped {drop_pct:.1f}% from baseline {BASELINE_APR}%",
                                "current_apr": current_apr,
                                "baseline_apr": BASELINE_APR,
                            }
                        )
                    else:
                        alerts.append(
                            {
                                "type": "apr_check",
                                "severity": "ok",
                                "message": f"APR at {current_apr:.2f}%, within normal range (baseline {BASELINE_APR}%)",
                                "current_apr": current_apr,
                                "baseline_apr": BASELINE_APR,
                            }
                        )
            except Exception as e:
                alerts.append(
                    {"type": "apr_check", "severity": "error", "message": str(e)}
                )

            # Check 2: Withdrawal queue depth
            try:
                async with session.get(
                    "https://wq-api.lido.fi/v2/request-time/calculate"
                ) as resp:
                    wq_data = await resp.json()
                alerts.append(
                    {
                        "type": "withdrawal_queue",
                        "severity": "info",
                        "data": wq_data,
                    }
                )
            except Exception as e:
                alerts.append(
                    {
                        "type": "withdrawal_queue",
                        "severity": "error",
                        "message": str(e),
                    }
                )

            # Check 3: Exchange rate anomaly (stETH/wstETH)
            try:
                result = await _eth_call(WSTETH_ADDRESS, "0x035faf82")
                rate_wei = int(result, 16)
                rate = rate_wei / 10**18
                # Normal range: 1.0 to 1.5 stETH per wstETH
                if rate < 1.0 or rate > 1.5:
                    alerts.append(
                        {
                            "type": "exchange_rate_anomaly",
                            "severity": "high",
                            "message": f"Exchange rate {rate:.6f} stETH/wstETH outside normal range [1.0, 1.5]",
                            "rate": rate,
                        }
                    )
                else:
                    alerts.append(
                        {
                            "type": "exchange_rate_check",
                            "severity": "ok",
                            "message": f"Exchange rate {rate:.6f} stETH/wstETH within normal range",
                            "rate": rate,
                        }
                    )
            except Exception as e:
                alerts.append(
                    {
                        "type": "exchange_rate_check",
                        "severity": "error",
                        "message": str(e),
                    }
                )

        return json.dumps({"alerts": alerts}, indent=2)
    except Exception as e:
        return json.dumps({"error": str(e)})


if __name__ == "__main__":
    mcp.run()
