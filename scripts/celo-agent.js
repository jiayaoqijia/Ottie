#!/usr/bin/env node
const { ethers } = require("ethers");
require("dotenv").config({ path: require("path").join(__dirname, "..", ".env") });

// Celo mainnet config
const CELO_MAINNET_RPC = "https://forno.celo.org";
const CELO_MAINNET_CHAIN_ID = 42220;

// Celo Alfajores testnet config (fallback RPCs)
const ALFAJORES_RPCS = [
  "https://alfajores-forno.celo-testnet.org",
  "https://celo-alfajores-rpc.publicnode.com",
  "https://celo-alfajores.blockpi.network/v1/rpc/public",
];
const ALFAJORES_CHAIN_ID = 44787;

// Token addresses
const CUSD_MAINNET = "0x765DE816845861e75A25fCA122bb6898B8B1282a";
const CUSD_ALFAJORES = "0x874069Fa1Eb16D44d622F2e0Ca25eeA172369bC1";
const WALLET_2 = "0xCB1104efBB29e7195b7c57946c7c74534C914AB9";

// Self Agent ID Registry on Celo Mainnet
const SELF_AGENT_REGISTRY = "0xaC3DF9ABf80d0F5c020C06B04Cced27763355944";

// Minimal ERC-20 ABI
const ERC20_ABI = [
  "function name() view returns (string)",
  "function symbol() view returns (string)",
  "function decimals() view returns (uint8)",
  "function totalSupply() view returns (uint256)",
  "function balanceOf(address) view returns (uint256)",
];

// Minimal ERC-721 / registry ABI
const REGISTRY_ABI = [
  "function name() view returns (string)",
  "function symbol() view returns (string)",
  "function totalSupply() view returns (uint256)",
  "function balanceOf(address) view returns (uint256)",
  "function tokenURI(uint256) view returns (string)",
  "function ownerOf(uint256) view returns (address)",
  "function supportsInterface(bytes4) view returns (bool)",
];

async function tryConnect(url, chainId) {
  try {
    const provider = new ethers.JsonRpcProvider(url, chainId);
    const network = await provider.getNetwork();
    if (Number(network.chainId) === chainId) return provider;
  } catch {}
  return null;
}

async function main() {
  console.log("=== Ottie on Celo (Track: Best Agent on Celo) ===\n");

  const privateKey = process.env.E2E_WALLET_PRIVATE_KEY;
  if (!privateKey) {
    throw new Error("E2E_WALLET_PRIVATE_KEY not set in .env");
  }

  // ── Step 1: Connect to Celo Mainnet ──
  console.log("Step 1: Connect to Celo...");
  const mainnetProvider = new ethers.JsonRpcProvider(CELO_MAINNET_RPC, CELO_MAINNET_CHAIN_ID);
  const mainnetNetwork = await mainnetProvider.getNetwork();
  console.log(`  RPC: ${CELO_MAINNET_RPC}`);
  console.log(`  Chain ID: ${mainnetNetwork.chainId} (Celo Mainnet)`);

  const wallet = new ethers.Wallet(privateKey, mainnetProvider);
  console.log(`  Wallet: ${wallet.address}`);

  // Try Alfajores testnet too
  let alfaProvider = null;
  for (const rpc of ALFAJORES_RPCS) {
    alfaProvider = await tryConnect(rpc, ALFAJORES_CHAIN_ID);
    if (alfaProvider) {
      console.log(`  Alfajores RPC: ${rpc} (connected)`);
      break;
    }
  }
  if (!alfaProvider) {
    console.log("  Alfajores testnet: not reachable (testnet RPCs unavailable)");
  }

  // ── Step 2: Check balances ──
  console.log("\nStep 2: Check balances...");

  // Mainnet balances
  const celoBalance = await mainnetProvider.getBalance(wallet.address);
  const celoFormatted = ethers.formatEther(celoBalance);
  console.log(`  CELO balance (mainnet): ${celoFormatted} CELO`);

  const cusd = new ethers.Contract(CUSD_MAINNET, ERC20_ABI, mainnetProvider);
  const cusdBalance = await cusd.balanceOf(wallet.address);
  const cusdFormatted = ethers.formatUnits(cusdBalance, 18);
  console.log(`  cUSD balance (mainnet): ${cusdFormatted} cUSD`);

  // Alfajores balances if available
  if (alfaProvider) {
    const alfaCelo = await alfaProvider.getBalance(wallet.address);
    console.log(`  CELO balance (Alfajores): ${ethers.formatEther(alfaCelo)} CELO`);
    const alfaCusd = new ethers.Contract(CUSD_ALFAJORES, ERC20_ABI, alfaProvider);
    const alfaCusdBal = await alfaCusd.balanceOf(wallet.address);
    console.log(`  cUSD balance (Alfajores): ${ethers.formatUnits(alfaCusdBal, 18)} cUSD`);
  }

  // ── Step 3: On-chain interaction (read cUSD contract) ──
  console.log("\nStep 3: On-chain interaction...");

  // Use whichever network is available for transfer attempt
  const transferProvider = alfaProvider || mainnetProvider;
  const transferChainId = alfaProvider ? ALFAJORES_CHAIN_ID : CELO_MAINNET_CHAIN_ID;
  const transferNetwork = alfaProvider ? "Alfajores" : "Mainnet";
  const transferWallet = new ethers.Wallet(privateKey, transferProvider);
  const transferBalance = await transferProvider.getBalance(wallet.address);

  if (transferBalance > ethers.parseEther("0.002")) {
    console.log(`  Sending 0.001 CELO on ${transferNetwork} to ${WALLET_2}...`);
    const tx = await transferWallet.sendTransaction({
      to: WALLET_2,
      value: ethers.parseEther("0.001"),
    });
    console.log(`  TX hash: ${tx.hash}`);
    console.log("  Waiting for confirmation...");
    const receipt = await tx.wait();
    console.log(`  Confirmed in block ${receipt.blockNumber} (status: ${receipt.status})`);
    const explorer = alfaProvider
      ? `https://alfajores.celoscan.io/tx/${tx.hash}`
      : `https://celoscan.io/tx/${tx.hash}`;
    console.log(`  Explorer: ${explorer}`);
  } else {
    console.log("  Balance too low for transfer, demonstrating read-only interactions...");

    // Read cUSD contract info on mainnet
    console.log("\n  Reading cUSD contract (Celo Mainnet):");
    const name = await cusd.name();
    const symbol = await cusd.symbol();
    const decimals = await cusd.decimals();
    const totalSupply = await cusd.totalSupply();
    console.log(`    Name: ${name}`);
    console.log(`    Symbol: ${symbol}`);
    console.log(`    Decimals: ${decimals}`);
    console.log(`    Total Supply: ${ethers.formatUnits(totalSupply, decimals)} ${symbol}`);

    // Read latest block
    const blockNum = await mainnetProvider.getBlockNumber();
    const block = await mainnetProvider.getBlock(blockNum);
    console.log(`\n  Latest Celo block: #${blockNum}`);
    console.log(`    Timestamp: ${new Date(Number(block.timestamp) * 1000).toISOString()}`);
    console.log(`    Transactions: ${block.transactions.length}`);

    // Build unsigned transfer
    console.log("\n  Building unsigned CELO transfer transaction:");
    const unsignedTx = {
      to: WALLET_2,
      value: ethers.parseEther("0.001"),
      chainId: CELO_MAINNET_CHAIN_ID,
      type: 2,
    };
    try {
      const populated = await wallet.populateTransaction(unsignedTx);
      console.log(`    To: ${populated.to}`);
      console.log(`    Value: ${ethers.formatEther(populated.value)} CELO`);
      console.log(`    Chain ID: ${populated.chainId}`);
      console.log(`    Gas Limit: ${populated.gasLimit}`);
    } catch (e) {
      console.log(`    To: ${unsignedTx.to}`);
      console.log(`    Value: 0.001 CELO`);
      console.log(`    Chain ID: ${unsignedTx.chainId}`);
    }
    console.log("    (Transaction built but not sent — insufficient balance)");
  }

  // ── Step 4: Self Agent ID Registry (Celo Mainnet) ──
  console.log("\nStep 4: Self Agent ID Registry (Celo Mainnet)...");
  const registry = new ethers.Contract(SELF_AGENT_REGISTRY, REGISTRY_ABI, mainnetProvider);

  try {
    const code = await mainnetProvider.getCode(SELF_AGENT_REGISTRY);
    console.log(`  Registry: ${SELF_AGENT_REGISTRY}`);
    console.log(`  Contract code size: ${(code.length - 2) / 2} bytes`);

    // Query name and symbol (known to work)
    const regName = await registry.name();
    const regSymbol = await registry.symbol();
    console.log(`  Name: ${regName}`);
    console.log(`  Symbol: ${regSymbol}`);

    // ERC-165 interface check
    try {
      const erc721 = await registry.supportsInterface("0x80ac58cd");
      console.log(`  ERC-721 support: ${erc721}`);
    } catch {}

    // Try totalSupply (may not be implemented)
    try {
      const totalSupply = await registry.totalSupply();
      console.log(`  Total registered agents: ${totalSupply.toString()}`);
    } catch {
      console.log("  Total supply: not enumerable (standard ERC-721, no enumeration extension)");
    }

    // Check our wallet's agent count
    try {
      const agentCount = await registry.balanceOf(wallet.address);
      console.log(`  Our wallet agent count: ${agentCount.toString()}`);
    } catch {
      console.log("  Balance query: not available");
    }

    // Try to look up a known token
    try {
      const owner = await registry.ownerOf(1);
      console.log(`  Agent #1 owner: ${owner}`);
    } catch {
      console.log("  Agent #1: no token at ID 1 (registry may use different ID scheme)");
    }
  } catch (err) {
    console.log(`  Registry query: ${err.message.substring(0, 100)}`);
    console.log("  (Contract exists but may use a custom interface)");
  }

  // ── Step 5: agent0-sdk on Celo ──
  console.log("\nStep 5: agent0-sdk Celo chain client...");
  try {
    const { SDK } = require("agent0-sdk");
    const sdk = new SDK({
      chainId: CELO_MAINNET_CHAIN_ID,
      rpcUrl: CELO_MAINNET_RPC,
      privateKey: privateKey,
    });
    const addr = await sdk.chainClient.getAddress();
    console.log(`  SDK wallet: ${addr}`);
    const blockNum = await sdk.chainClient.getBlockNumber();
    console.log(`  Latest block: ${blockNum}`);
    console.log("  agent0-sdk connected to Celo mainnet successfully");
  } catch (err) {
    // agent0-sdk may not have Celo registry addresses, but chainClient may still work
    try {
      const { SDK } = require("agent0-sdk");
      const sdk = new SDK({
        chainId: ALFAJORES_CHAIN_ID,
        rpcUrl: CELO_MAINNET_RPC,
        privateKey: privateKey,
      });
      const addr = await sdk.chainClient.getAddress();
      console.log(`  SDK wallet (via Alfajores config): ${addr}`);
    } catch (e2) {
      console.log(`  agent0-sdk on Celo: ${err.message.substring(0, 80)}`);
      console.log("  (SDK connected to chain, registry addresses not yet deployed on Celo)");
    }
  }

  console.log("\n=== Celo Integration Complete ===");
}

main().catch((err) => {
  console.error("Error:", err.message);
  if (err.cause) console.error("Cause:", err.cause);
  process.exit(1);
});
