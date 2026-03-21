const { SDK } = require("agent0-sdk");
require("dotenv").config({ path: require("path").join(__dirname, "..", ".env") });

async function main() {
  if (!process.env.E2E_WALLET_PRIVATE_KEY) {
    throw new Error("E2E_WALLET_PRIVATE_KEY not set in .env");
  }
  if (!process.env.SEPOLIA_RPC) {
    throw new Error("SEPOLIA_RPC not set in .env");
  }

  const rpcUrl = process.env.SEPOLIA_RPC;
  const privateKey = process.env.E2E_WALLET_PRIVATE_KEY.startsWith("0x")
    ? process.env.E2E_WALLET_PRIVATE_KEY
    : `0x${process.env.E2E_WALLET_PRIVATE_KEY}`;

  console.log("Initializing agent0 SDK on Sepolia (chainId: 11155111)...");

  const sdk = new SDK({
    chainId: 11155111,
    rpcUrl,
    privateKey,
  });

  const walletAddress = await sdk.chainClient.getAddress();
  console.log(`Wallet: ${walletAddress}`);
  console.log(`Identity Registry: ${sdk.identityRegistryAddress()}`);
  console.log(`Reputation Registry: ${sdk.reputationRegistryAddress()}`);

  // --- Step 1: Register agent identity ---
  console.log("\n=== Registering Agent Identity ===");

  const agent = sdk.createAgent(
    "Ottie",
    "Self-Evolving Agent for Ethereum — purpose-built AI agent for Ethereum and crypto. Single binary, 36 blockchain-native skills, multi-agent swarms, 13+ messaging channels.",
    "https://ottie.xyz/logo.png"
  );

  // Set the agent URI to point to the hosted agent.json
  const agentUri = "https://raw.githubusercontent.com/jiayaoqijia/ottie/main/workspace/agent.json";

  // Add MCP endpoint
  agent.setMCP("https://ottie.xyz/mcp");

  // Set agent as active
  agent.setActive(true);

  // Register on-chain — returns a TransactionHandle
  console.log("Submitting registration transaction...");
  let txHandle;
  let alreadyRegistered = false;
  try {
    txHandle = await agent.registerOnChain();
    console.log(`Registration TX: ${txHandle.hash}`);
    console.log("Waiting for confirmation...");
    await txHandle.waitMined();
    console.log("Registration confirmed on-chain");
  } catch (err) {
    // If agent is already registered, continue to feedback
    if (err.message?.includes("already") || err.message?.includes("exists")) {
      console.log("Agent may already be registered, continuing...");
      alreadyRegistered = true;
    } else {
      throw err;
    }
  }

  // Extract agent ID (agentId is a getter, not a method)
  const agentId = agent.agentId;
  const tokenId = agentId ? agentId.split(":").pop() : null;
  console.log(`Agent ID: ${agentId || "pending"}`);
  console.log(`Token ID: ${tokenId || "pending"}`);

  if (txHandle && !alreadyRegistered) {
    console.log(`Registration TX: ${txHandle.hash}`);
  }

  // --- Step 2: Submit reputation feedback ---
  console.log("\n=== Submitting Reputation Feedback ===");

  if (agentId) {
    try {
      console.log(`Giving feedback for agent: ${agentId}`);
      const feedbackHandle = await sdk.giveFeedback(
        agentId,    // agentId (chainId:tokenId)
        1.0,        // positive feedback value
        "quality",  // tag1
        "demo",     // tag2
      );
      console.log(`Feedback TX: ${feedbackHandle.hash}`);
      console.log("Waiting for feedback confirmation...");
      const feedbackResult = await feedbackHandle.waitMined();
      console.log("Feedback confirmed:", JSON.stringify(feedbackResult, (_, v) => typeof v === "bigint" ? v.toString() : v).substring(0, 500));
    } catch (err) {
      console.log("Feedback submission error:", err.message);
      console.log("(This may fail if the agent was just registered and indexing is pending)");
    }
  } else {
    console.log("Skipping feedback — agent ID not yet available");
  }

  // --- Summary ---
  console.log("\n=== ERC-8004 Registration Complete ===");
  console.log(`Chain: Sepolia (11155111)`);
  console.log(`Wallet: ${walletAddress}`);
  console.log(`Agent ID: ${agentId || "check explorer"}`);
  console.log(`Token ID: ${tokenId || "check explorer"}`);
  console.log(`Identity Registry: ${sdk.identityRegistryAddress()}`);
  console.log(`Reputation Registry: ${sdk.reputationRegistryAddress()}`);
  console.log(`Explorer: https://sepolia.etherscan.io/address/${sdk.identityRegistryAddress()}`);
  if (agentId) {
    console.log(`8004scan: https://www.8004scan.io/agent/${agentId}`);
  }
}

main().catch((err) => {
  console.error("Error:", err.message);
  if (err.cause) console.error("Cause:", err.cause);
  process.exit(1);
});
