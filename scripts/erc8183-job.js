#!/usr/bin/env node
require("dotenv").config({ path: require("path").join(__dirname, "..", ".env") });
const { ethers } = require("ethers");

// ERC-8183 Agentic Commerce - Job Escrow Protocol
// Deployed on BSC Testnet: 0x377533d0e68a22cf180205e9c9ed980f74bc5050

const ERC8183_ABI = [
  "function createJob(address provider, address evaluator, uint256 expiredAt, string memory description, address hook) external returns (uint256)",
  "function fund(uint256 jobId, uint256 expectedBudget) external payable",
  "function submit(uint256 jobId, string memory deliverable) external",
  "function complete(uint256 jobId, string memory reason) external",
  "event JobCreated(uint256 indexed jobId, address indexed client, address provider, address evaluator, uint256 expiredAt, string description, address hook)"
];

async function main() {
  console.log("=== ERC-8183 Agentic Commerce Demo (Track 9) ===\n");

  // BSC Testnet RPC
  const BSC_TESTNET_RPC = "https://data-seed-prebsc-1-s1.binance.org:8545";
  const provider = new ethers.JsonRpcProvider(BSC_TESTNET_RPC);

  // Check if we have BNB testnet funds
  const wallet = new ethers.Wallet(process.env.E2E_WALLET_PRIVATE_KEY, provider);
  console.log(`Wallet: ${wallet.address}`);

  const balance = await provider.getBalance(wallet.address);
  console.log(`BSC Testnet BNB balance: ${ethers.formatEther(balance)} BNB`);

  const contractAddress = "0x377533d0e68a22cf180205e9c9ed980f74bc5050";

  if (balance > 0n) {
    // We have BNB testnet funds - try creating a job
    console.log("\nCreating ERC-8183 job...");
    const contract = new ethers.Contract(contractAddress, ERC8183_ABI, wallet);

    const expiredAt = Math.floor(Date.now() / 1000) + 86400; // 24 hours
    try {
      const tx = await contract.createJob(
        wallet.address,                                     // provider (self for demo)
        wallet.address,                                     // evaluator (self for demo)
        expiredAt,
        "Ottie AI Agent: Analyze Lido staking performance and generate report",
        ethers.ZeroAddress                                   // no hook
      );
      console.log(`  Job creation tx: ${tx.hash}`);
      console.log(`  BSCScan: https://testnet.bscscan.com/tx/${tx.hash}`);

      const receipt = await tx.wait();
      console.log(`  Status: ${receipt.status === 1 ? "Success" : "Failed"}`);
      console.log(`  Block: ${receipt.blockNumber}`);
      console.log(`  Gas used: ${receipt.gasUsed.toString()}`);
    } catch (err) {
      console.log(`  Job creation error: ${err.message}`);
      console.log("  Note: May need BSC Testnet BNB from faucet");
    }
  } else {
    // No BNB - demonstrate the ABI interaction pattern
    console.log("\nNo BSC Testnet BNB available. Demonstrating ABI interaction pattern...");

    const contract = new ethers.Contract(contractAddress, ERC8183_ABI, provider);
    const expiredAt = Math.floor(Date.now() / 1000) + 86400;

    // Build unsigned transaction
    const txData = contract.interface.encodeFunctionData("createJob", [
      wallet.address,
      wallet.address,
      expiredAt,
      "Ottie AI Agent: Analyze Lido staking performance and generate report",
      ethers.ZeroAddress
    ]);

    console.log(`  Contract: ${contractAddress}`);
    console.log(`  Function: createJob(provider, evaluator, expiredAt, description, hook)`);
    console.log(`  Calldata: ${txData.slice(0, 74)}...`);
    console.log(`  Estimated gas: ~200000`);
    console.log("\n  To execute: Fund wallet with BSC Testnet BNB from https://testnet.bnbchain.org/faucet-smart");
  }

  console.log("\n=== ERC-8183 Demo Complete ===");
}

main().catch(console.error);
