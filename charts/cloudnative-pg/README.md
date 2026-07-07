# CloudNativePG Helm Charts

> 📚 **Quick Reference**: This repository contains CloudNativePG Helm charts organized by version. Chart versions are **not synchronized 1:1** with operator versions.

## Overview

This repository provides CloudNativePG Helm charts with each version stored in its own directory (e.g., `v0.26.0/`, `v0.27.0/`). The operator and chart use **different version numbering schemes**.

## Key Features

* 🔢 **Independent Versioning** - Operator and chart follow separate semantic versioning
* 📊 **Predictable Pattern** - Chart version typically one minor behind operator
* 🔄 **Patch Alignment** - Patch versions stay synchronized
* 📦 **Flexible Updates** - Chart can update independently for configuration changes

## Version Mapping

| Operator Version | Helm Chart Version |
|-----------------|-------------------|
| 1.27.0          | 0.26.0           |
| 1.27.1          | 0.26.1           |
| 1.28.0          | 0.27.0           |
| 1.28.1          | 0.27.1           |
| 1.29.0          | 0.28.0           |
| 1.29.1          | 0.28.1, 0.28.2, 0.28.3 |
| 1.30.0          | 0.29.0           |

## Understanding the Pattern

### Version Components

* **Operator versions** start at `1.x.x`
* **Chart versions** start at `0.x.x`
* The chart version is typically **one minor version behind** the operator
  + Example: operator `1.27.x` uses chart `0.26.x`
  + Example: operator `1.28.x` uses chart `0.27.x`

### Patch Version Alignment

**Patch versions stay synchronized:**

* Operator `1.27.1` → Chart `0.26.1`
* Operator `1.27.2` → Chart `0.26.2`

This ensures bug fixes and minor updates are tracked consistently.

## Installation Guide

### Repository Structure

Charts are stored in version-specific directories:

```
cloudnative-pg/
├── v0.26.0/    # Chart for operator 1.27.0
├── v0.26.1/    # Chart for operator 1.27.1
├── v0.27.0/    # Chart for operator 1.28.0
├── v0.27.1/    # Chart for operator 1.28.1
├── v0.28.0/    # Chart for operator 1.29.0
├── v0.28.1/    # Chart for operator 1.29.1
├── v0.28.2/    # Chart for operator 1.29.1
├── v0.28.3/    # Chart for operator 1.29.1
└── v0.29.0/    # Chart for operator 1.30.0
```

### Installing from Local Path

**Step 1:** Clone or download this repository

**Step 2:** Navigate to the repository directory

**Step 3:** Install using Helm with the local chart path:

```bash
# Install from local chart directory
helm install cloudnative-pg \
  --namespace cnpg-system \
  --create-namespace \
  ./v0.29.0
```

### Upgrading

```bash
# Upgrade to a specific chart version
helm upgrade cloudnative-pg \
  --namespace cnpg-system \
  ./v0.29.0
```

## Why Different Versions?

The chart and operator are versioned independently because:

* ✅ **Operator versioning** - Follows semantic versioning for the software itself
* ✅ **Chart versioning** - Independent release cycle for packaging and deployment configuration
* ✅ **Flexibility** - Allows chart updates (configuration changes, fixes) without operator changes
* ✅ **Clarity** - Separate version numbers indicate separate concerns

This separation enables:

* Chart bug fixes without rebuilding the operator
* Configuration improvements independent of operator features
* Better tracking of packaging vs. software changes

## Best Practices

### ✅ Always Verify Versions

> **Important:** When upgrading, **always verify** the correct chart version for your desired operator version using the version mapping table in this README.

### Check Release Notes

Before any installation or upgrade:

1. Read the operator release notes
2. Identify the corresponding chart version
3. Review any breaking changes or migration steps
4. Test in a non-production environment first

### Version Compatibility

```bash
# Check current operator version
kubectl get deployment -n cnpg-system \
  cloudnative-pg -o jsonpath='{.spec.template.spec.containers[0].image}'

# Check current chart version
helm list -n cnpg-system
```

## Troubleshooting

**Chart version not found?**

* Verify the version directory exists in this repository (e.g., `v0.26.0/`)
* Check the version mapping table above for available versions

**Operator version mismatch?**

* Verify the chart version matches your desired operator version
* Review the version mapping table in this README

**Upgrade issues?**

* Always backup your cluster before upgrading
* Test upgrades in a non-production environment

