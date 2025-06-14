# Lambda NAT Proxy - Makefile
# Simple build and test operations

# Build configuration
BUILD_DIR := build
LAMBDA_PROXY_BIN := $(BUILD_DIR)/lambda-nat-proxy
LAMBDA_BOOTSTRAP := $(BUILD_DIR)/bootstrap

.PHONY: help build test e2e clean tidy

# Default target
all: build

help: ## Show this help
	@echo "Lambda NAT Proxy - Build & Test"
	@echo ""
	@echo "Build Options:"
	@echo "  make build            Build locally (requires Node.js + Go)"
	@echo "  make docker-build     Build using Docker (no local deps needed)"
	@echo "  make build-all        Build for all platforms using Docker"
	@echo ""
	@echo "Development:"
	@echo "  make test             Run all tests"
	@echo "  make e2e              Run end-to-end connectivity test"
	@echo "  make clean            Remove build artifacts"
	@echo "  make tidy             Tidy Go modules"
	@echo ""
	@echo "CLI Usage:"
	@echo "  ./build/lambda-nat-proxy deploy         Deploy infrastructure"
	@echo "  ./build/lambda-nat-proxy run            Start proxy with dashboard"
	@echo "  ./build/lambda-nat-proxy status         Check deployment status"
	@echo "  ./build/lambda-nat-proxy destroy        Remove all resources"

build: ## Build lambda-nat-proxy CLI with embedded Lambda function and dashboard
	@echo "Building dashboard frontend..."
	@./scripts/build-dashboard.sh
	@echo "Building Lambda function..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p cmd/lambda-nat-proxy/assets
	@cd lambda && GOOS=linux GOARCH=amd64 go build -o ../cmd/lambda-nat-proxy/assets/bootstrap .
	@chmod +x cmd/lambda-nat-proxy/assets/bootstrap
	@echo "✅ Built: cmd/lambda-nat-proxy/assets/bootstrap"
	@echo "Building lambda-nat-proxy CLI with embedded Lambda and dashboard..."
	@go build -o $(LAMBDA_PROXY_BIN) ./cmd/lambda-nat-proxy
	@echo "✅ Built: $(LAMBDA_PROXY_BIN) (with embedded Lambda function and dashboard)"
	@echo "Copying bootstrap to build directory for consistency..."
	@cp cmd/lambda-nat-proxy/assets/bootstrap $(LAMBDA_BOOTSTRAP)
	@echo "✅ Built: $(LAMBDA_BOOTSTRAP)"

docker-build: ## Build using Docker (no local dependencies required)
	@echo "🐳 Building with Docker (includes all dependencies)..."
	@./scripts/docker-build.sh

build-all: ## Build for multiple platforms using Docker
	@echo "🌍 Building for multiple platforms using Docker..."
	@./scripts/docker-build-all.sh

test: ## Run all tests
	@echo "Running all tests..."
	@echo "Building dashboard for embedded tests..."
	@./scripts/build-dashboard.sh
	@echo "Building Lambda function for embedded tests..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p cmd/lambda-nat-proxy/assets
	@cd lambda && GOOS=linux GOARCH=amd64 go build -o ../cmd/lambda-nat-proxy/assets/bootstrap .
	@chmod +x cmd/lambda-nat-proxy/assets/bootstrap
	@go test -v ./...
	@echo "✅ All tests passed"

e2e: build ## Run end-to-end connectivity test
	@echo "Running end-to-end tests..."
	@cd test/e2e && go test -v .
	@echo "✅ End-to-end tests passed"

clean: ## Remove build artifacts
	@echo "Cleaning build directory..."
	@rm -rf $(BUILD_DIR)
	@echo "Cleaning embedded assets..."
	@rm -rf cmd/lambda-nat-proxy/assets
	@echo "Cleaning dashboard build artifacts..."
	@rm -rf web/dist
	@rm -rf internal/dashboard/web
	@echo "✅ Build artifacts removed"

tidy: ## Tidy Go modules
	@echo "Tidying Go modules..."
	@go mod tidy
	@cd lambda && go mod tidy
	@cd test/e2e && go mod tidy
	@echo "✅ Go modules tidied"