# Makefile for CNPG E2E Testing with pgEdge (Go/Terratest)

# Configuration file
VERSIONS_FILE := tests/config/versions.yaml

# Dynamic version lists from versions.yaml
ALL_CNPG_VERSIONS := $(shell yq eval '.cnpg_versions[].version' $(VERSIONS_FILE))
ALL_POSTGRES_VERSIONS := $(shell yq eval '.cnpg_versions[0].postgres_versions[]' $(VERSIONS_FILE))
ALL_VARIANTS := $(shell yq eval '.postgres_images.variants[].name' $(VERSIONS_FILE))

# Default configuration
CNPG_VERSION ?= $(firstword $(ALL_CNPG_VERSIONS))
POSTGRES_VERSION ?= $(lastword $(ALL_POSTGRES_VERSIONS))
POSTGRES_VARIANT ?= standard
POSTGRES_IMAGE_REGISTRY ?= public

# Provider configuration
CLUSTER_PROVIDER ?= kind
KUBERNETES_VERSION ?= 1.32
NODE_COUNT ?= 3
CLOUD_REGION ?=

# Test configuration
TEST_TIMEOUT ?= 30m
TEST_PARALLEL ?= 8
TEST_FLAGS ?= -v
LABEL_FILTER ?=

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

.PHONY: help
help: ## Show this help message
	@echo "CNPG E2E Testing Makefile (Go/Terratest)"
	@echo "========================================"
	@echo ""
	@echo "Quick Start:"
	@echo "  make test-smoke              - Run smoke tests (fastest)"
	@echo "  make test-infra              - Run infrastructure validation tests"
	@echo "  make test-operator           - Run operator deployment tests"
	@echo "  make test-image-validation   - Run image validation policy tests"
	@echo "  make test-comprehensive      - Run comprehensive upstream E2E tests"
	@echo "  make test-upstream LABEL_FILTER=<label> - Run upstream tests with custom label"
	@echo "  make test-all                - Run all tests"
	@echo ""
	@echo "Version-Specific Targets:"
	@echo "  make test-cnpg-VERSION   - Test specific CNPG version (e.g., test-cnpg-1.28.0)"
	@echo "  make test-pg-VERSION     - Test specific PostgreSQL version (e.g., test-pg-18)"
	@echo "  make test-VARIANT        - Test specific variant (e.g., test-standard)"
	@echo "  make list-versions       - List all available versions"
	@echo ""
	@echo "Registry Targets:"
	@echo "  make test-public         - Test with public images (default)"
	@echo "  make test-internal       - Test with internal pre-release images"
	@echo ""
	@echo "Multi-Version Testing:"
	@echo "  make test-all-cnpg       - Test all CNPG versions"
	@echo "  make test-all-postgres   - Test all PostgreSQL versions"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean               - Clean all test artifacts"
	@echo "  make clean-clusters      - Delete all Kind clusters"
	@echo ""
	@echo "Development:"
	@echo "  make deps                - Download Go dependencies"
	@echo "  make fmt                 - Format Go code"
	@echo "  make lint                - Run linters"
	@echo "  make check-prereqs       - Verify required tools are installed"
	@echo ""
	@echo "Configuration:"
	@echo "  CNPG_VERSION=$(CNPG_VERSION)"
	@echo "  POSTGRES_VERSION=$(POSTGRES_VERSION)"
	@echo "  POSTGRES_VARIANT=$(POSTGRES_VARIANT)"
	@echo "  POSTGRES_IMAGE_REGISTRY=$(POSTGRES_IMAGE_REGISTRY)"
	@echo "  CLUSTER_PROVIDER=$(CLUSTER_PROVIDER)"
	@echo "  KUBERNETES_VERSION=$(KUBERNETES_VERSION)"
	@echo "  NODE_COUNT=$(NODE_COUNT)"
	@echo "  TEST_TIMEOUT=$(TEST_TIMEOUT)"
	@echo "  TEST_PARALLEL=$(TEST_PARALLEL)"
	@echo ""

.PHONY: check-prereqs
check-prereqs: ## Check if required tools are installed
	@echo "Checking prerequisites..."
	@command -v docker >/dev/null 2>&1 || { echo "$(RED)Error: docker is not installed$(NC)"; exit 1; }
	@command -v kind >/dev/null 2>&1 || { echo "$(RED)Error: kind is not installed$(NC)"; exit 1; }
	@command -v kubectl >/dev/null 2>&1 || { echo "$(RED)Error: kubectl is not installed$(NC)"; exit 1; }
	@command -v helm >/dev/null 2>&1 || { echo "$(RED)Error: helm is not installed$(NC)"; exit 1; }
	@command -v git >/dev/null 2>&1 || { echo "$(RED)Error: git is not installed$(NC)"; exit 1; }
	@command -v go >/dev/null 2>&1 || { echo "$(RED)Error: go is not installed$(NC)"; exit 1; }
	@command -v ginkgo >/dev/null 2>&1 || { echo "$(YELLOW)Warning: ginkgo not installed (upstream E2E tests will fail)$(NC)"; }
	@docker ps >/dev/null 2>&1 || { echo "$(RED)Error: Docker is not running$(NC)"; exit 1; }
	@echo "$(GREEN)All prerequisites satisfied!$(NC)"

# Quick Start Targets

.PHONY: test-smoke
test-smoke: check-prereqs ## Run smoke tests (fastest)
	@$(MAKE) test-upstream LABEL_FILTER=smoke

.PHONY: test-infra
test-infra: check-prereqs ## Run infrastructure validation tests
	@echo "$(BLUE)Running infrastructure tests...$(NC)"
	cd tests && CLUSTER_PROVIDER=$(CLUSTER_PROVIDER) KUBERNETES_VERSION=$(KUBERNETES_VERSION) NODE_COUNT=$(NODE_COUNT) CLOUD_REGION=$(CLOUD_REGION) \
		go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) . -run TestInfra

.PHONY: test-operator
test-operator: check-prereqs ## Run CNPG operator deployment tests
	@echo "$(BLUE)Running operator deployment tests...$(NC)"
	cd tests && CLUSTER_PROVIDER=$(CLUSTER_PROVIDER) KUBERNETES_VERSION=$(KUBERNETES_VERSION) NODE_COUNT=$(NODE_COUNT) CLOUD_REGION=$(CLOUD_REGION) \
		go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) . -run TestOperator

.PHONY: test-image-validation
test-image-validation: check-prereqs ## Run image validation policy tests
	@echo "$(BLUE)Running image validation policy tests...$(NC)"
	cd tests && CLUSTER_PROVIDER=$(CLUSTER_PROVIDER) KUBERNETES_VERSION=$(KUBERNETES_VERSION) NODE_COUNT=$(NODE_COUNT) CLOUD_REGION=$(CLOUD_REGION) \
		go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) . -run TestImageValidation

.PHONY: test-comprehensive
test-comprehensive: check-prereqs ## Run comprehensive upstream E2E tests (all labels)
	@$(MAKE) test-upstream

.PHONY: test-upstream
test-upstream: check-prereqs ## Run upstream E2E tests with custom label filter (LABEL_FILTER=postgres-configuration)
	@echo "$(BLUE)Running upstream E2E tests$(if $(LABEL_FILTER), with label filter: $(LABEL_FILTER),)...$(NC)"
	cd tests && CLUSTER_PROVIDER=$(CLUSTER_PROVIDER) KUBERNETES_VERSION=$(KUBERNETES_VERSION) NODE_COUNT=$(NODE_COUNT) CLOUD_REGION=$(CLOUD_REGION) \
		LABEL_FILTER="$(LABEL_FILTER)" go test $(TEST_FLAGS) -timeout 3h . -run TestUpstream

.PHONY: test-all
test-all: check-prereqs ## Run all tests in parallel
	@echo "$(BLUE)Running all tests...$(NC)"
	cd tests && CLUSTER_PROVIDER=$(CLUSTER_PROVIDER) KUBERNETES_VERSION=$(KUBERNETES_VERSION) NODE_COUNT=$(NODE_COUNT) CLOUD_REGION=$(CLOUD_REGION) \
		go test $(TEST_FLAGS) -timeout 4h -parallel $(TEST_PARALLEL) .

# Version-Specific Targets

define CNPG_VERSION_RULE
.PHONY: test-cnpg-$(1)
test-cnpg-$(1): check-prereqs
	@echo "$(BLUE)Testing CNPG version $(1)...$(NC)"
	cd tests && CNPG_VERSION=$(1) go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) . -run TestOperator
endef

$(foreach version,$(ALL_CNPG_VERSIONS),$(eval $(call CNPG_VERSION_RULE,$(version))))

define POSTGRES_VERSION_RULE
.PHONY: test-pg-$(1)
test-pg-$(1): check-prereqs
	@echo "$(BLUE)Testing PostgreSQL version $(1)...$(NC)"
	cd tests && POSTGRES_VERSION=$(1) go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) . -run TestOperator
endef

$(foreach version,$(ALL_POSTGRES_VERSIONS),$(eval $(call POSTGRES_VERSION_RULE,$(version))))

define VARIANT_RULE
.PHONY: test-$(1)
test-$(1): check-prereqs
	@echo "$(BLUE)Testing variant $(1)...$(NC)"
	cd tests && POSTGRES_VARIANT=$(1) go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) . -run TestOperator
endef

$(foreach variant,$(ALL_VARIANTS),$(eval $(call VARIANT_RULE,$(variant))))

# Multi-Version Testing

.PHONY: test-all-cnpg
test-all-cnpg: check-prereqs ## Test all CNPG versions
	@echo "$(BLUE)Testing all CNPG versions...$(NC)"
	@for cnpg_version in $(ALL_CNPG_VERSIONS); do \
		echo "$(GREEN)Testing CNPG $$cnpg_version$(NC)"; \
		cd tests && CNPG_VERSION=$$cnpg_version go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) . -run TestOperator || exit 1; \
	done

.PHONY: test-all-postgres
test-all-postgres: check-prereqs ## Test all PostgreSQL versions
	@echo "$(BLUE)Testing all PostgreSQL versions with current CNPG version...$(NC)"
	@for pg_version in $(ALL_POSTGRES_VERSIONS); do \
		echo "$(GREEN)Testing PostgreSQL $$pg_version$(NC)"; \
		cd tests && POSTGRES_VERSION=$$pg_version go test $(TEST_FLAGS) -timeout $(TEST_TIMEOUT) . -run TestOperator || exit 1; \
	done

# Cleanup Targets

.PHONY: clean-clusters
clean-clusters: ## Delete all Kind clusters
	@echo "$(BLUE)Deleting all Kind clusters...$(NC)"
	@kind get clusters | grep "cnpg" | xargs -r kind delete cluster --name || true
	@rm -f /tmp/cnpg-*.kubeconfig
	@echo "$(GREEN)Clusters cleaned up$(NC)"

.PHONY: clean-results
clean-results: ## Delete test results
	@echo "$(BLUE)Deleting test results...$(NC)"
	@rm -rf tests/test-results
	@echo "$(GREEN)Test results cleaned up$(NC)"

.PHONY: clean-temp
clean-temp: ## Delete temporary CNPG clones
	@echo "$(BLUE)Deleting temporary CNPG clones...$(NC)"
	@rm -rf /tmp/cnpg-e2e-*
	@echo "$(GREEN)Temporary files cleaned up$(NC)"

.PHONY: clean
clean: clean-clusters clean-results clean-temp ## Clean all test artifacts
	@echo "$(GREEN)All artifacts cleaned!$(NC)"

# Development Targets

.PHONY: deps
deps: ## Download Go dependencies
	@echo "$(BLUE)Downloading Go dependencies...$(NC)"
	go mod download
	go mod tidy
	@echo "$(GREEN)Dependencies updated!$(NC)"

.PHONY: fmt
fmt: ## Format Go code
	@echo "$(BLUE)Formatting Go code...$(NC)"
	gofmt -w -s ./tests
	go mod tidy
	@echo "$(GREEN)Code formatted!$(NC)"

.PHONY: lint
lint: ## Run linters
	@echo "$(BLUE)Running linters...$(NC)"
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "$(YELLOW)Installing golangci-lint...$(NC)"; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}
	golangci-lint run ./tests/...
	@echo "$(GREEN)Linting complete!$(NC)"

.PHONY: test-dry-run
test-dry-run: ## Show configuration without running tests
	@echo "Current Configuration:"
	@echo "  CNPG_VERSION:             $(CNPG_VERSION)"
	@echo "  POSTGRES_VERSION:         $(POSTGRES_VERSION)"
	@echo "  POSTGRES_VARIANT:         $(POSTGRES_VARIANT)"
	@echo "  POSTGRES_IMAGE_REGISTRY:  $(POSTGRES_IMAGE_REGISTRY)"
	@echo "  TEST_TIMEOUT:             $(TEST_TIMEOUT)"
	@echo "  TEST_PARALLEL:            $(TEST_PARALLEL)"
	@echo ""
	@echo "Available Versions (from $(VERSIONS_FILE)):"
	@echo "  CNPG Versions:            $(ALL_CNPG_VERSIONS)"
	@echo "  PostgreSQL Versions:      $(ALL_POSTGRES_VERSIONS)"
	@echo "  Image Variants:           $(ALL_VARIANTS)"

.PHONY: list-versions
list-versions: ## List all available versions from versions.yaml
	@echo "Available Versions from $(VERSIONS_FILE):"
	@echo ""
	@echo "$(GREEN)CNPG Versions:$(NC)"
	@for version in $(ALL_CNPG_VERSIONS); do \
		echo "  - $$version (make test-cnpg-$$version)"; \
	done
	@echo ""
	@echo "$(GREEN)PostgreSQL Versions:$(NC)"
	@for version in $(ALL_POSTGRES_VERSIONS); do \
		echo "  - $$version (make test-pg-$$version)"; \
	done
	@echo ""
	@echo "$(GREEN)Image Variants:$(NC)"
	@for variant in $(ALL_VARIANTS); do \
		echo "  - $$variant (make test-$$variant)"; \
	done

.PHONY: list-clusters
list-clusters: ## List active Kind clusters
	@echo "Active Kind clusters:"
	@kind get clusters | grep "cnpg" || echo "No CNPG clusters found"

.PHONY: list-tests
list-tests: ## List all available tests
	@echo "Available tests:"
	@cd tests && go test -list . . 2>/dev/null | grep -E "^Test" || true

.PHONY: install-tools
install-tools: ## Install required tools (macOS with Homebrew)
	@echo "$(BLUE)Installing required tools...$(NC)"
	@command -v brew >/dev/null 2>&1 || { echo "$(RED)Error: Homebrew not found$(NC)"; exit 1; }
	brew install kind kubectl helm go || true
	go install github.com/onsi/ginkgo/v2/ginkgo@latest
	@echo "$(GREEN)Tools installed!$(NC)"
	@echo "$(YELLOW)Note: Please install Docker Desktop separately if not already installed$(NC)"

# Default target
.DEFAULT_GOAL := help
