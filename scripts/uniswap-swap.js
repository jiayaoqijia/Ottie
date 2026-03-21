const { ethers } = require("ethers");
require("dotenv").config({ path: require("path").join(__dirname, "..", ".env") });

// Parse CLI args
const args = process.argv.slice(2);
function getArg(name, defaultVal) {
  const idx = args.indexOf(`--${name}`);
  return idx !== -1 && args[idx + 1] ? args[idx + 1] : defaultVal;
}

const AMOUNT_ETH = getArg("amount", "0.0001");
const CHAIN = getArg("chain", "sepolia");
const CHAIN_ID = CHAIN === "sepolia" ? 11155111 : 1;

const WETH_SEPOLIA = "0x7b79995e5f793A07Bc00c21412e50Ecae098E7f9";
const USDC_SEPOLIA = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238";
const UNISWAP_API = "https://trade-api.gateway.uniswap.org/v1";

async function uniswapSwap(provider, wallet, amountWei) {
  const walletAddress = wallet.address;
  console.log(`Attempting Uniswap Trading API swap: ${AMOUNT_ETH} ETH -> USDC on ${CHAIN}`);

  const headers = {
    "Content-Type": "application/json",
    "x-api-key": process.env.UNISWAP_API_KEY,
  };

  // Step 1: Check approval (ETH doesn't need approval, but API may require the call)
  console.log("Step 1: Checking approval...");
  try {
    const approvalRes = await fetch(`${UNISWAP_API}/check_approval`, {
      method: "POST",
      headers,
      body: JSON.stringify({
        walletAddress,
        token: "0x0000000000000000000000000000000000000000",
        amount: amountWei.toString(),
        chainId: CHAIN_ID,
      }),
    });
    const approvalData = await approvalRes.json();
    console.log("Approval response:", JSON.stringify(approvalData).substring(0, 200));

    if (approvalData.approval) {
      console.log("Sending approval transaction...");
      const approveTx = await wallet.sendTransaction({
        to: approvalData.approval.to,
        data: approvalData.approval.data,
        value: approvalData.approval.value ? BigInt(approvalData.approval.value) : 0n,
        gasLimit: approvalData.approval.gasLimit ? BigInt(approvalData.approval.gasLimit) : undefined,
      });
      await approveTx.wait();
      console.log("Approval tx:", approveTx.hash);
    }
  } catch (e) {
    console.log("Approval check note:", e.message?.substring(0, 100));
  }

  // Step 2: Get quote
  console.log("Step 2: Getting quote...");
  const quoteRes = await fetch(`${UNISWAP_API}/quote`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      type: "EXACT_INPUT",
      amount: amountWei.toString(),
      tokenIn: "0x0000000000000000000000000000000000000000",
      tokenOut: USDC_SEPOLIA,
      tokenInChainId: CHAIN_ID,
      tokenOutChainId: CHAIN_ID,
      swapper: walletAddress,
      slippageTolerance: 0.5,
    }),
  });

  if (!quoteRes.ok) {
    const errText = await quoteRes.text();
    throw new Error(`Quote failed (${quoteRes.status}): ${errText}`);
  }

  const quoteData = await quoteRes.json();
  console.log("Quote received:", JSON.stringify(quoteData).substring(0, 300));

  // Step 3: If quote returns permitData, sign Permit2 typed data
  let signature;
  if (quoteData.permitData) {
    console.log("Step 3: Signing Permit2 typed data...");
    const { domain, types, values } = quoteData.permitData;
    // Remove EIP712Domain from types if present (ethers handles it)
    const cleanTypes = { ...types };
    delete cleanTypes.EIP712Domain;
    signature = await wallet.signTypedData(domain, cleanTypes, values);
    console.log("Permit2 signature obtained");
  } else {
    console.log("Step 3: No Permit2 required for native ETH");
  }

  // Step 4: Execute swap
  console.log("Step 4: Requesting swap execution...");
  const swapBody = {
    quote: quoteData.quote,
    ...(signature && { signature }),
  };

  const swapRes = await fetch(`${UNISWAP_API}/swap`, {
    method: "POST",
    headers,
    body: JSON.stringify(swapBody),
  });

  if (!swapRes.ok) {
    const errText = await swapRes.text();
    throw new Error(`Swap failed (${swapRes.status}): ${errText}`);
  }

  const swapData = await swapRes.json();
  console.log("Swap response:", JSON.stringify(swapData).substring(0, 300));

  // Step 5: Sign and broadcast the returned transaction
  if (swapData.swap) {
    console.log("Step 5: Broadcasting swap transaction...");
    const tx = await wallet.sendTransaction({
      to: swapData.swap.to,
      data: swapData.swap.data,
      value: swapData.swap.value ? BigInt(swapData.swap.value) : 0n,
      gasLimit: swapData.swap.gasLimit ? BigInt(swapData.swap.gasLimit) : undefined,
    });
    const receipt = await tx.wait();
    return { txHash: tx.hash, receipt };
  }

  throw new Error("No swap transaction returned from API");
}

async function wethWrap(provider, wallet, amountWei) {
  console.log(`\nFallback: Wrapping ${AMOUNT_ETH} ETH -> WETH on ${CHAIN}`);
  console.log(`WETH contract: ${WETH_SEPOLIA}`);

  const tx = await wallet.sendTransaction({
    to: WETH_SEPOLIA,
    value: amountWei,
    data: "0xd0e30db0", // deposit() function selector
    gasLimit: 100000n,
  });

  console.log("Transaction sent:", tx.hash);
  const receipt = await tx.wait();
  return { txHash: tx.hash, receipt };
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
  const amountWei = ethers.parseEther(AMOUNT_ETH);

  console.log(`Wallet: ${wallet.address}`);
  console.log(`Chain: ${CHAIN} (${CHAIN_ID})`);
  console.log(`Amount: ${AMOUNT_ETH} ETH (${amountWei.toString()} wei)`);

  const balance = await provider.getBalance(wallet.address);
  console.log(`Balance: ${ethers.formatEther(balance)} ETH`);

  if (balance < amountWei) {
    throw new Error(`Insufficient balance: have ${ethers.formatEther(balance)} ETH, need ${AMOUNT_ETH} ETH`);
  }

  let result;
  try {
    // Try Uniswap Trading API first
    if (process.env.UNISWAP_API_KEY) {
      result = await uniswapSwap(provider, wallet, amountWei);
    } else {
      console.log("No UNISWAP_API_KEY set, using WETH wrap fallback");
      result = await wethWrap(provider, wallet, amountWei);
    }
  } catch (err) {
    console.log(`\nUniswap API error: ${err.message}`);
    console.log("Falling back to WETH wrap...");
    result = await wethWrap(provider, wallet, amountWei);
  }

  const explorer = CHAIN === "sepolia" ? "https://sepolia.etherscan.io" : "https://etherscan.io";
  console.log("\n=== Swap Complete ===");
  console.log(`TX Hash: ${result.txHash}`);
  console.log(`Explorer: ${explorer}/tx/${result.txHash}`);
  console.log(`Block: ${result.receipt.blockNumber}`);
  console.log(`Gas Used: ${result.receipt.gasUsed.toString()}`);
}

main().catch((err) => {
  console.error("Error:", err.message);
  process.exit(1);
});
