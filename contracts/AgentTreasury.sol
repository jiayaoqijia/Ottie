// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

contract MockWstETH {
    string public name = "Mock Wrapped stETH";
    string public symbol = "mwstETH";
    uint8 public decimals = 18;
    uint256 public totalSupply;
    uint256 public deployTime;

    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);

    constructor() {
        deployTime = block.timestamp;
    }

    function exchangeRate() public view returns (uint256) {
        return 1e18 + (block.timestamp - deployTime) * 1e12;
    }

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
        totalSupply += amount;
        emit Transfer(address(0), to, amount);
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        emit Approval(msg.sender, spender, amount);
        return true;
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        require(balanceOf[msg.sender] >= amount, "Insufficient balance");
        balanceOf[msg.sender] -= amount;
        balanceOf[to] += amount;
        emit Transfer(msg.sender, to, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        require(balanceOf[from] >= amount, "Insufficient balance");
        require(allowance[from][msg.sender] >= amount, "Insufficient allowance");
        allowance[from][msg.sender] -= amount;
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
        emit Transfer(from, to, amount);
        return true;
    }
}

contract AgentTreasury {
    struct Deposit {
        uint256 amount;
        uint256 exchangeRateAtDeposit;
        address agent;
        uint256 yieldSpent;
    }

    MockWstETH public token;
    mapping(address => Deposit) public deposits;
    mapping(address => mapping(address => bool)) public allowedRecipients;
    mapping(address => uint256) public perTxCap;

    event Deposited(address indexed depositor, uint256 amount, address agent, uint256 exchangeRate);
    event YieldSpent(address indexed depositor, address indexed recipient, uint256 amount);
    event PrincipalWithdrawn(address indexed depositor, uint256 amount);
    event RecipientSet(address indexed depositor, address indexed recipient, bool allowed);
    event PerTxCapSet(address indexed depositor, uint256 cap);

    constructor(address _token) {
        token = MockWstETH(_token);
    }

    function deposit(uint256 amount, address agent) external {
        require(amount > 0, "Amount must be > 0");
        require(deposits[msg.sender].amount == 0, "Already deposited");
        require(token.transferFrom(msg.sender, address(this), amount), "Transfer failed");

        deposits[msg.sender] = Deposit({
            amount: amount,
            exchangeRateAtDeposit: token.exchangeRate(),
            agent: agent,
            yieldSpent: 0
        });

        emit Deposited(msg.sender, amount, agent, token.exchangeRate());
    }

    function queryYield(address depositor) public view returns (uint256) {
        Deposit storage d = deposits[depositor];
        if (d.amount == 0) return 0;
        uint256 currentRate = token.exchangeRate();
        uint256 totalYield = d.amount * (currentRate - d.exchangeRateAtDeposit) / 1e18;
        return totalYield > d.yieldSpent ? totalYield - d.yieldSpent : 0;
    }

    function spendYield(address depositor, address recipient, uint256 amount) external {
        Deposit storage d = deposits[depositor];
        require(d.agent == msg.sender, "Only authorized agent");
        require(amount > 0, "Amount must be > 0");
        require(allowedRecipients[depositor][recipient], "Recipient not allowed");
        require(perTxCap[depositor] == 0 || amount <= perTxCap[depositor], "Exceeds per-tx cap");
        require(amount <= queryYield(depositor), "Insufficient yield");

        d.yieldSpent += amount;
        require(token.transfer(recipient, amount), "Transfer failed");

        emit YieldSpent(depositor, recipient, amount);
    }

    function withdrawPrincipal() external {
        Deposit storage d = deposits[msg.sender];
        require(d.amount > 0, "No deposit");
        uint256 amount = d.amount;
        d.amount = 0;
        require(token.transfer(msg.sender, amount), "Transfer failed");

        emit PrincipalWithdrawn(msg.sender, amount);
    }

    function setAllowedRecipient(address recipient, bool allowed) external {
        allowedRecipients[msg.sender][recipient] = allowed;
        emit RecipientSet(msg.sender, recipient, allowed);
    }

    function setPerTxCap(uint256 cap) external {
        perTxCap[msg.sender] = cap;
        emit PerTxCapSet(msg.sender, cap);
    }
}
