# pgEdge Enterprise Postgres for Kubernetes

Helm charts, manifests, container images, and testing infrastructure for running [pgEdge Enterprise Postgres](https://github.com/pgEdge/postgres-images) on Kubernetes using the CloudNativePG operator.

## Overview

This repository provides:

- **Rebuilt Operator Images**: CloudNativePG operator images rebuilt from source and published to pgEdge's container registry
- **Configured Helm Charts & Manifests**: Redistributes CloudNativePG charts and manifests with modifications to use pgEdge-built operator images
- **End-to-End Testing**: Automated validation against the upstream CloudNativePG test suite with pgEdge Enterprise Postgres images
- **Multi-Version Support**: Testing across PostgreSQL versions, Kubernetes distributions, and operator versions

## Relationship to CloudNativePG

This project builds upon [CloudNativePG](https://cloudnative-pg.io/), a [CNCF Sandbox project](https://www.cncf.io/projects/cloudnativepg/) for managing PostgreSQL on Kubernetes.

This project is **not affiliated with, endorsed by, or sponsored by** the CloudNativePG project or the Cloud Native Computing Foundation.

## Container Images

We rebuild and publish CloudNativePG operator images from upstream source:

| Image | Source | Registry |
|-------|--------|----------|
| CloudNativePG Operator | [cloudnative-pg/cloudnative-pg](https://github.com/cloudnative-pg/cloudnative-pg) | `ghcr.io/pgedge/cloudnative-pg` |
| Barman Cloud Plugin | [cloudnative-pg/plugin-barman-cloud](https://github.com/cloudnative-pg/plugin-barman-cloud) | `ghcr.io/pgedge/plugin-barman-cloud-*` |

Images are built via GitHub Actions workflows from upstream source tags without modification.

## Helm Charts

We redistribute CloudNativePG Helm charts with modifications to use pgEdge-built operator images:

| Chart | Versions | Upstream Source |
|-------|----------|-----------------|
| `charts/cloudnative-pg/` | v0.26.0, v0.26.1, v0.27.0 | [cloudnative-pg/charts](https://github.com/cloudnative-pg/charts) |
| `charts/plugin-barman-cloud/` | v0.7.0 | [cloudnative-pg/plugin-barman-cloud](https://github.com/cloudnative-pg/plugin-barman-cloud) |

**Modification:** Default image references changed to `ghcr.io/pgedge/` registry.

## Manifests

We redistribute CloudNativePG installation manifests with modifications to use pgEdge-built operator images:

| Version | Upstream Source |
|---------|-----------------|
| v1.27.0 | [cloudnative-pg v1.27.0](https://github.com/cloudnative-pg/cloudnative-pg/releases/tag/v1.27.0) |
| v1.27.1 | [cloudnative-pg v1.27.1](https://github.com/cloudnative-pg/cloudnative-pg/releases/tag/v1.27.1) |
| v1.28.0 | [cloudnative-pg v1.28.0](https://github.com/cloudnative-pg/cloudnative-pg/releases/tag/v1.28.0) |

**Modification:** Operator image references changed to `ghcr.io/pgedge/` registry.

## Quick Start

### Prerequisites

```bash
# macOS
make install-tools

# Verify Docker is running
docker ps
```

### Run Tests

```bash
# Check prerequisites
make check-prereqs

# Run infrastructure validation
make test-infra

# Run operator deployment test
make test-operator

# Run smoke tests
make test-smoke

# Run comprehensive tests
make test-comprehensive
```

## Testing

### Test Categories

| Test | Description | Command |
|------|-------------|---------|
| Infrastructure | Kubernetes cluster provisioning and CSI storage | `make test-infra` |
| Operator | Operator deployment with pgEdge images | `make test-operator` |
| Image Validation | Admission control blocks non-pgEdge images | `make test-image-validation` |
| Smoke | Quick upstream E2E test subset | `make test-smoke` |
| Comprehensive | Full upstream E2E test suite | `make test-comprehensive` |

### Version-Specific Tests

```bash
make test-cnpg-1.28.0       # Specific operator version
make test-pg-18             # Specific PostgreSQL version
make test-all-cnpg          # All operator versions
make test-all-postgres      # All PostgreSQL versions
```

### Image Validation Policy

All test clusters enforce pgEdge Enterprise Postgres image usage via Kubernetes `ValidatingAdmissionPolicy`:

**Allowed:**
- `ghcr.io/pgedge/pgedge-postgres:*`
- `ghcr.io/pgedge/pgedge-postgres-internal:*`

**Blocked:**
- `ghcr.io/cloudnative-pg/postgresql:*` (upstream CNPG images)
- `postgres:*` (Docker Hub images)
- Any other PostgreSQL image

This prevents accidental use of non-pgEdge images during testing.

## Configuration

Tests are configured via [`tests/config/versions.yaml`](tests/config/versions.yaml):

```yaml
cnpg_versions:
  - version: "1.28.0"
    chart_version: "0.27.0"
    git_tag: "v1.28.0"
    operator_image: "ghcr.io/pgedge/cloudnative-pg:1.28.0"
    postgres_versions: ["18", "17", "16"]
    providers:
      kind:
        kubernetes_versions: ["1.32", "1.33", "1.34"]

postgres_images:
  registries:
    public:
      base: "ghcr.io/pgedge/pgedge-postgres"
  variants:
    - name: "standard"
      tag_suffix: "-standard"
```

## License

This repository contains components under different licenses:

| Component | License | Location |
|-----------|---------|----------|
| pgEdge tests and tooling | [PostgreSQL License](LICENSE) | `tests/`, `.github/`, `Makefile` |
| CloudNativePG charts | [Apache License 2.0](charts/cloudnative-pg/v0.27.0/LICENSE) | `charts/` |
| CloudNativePG manifests | [Apache License 2.0](manifests/cloudnative-pg/v1.28.0/LICENSE) | `manifests/` |

See [NOTICE](NOTICE) for full attribution and trademark details.

This project is not affiliated with, endorsed by, or sponsored by CloudNativePG, the Cloud Native Computing Foundation, or the PostgreSQL Global Development Group.
