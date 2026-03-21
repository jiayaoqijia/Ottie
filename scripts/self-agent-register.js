#!/usr/bin/env node
const https = require("https");
const http = require("http");

// Self Agent ID registration flow
// Note: Full registration requires human passport scan via Self app
// This demonstrates the initiation flow

async function fetchJSON(url, options = {}) {
  return new Promise((resolve, reject) => {
    const mod = url.startsWith("https") ? https : http;
    const req = mod.request(url, {
      method: options.method || "GET",
      headers: { "Content-Type": "application/json", ...options.headers },
    }, (res) => {
      let data = "";
      res.on("data", (chunk) => data += chunk);
      res.on("end", () => {
        try { resolve({ status: res.statusCode, data: JSON.parse(data) }); }
        catch { resolve({ status: res.statusCode, data: data }); }
      });
    });
    req.on("error", reject);
    if (options.body) req.write(JSON.stringify(options.body));
    req.end();
  });
}

async function main() {
  console.log("=== Self Agent ID Registration (Track 8) ===\n");

  const walletAddress = process.env.E2E_WALLET_ADDRESS || "0xe90e61edF69B2cF46b835409d87A2C0E36b641B2";

  // Step 1: Initiate registration
  console.log("Step 1: Initiating agent registration...");
  console.log(`  Wallet: ${walletAddress}`);
  console.log(`  Mode: linked (agent with human oversight)`);
  console.log(`  Network: mainnet (Celo 42220)`);

  try {
    const regResult = await fetchJSON("https://selfagentid.xyz/api/agent/register", {
      method: "POST",
      body: {
        mode: "linked",
        network: "mainnet",
        humanAddress: walletAddress,
        disclosures: {
          age: true,
          ofac: true
        }
      }
    });

    console.log(`  Registration status: ${regResult.status}`);
    console.log(`  Response: ${JSON.stringify(regResult.data, null, 2)}`);

    if (regResult.data.sessionToken) {
      console.log(`\n  Session Token: ${regResult.data.sessionToken}`);
      console.log(`  QR Code URL: ${regResult.data.qrCode || "embedded in response"}`);
      console.log(`  Deep Link: ${regResult.data.deepLink || "self://verify/" + regResult.data.sessionToken}`);
      console.log("\n  Next step: Human scans QR code with Self app to complete ZK proof");
    }
  } catch (err) {
    console.log(`  Registration API response: ${err.message}`);
    console.log("  Note: Full registration requires Self app interaction");
  }

  // Step 2: Check existing agent info (query API)
  console.log("\nStep 2: Querying agent directory...");
  try {
    const infoResult = await fetchJSON(`https://selfagentid.xyz/api/agent/agents/42220/${walletAddress}`);
    console.log(`  Query status: ${infoResult.status}`);
    if (infoResult.status === 200) {
      console.log(`  Registered agents: ${JSON.stringify(infoResult.data, null, 2)}`);
    } else {
      console.log("  No agents registered for this wallet yet");
    }
  } catch (err) {
    console.log(`  Query error: ${err.message}`);
  }

  console.log("\n=== Self Agent ID Demo Complete ===");
  console.log("Registration flow initiated. Full completion requires Self app passport scan.");
}

main().catch(console.error);
