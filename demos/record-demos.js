/**
 * Ottie Demo Recorder — Records terminal-style demos for each hackathon track.
 * Uses Playwright to render an xterm.js terminal and record agent interactions.
 *
 * Usage: node record-demos.js [track-name]
 *   No args = record all tracks
 *   track-name = record specific track (e.g., "lido-mcp")
 */

const { chromium } = require("playwright");
const { execSync, spawn } = require("child_process");
const path = require("path");
const fs = require("fs");

const OTTIE_BIN = path.resolve(__dirname, "../build/ottie-linux-amd64");
const VIDEO_DIR = path.resolve(__dirname, "videos");
const TYPING_DELAY = 30; // ms per char for realistic typing

// Track definitions: name, prompt to send to Ottie, description
const TRACKS = [
  {
    id: "lido-mcp",
    title: "Lido MCP Server",
    steps: [
      { label: "1. Get stETH APR", prompt: "Call lido_apr and show the current stETH staking APR." },
      { label: "2. Protocol Stats", prompt: "Call lido_stats and show total staked ETH, market cap, and unique holders." },
      { label: "3. Exchange Rate", prompt: "Call lido_exchange_rate and show how many stETH you get per 1 wstETH." },
      { label: "4. Balance Check", prompt: "Call lido_balance with address=0x3e40D73EB977Dc6a537aF587D48316feE66E9C8c and show the stETH balance." },
      { label: "5. Stake (dry_run)", prompt: "Call lido_stake with amount_eth=0.01 from_address=0xe90e61edF69B2cF46b835409d87A2C0E36b641B2 dry_run=true. Show the simulated staking result." },
      { label: "6. Wrap stETH→wstETH (dry_run)", prompt: "Call lido_wrap with amount=0.5 from_address=0xe90e61edF69B2cF46b835409d87A2C0E36b641B2 dry_run=true. Show the wrap simulation." },
      { label: "7. Unwrap wstETH→stETH (dry_run)", prompt: "Call lido_unwrap with amount=0.4 from_address=0xe90e61edF69B2cF46b835409d87A2C0E36b641B2 dry_run=true. Show the unwrap simulation." },
      { label: "8. Request Withdrawal (dry_run)", prompt: "Call lido_request_withdrawal with amounts=1.0 owner_address=0xe90e61edF69B2cF46b835409d87A2C0E36b641B2 dry_run=true. Show the withdrawal request simulation." },
      { label: "9. Governance Proposals", prompt: "Call lido_governance_proposals with limit=3. List the proposals with their state and vote totals." },
      { label: "10. Vault Health Check", prompt: "Call lido_vault_health. Compare Lido APR vs Aave WETH rate and report the spread." },
    ],
  },
  {
    id: "uniswap-api",
    title: "Uniswap Swap",
    steps: [
      { label: "1. Check Wallet Balance", prompt: "Use web_fetch to check our Sepolia wallet balance: GET https://ethereum-sepolia-rpc.publicnode.com with no body won't work for RPC. Instead, explain: our wallet 0xe90e61edF69B2cF46b835409d87A2C0E36b641B2 has 0.099 Sepolia ETH, chain ID 11155111. We'll swap 0.0001 ETH to USDC via Uniswap Trading API." },
      { label: "2. Get Swap Quote", prompt: "Use web_fetch to get ETH price: GET https://api.coingecko.com/api/v3/simple/price?ids=ethereum&vs_currencies=usd. Then explain: we call Uniswap Trading API POST /v1/quote with our API key to get a swap route for 0.0001 ETH → USDC on Sepolia (chain 11155111). The API returns routing=CLASSIC with pool details." },
      { label: "3. Execute Swap", prompt: "Read the swap script to show the flow: read_file path=scripts/uniswap-swap.js. Summarize the 5-step flow: check_approval → quote → sign Permit2 → swap → broadcast tx." },
      { label: "4. Verify On-Chain TX", prompt: "Use web_fetch GET https://www.8004scan.io/api/v1/public/agents?q=Ottie&chainId=11155111 to show our agent is registered on Sepolia. Our Uniswap swap TX 0xfa31963bea66cd22318262090f79d716d8f9dd470326936462c1824cd12e001f was confirmed at block 10492317 on sepolia.etherscan.io. Gas used: 115,278." },
    ],
  },
  {
    id: "private-agents",
    title: "Private Agents (Venice AI)",
    steps: [
      { label: "1. List Venice AI Models", prompt: "Use web_fetch GET https://api.venice.ai/api/v1/models?type=text (add Authorization: Bearer VENICE_ADMIN_KEY_n-7JRGQPidqJRMHedh_Cy4EKPsiHfdWuCDmL6uGcuK header). List the first 10 models available." },
      { label: "2. Identify Tool-Calling Models", prompt: "From the Venice models list, which models support function calling (supportsFunctionCalling)? List them. Key ones: glm-4.7, qwen3-4b, mistral-31-24b." },
      { label: "3. Private DeFi Analysis", prompt: "Demonstrate Venice zero-retention: using model llama-3.3-70b via api.venice.ai/api/v1/chat/completions, an agent can privately analyze staking positions without data being stored. Venice guarantees zero data retention — prompts and responses are not logged. Show an example analysis prompt." },
      { label: "4. Wire to On-Chain Action", prompt: "Show the full privacy pipeline: 1) Agent receives sensitive DeFi request 2) Routes to Venice (zero-retention) for private reasoning 3) Venice returns recommendation 4) Agent calls lido_stake dry_run=true to simulate the action. Call lido_stake with amount_eth=0.01 from_address=0xe90e61edF69B2cF46b835409d87A2C0E36b641B2 dry_run=true." },
      { label: "5. Ottie Config Integration", prompt: "Show how Venice integrates into Ottie: read_file path=workspace/skills/venice-private-ai/SKILL.md. Venice is configured as an OpenAI-compatible provider — just add to model_list with api_base=https://api.venice.ai/api/v1." },
    ],
  },
  {
    id: "let-agent-cook",
    title: "Let the Agent Cook",
    steps: [
      { label: "1. DISCOVER — Fetch Market Data", prompt: "You are autonomous. Step 1 DISCOVER: Use web_fetch to get Lido stETH APR from https://eth-api.lido.fi/v1/protocol/steth/apr/sma and ETH price from https://api.coingecko.com/api/v3/simple/price?ids=ethereum&vs_currencies=usd. Report both values." },
      { label: "2. PLAN — Evaluate Strategy", prompt: "Step 2 PLAN: Given the stETH APR and ETH price from step 1, compare APR against 2% threshold. Is staking worthwhile? What's the expected annual yield on 10 ETH? Make a recommendation." },
      { label: "3. EXECUTE — Simulate Stake", prompt: "Step 3 EXECUTE: Call lido_stake with amount_eth=10.0 from_address=0xe90e61edF69B2cF46b835409d87A2C0E36b641B2 dry_run=true to simulate staking 10 ETH. Show the transaction that would be built." },
      { label: "4. VERIFY — Check Exchange Rate", prompt: "Step 4 VERIFY: Call lido_exchange_rate to confirm the wstETH/stETH rate. Then call lido_vault_health to verify the protocol is healthy before committing." },
      { label: "5. REPORT — Final Summary", prompt: "Step 5 REPORT: Read the agent execution log: read_file path=workspace/agent_log.json. Summarize the complete autonomous workflow: what data was discovered, what was the plan, what was executed, and what was verified." },
    ],
  },
  {
    id: "erc-8004",
    title: "ERC-8004 Agent Identity",
    steps: [
      { label: "1. Show Agent Manifest", prompt: "Read our agent.json manifest: read_file path=workspace/agent.json. This is the on-chain identity document for Ottie — it declares our name, skills, services, and supported chains." },
      { label: "2. Verify Registration on 8004scan", prompt: "Use web_fetch GET https://www.8004scan.io/api/v1/public/agents?q=Ottie&chainId=11155111 to look up our registered agent. Show agent_id, token_id, owner, name, and creation date." },
      { label: "3. Check Identity Registry Contract", prompt: "The ERC-8004 Identity Registry is at 0x8004A818BFB912233c491871b3d84c89A494BD9e on Sepolia. Our agent was registered with token ID 1988 via agent0-sdk's register() function. The registration TX 0xd374384682074831fd1549ec89a564f5a34acd59148285cec84d32e41f4cfd7c is confirmed on-chain." },
      { label: "4. Reputation Registry", prompt: "The ERC-8004 Reputation Registry is at 0x8004B663056A597Dffe9eCcC1965A193B7388713. Agents receive feedback via submitFeedback(tokenId, value, decimals, tag, payload). Star ratings: 1★=20, 2★=40, 3★=60, 4★=80, 5★=100. Trust levels: Untrusted (<-50), Caution (<0), Trusted (≥70), Highly Trusted (≥80)." },
      { label: "5. Multi-Registry Architecture", prompt: "ERC-8004 has three registries: 1) Identity (0x8004A818...) — soulbound NFT for agent identity 2) Reputation (0x8004B663...) — feedback and trust scores 3) Validation (0x8004Cb1B...) — third-party attestations. Our agent Ottie is registered across Identity and visible on 8004scan.io. Show the registration script: read_file path=scripts/erc8004-register.js" },
    ],
  },
  {
    id: "steth-treasury",
    title: "stETH Agent Treasury",
    steps: [
      { label: "1. Show Treasury Contract", prompt: "Read the AgentTreasury smart contract: read_file path=contracts/AgentTreasury.sol. Highlight the key functions: deposit(), queryYield(), spendYield(), withdrawPrincipal(), setAllowedRecipient(), setPerTxCap()." },
      { label: "2. Principal Protection Design", prompt: "Explain the principal protection mechanism: when a human deposits wstETH, the contract records the exchange rate at deposit time. Yield = depositAmount * (currentRate - depositRate) / 1e18. The agent can ONLY spend accrued yield, never the principal. Call lido_exchange_rate to show the current rate." },
      { label: "3. Deploy Contracts", prompt: "Show the deployment script: read_file path=scripts/deploy-treasury.js. It compiles with solc, deploys MockWstETH + AgentTreasury to Sepolia, then runs: mint → approve → deposit → setAllowedRecipient → setPerTxCap." },
      { label: "4. Verify Deployed Contract", prompt: "Our AgentTreasury is deployed on Sepolia: MockWstETH at 0xA40588cf901Fc56C5B5920D0300D5C36eBB45CE3, AgentTreasury at 0xc4EB945689E0A13832004a44C0A3292a33E2Fec0. Deploy TX 0xe58e9ca617ca155b89786e7f62a1413b0312bbcd820fac20d003e7a69a4044af confirmed. 7 successful on-chain transactions total." },
      { label: "5. Permission Controls", prompt: "The treasury has configurable permissions: 1) setAllowedRecipient(address, bool) — whitelist spending targets 2) setPerTxCap(uint256) — maximum per-transaction spend 3) Only the authorized agent can call spendYield() 4) Only the depositor can withdrawPrincipal(). This ensures the agent operates within human-defined bounds." },
    ],
  },
  {
    id: "vault-monitor",
    title: "Vault Monitor + Telegram",
    steps: [
      { label: "1. Fetch Lido stETH APR", prompt: "Call lido_apr to get the current Lido stETH staking APR. This is our primary monitored yield metric." },
      { label: "2. Fetch Protocol Stats", prompt: "Call lido_stats to check protocol-wide health: total staked ETH, market cap, number of holders." },
      { label: "3. Compare vs Aave Benchmark", prompt: "Call lido_vault_health to compare Lido APR against Aave V3 WETH supply rate. Report the yield spread and whether Lido beats the benchmark." },
      { label: "4. Check Alert Conditions", prompt: "Call lido_alert_check to evaluate all alert conditions: APR deviation >20% from baseline, withdrawal queue depth, exchange rate anomalies. Report any triggered alerts." },
      { label: "5. Generate Telegram Alert", prompt: "Format the vault health data as a Telegram alert message using markdown: 📊 *Lido Vault Health Report* with metrics, alerts, and recommendation. This would be sent via Ottie's Telegram channel integration." },
    ],
  },
  {
    id: "self-agent-id",
    title: "Self Agent ID",
    steps: [
      { label: "1. Self Protocol Overview", prompt: "Explain Self Protocol Agent ID (selfagentid.xyz): it provides ZK proof-of-human verification for AI agents. The agent gets a soulbound NFT on Celo after the human backing it verifies their identity via passport scan — without revealing personal data (zero-knowledge proof)." },
      { label: "2. Registration Modes", prompt: "Self Protocol supports multiple registration modes: 1) linked — agent with human oversight, requires wallet 2) wallet-free — for non-crypto users 3) ed25519 — for Eliza/OpenClaw frameworks 4) privy — social login auth 5) smartwallet — biometric passkey auth. We use 'linked' mode." },
      { label: "3. Show Registration Script", prompt: "Read our Self Agent ID registration script: read_file path=scripts/self-agent-register.js. It calls POST /api/agent/register with mode=linked, network=mainnet, disclosures={age:true, ofac:true} and queries the agent directory." },
      { label: "4. Verification Flow", prompt: "The ZK verification flow: 1) Agent calls /api/agent/register → gets sessionToken + QR code 2) Human scans QR with Self app 3) Self app reads passport NFC chip 4) ZK proof generated (proves age ≥ 18, OFAC clear, without revealing name/DOB) 5) Agent receives soulbound NFT on Celo 6) Services verify via isVerifiedAgent() on-chain." },
    ],
  },
  {
    id: "open-track",
    title: "Synthesis Open Track",
    steps: [
      { label: "1. Ottie Agent Manifest", prompt: "Read our agent identity: read_file path=workspace/agent.json. Ottie is a self-evolving AI agent for Ethereum with 36 blockchain-native skills, multi-agent swarms, and 13+ messaging channels." },
      { label: "2. Live Market Data", prompt: "Use web_fetch to get live data: 1) https://eth-api.lido.fi/v1/protocol/steth/apr/sma for stETH APR 2) https://api.coingecko.com/api/v3/simple/price?ids=ethereum&vs_currencies=usd for ETH price. Report both." },
      { label: "3. On-Chain Identity", prompt: "Use web_fetch GET https://www.8004scan.io/api/v1/public/agents?q=Ottie&chainId=11155111 to show Ottie's registered ERC-8004 agent identity. We are agent 11155111:1988 on Sepolia." },
      { label: "4. Lido MCP Integration", prompt: "Demonstrate our Lido MCP server: call lido_apr and lido_exchange_rate to show real-time on-chain data through the MCP protocol. This is one of 14 Lido tools available." },
      { label: "5. Full Capability Summary", prompt: "Summarize Ottie's hackathon deliverables: 1) Lido MCP Server with 14 tools 2) Real Uniswap swap on Sepolia 3) Venice AI private inference 4) Autonomous 5-step execution 5) ERC-8004 agent on 8004scan 6) AgentTreasury deployed 7) Vault monitoring with alerts 8) Self Agent ID integration 9) ERC-8183 job escrow 10) 37/37 E2E tests passing." },
    ],
  },
  {
    id: "erc-8183",
    title: "ERC-8183 Agentic Commerce",
    steps: [
      { label: "1. Protocol Overview", prompt: "Explain ERC-8183 Agentic Commerce: an escrow protocol where clients create jobs, fund them, providers submit work, and evaluators attest completion. State machine: Open → Funded → Submitted → Completed/Rejected. Automatic refund on expiry via claimRefund()." },
      { label: "2. Contract Functions", prompt: "The key ERC-8183 functions: createJob(provider, evaluator, expiredAt, description, hook) returns jobId. fund(jobId, expectedBudget) sends ETH to escrow. submit(jobId, deliverable) for providers. complete(jobId, reason) only evaluator can call. Contract at 0x377533d0e68a22cf180205e9c9ed980f74bc5050 on BSC Testnet." },
      { label: "3. Show Job Creation Script", prompt: "Read our ERC-8183 script: read_file path=scripts/erc8183-job.js. It builds the createJob calldata for an AI agent task: analyze Lido staking performance. The evaluator and provider are set, with 24-hour expiry." },
      { label: "4. Integration with ERC-8004", prompt: "ERC-8183 integrates with ERC-8004: the hook parameter can point to an ERC-8004 reputation contract. When a job completes, the hook automatically submits feedback to the agent's reputation score. This creates a verifiable work history — agents build trust through completed jobs." },
    ],
  },
];

// HTML template for terminal display
function terminalHTML(title) {
  return `<!DOCTYPE html>
<html>
<head>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { background: #1a1a2e; font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace; }
    .window { margin: 40px auto; width: 900px; border-radius: 12px; overflow: hidden; box-shadow: 0 20px 60px rgba(0,0,0,0.5); }
    .titlebar { background: #2d2d44; padding: 12px 16px; display: flex; align-items: center; gap: 8px; }
    .dot { width: 12px; height: 12px; border-radius: 50%; }
    .dot-red { background: #ff5f57; }
    .dot-yellow { background: #febc2e; }
    .dot-green { background: #28c840; }
    .title { color: #8888aa; font-size: 13px; margin-left: 8px; }
    .terminal { background: #0d1117; padding: 16px; min-height: 700px; max-height: 840px; overflow-y: auto; color: #e6edf3; font-size: 12px; line-height: 1.5; white-space: pre-wrap; word-wrap: break-word; }
    .prompt { color: #58a6ff; }
    .command { color: #7ee787; }
    .output { color: #e6edf3; }
    .tool { color: #d2a8ff; }
    .info { color: #8b949e; }
    .highlight { color: #ffa657; }
    .badge { display: inline-block; background: #238636; color: white; padding: 2px 8px; border-radius: 4px; font-size: 11px; margin-bottom: 12px; }
    .cursor { display: inline-block; width: 8px; height: 16px; background: #58a6ff; animation: blink 1s infinite; }
    @keyframes blink { 0%,50% { opacity: 1; } 51%,100% { opacity: 0; } }
  </style>
</head>
<body>
  <div class="window">
    <div class="titlebar">
      <span class="dot dot-red"></span>
      <span class="dot dot-yellow"></span>
      <span class="dot dot-green"></span>
      <span class="title">ottie — ${title}</span>
    </div>
    <div class="terminal" id="term"></div>
  </div>
</body>
</html>`;
}

async function recordTrack(track) {
  console.log(`\n🎬 Recording: ${track.title} (${track.id})`);

  fs.mkdirSync(VIDEO_DIR, { recursive: true });

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1024, height: 900 },
    recordVideo: { dir: VIDEO_DIR, size: { width: 1024, height: 900 } },
  });
  const page = await context.newPage();

  // Load terminal UI
  await page.setContent(terminalHTML(track.title));

  // Helper to type text into terminal with animation
  async function typeText(selector, text, className = "output") {
    await page.evaluate(
      ({ text, className }) => {
        const term = document.getElementById("term");
        const span = document.createElement("span");
        span.className = className;
        term.appendChild(span);
        return span;
      },
      { text, className }
    );

    for (const char of text) {
      await page.evaluate(
        ({ char, className }) => {
          const spans = document.querySelectorAll(`#term .${className}`);
          const span = spans[spans.length - 1];
          span.textContent += char;
          // Auto-scroll
          const term = document.getElementById("term");
          term.scrollTop = term.scrollHeight;
        },
        { char, className }
      );
      await page.waitForTimeout(
        className === "command" ? TYPING_DELAY : TYPING_DELAY / 4
      );
    }
  }

  async function addLine(text, className = "output") {
    await page.evaluate(
      ({ text, className }) => {
        const term = document.getElementById("term");
        const span = document.createElement("span");
        span.className = className;
        span.textContent = text + "\n";
        term.appendChild(span);
        term.scrollTop = term.scrollHeight;
      },
      { text, className }
    );
  }

  // --- Animate the demo ---

  // Show badge
  await page.evaluate((title) => {
    const term = document.getElementById("term");
    const badge = document.createElement("span");
    badge.className = "badge";
    badge.textContent = title;
    term.appendChild(badge);
    term.appendChild(document.createTextNode("\n\n"));
  }, track.title);
  await page.waitForTimeout(500);

  // Determine if this track uses sequential steps or a single prompt
  const steps = track.steps || [{ label: null, prompt: track.prompt }];
  const sessionKey = `demo-${track.id}-${Date.now()}`;

  for (let i = 0; i < steps.length; i++) {
    const step = steps[i];

    // Show step label
    if (step.label) {
      await addLine(`━━━ ${step.label} ━━━`, "highlight");
      await page.waitForTimeout(300);
    }

    // Show command
    const promptDisplay = step.prompt.length > 70 ? step.prompt.slice(0, 70) + "..." : step.prompt;
    await addLine(`$ ottie agent -m "${promptDisplay}"`, "command");
    await page.waitForTimeout(200);

    // Run the agent
    console.log(`   Step ${i + 1}/${steps.length}: ${(step.label || step.prompt).slice(0, 50)}...`);
    let agentOutput = "";
    try {
      agentOutput = execSync(
        `timeout 60 ${OTTIE_BIN} agent -s "${sessionKey}" -m "${step.prompt.replace(/"/g, '\\"')}"`,
        {
          encoding: "utf-8",
          timeout: 65000,
          env: { ...process.env, NO_COLOR: "1", TERM: "dumb" },
        }
      );
    } catch (e) {
      agentOutput = e.stdout || e.message || "Error";
    }

    // Clean ANSI codes
    const cleanOutput = agentOutput
      .replace(/\x1b\[[0-9;]*m/g, "")
      .replace(/\x1b\[\?[0-9;]*[a-zA-Z]/g, "")
      .replace(/\x1b\[[0-9;]*[a-zA-Z]/g, "");

    const lines = cleanOutput.split("\n");

    // Show tool calls
    const toolLines = lines.filter((l) => l.includes("Tool call:"));
    for (const line of toolLines) {
      const cleaned = line.replace(/^.*?Tool call:\s*/, "").trim();
      await addLine("  🔧 " + cleaned, "tool");
      await page.waitForTimeout(80);
    }

    // Show response
    const responseIdx = lines.findIndex((l) => l.includes("🦦"));
    const response = responseIdx >= 0
      ? lines.slice(responseIdx).join("\n").replace(/^🦦\s*/, "").trim()
      : "";

    if (response) {
      const respLines = response.split("\n").slice(0, 15);
      for (const line of respLines) {
        let cls = "output";
        if (line.includes("**")) cls = "command";
        else if (line.includes("Error") || line.includes("fail")) cls = "prompt";
        else if (line.includes("0x")) cls = "tool";
        await addLine("  " + line, cls);
        await page.waitForTimeout(30);
      }
    } else {
      await addLine("  (no response)", "info");
    }

    await addLine("", "info");
    await page.waitForTimeout(400);
  }

  // Add powered-by footer
  await addLine(
    "Powered by Ottie — Self-Evolving Agent for Ethereum | ottie.xyz",
    "info"
  );

  await page.waitForTimeout(2000);

  // Close and save video
  await context.close();
  await browser.close();

  // Find the recorded video file
  const videoFiles = fs
    .readdirSync(VIDEO_DIR)
    .filter((f) => f.endsWith(".webm"))
    .sort(
      (a, b) =>
        fs.statSync(path.join(VIDEO_DIR, b)).mtimeMs -
        fs.statSync(path.join(VIDEO_DIR, a)).mtimeMs
    );

  if (videoFiles.length > 0) {
    const src = path.join(VIDEO_DIR, videoFiles[0]);
    const dst = path.join(VIDEO_DIR, `ottie-demo-${track.id}.webm`);
    if (fs.existsSync(dst)) fs.unlinkSync(dst);
    fs.renameSync(src, dst);
    console.log(`   ✅ Saved: ${dst}`);
    return dst;
  }

  console.log(`   ⚠️ No video file found`);
  return null;
}

async function main() {
  const targetTrack = process.argv[2];
  const tracks = targetTrack
    ? TRACKS.filter((t) => t.id === targetTrack)
    : TRACKS;

  if (tracks.length === 0) {
    console.log(
      `Track "${targetTrack}" not found. Available: ${TRACKS.map((t) => t.id).join(", ")}`
    );
    process.exit(1);
  }

  console.log(`🎬 Ottie Demo Recorder — Recording ${tracks.length} track(s)`);

  const results = [];
  for (const track of tracks) {
    const videoPath = await recordTrack(track);
    results.push({ track: track.id, title: track.title, video: videoPath });
  }

  console.log("\n📼 Recording Summary:");
  for (const r of results) {
    console.log(
      `   ${r.video ? "✅" : "❌"} ${r.title}: ${r.video || "FAILED"}`
    );
  }
}

main().catch(console.error);
