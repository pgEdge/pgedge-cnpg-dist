# CNPG Build - pgEdge CloudNativePG Distribution

Build, package, and test pgEdge's distribution of CloudNativePG (CNPG) with [pgEdge Enterprise Postgres images](https://github.com/pgEdge/postgres-images).

## Overview

This project serves two primary purposes:

### Build & Package pgEdge CNPG Distribution
- **Image Building**: Build and distribute CNPG operator and plugin images
- **Chart Management**: Generate and maintain Helm charts and manifests for multiple CNPG versions

### End-to-End Testing Infrastructure
- **Go/Terratest Framework**: Automated Kubernetes cluster provisioning (Kind, EKS, AKS, GKE)
- **Multi-Version Testing**: Test CNPG versions across PostgreSQL versions and Kubernetes distributions
- **Upstream Validation**: Run official CNPG test suite with pgEdge distributed operator and pgEdge Enterprise Postgres images

## Quick Start

### Prerequisites

```bash
# macOS
make install-tools

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

# Run comprehensive tests (more extensive E2E tests)
make test-smoke
```

## Building Operator & Plugin Images

All images and plugins are built via GitHub Actions workflows triggered manually through the Actions UI.

### CNPG Operator Image

Builds and pushes the CloudNativePG operator image to GHCR.

**Workflow:** [build-cnpg-operator.yml](.github/workflows/build-cnpg-operator.yml)

| Input | Description | Default |
|-------|-------------|---------|
| `image_name` | Image repository (without `ghcr.io/`) | `pgedge/cloudnative-pg-internal` |
| `ref` | CNPG Git tag or branch | `v1.27.0` |
| `version` | Image tag to publish | `1.27.0` |

**Example:** To build CNPG 1.28.0:
- `ref`: `v1.28.0`
- `version`: `1.28.0`
- `image_name`: `pgedge/cloudnative-pg` (public) or `pgedge/cloudnative-pg-internal` (pre-release)

**Output:** `ghcr.io/pgedge/cloudnative-pg:1.28.0` (linux/amd64, linux/arm64)

### kubectl-cnpg Plugin

Builds the kubectl plugin for all platforms and optionally publishes to a Krew index.

**Workflow:** [build-kubectl-cnpg-plugin.yml](.github/workflows/build-kubectl-cnpg-plugin.yml)

| Input | Description | Default |
|-------|-------------|---------|
| `source_repo` | Source repository | `cloudnative-pg/cloudnative-pg` |
| `ref` | Tag or branch to build | `main` |
| `release_version` | Release version (e.g., `v1.27.1`) | - |
| `is_release` | Create Krew index PR | `false` |

**Platforms:** linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64

**Output:**
- GitHub Release with binaries and checksums
- Krew manifest (if `is_release: true`)
- PR to krew-index repository (if `is_release: true`)

### Barman Cloud Sidecar

Builds the Barman Cloud backup/restore sidecar images (base, plugin, and sidecar).

**Workflow:** [build-barman-sidecar.yml](.github/workflows/build-barman-sidecar.yml)

| Input | Description | Default |
|-------|-------------|---------|
| `image_name` | Sidecar image repository | `pgedge/plugin-barman-cloud-sidecar-internal` |
| `ref` | Git tag from `cloudnative-pg/plugin-barman-cloud` | `v0.7.0` |
| `version` | Version tag | `0.7.0` |

**Output (all linux/amd64, linux/arm64):**
- `ghcr.io/pgedge/plugin-barman-cloud-base:v0.7.0`
- `ghcr.io/pgedge/plugin-barman-cloud-plugin:v0.7.0`
- `ghcr.io/pgedge/plugin-barman-cloud-sidecar:v0.7.0`

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

**Excluded tests:**
- `backup-restore`, `snapshot` - pgEdge images use new Barman Cloud Plugin architecture
- `postgres-major-upgrade` - requires specific upgrade path setup
- `plugin` - requires plugin infrastructure not available in test environment
- `observability` - requires PodMonitor CRD from prometheus-operator

## Configuration

All tests are configured via [`tests/config/versions.yaml`](tests/config/versions.yaml):

```yaml
cnpg_versions:
  - version: "1.28.0"
    chart_version: "0.27.0"  # Helm chart version (different from operator version)
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