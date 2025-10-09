# Password Manager Makefile
# Supports multi-architecture builds and release management

# Application info
APP_NAME = pwmgr-go
MODULE_NAME = github.com/gerry/password-manager
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
COMMIT_HASH = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build settings
BUILD_DIR = build
DIST_DIR = dist
SRC_DIR = ./cmd/pwmgr

# Go settings
GO = go
GOFLAGS = -ldflags="-s -w -X '$(MODULE_NAME)/internal/version.Version=$(VERSION)' -X '$(MODULE_NAME)/internal/version.BuildTime=$(BUILD_TIME)' -X '$(MODULE_NAME)/internal/version.CommitHash=$(COMMIT_HASH)'"

# Target architectures and operating systems
TARGETS = \
	linux/amd64 \
	linux/arm64 \
	linux/386 \
	linux/arm \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/386 \
	freebsd/amd64 \
	openbsd/amd64

# Colors for output
RED = \033[0;31m
GREEN = \033[0;32m
YELLOW = \033[0;33m
BLUE = \033[0;34m
NC = \033[0m # No Color

# Default target
.PHONY: all
all: clean test build

# Help target
.PHONY: help
help:
	@echo "$(GREEN)Password Manager Build System$(NC)"
	@echo ""
	@echo "$(YELLOW)Available targets:$(NC)"
	@echo "  $(BLUE)build$(NC)          - Build for current platform"
	@echo "  $(BLUE)build-all$(NC)      - Build for all supported platforms"
	@echo "  $(BLUE)release$(NC)        - Create release archives for all platforms"
	@echo "  $(BLUE)test$(NC)           - Run all tests"
	@echo "  $(BLUE)test-verbose$(NC)   - Run tests with verbose output"
	@echo "  $(BLUE)clean$(NC)          - Clean build artifacts"
	@echo "  $(BLUE)deps$(NC)           - Download dependencies"
	@echo "  $(BLUE)tidy$(NC)           - Tidy go modules"
	@echo "  $(BLUE)fmt$(NC)            - Format code"
	@echo "  $(BLUE)vet$(NC)            - Run go vet"
	@echo "  $(BLUE)lint$(NC)           - Run linter (requires golangci-lint)"
	@echo "  $(BLUE)check$(NC)          - Run all checks (fmt, vet, test)"
	@echo "  $(BLUE)install$(NC)        - Install binary to $$GOBIN or $$GOPATH/bin"
	@echo "  $(BLUE)uninstall$(NC)      - Remove installed binary"
	@echo "  $(BLUE)version$(NC)        - Show version information"
	@echo ""
	@echo "$(YELLOW)Variables:$(NC)"
	@echo "  VERSION=$(VERSION)"
	@echo "  BUILD_TIME=$(BUILD_TIME)"
	@echo "  COMMIT_HASH=$(COMMIT_HASH)"

# Version information
.PHONY: version
version:
	@echo "$(GREEN)Version Information:$(NC)"
	@echo "  Version: $(VERSION)"
	@echo "  Build Time: $(BUILD_TIME)"
	@echo "  Commit Hash: $(COMMIT_HASH)"
	@echo "  Module: $(MODULE_NAME)"

# Build for current platform
.PHONY: build
build: deps
	@echo "$(GREEN)Building $(APP_NAME) for current platform...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME) $(SRC_DIR)
	@echo "$(GREEN)✓ Build complete: $(BUILD_DIR)/$(APP_NAME)$(NC)"

# Build for all platforms
.PHONY: build-all
build-all: deps
	@echo "$(GREEN)Building $(APP_NAME) for all platforms...$(NC)"
	@mkdir -p $(BUILD_DIR)
	@$(foreach target,$(TARGETS), \
		echo "$(YELLOW)Building for $(target)...$(NC)"; \
		GOOS=$(word 1,$(subst /, ,$(target))) \
		GOARCH=$(word 2,$(subst /, ,$(target))) \
		$(GO) build $(GOFLAGS) \
		-o $(BUILD_DIR)/$(APP_NAME)-$(subst /,-,$(target))$(if $(findstring windows,$(target)),.exe,) \
		$(SRC_DIR) || exit 1; \
	)
	@echo "$(GREEN)✓ All builds complete$(NC)"

# Create release archives
.PHONY: release
release: build-all
	@echo "$(GREEN)Creating release archives...$(NC)"
	@mkdir -p $(DIST_DIR)
	@for target in $(TARGETS); do \
		os=$$(echo $$target | cut -d'/' -f1); \
		arch=$$(echo $$target | cut -d'/' -f2); \
		binary_name="$(APP_NAME)-$$os-$$arch"; \
		if [ "$$os" = "windows" ]; then \
			binary_name="$$binary_name.exe"; \
			target_binary="$(APP_NAME).exe"; \
		else \
			target_binary="$(APP_NAME)"; \
		fi; \
		if [ -f "$(BUILD_DIR)/$$binary_name" ]; then \
			echo "$(YELLOW)Creating archive for $$target...$(NC)"; \
			archive_name="$(APP_NAME)-$(VERSION)-$$os-$$arch"; \
			mkdir -p "temp/$$archive_name"; \
			cp "$(BUILD_DIR)/$$binary_name" "temp/$$archive_name/$$target_binary"; \
			cp README.md "temp/$$archive_name/" 2>/dev/null || echo "README.md not found, skipping"; \
			cp LICENSE "temp/$$archive_name/" 2>/dev/null || echo "LICENSE not found, skipping"; \
			if [ "$$os" = "windows" ]; then \
				cd temp && zip -q -r "../$(DIST_DIR)/$$archive_name.zip" "$$archive_name" && cd ..; \
				echo "$(GREEN)✓ Created $(DIST_DIR)/$$archive_name.zip$(NC)"; \
			else \
				cd temp && tar -czf "../$(DIST_DIR)/$$archive_name.tar.gz" "$$archive_name" && cd ..; \
				echo "$(GREEN)✓ Created $(DIST_DIR)/$$archive_name.tar.gz$(NC)"; \
			fi; \
			rm -rf "temp/$$archive_name"; \
		fi; \
	done
	@rmdir temp 2>/dev/null || true
	@echo "$(GREEN)✓ Release archives created in $(DIST_DIR)/$(NC)"
	@ls -la $(DIST_DIR)/

# Run tests
.PHONY: test
test:
	@echo "$(GREEN)Running tests...$(NC)"
	$(GO) test ./...
	@echo "$(GREEN)✓ Tests passed$(NC)"

# Run tests with verbose output
.PHONY: test-verbose
test-verbose:
	@echo "$(GREEN)Running tests (verbose)...$(NC)"
	$(GO) test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✓ Coverage report generated: coverage.html$(NC)"

# Download dependencies
.PHONY: deps
deps:
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	$(GO) mod download
	@echo "$(GREEN)✓ Dependencies downloaded$(NC)"

# Tidy modules
.PHONY: tidy
tidy:
	@echo "$(GREEN)Tidying modules...$(NC)"
	$(GO) mod tidy
	@echo "$(GREEN)✓ Modules tidied$(NC)"

# Format code
.PHONY: fmt
fmt:
	@echo "$(GREEN)Formatting code...$(NC)"
	$(GO) fmt ./...
	@echo "$(GREEN)✓ Code formatted$(NC)"

# Run go vet
.PHONY: vet
vet:
	@echo "$(GREEN)Running go vet...$(NC)"
	$(GO) vet ./...
	@echo "$(GREEN)✓ No issues found$(NC)"

# Run linter (requires golangci-lint)
.PHONY: lint
lint:
	@echo "$(GREEN)Running linter...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
		echo "$(GREEN)✓ Linting complete$(NC)"; \
	else \
		echo "$(RED)golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(NC)"; \
		exit 1; \
	fi

# Run all checks
.PHONY: check
check: fmt vet test
	@echo "$(GREEN)✓ All checks passed$(NC)"

# Install binary
.PHONY: install
install: build
	@echo "$(GREEN)Installing $(APP_NAME)...$(NC)"
	$(GO) install $(GOFLAGS) $(SRC_DIR)
	@echo "$(GREEN)✓ $(APP_NAME) installed$(NC)"

# Uninstall binary
.PHONY: uninstall
uninstall:
	@echo "$(GREEN)Uninstalling $(APP_NAME)...$(NC)"
	@if command -v $(APP_NAME) >/dev/null 2>&1; then \
		rm -f "$$(which $(APP_NAME))"; \
		echo "$(GREEN)✓ $(APP_NAME) uninstalled$(NC)"; \
	else \
		echo "$(YELLOW)$(APP_NAME) not found in PATH$(NC)"; \
	fi

# Clean build artifacts
.PHONY: clean
clean:
	@echo "$(GREEN)Cleaning build artifacts...$(NC)"
	rm -rf $(BUILD_DIR) $(DIST_DIR) temp coverage.out coverage.html
	$(GO) clean -cache -modcache -testcache
	@echo "$(GREEN)✓ Clean complete$(NC)"

# Quick development build and test
.PHONY: dev
dev: fmt vet test build
	@echo "$(GREEN)✓ Development build complete$(NC)"

# Create a git tag and push (use with VERSION=vX.Y.Z)
.PHONY: tag
tag:
	@if [ "$(VERSION)" = "dev" ] || [ "$(VERSION)" = "" ]; then \
		echo "$(RED)Please specify a version: make tag VERSION=vX.Y.Z$(NC)"; \
		exit 1; \
	fi
	@echo "$(GREEN)Creating and pushing tag $(VERSION)...$(NC)"
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)
	@echo "$(GREEN)✓ Tag $(VERSION) created and pushed$(NC)"

# Show binary information
.PHONY: info
info: build
	@echo "$(GREEN)Binary Information:$(NC)"
	@ls -la $(BUILD_DIR)/$(APP_NAME)
	@echo ""
	@echo "$(GREEN)Dependencies:$(NC)"
	@ldd $(BUILD_DIR)/$(APP_NAME) 2>/dev/null || echo "Static binary (no dynamic dependencies)"
	@echo ""
	@echo "$(GREEN)Binary size:$(NC)"
	@du -h $(BUILD_DIR)/$(APP_NAME) | cut -f1

# Benchmark tests
.PHONY: bench
bench:
	@echo "$(GREEN)Running benchmarks...$(NC)"
	$(GO) test -bench=. -benchmem ./...

# Build for Raspberry Pi (ARM)
.PHONY: build-pi
build-pi: deps
	@echo "$(GREEN)Building for Raspberry Pi (ARM)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-pi $(SRC_DIR)
	@echo "$(GREEN)✓ Raspberry Pi build complete: $(BUILD_DIR)/$(APP_NAME)-pi$(NC)"

# Docker-related targets (optional)
.PHONY: docker-build
docker-build:
	@echo "$(GREEN)Building Docker image...$(NC)"
	docker build -t $(APP_NAME):$(VERSION) .
	docker build -t $(APP_NAME):latest .
	@echo "$(GREEN)✓ Docker image built$(NC)"