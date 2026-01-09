# CNPG Build - pgEdge CloudNativePG Distribution

Build, package, and test pgEdge's distribution of CloudNativePG (CNPG) with pgEdge Enterprise Postgres images.

## Overview

This project serves two primary purposes:

### Build & Package pgEdge CNPG Distribution
- **Operator Customization**: Patch upstream CNPG operator for pgEdge PostgreSQL compatibility
- **Image Building**: Produce pgEdge distributed CNPG operator images
- **Chart Management**: Generate and maintain Helm charts for multiple CNPG versions
- **Release Automation**: Package and publish pgEdge CNPG releases

### End-to-End Testing Infrastructure
- **Go/Terratest Framework**: Automated Kubernetes cluster provisioning (Kind, EKS, AKS, GKE)
- **Multi-Version Testing**: Test CNPG versions across PostgreSQL versions and variants
- **Upstream Validation**: Run official CNPG test suite with pgEdge images


## Quick Start

### Prerequisites

```bash
# macOS
brew install go kind kubectl helm git docker

# Install Ginkgo (for upstream CNPG tests)
go install github.com/onsi/ginkgo/v2/ginkgo@latest

# Verify Docker is running
docker ps
```

### Run Your First Test

```bash
# Check prerequisites
make check-prereqs

# Run infrastructure validation
make test-infra

# Run operator deployment test
make test-operator

# Run smoke tests (fastest E2E tests)
make test-smoke
```

## Building pgEdge CNPG

[TODO: Add build documentation once build scripts are implemented]

## Testing

### Test Categories

#### 1. Infrastructure Tests
Validate Kubernetes cluster provisioning and CSI storage setup.

```bash
make test-infra
```

#### 2. Operator Tests
Validate CNPG operator deployment with pgEdge images.

```bash
make test-operator           # Single version
make test-all-cnpg          # All CNPG versions
make test-cnpg-1.28.0       # Specific CNPG version
```

#### 3. PostgreSQL Version Tests
Test pgEdge PostgreSQL images across versions and variants.

```bash
make test-pg-18             # PostgreSQL 18
make test-all-postgres      # All PostgreSQL versions
make test-standard          # Standard variant
make test-minimal           # Minimal variant
```

#### 4. Image Validation Tests
Verify Kubernetes admission control blocks non-pgEdge images.

```bash
make test-image-validation
```

#### 5. Upstream E2E Tests
Run official CNPG test suite with pgEdge PostgreSQL images.

```bash
make test-smoke           # Smoke tests (~30min)
make test-comprehensive   # Full E2E suite (~3h)
```

**Note**: `backup-restore` and `snapshot` tests are automatically excluded because pgEdge images use the new Barman Cloud Plugin architecture instead of embedded Barman tools.

### Registry Selection

```bash
# Test with public images (default)
make test-public

# Test with internal pre-release images
make test-internal
```

### Multi-Version Testing

```bash
# Test full version matrix (VERY SLOW!)
make test-matrix
```

## Configuration

All tests are configured via [`tests/config/versions.yaml`](tests/config/versions.yaml):

```yaml
cnpg_versions:
  - version: "1.28.0"
    git_tag: "v1.28.0"
    operator_image: "ghcr.io/pgedge/cloudnative-pg:1.28.0"
    postgres_versions: ["16", "17", "18"]
    providers:
      kind:
        kubernetes_versions: ["1.32", "1.33", "1.34"]

postgres_images:
  registries:
    public:
      base: "ghcr.io/pgedge/pgedge-postgres"
    internal:
      base: "ghcr.io/pgedge/pgedge-postgres-internal"
  spock_version: "spock5"
  variants:
    - name: "minimal"
      tag_suffix: "-minimal"
    - name: "standard"
      tag_suffix: "-standard"
```

## Image Validation Policy

All test clusters automatically enforce pgEdge PostgreSQL image usage via Kubernetes `ValidatingAdmissionPolicy`:

**Allowed:**
- `ghcr.io/pgedge/pgedge-postgres:*`
- `ghcr.io/pgedge/pgedge-postgres-internal:*`

**Blocked:**
- `ghcr.io/cloudnative-pg/postgresql:*` (upstream CNPG images)
- `postgres:*` (Docker Hub images)
- Any other PostgreSQL image

This prevents accidental use of non-pgEdge images during testing. 

## License

[Your license here]
