# Makefile for CNPG E2E Testing with pgEdge

# Configuration file
VERSIONS_FILE := config/versions.yaml

# Dynamic version lists from versions.yaml
ALL_CNPG_VERSIONS := $(shell yq eval '.cnpg_versions[].version' $(VERSIONS_FILE))
ALL_POSTGRES_VERSIONS := $(shell yq eval '.cnpg_versions[0].postgres_versions[]' $(VERSIONS_FILE))
ALL_VARIANTS := $(shell yq eval '.postgres_images.variants[].name' $(VERSIONS_FILE))

# Default configuration
CNPG_VERSION ?= $(firstword $(ALL_CNPG_VERSIONS))
POSTGRES_VERSION ?= $(lastword $(ALL_POSTGRES_VERSIONS))
POSTGRES_VARIANT ?= standard
POSTGRES_IMAGE_REGISTRY ?= public
PROVIDER ?= kind
FEATURE_TYPE ?= smoke
TEST_DEPTH ?= 0
PARALLEL ?= true
MAX_WORKERS ?= 3

# Directories
SCRIPT_DIR := scripts
TEST_RUNNER := $(SCRIPT_DIR)/run-e2e-tests.sh

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
NC := \033[0m # No Color

.PHONY: help
help: ## Show this help message
	@echo "CNPG E2E Testing Makefile"
	@echo "========================="
	@echo ""
	@echo "Quick Start Targets:"
	@echo "  make test-smoke          - Run smoke tests (default config)"
	@echo "  make test-basic          - Run smoke + basic tests"
	@echo "  make test-all-postgres   - Test all PostgreSQL versions in parallel"
	@echo "  make test-all-variants   - Test minimal and standard variants"
	@echo "  make list-versions       - List all available versions from versions.yaml"
	@echo ""
	@echo "Version-Specific Targets (auto-generated from versions.yaml):"
	@echo "  make test-cnpg-<version> - Test specific CNPG version"
	@echo "  make test-pg-<version>   - Test specific PostgreSQL version"
	@echo "  make test-<variant>      - Test specific image variant"
	@echo "  Run 'make list-versions' to see all available targets"
	@echo ""
	@echo "Registry Targets:"
	@echo "  make test-public         - Test with public images (default)"
	@echo "  make test-internal       - Test with internal pre-release images"
	@echo ""
	@echo "Comprehensive Testing:"
	@echo "  make test-all-cnpg-versions - Test all CNPG versions sequentially"
	@echo "  make test-full-matrix       - Test all combinations (VERY SLOW!)"
	@echo "  make test-sequential        - Run tests sequentially (no parallel)"
	@echo ""
	@echo "Configuration Variables (defaults from versions.yaml):"
	@echo "  CNPG_VERSION             - CNPG version (default: $(CNPG_VERSION))"
	@echo "  POSTGRES_VERSION         - PostgreSQL version (default: $(POSTGRES_VERSION))"
	@echo "  POSTGRES_VARIANT         - Variant: minimal|standard (default: $(POSTGRES_VARIANT))"
	@echo "  POSTGRES_IMAGE_REGISTRY  - Registry: public|internal (default: $(POSTGRES_IMAGE_REGISTRY))"
	@echo "  PROVIDER                 - Provider: kind|aks|eks (default: $(PROVIDER))"
	@echo "  FEATURE_TYPE             - Test features (default: $(FEATURE_TYPE))"
	@echo "  TEST_DEPTH               - Test depth 0-4 (default: $(TEST_DEPTH))"
	@echo "  PARALLEL                 - Parallel execution (default: $(PARALLEL))"
	@echo "  MAX_WORKERS              - Max parallel workers (default: $(MAX_WORKERS))"
	@echo ""
	@echo "Examples:"
	@echo "  make test-smoke POSTGRES_VERSION=17"
	@echo "  make test-basic CNPG_VERSION=1.27.1"
	@echo "  make test-internal POSTGRES_VERSION=18"
	@echo "  make test-all-postgres FEATURE_TYPE=smoke,basic"
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
	@command -v yq >/dev/null 2>&1 || { echo "$(RED)Error: yq is not installed$(NC)"; exit 1; }
	@echo "$(GREEN)All prerequisites satisfied!$(NC)"

# Quick Start Targets

.PHONY: test-smoke
test-smoke: check-prereqs ## Run smoke tests (fastest)
	$(TEST_RUNNER) \
		--cnpg-version $(CNPG_VERSION) \
		--postgres-version $(POSTGRES_VERSION) \
		--postgres-variant $(POSTGRES_VARIANT) \
		--postgres-registry $(POSTGRES_IMAGE_REGISTRY) \
		--provider $(PROVIDER) \
		--feature-type smoke \
		--test-depth 0

.PHONY: test-basic
test-basic: check-prereqs ## Run smoke + basic tests
	$(TEST_RUNNER) \
		--cnpg-version $(CNPG_VERSION) \
		--postgres-version $(POSTGRES_VERSION) \
		--postgres-variant $(POSTGRES_VARIANT) \
		--postgres-registry $(POSTGRES_IMAGE_REGISTRY) \
		--provider $(PROVIDER) \
		--feature-type smoke,basic \
		--test-depth 1

.PHONY: test-comprehensive
test-comprehensive: check-prereqs ## Run comprehensive tests (slow)
	$(TEST_RUNNER) \
		--cnpg-version $(CNPG_VERSION) \
		--postgres-version $(POSTGRES_VERSION) \
		--postgres-variant $(POSTGRES_VARIANT) \
		--postgres-registry $(POSTGRES_IMAGE_REGISTRY) \
		--provider $(PROVIDER) \
		--feature-type smoke,basic,security,backup-restore,observability,replication \
		--test-depth 2

# Version-Specific Targets (dynamically generated from versions.yaml)

# CNPG version targets
define CNPG_VERSION_RULE
.PHONY: test-cnpg-$(1)
test-cnpg-$(1): check-prereqs
	$$(MAKE) test-smoke CNPG_VERSION=$(1)
endef

$(foreach version,$(ALL_CNPG_VERSIONS),$(eval $(call CNPG_VERSION_RULE,$(version))))

# PostgreSQL version targets
define POSTGRES_VERSION_RULE
.PHONY: test-pg-$(1)
test-pg-$(1): check-prereqs
	$$(MAKE) test-smoke POSTGRES_VERSION=$(1)
endef

$(foreach version,$(ALL_POSTGRES_VERSIONS),$(eval $(call POSTGRES_VERSION_RULE,$(version))))

# Variant Targets (dynamically generated from versions.yaml)

define VARIANT_RULE
.PHONY: test-$(1)
test-$(1): check-prereqs
	$$(MAKE) test-smoke POSTGRES_VARIANT=$(1)
endef

$(foreach variant,$(ALL_VARIANTS),$(eval $(call VARIANT_RULE,$(variant))))

# Registry Targets

.PHONY: test-public
test-public: check-prereqs ## Test with public release images
	$(MAKE) test-smoke POSTGRES_IMAGE_REGISTRY=public

.PHONY: test-internal
test-internal: check-prereqs ## Test with internal pre-release images
	$(MAKE) test-smoke POSTGRES_IMAGE_REGISTRY=internal

# Multi-Version Testing

.PHONY: test-all-postgres
test-all-postgres: check-prereqs ## Test all PostgreSQL versions (16,17,18) in parallel
	$(TEST_RUNNER) \
		--cnpg-version $(CNPG_VERSION) \
		--postgres-variant $(POSTGRES_VARIANT) \
		--postgres-registry $(POSTGRES_IMAGE_REGISTRY) \
		--provider $(PROVIDER) \
		--feature-type $(FEATURE_TYPE) \
		--test-depth $(TEST_DEPTH) \
		--all-postgres-versions \
		--parallel \
		--max-workers $(MAX_WORKERS)

.PHONY: test-all-variants
test-all-variants: check-prereqs ## Test both minimal and standard variants
	$(TEST_RUNNER) \
		--cnpg-version $(CNPG_VERSION) \
		--postgres-version $(POSTGRES_VERSION) \
		--postgres-registry $(POSTGRES_IMAGE_REGISTRY) \
		--provider $(PROVIDER) \
		--feature-type $(FEATURE_TYPE) \
		--test-depth $(TEST_DEPTH) \
		--all-postgres-versions \
		--all-variants \
		--parallel \
		--max-workers $(MAX_WORKERS)

.PHONY: test-all-cnpg-versions
test-all-cnpg-versions: check-prereqs ## Test all CNPG versions (sequential)
	@echo "Testing all CNPG versions from versions.yaml..."
	@for version in $(ALL_CNPG_VERSIONS); do \
		echo "$(GREEN)Testing CNPG version: $$version$(NC)"; \
		$(MAKE) test-smoke CNPG_VERSION=$$version || exit 1; \
	done

# Comprehensive Matrix Testing

.PHONY: test-full-matrix
test-full-matrix: check-prereqs ## Test full matrix (all versions, all variants) - VERY SLOW!
	@echo "$(YELLOW)Warning: This will test all combinations and may take hours!$(NC)"
	@echo "CNPG Versions: $(ALL_CNPG_VERSIONS)"
	@echo "PostgreSQL Versions: $(ALL_POSTGRES_VERSIONS)"
	@echo "Image Variants: $(ALL_VARIANTS)"
	@echo "Press Ctrl+C to cancel, or wait 10 seconds to continue..."
	@sleep 10
	@for cnpg in $(ALL_CNPG_VERSIONS); do \
		echo "$(GREEN)Testing CNPG $$cnpg$(NC)"; \
		$(TEST_RUNNER) \
			--cnpg-version $$cnpg \
			--provider $(PROVIDER) \
			--feature-type $(FEATURE_TYPE) \
			--test-depth $(TEST_DEPTH) \
			--all-postgres-versions \
			--all-variants \
			--parallel \
			--max-workers $(MAX_WORKERS) || exit 1; \
	done
	@echo "$(GREEN)All matrix tests completed!$(NC)"

# Sequential Testing (no parallelism)

.PHONY: test-sequential
test-sequential: check-prereqs ## Run tests sequentially (disable parallel)
	$(TEST_RUNNER) \
		--cnpg-version $(CNPG_VERSION) \
		--postgres-version $(POSTGRES_VERSION) \
		--postgres-variant $(POSTGRES_VARIANT) \
		--postgres-registry $(POSTGRES_IMAGE_REGISTRY) \
		--provider $(PROVIDER) \
		--feature-type $(FEATURE_TYPE) \
		--test-depth $(TEST_DEPTH) \
		--no-parallel

# Cleanup Targets

.PHONY: clean-clusters
clean-clusters: ## Delete all cnpg-e2e kind clusters
	@echo "Deleting all cnpg-e2e-* kind clusters..."
	@kind get clusters | grep "^cnpg-e2e-" | xargs -r kind delete cluster --name || true
	@echo "$(GREEN)Clusters cleaned up$(NC)"

.PHONY: clean-results
clean-results: ## Delete test results
	@echo "Deleting test results..."
	@rm -rf ../test-results
	@echo "$(GREEN)Test results cleaned up$(NC)"

.PHONY: clean-temp
clean-temp: ## Delete temporary CNPG clones
	@echo "Deleting temporary CNPG clones..."
	@rm -rf /tmp/cnpg-e2e-*
	@echo "$(GREEN)Temporary files cleaned up$(NC)"

.PHONY: clean
clean: clean-clusters clean-results clean-temp ## Clean all test artifacts

# Development Targets

.PHONY: test-dry-run
test-dry-run: ## Show what would be tested without running tests
	@echo "Configuration:"
	@echo "  CNPG_VERSION:             $(CNPG_VERSION)"
	@echo "  POSTGRES_VERSION:         $(POSTGRES_VERSION)"
	@echo "  POSTGRES_VARIANT:         $(POSTGRES_VARIANT)"
	@echo "  POSTGRES_IMAGE_REGISTRY:  $(POSTGRES_IMAGE_REGISTRY)"
	@echo "  PROVIDER:                 $(PROVIDER)"
	@echo "  FEATURE_TYPE:             $(FEATURE_TYPE)"
	@echo "  TEST_DEPTH:               $(TEST_DEPTH)"
	@echo "  PARALLEL:                 $(PARALLEL)"
	@echo "  MAX_WORKERS:              $(MAX_WORKERS)"
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
list-clusters: ## List active kind clusters
	@echo "Active kind clusters:"
	@kind get clusters | grep "cnpg-e2e" || echo "No cnpg-e2e clusters found"

.PHONY: install-tools
install-tools: ## Install required tools (macOS with Homebrew)
	@echo "Installing required tools..."
	@command -v brew >/dev/null 2>&1 || { echo "$(RED)Error: Homebrew not found$(NC)"; exit 1; }
	brew install kind kubectl helm yq go || true
	@echo "$(GREEN)Tools installed!$(NC)"
	@echo "Note: Please install Docker Desktop separately if not already installed"

# Default target
.DEFAULT_GOAL := help
