#!/usr/bin/env bash
# setup-lightpanda.sh — Install Lightpanda browser for Ottie
# Usage: ./setup-lightpanda.sh [--docker | --binary]
set -euo pipefail

MODE="${1:---docker}"

case "$MODE" in
  --docker)
    echo "Setting up Lightpanda via Docker..."
    if ! command -v docker &>/dev/null; then
      echo "Error: Docker not found. Install Docker first." >&2
      exit 1
    fi
    docker pull lightpanda/browser:nightly
    echo "Run with: docker run -d -p 9222:9222 lightpanda/browser:nightly"
    echo "Then configure MCP or connect via CDP at ws://127.0.0.1:9222"
    ;;

  --binary)
    echo "Downloading Lightpanda binary..."
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case "$ARCH" in
      x86_64) ARCH="x86_64" ;;
      aarch64|arm64) ARCH="aarch64" ;;
      *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
    esac

    URL="https://github.com/nickel-org/lightpanda-io/releases/latest/download/lightpanda-${OS}-${ARCH}"
    DEST="${HOME}/.local/bin/lightpanda"
    mkdir -p "$(dirname "$DEST")"

    if command -v curl &>/dev/null; then
      curl -sL "$URL" -o "$DEST"
    elif command -v wget &>/dev/null; then
      wget -q "$URL" -O "$DEST"
    else
      echo "Error: curl or wget required" >&2
      exit 1
    fi

    chmod +x "$DEST"
    echo "Installed to: $DEST"
    echo "Verify: lightpanda --version"
    ;;

  *)
    echo "Usage: $0 [--docker | --binary]"
    exit 1
    ;;
esac

echo ""
echo "MCP config for Ottie (add to config.json):"
echo '  "mcp": {'
echo '    "enabled": true,'
echo '    "servers": {'
echo '      "browser": {'
echo '        "command": "lightpanda",'
echo '        "args": ["mcp"]'
echo '      }'
echo '    }'
echo '  }'
