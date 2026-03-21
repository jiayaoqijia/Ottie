"""Shared test helpers for E2E test suite."""
import json
import os
import sys

ROOT_DIR = os.environ.get("ROOT_DIR", os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


def load_json(path):
    with open(os.path.join(ROOT_DIR, path)) as f:
        return json.load(f)


def test_agent_log():
    log = load_json("workspace/agent_log.json")
    assert log["agent"] == "ottie", f"Wrong agent: {log['agent']}"
    assert log["total_steps"] >= 5, f"Only {log['total_steps']} steps"
    assert log["status"] == "completed"
    steps = log["steps"]
    actions = [s["action"] for s in steps]
    tools = [s["tool"] for s in steps]
    print(f"Agent:   {log['agent']} v{log['version']}")
    print(f"Steps:   {log['total_steps']}")
    print(f"Actions: {actions}")
    print(f"Tools:   {tools}")
    for s in steps:
        assert "timestamp" in s, f"Step {s['step']} missing timestamp"
        assert "tool" in s, f"Step {s['step']} missing tool"
        assert "result" in s, f"Step {s['step']} missing result"
        assert s["status"] == "success", f"Step {s['step']} failed"
    print("LOG_STRUCTURE_OK")


def test_agent_log_real_data():
    log = load_json("workspace/agent_log.json")
    steps = log["steps"]
    apr_step = [s for s in steps if "APR" in s.get("result", "")]
    assert apr_step, "No APR data in log"
    print(f"Real data: {apr_step[0]['result'][:60]}")
    price_step = [s for s in steps if "price" in s.get("result", "").lower() or "$" in s.get("result", "")]
    if price_step:
        print(f"Real data: {price_step[0]['result'][:60]}")
    rate_step = [s for s in steps if "rate" in s.get("result", "").lower()]
    if rate_step:
        print(f"Real data: {rate_step[0]['result'][:60]}")
    print("REAL_DATA_OK")


def test_agent_manifest():
    d = load_json("workspace/agent.json")
    required = ["name", "description", "version", "services", "skills", "domains", "active"]
    for key in required:
        assert key in d, f"Missing required field: {key}"
    assert d["name"] == "Ottie"
    assert d["active"] is True
    assert len(d["skills"]) >= 10, f"Only {len(d['skills'])} skills"
    assert "mcp" in d["services"], "Missing MCP service endpoint"
    assert 11155111 in d.get("chains", []), "Sepolia not in chains"
    print(f"Name:     {d['name']}")
    print(f"Skills:   {len(d['skills'])} ({d['skills'][:4]}...)")
    print(f"Domains:  {d['domains']}")
    print(f"Chains:   {d['chains']}")
    print(f"Services: {list(d['services'].keys())}")
    print("MANIFEST_VALID")


def test_manifest_simple():
    d = load_json("workspace/agent.json")
    print(f"Agent: {d['name']} v{d['version']}")
    print(f"Skills: {len(d['skills'])} blockchain-native skills")
    print(f"Chains: {d['chains']}")
    print(f"Services: {list(d['services'].keys())}")
    print(f"Domains: {d['domains']}")
    assert d["active"] is True
    print("MANIFEST_OK")


def test_defi_llama():
    import urllib.request
    req = urllib.request.Request("https://yields.llama.fi/pools", headers={"User-Agent": "Ottie/1.0"})
    data = json.loads(urllib.request.urlopen(req, timeout=30).read())
    pools = data.get("data", [])
    aave = [p for p in pools if p.get("project") == "aave-v3" and "WETH" in p.get("symbol", "") and p.get("chain") == "Ethereum"]
    lido = [p for p in pools if p.get("project") == "lido" and p.get("chain") == "Ethereum"]
    if aave:
        print(f"Aave V3 WETH APY: {aave[0]['apy']:.4f}%")
    if lido:
        print(f"Lido stETH APY:   {lido[0]['apy']:.4f}%")
    if aave and lido:
        diff = lido[0]["apy"] - aave[0]["apy"]
        direction = "beats" if diff > 0 else "trails"
        print(f"Spread: {diff:+.4f}% (Lido {direction} Aave)")
    print("BENCHMARK_OK")


def test_8004scan():
    import urllib.request
    wallet = os.environ.get("E2E_WALLET_ADDRESS", "").lower()
    req = urllib.request.Request(
        f"https://www.8004scan.io/api/v1/public/agents?q=Ottie&chainId=11155111",
        headers={"User-Agent": "Ottie/1.0"},
    )
    resp = json.loads(urllib.request.urlopen(req, timeout=15).read())
    agents = resp.get("data", [])
    ottie = [a for a in agents if a.get("name") == "Ottie" and a.get("owner_address", "").lower() == wallet]
    assert ottie, "Ottie not found on 8004scan"
    agent = ottie[0]
    print(f"Agent ID:    {agent['agent_id']}")
    print(f"Token ID:    {agent['token_id']}")
    print(f"Owner:       {agent['owner_address']}")
    print(f"Name:        {agent['name']}")
    print(f"Description: {agent['description'][:60]}...")
    print(f"Created:     {agent['created_at']}")
    print("8004SCAN_VERIFIED")


if __name__ == "__main__":
    func = sys.argv[1] if len(sys.argv) > 1 else "test_agent_manifest"
    globals()[func]()
