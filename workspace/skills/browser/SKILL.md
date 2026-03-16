---
name: browser
description: "Headless browser automation for web searching, scraping, testing, and interaction. Uses Lightpanda (ultra-fast, 9x less memory than Chrome) with MCP integration, or falls back to agent-browser CLI. Activates when the user needs to browse websites, extract data, fill forms, take screenshots, test web pages, or interact with web applications."
allowed-tools:
  - exec
  - read_file
  - write_file
---

# Browser Skill

Headless browser automation for AI agents. Supports two backends:
1. **Lightpanda** (preferred) — 9x less memory, 11x faster than Chrome, MCP-native
2. **agent-browser** (fallback) — Rust/Node.js CLI with snapshot-ref interaction

## Quick Start

### Option A: Lightpanda MCP (Recommended)

Add to `config.json` under `tools.mcp`:
```json
{
  "mcp": {
    "enabled": true,
    "servers": {
      "browser": {
        "command": "lightpanda",
        "args": ["mcp"]
      }
    }
  }
}
```

MCP tools become available directly: `goto`, `markdown`, `links`, `evaluate`, `semantic_tree`, `structuredData`, `interactiveElements`.

### Option B: Lightpanda CLI

```bash
# Fetch a page as markdown (best for AI consumption)
lightpanda fetch --dump_mode markdown "https://example.com"

# Fetch as semantic tree (optimized for LLM reasoning)
lightpanda fetch --dump_mode semantic_tree "https://example.com"

# Start CDP server for Puppeteer/Playwright
lightpanda serve --host 127.0.0.1 --port 9222
```

### Option C: agent-browser CLI (Fallback)

```bash
# Install
npm install -g agent-browser && agent-browser install

# Core workflow: snapshot → ref → interact
agent-browser open "https://example.com"
agent-browser snapshot -i --json    # Get interactive elements with refs
agent-browser click @e1             # Click element by ref
agent-browser fill @e2 "query"      # Fill input
agent-browser get text @e3          # Extract text
```

## Core Workflows

### 1. Web Search & Data Extraction

```bash
# Navigate and extract content
lightpanda fetch --dump_mode markdown "https://example.com/page"

# Extract structured data (JSON-LD, OpenGraph, microdata)
# Via MCP: call structuredData tool with URL

# Extract all links from a page
# Via MCP: call links tool with URL
```

### 2. Form Interaction & Multi-Step Flows

```bash
# Start CDP server
lightpanda serve --port 9222 &

# Use with chromedp (Go), Puppeteer (Node), or Playwright
# Example with Puppeteer:
node -e "
const puppeteer = require('puppeteer');
(async () => {
  const browser = await puppeteer.connect({browserWSEndpoint: 'ws://127.0.0.1:9222'});
  const page = (await browser.pages())[0];
  await page.goto('https://example.com/login');
  await page.type('#username', 'user');
  await page.type('#password', 'pass');
  await page.click('#submit');
  await page.waitForNavigation();
  console.log(await page.content());
  await browser.close();
})();
"
```

### 3. Page Testing & Verification

```bash
# Fetch and check content
CONTENT=$(lightpanda fetch --dump_mode markdown "https://example.com")
echo "$CONTENT" | grep -q "Expected Text" && echo "PASS" || echo "FAIL"

# Screenshot via CDP
# page.captureScreenshot() returns PNG data

# Check interactive elements
# Via MCP: call interactiveElements tool — returns clickable buttons, links, forms
```

### 4. JavaScript Execution on Pages

```bash
# Via MCP evaluate tool:
# Execute JS and get results from any page

# Via CDP Runtime.evaluate:
# Full V8 JavaScript engine — handles SPAs, dynamic content, APIs
```

## Output Formats

| Format | Use Case | Command |
|--------|----------|---------|
| **markdown** | AI consumption, readable text | `--dump_mode markdown` |
| **semantic_tree** | LLM reasoning, structured DOM | `--dump_mode semantic_tree` |
| **html** | Raw page source | `--dump_mode html` |
| **links** | Link extraction | MCP `links` tool |
| **structuredData** | JSON-LD, OpenGraph | MCP `structuredData` tool |

## CDP Features (via `lightpanda serve`)

- **DOM**: querySelector, querySelectorAll, getDocument, performSearch
- **Input**: dispatchKeyEvent, dispatchMouseEvent, insertText
- **Fetch**: request interception (enable, continue, fulfill, fail)
- **Network**: monitor all requests/responses
- **Storage**: cookies, localStorage, sessionStorage
- **Page**: navigate, captureScreenshot, lifecycle events
- **Runtime**: evaluate JavaScript, callFunctionOn

## Performance

| Metric | Lightpanda | Chrome Headless |
|--------|-----------|-----------------|
| Memory | ~50MB | ~450MB |
| Startup | Instant | 2-5 seconds |
| Page load | 11x faster | Baseline |
| Binary size | ~15MB | ~200MB |

## Installation

### Lightpanda

```bash
# Docker (easiest)
docker run -d -p 9222:9222 lightpanda/browser:nightly

# Or use the setup script
./workspace/skills/browser/scripts/setup-lightpanda.sh
```

### agent-browser (fallback)

```bash
npm install -g agent-browser
agent-browser install
```

## Rules

- Prefer `markdown` or `semantic_tree` output for AI processing — avoid raw HTML
- Use MCP integration when available — it's the most efficient path
- Respect `robots.txt` — use `--obey_robots` flag when fetching
- Set timeouts on all operations to prevent hanging
- For multi-step flows, use CDP sessions to maintain state
- Clean up CDP sessions when done

## References

- `references/lightpanda-browser/` — Lightpanda source and docs
- [Lightpanda GitHub](https://github.com/lightpanda-io/browser)
- [agent-browser on ClawHub](https://clawhub.ai/TheSethRose/agent-browser) — 125K+ downloads
