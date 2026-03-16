.PHONY: all build install uninstall clean help test

# Build variables
BINARY_NAME=ottie
BUILD_DIR=build
CMD_DIR=cmd/$(BINARY_NAME)
MAIN_GO=$(CMD_DIR)/main.go

# Version
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date +%FT%T%z)
GO_VERSION=$(shell $(GO) version | awk '{print $$3}')
CONFIG_PKG=github.com/jiayaoqijia/ottie/pkg/config
LDFLAGS=-ldflags "-X $(CONFIG_PKG).Version=$(VERSION) -X $(CONFIG_PKG).GitCommit=$(GIT_COMMIT) -X $(CONFIG_PKG).BuildTime=$(BUILD_TIME) -X $(CONFIG_PKG).GoVersion=$(GO_VERSION) -s -w"

# Go variables
GO?=CGO_ENABLED=0 go
GOFLAGS?=-v -tags stdjson

# Golangci-lint
GOLANGCI_LINT?=golangci-lint

# Installation
INSTALL_PREFIX?=$(HOME)/.local
INSTALL_BIN_DIR=$(INSTALL_PREFIX)/bin
INSTALL_MAN_DIR=$(INSTALL_PREFIX)/share/man/man1
INSTALL_TMP_SUFFIX=.new

# Workspace and Skills
OTTIE_HOME?=$(HOME)/.ottie
WORKSPACE_DIR?=$(OTTIE_HOME)/workspace
WORKSPACE_SKILLS_DIR=$(WORKSPACE_DIR)/skills
BUILTIN_SKILLS_DIR=$(CURDIR)/skills

# OS detection
UNAME_S:=$(shell uname -s)
UNAME_M:=$(shell uname -m)

# Platform-specific settings
ifeq ($(UNAME_S),Linux)
	PLATFORM=linux
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),aarch64)
		ARCH=arm64
	else ifeq ($(UNAME_M),armv81)
		ARCH=arm64
	else
		ARCH=$(UNAME_M)
	endif
else ifeq ($(UNAME_S),Darwin)
	PLATFORM=darwin
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),arm64)
		ARCH=arm64
	else
		ARCH=$(UNAME_M)
	endif
else
	PLATFORM=$(UNAME_S)
	ARCH=$(UNAME_M)
endif

BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)-$(PLATFORM)-$(ARCH)

# Default target
all: build

## generate: Run generate
generate:
	@echo "Run generate..."
	@rm -r ./$(CMD_DIR)/workspace 2>/dev/null || true
	@$(GO) generate ./...
	@echo "Run generate complete"

## build: Build the ottie binary for current platform
build: generate
	@echo "Building $(BINARY_NAME) for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_PATH) ./$(CMD_DIR)
	@echo "Build complete: $(BINARY_PATH)"
	@ln -sf $(BINARY_NAME)-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/$(BINARY_NAME)

## build-launcher: Build the ottie-launcher (web console) binary
build-launcher:
	@echo "Building ottie-launcher for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	@if [ ! -f web/backend/dist/index.html ]; then \
		echo "Building frontend..."; \
		cd web/frontend && pnpm install && pnpm build:backend; \
	fi
	@$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/ottie-launcher-$(PLATFORM)-$(ARCH) ./web/backend
	@ln -sf ottie-launcher-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/ottie-launcher
	@echo "Build complete: $(BUILD_DIR)/ottie-launcher"

## build-whatsapp-native: Build with WhatsApp native (whatsmeow) support; larger binary
build-whatsapp-native: generate
## @echo "Building $(BINARY_NAME) with WhatsApp native for $(PLATFORM)/$(ARCH)..."
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -tags whatsapp_native $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build -tags whatsapp_native $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build -tags whatsapp_native $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build -tags whatsapp_native $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
## @$(GO) build $(GOFLAGS) -tags whatsapp_native $(LDFLAGS) -o $(BINARY_PATH) ./$(CMD_DIR)
	@echo "Build complete"
##	@ln -sf $(BINARY_NAME)-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/$(BINARY_NAME)

## build-all: Build ottie for all platforms
build-all: generate
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm ./$(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	@echo "All builds complete"

## install: Install ottie to system and copy builtin skills
install: build
	@echo "Installing $(BINARY_NAME)..."
	@mkdir -p $(INSTALL_BIN_DIR)
	# Copy binary with temporary suffix to ensure atomic update
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_BIN_DIR)/$(BINARY_NAME)$(INSTALL_TMP_SUFFIX)
	@chmod +x $(INSTALL_BIN_DIR)/$(BINARY_NAME)$(INSTALL_TMP_SUFFIX)
	@mv -f $(INSTALL_BIN_DIR)/$(BINARY_NAME)$(INSTALL_TMP_SUFFIX) $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Installed binary to $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo "Installation complete!"

## uninstall: Remove ottie from system
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Removed binary from $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo "Note: Only the executable file has been deleted."
	@echo "If you need to delete all configurations (config.json, workspace, etc.), run 'make uninstall-all'"

## uninstall-all: Remove ottie and all data
uninstall-all:
	@echo "Removing workspace and skills..."
	@rm -rf $(OTTIE_HOME)
	@echo "Removed workspace: $(OTTIE_HOME)"
	@echo "Complete uninstallation done!"

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

## vet: Run go vet for static analysis
vet: generate
	@$(GO) vet ./...

## test: Test Go code
test: generate
	@$(GO) test ./...

## fmt: Format Go code
fmt:
	@$(GOLANGCI_LINT) fmt

## lint: Run linters
lint:
	@$(GOLANGCI_LINT) run

## fix: Fix linting issues
fix:
	@$(GOLANGCI_LINT) run --fix

## deps: Download dependencies
deps:
	@$(GO) mod download
	@$(GO) mod verify

## update-deps: Update dependencies
update-deps:
	@$(GO) get -u ./...
	@$(GO) mod tidy

## check: Run vet, fmt, and verify dependencies
check: deps fmt vet test

## run: Build and run ottie
run: build
	@$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

## docker-build: Build Docker image (minimal Alpine-based)
docker-build:
	@echo "Building minimal Docker image (Alpine-based)..."
	docker compose -f docker/docker-compose.yml build ottie-agent ottie-gateway

## docker-build-full: Build Docker image with full MCP support (Node.js 24)
docker-build-full:
	@echo "Building full-featured Docker image (Node.js 24)..."
	docker compose -f docker/docker-compose.full.yml build ottie-agent ottie-gateway

## docker-test: Test MCP tools in Docker container
docker-test:
	@echo "Testing MCP tools in Docker..."
	@chmod +x scripts/test-docker-mcp.sh
	@./scripts/test-docker-mcp.sh

## docker-run: Run ottie gateway in Docker (Alpine-based)
docker-run:
	docker compose -f docker/docker-compose.yml --profile gateway up

## docker-run-full: Run ottie gateway in Docker (full-featured)
docker-run-full:
	docker compose -f docker/docker-compose.full.yml --profile gateway up

## docker-run-agent: Run ottie agent in Docker (interactive, Alpine-based)
docker-run-agent:
	docker compose -f docker/docker-compose.yml run --rm ottie-agent

## docker-run-agent-full: Run ottie agent in Docker (interactive, full-featured)
docker-run-agent-full:
	docker compose -f docker/docker-compose.full.yml run --rm ottie-agent

## docker-clean: Clean Docker images and volumes
docker-clean:
	docker compose -f docker/docker-compose.yml down -v
	docker compose -f docker/docker-compose.full.yml down -v
	docker rmi ottie:latest ottie:full 2>/dev/null || true

## help: Show this help message
help:
	@echo "ottie Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sort | awk -F': ' '{printf "  %-16s %s\n", substr($$1, 4), $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build              # Build for current platform"
	@echo "  make install            # Install to ~/.local/bin"
	@echo "  make uninstall          # Remove from /usr/local/bin"
	@echo "  make install-skills     # Install skills to workspace"
	@echo "  make docker-build       # Build minimal Docker image"
	@echo "  make docker-test        # Test MCP tools in Docker"
	@echo ""
	@echo "Environment Variables:"
	@echo "  INSTALL_PREFIX          # Installation prefix (default: ~/.local)"
	@echo "  WORKSPACE_DIR           # Workspace directory (default: ~/.ottie/workspace)"
	@echo "  VERSION                 # Version string (default: git describe)"
	@echo ""
	@echo "Current Configuration:"
	@echo "  Platform: $(PLATFORM)/$(ARCH)"
	@echo "  Binary: $(BINARY_PATH)"
	@echo "  Install Prefix: $(INSTALL_PREFIX)"
	@echo "  Workspace: $(WORKSPACE_DIR)"
