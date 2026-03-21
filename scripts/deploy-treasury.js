const { ethers } = require("ethers");
const solc = require("solc");
const fs = require("fs");
const path = require("path");
require("dotenv").config({ path: path.join(__dirname, "..", ".env") });

async function compileSolidity() {
  const sourceCode = fs.readFileSync(
    path.join(__dirname, "..", "contracts", "AgentTreasury.sol"),
    "utf8"
  );

  const input = {
    language: "Solidity",
    sources: {
      "AgentTreasury.sol": { content: sourceCode },
    },
    settings: {
      outputSelection: {
        "*": {
          "*": ["abi", "evm.bytecode"],
        },
      },
    },
  };

  console.log("Compiling contracts...");
  const output = JSON.parse(solc.compile(JSON.stringify(input)));

  if (output.errors) {
    const errors = output.errors.filter((e) => e.severity === "error");
    if (errors.length > 0) {
      console.error("Compilation errors:");
      errors.forEach((e) => console.error(e.formattedMessage));
      throw new Error("Compilation failed");
    }
    // Print warnings but continue
    output.errors
      .filter((e) => e.severity === "warning")
      .forEach((e) => console.log("Warning:", e.message));
  }

  const mockWstETH = output.contracts["AgentTreasury.sol"]["MockWstETH"];
  const agentTreasury = output.contracts["AgentTreasury.sol"]["AgentTreasury"];

  if (!mockWstETH || !agentTreasury) {
    throw new Error("Failed to compile one or both contracts");
  }

  console.log("Compilation successful");
  return { mockWstETH, agentTreasury };
}

async function deployContract(wallet, abi, bytecode, constructorArgs = []) {
  const factory = new ethers.ContractFactory(abi, bytecode, wallet);
  const contract = await factory.deploy(...constructorArgs);
  const deployTx = contract.deploymentTransaction();
  console.log(`  Deploy TX: ${deployTx.hash}`);
  await contract.waitForDeployment();
  const address = await contract.getAddress();
  console.log(`  Contract address: ${address}`);
  return { contract, deployTx, address };
}

async function main() {
  if (!process.env.E2E_WALLET_PRIVATE_KEY) {
    throw new Error("E2E_WALLET_PRIVATE_KEY not set in .env");
  }
  if (!process.env.SEPOLIA_RPC) {
    throw new Error("SEPOLIA_RPC not set in .env");
  }

  const provider = new ethers.JsonRpcProvider(process.env.SEPOLIA_RPC);
  const wallet = new ethers.Wallet(process.env.E2E_WALLET_PRIVATE_KEY, provider);

  console.log(`Deployer: ${wallet.address}`);
  const balance = await provider.getBalance(wallet.address);
  console.log(`Balance: ${ethers.formatEther(balance)} ETH`);

  // Use a second wallet as the "agent" if available, otherwise use same wallet
  const agentKey = process.env.E2E_WALLET_2_PRIVATE_KEY;
  const agentWallet = agentKey
    ? new ethers.Wallet(agentKey.startsWith("0x") ? agentKey : `0x${agentKey}`, provider)
    : wallet;
  console.log(`Agent wallet: ${agentWallet.address}`);

  // Use a third wallet as recipient if available, otherwise derive one
  const recipientAddress = process.env.E2E_WALLET_3_ADDRESS || "0x000000000000000000000000000000000000dEaD";
  console.log(`Recipient: ${recipientAddress}`);

  // Step 1: Compile
  const { mockWstETH, agentTreasury } = await compileSolidity();

  // Step 2: Deploy MockWstETH
  console.log("\n=== Deploying MockWstETH ===");
  const token = await deployContract(
    wallet,
    mockWstETH.abi,
    mockWstETH.evm.bytecode.object
  );

  // Step 3: Deploy AgentTreasury
  console.log("\n=== Deploying AgentTreasury ===");
  const treasury = await deployContract(
    wallet,
    agentTreasury.abi,
    agentTreasury.evm.bytecode.object,
    [token.address]
  );

  // Step 4: Mint tokens to deployer
  const mintAmount = ethers.parseEther("10"); // 10 tokens
  console.log(`\n=== Minting ${ethers.formatEther(mintAmount)} tokens to deployer ===`);
  const mintTx = await token.contract.mint(wallet.address, mintAmount);
  await mintTx.wait();
  console.log(`  Mint TX: ${mintTx.hash}`);

  // Step 5: Approve treasury to spend tokens
  console.log("\n=== Approving Treasury ===");
  const approveTx = await token.contract.approve(treasury.address, mintAmount);
  await approveTx.wait();
  console.log(`  Approve TX: ${approveTx.hash}`);

  // Step 6: Deposit tokens into treasury with agent authorization
  const depositAmount = ethers.parseEther("5"); // Deposit 5 tokens
  console.log(`\n=== Depositing ${ethers.formatEther(depositAmount)} tokens ===`);
  const depositTx = await treasury.contract.deposit(depositAmount, agentWallet.address);
  await depositTx.wait();
  console.log(`  Deposit TX: ${depositTx.hash}`);

  // Check exchange rate at deposit
  const rateAtDeposit = await token.contract.exchangeRate();
  console.log(`  Exchange rate at deposit: ${rateAtDeposit.toString()}`);

  // Step 7: Set allowed recipient and per-tx cap
  console.log("\n=== Configuring access controls ===");
  const setRecipientTx = await treasury.contract.setAllowedRecipient(recipientAddress, true);
  await setRecipientTx.wait();
  console.log(`  Set recipient TX: ${setRecipientTx.hash}`);

  const capAmount = ethers.parseEther("1");
  const setCapTx = await treasury.contract.setPerTxCap(capAmount);
  await setCapTx.wait();
  console.log(`  Set cap TX: ${setCapTx.hash}`);

  // Step 8: Wait for some time to accumulate yield, then query
  console.log("\n=== Querying yield ===");
  // On testnet, blocks are fast. Just query what we have.
  let yieldAmount = await treasury.contract.queryYield(wallet.address);
  console.log(`  Current yield: ${ethers.formatEther(yieldAmount)} tokens`);

  if (yieldAmount === 0n) {
    console.log("  Yield is 0 (same block). Sending a dummy tx to advance time...");
    // Send a small self-transfer to advance block
    const dummyTx = await wallet.sendTransaction({
      to: wallet.address,
      value: 0n,
    });
    await dummyTx.wait();
    yieldAmount = await treasury.contract.queryYield(wallet.address);
    console.log(`  Yield after wait: ${ethers.formatEther(yieldAmount)} tokens`);
  }

  // Step 9: Agent spends yield
  if (yieldAmount > 0n) {
    console.log("\n=== Agent spending yield ===");
    const spendAmount = yieldAmount < capAmount ? yieldAmount : capAmount;
    // Connect treasury with agent wallet
    const treasuryAsAgent = treasury.contract.connect(agentWallet);
    try {
      const spendTx = await treasuryAsAgent.spendYield(
        wallet.address,
        recipientAddress,
        spendAmount
      );
      await spendTx.wait();
      console.log(`  SpendYield TX: ${spendTx.hash}`);
      console.log(`  Amount spent: ${ethers.formatEther(spendAmount)} tokens`);
    } catch (err) {
      console.log(`  SpendYield error: ${err.message?.substring(0, 200)}`);
      console.log("  (Yield may be too small or time hasn't advanced enough)");
    }
  } else {
    console.log("\n  No yield to spend yet (blocks too fast)");
  }

  // Final summary
  const explorer = "https://sepolia.etherscan.io";
  console.log("\n=== Deployment Complete ===");
  console.log(`MockWstETH: ${token.address}`);
  console.log(`  ${explorer}/address/${token.address}`);
  console.log(`AgentTreasury: ${treasury.address}`);
  console.log(`  ${explorer}/address/${treasury.address}`);
  console.log("\nTransaction Hashes:");
  console.log(`  MockWstETH deploy: ${token.deployTx.hash}`);
  console.log(`  AgentTreasury deploy: ${treasury.deployTx.hash}`);
  console.log(`  Mint: ${mintTx.hash}`);
  console.log(`  Approve: ${approveTx.hash}`);
  console.log(`  Deposit: ${depositTx.hash}`);
  console.log(`  SetRecipient: ${setRecipientTx.hash}`);
  console.log(`  SetCap: ${setCapTx.hash}`);
}

main().catch((err) => {
  console.error("Error:", err.message);
  if (err.data) console.error("Data:", err.data);
  process.exit(1);
});
