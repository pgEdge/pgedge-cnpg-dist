#!/usr/bin/env bash

# CNPG E2E Test Runner for pgEdge
# This script dynamically clones CNPG at specific versions and runs E2E tests
# with pgEdge operator and postgres images

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TEST_DIR="${PROJECT_ROOT}/tests"
CONFIG_DIR="${TEST_DIR}/config"

# Default configuration
CNPG_VERSION="${CNPG_VERSION:-1.28.0}"
POSTGRES_VERSION="${POSTGRES_VERSION:-16}"
POSTGRES_VARIANT="${POSTGRES_VARIANT:-standard}"
POSTGRES_IMAGE_REGISTRY="${POSTGRES_IMAGE_REGISTRY:-public}"
PROVIDER="${PROVIDER:-kind}"
K8S_VERSION="${K8S_VERSION:-}"
FEATURE_TYPE="${FEATURE_TYPE:-smoke}"
TEST_DEPTH="${TEST_DEPTH:-0}"
PARALLEL="${PARALLEL:-true}"
MAX_WORKERS="${MAX_WORKERS:-3}"
CLEANUP="${CLEANUP:-true}"
KEEP_CNPG_CLONE="${KEEP_CNPG_CLONE:-false}"
RESULTS_DIR="${RESULTS_DIR:-${PROJECT_ROOT}/test-results}"

# CSI driver component versions (matches CNPG's setup-cluster.sh)
# These versions are tested and known to work together
EXTERNAL_SNAPSHOTTER_VERSION="${EXTERNAL_SNAPSHOTTER_VERSION:-v8.4.0}"
EXTERNAL_PROVISIONER_VERSION="${EXTERNAL_PROVISIONER_VERSION:-v6.1.0}"
EXTERNAL_RESIZER_VERSION="${EXTERNAL_RESIZER_VERSION:-v2.0.0}"
EXTERNAL_ATTACHER_VERSION="${EXTERNAL_ATTACHER_VERSION:-v4.10.0}"
CSI_DRIVER_HOST_PATH_VERSION="${CSI_DRIVER_HOST_PATH_VERSION:-v1.17.0}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions - all output to stderr to avoid contaminating return values
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Print usage
usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Run CNPG E2E tests with pgEdge operator and postgres images.

OPTIONS:
    --cnpg-version VERSION        CNPG version to test (default: 1.28.0)
    --postgres-version VERSION    PostgreSQL version to test (default: 16)
    --postgres-variant VARIANT    PostgreSQL image variant: minimal|standard (default: standard)
    --postgres-registry REGISTRY  PostgreSQL image registry: public|internal (default: public)
                                 public: ghcr.io/pgedge/pgedge-postgres
                                 internal: ghcr.io/pgedge/pgedge-postgres-internal
    --provider PROVIDER           Provider: kind|aks|eks (default: kind)
    --k8s-version VERSION         Kubernetes version (auto-detected from config if not specified)
    --feature-type TYPES          Comma-separated test features (default: smoke)
                                 Available: smoke,basic,security,backup-restore,etc.
    --test-depth DEPTH            Test depth level 0-4 (default: 0)
    --parallel                    Run tests in parallel (default: true)
    --no-parallel                 Disable parallel execution
    --max-workers NUM             Max parallel workers (default: 3)
    --cleanup                     Cleanup cluster after tests (default: true)
    --no-cleanup                  Keep cluster after tests
    --keep-cnpg-clone             Keep CNPG repository clone after tests
    --results-dir DIR             Test results directory (default: ./test-results)
    --all-postgres-versions       Run tests for all supported PG versions (16,17,18)
    --all-variants                Run tests for both minimal and standard variants
    -h, --help                    Show this help message

EXAMPLES:
    # Run smoke tests for CNPG 1.28.0 with PG 16 standard
    $0 --cnpg-version 1.28.0 --postgres-version 16

    # Run basic tests with minimal variant
    $0 --feature-type basic --postgres-variant minimal

    # Test with internal pre-release images
    $0 --postgres-registry internal --postgres-version 18

    # Run all tests for all PG versions
    $0 --all-postgres-versions --feature-type smoke,basic

    # Run tests on specific K8s version
    $0 --k8s-version 1.33 --feature-type smoke

ENVIRONMENT VARIABLES:
    CNPG_VERSION, POSTGRES_VERSION, POSTGRES_VARIANT, POSTGRES_IMAGE_REGISTRY,
    PROVIDER, K8S_VERSION, FEATURE_TYPE, TEST_DEPTH, PARALLEL, MAX_WORKERS,
    CLEANUP, RESULTS_DIR

EOF
    exit 0
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --cnpg-version)
                CNPG_VERSION="$2"
                shift 2
                ;;
            --postgres-version)
                POSTGRES_VERSION="$2"
                shift 2
                ;;
            --postgres-variant)
                POSTGRES_VARIANT="$2"
                shift 2
                ;;
            --postgres-registry)
                POSTGRES_IMAGE_REGISTRY="$2"
                shift 2
                ;;
            --provider)
                PROVIDER="$2"
                shift 2
                ;;
            --k8s-version)
                K8S_VERSION="$2"
                shift 2
                ;;
            --feature-type)
                FEATURE_TYPE="$2"
                shift 2
                ;;
            --test-depth)
                TEST_DEPTH="$2"
                shift 2
                ;;
            --parallel)
                PARALLEL="true"
                shift
                ;;
            --no-parallel)
                PARALLEL="false"
                shift
                ;;
            --max-workers)
                MAX_WORKERS="$2"
                shift 2
                ;;
            --cleanup)
                CLEANUP="true"
                shift
                ;;
            --no-cleanup)
                CLEANUP="false"
                shift
                ;;
            --keep-cnpg-clone)
                KEEP_CNPG_CLONE="true"
                shift
                ;;
            --results-dir)
                RESULTS_DIR="$2"
                shift 2
                ;;
            --all-postgres-versions)
                ALL_POSTGRES_VERSIONS="true"
                shift
                ;;
            --all-variants)
                ALL_VARIANTS="true"
                shift
                ;;
            -h|--help)
                usage
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                ;;
        esac
    done
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    local missing=()

    # Check required tools
    command -v docker >/dev/null 2>&1 || missing+=("docker")
    command -v kind >/dev/null 2>&1 || missing+=("kind")
    command -v kubectl >/dev/null 2>&1 || missing+=("kubectl")
    command -v helm >/dev/null 2>&1 || missing+=("helm")
    command -v git >/dev/null 2>&1 || missing+=("git")
    command -v go >/dev/null 2>&1 || missing+=("go")
    command -v yq >/dev/null 2>&1 || missing+=("yq")

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        log_info "Please install missing tools and try again."
        exit 1
    fi

    log_success "All prerequisites satisfied"
}

# Load configuration from versions.yaml
load_config() {
    log_info "Loading configuration for CNPG ${CNPG_VERSION}..."

    local versions_file="${CONFIG_DIR}/versions.yaml"

    if [[ ! -f "${versions_file}" ]]; then
        log_error "Configuration file not found: ${versions_file}"
        exit 1
    fi

    # Extract CNPG configuration using yq
    CNPG_GIT_TAG=$(yq eval ".cnpg_versions[] | select(.version == \"${CNPG_VERSION}\") | .git_tag" "${versions_file}")
    OPERATOR_IMAGE=$(yq eval ".cnpg_versions[] | select(.version == \"${CNPG_VERSION}\") | .operator_image" "${versions_file}")

    if [[ -z "${CNPG_GIT_TAG}" || "${CNPG_GIT_TAG}" == "null" ]]; then
        log_error "CNPG version ${CNPG_VERSION} not found in configuration"
        exit 1
    fi

    # Get Kubernetes version if not specified
    if [[ -z "${K8S_VERSION}" ]]; then
        K8S_VERSION=$(yq eval ".cnpg_versions[] | select(.version == \"${CNPG_VERSION}\") | .providers.${PROVIDER}.kubernetes_versions[0]" "${versions_file}")
        log_info "Using Kubernetes version: ${K8S_VERSION}"
    fi

    # Get PostgreSQL image registry base
    local registry_base=$(yq eval ".postgres_images.registries.${POSTGRES_IMAGE_REGISTRY}.base" "${versions_file}")
    if [[ -z "${registry_base}" || "${registry_base}" == "null" ]]; then
        log_error "PostgreSQL image registry '${POSTGRES_IMAGE_REGISTRY}' not found in configuration"
        log_info "Available registries: public, internal"
        exit 1
    fi

    # Get Spock version
    local spock_version=$(yq eval ".postgres_images.spock_version" "${versions_file}")
    if [[ -z "${spock_version}" || "${spock_version}" == "null" ]]; then
        log_error "Spock version not found in configuration"
        exit 1
    fi

    # Get variant tag suffix
    local tag_suffix=$(yq eval ".postgres_images.variants[] | select(.name == \"${POSTGRES_VARIANT}\") | .tag_suffix" "${versions_file}")
    if [[ "${tag_suffix}" == "null" ]]; then
        tag_suffix=""
    fi

    # Construct PostgreSQL image name: {pg_version}-{spock_version}-{variant}
    # Example: 18-spock5-standard
    POSTGRES_IMAGE="${registry_base}:${POSTGRES_VERSION}-${spock_version}${tag_suffix}"

    log_success "Configuration loaded successfully"
    log_info "CNPG Git Tag: ${CNPG_GIT_TAG}"
    log_info "Operator Image: ${OPERATOR_IMAGE}"
    log_info "PostgreSQL Image: ${POSTGRES_IMAGE}"
    log_info "PostgreSQL Registry: ${POSTGRES_IMAGE_REGISTRY}"
    log_info "Kubernetes Version: ${K8S_VERSION}"
}

# Clone CNPG repository at specific version
clone_cnpg_repo() {
    local temp_dir="/tmp/cnpg-e2e-${CNPG_VERSION}-${POSTGRES_VERSION}-${POSTGRES_VARIANT}"

    log_info "Cloning CNPG repository at ${CNPG_GIT_TAG}..."

    if [[ -d "${temp_dir}" ]]; then
        log_warn "CNPG clone already exists at ${temp_dir}"
        if [[ "${KEEP_CNPG_CLONE}" != "true" ]]; then
            log_info "Removing existing clone..."
            rm -rf "${temp_dir}"
        else
            log_info "Reusing existing clone..."
            echo "${temp_dir}"
            return 0
        fi
    fi

    git clone --depth 1 --branch "${CNPG_GIT_TAG}" \
        https://github.com/cloudnative-pg/cloudnative-pg.git \
        "${temp_dir}" >/dev/null 2>&1

    log_success "CNPG repository cloned to ${temp_dir}"
    echo "${temp_dir}"
}

# Create kind cluster
create_kind_cluster() {
    local cluster_name="cnpg-e2e-${CNPG_VERSION}-pg${POSTGRES_VERSION}"

    log_info "Creating kind cluster: ${cluster_name}..."

    if kind get clusters | grep -q "^${cluster_name}$"; then
        log_warn "Cluster ${cluster_name} already exists"
        log_info "Deleting existing cluster..."
        kind delete cluster --name "${cluster_name}"
        sleep 5  # Wait for cleanup to complete
    fi

    # Create cluster with specific K8s version
    local kind_image="kindest/node:v${K8S_VERSION}.0"
    log_info "Creating cluster with image: ${kind_image}"

    # Try to create cluster with retries
    local max_retries=2
    local retry=0
    while [[ ${retry} -le ${max_retries} ]]; do
        if [[ ${retry} -gt 0 ]]; then
            log_warn "Retry ${retry}/${max_retries}: Attempting to create cluster again..."
            # Clean up any partial cluster
            kind delete cluster --name "${cluster_name}" 2>/dev/null || true
            sleep 10
        fi

        if kind create cluster \
            --name "${cluster_name}" \
            --config "${TEST_DIR}/kind/kind-config.yaml" \
            --image "${kind_image}" \
            --wait 300s; then
            break
        fi

        ((retry++))
        if [[ ${retry} -gt ${max_retries} ]]; then
            log_error "Failed to create cluster after ${max_retries} retries"
            log_info "Try freeing up system resources and running again"
            exit 1
        fi
    done

    # Install CSI driver for volume snapshots (required for CNPG tests)
    # Using pinned versions that match CNPG's tested configuration
    log_info "Installing CSI hostpath driver (versions: snapshotter=${EXTERNAL_SNAPSHOTTER_VERSION}, provisioner=${EXTERNAL_PROVISIONER_VERSION})..."

    local CSI_BASE_URL="https://raw.githubusercontent.com/kubernetes-csi"

    # Install external snapshotter CRDs and controller
    kubectl apply -f "${CSI_BASE_URL}/external-snapshotter/${EXTERNAL_SNAPSHOTTER_VERSION}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml" >&2
    kubectl apply -f "${CSI_BASE_URL}/external-snapshotter/${EXTERNAL_SNAPSHOTTER_VERSION}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml" >&2
    kubectl apply -f "${CSI_BASE_URL}/external-snapshotter/${EXTERNAL_SNAPSHOTTER_VERSION}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml" >&2
    kubectl apply -f "${CSI_BASE_URL}/external-snapshotter/${EXTERNAL_SNAPSHOTTER_VERSION}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml" >&2
    kubectl apply -f "${CSI_BASE_URL}/external-snapshotter/${EXTERNAL_SNAPSHOTTER_VERSION}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml" >&2
    kubectl apply -f "${CSI_BASE_URL}/external-snapshotter/${EXTERNAL_SNAPSHOTTER_VERSION}/deploy/kubernetes/csi-snapshotter/rbac-csi-snapshotter.yaml" >&2

    # Install external provisioner RBAC
    kubectl apply -f "${CSI_BASE_URL}/external-provisioner/${EXTERNAL_PROVISIONER_VERSION}/deploy/kubernetes/rbac.yaml" >&2

    # Install external attacher RBAC
    kubectl apply -f "${CSI_BASE_URL}/external-attacher/${EXTERNAL_ATTACHER_VERSION}/deploy/kubernetes/rbac.yaml" >&2

    # Install external resizer RBAC
    kubectl apply -f "${CSI_BASE_URL}/external-resizer/${EXTERNAL_RESIZER_VERSION}/deploy/kubernetes/rbac.yaml" >&2

    # Install CSI hostpath driver and plugin
    log_info "Installing CSI hostpath driver plugin (${CSI_DRIVER_HOST_PATH_VERSION})..."
    kubectl apply -f "${CSI_BASE_URL}/csi-driver-host-path/${CSI_DRIVER_HOST_PATH_VERSION}/deploy/kubernetes-1.30/hostpath/csi-hostpath-driverinfo.yaml" >&2
    kubectl apply -f "${CSI_BASE_URL}/csi-driver-host-path/${CSI_DRIVER_HOST_PATH_VERSION}/deploy/kubernetes-1.30/hostpath/csi-hostpath-plugin.yaml" >&2

    # Create storage class
    kubectl apply -f - <<EOF >&2
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-hostpath-sc
provisioner: hostpath.csi.k8s.io
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
EOF

    # Create volume snapshot class
    kubectl apply -f "${CSI_BASE_URL}/csi-driver-host-path/${CSI_DRIVER_HOST_PATH_VERSION}/deploy/kubernetes-1.30/hostpath/csi-hostpath-snapshotclass.yaml" >&2

    # Patch VolumeSnapshotClass to prevent snapshot failures with running PostgreSQL instances
    # This matches CNPG's configuration for handling online backups
    kubectl patch volumesnapshotclass csi-hostpath-snapclass -p '{"parameters":{"ignoreFailedRead":"true"}}' --type merge >&2 2>&1 || true

    # Wait for CSI driver to be ready
    log_info "Waiting for CSI driver to be ready..."
    kubectl wait --for=condition=ready pod -l app=csi-hostpath-plugin -n default --timeout=300s >&2 2>&1 || true

    log_success "Kind cluster created: ${cluster_name}"
    # Return only the cluster name to stdout
    echo "${cluster_name}"
}

# Deploy pgEdge CNPG operator using Helm chart
deploy_pgedge_operator() {
    local cluster_name="$1"

    log_info "Deploying pgEdge CNPG operator from Helm chart..."

    # Set kubeconfig context
    kubectl config use-context "kind-${cluster_name}"

    # Path to pgEdge CNPG chart
    local chart_path="${PROJECT_ROOT}/charts/cloudnative-pg/v${CNPG_VERSION}"

    if [[ ! -d "${chart_path}" ]]; then
        log_error "Chart not found: ${chart_path}"
        log_info "Available charts:"
        ls -la "${PROJECT_ROOT}/charts/cloudnative-pg/" || true
        exit 1
    fi

    log_info "Using chart: ${chart_path}"
    log_info "Operator image: ${OPERATOR_IMAGE}"
    log_info "Default PostgreSQL image: ${POSTGRES_IMAGE}"

    # Install operator using Helm
    # Set POSTGRES_IMAGE_NAME env var to override operator's default PostgreSQL image
    helm install cnpg-operator "${chart_path}" \
        --create-namespace \
        --namespace cnpg-system \
        --set image.repository="$(echo ${OPERATOR_IMAGE} | cut -d: -f1)" \
        --set image.tag="$(echo ${OPERATOR_IMAGE} | cut -d: -f2)" \
        --set "additionalEnv[0].name=POSTGRES_IMAGE_NAME" \
        --set "additionalEnv[0].value=${POSTGRES_IMAGE}" \
        --wait \
        --timeout 5m

    # Wait for operator to be ready
    log_info "Waiting for operator deployment to be ready..."
    kubectl wait --for=condition=available \
        --timeout=300s \
        -n cnpg-system \
        deployment/cnpg-operator-cloudnative-pg || \
        kubectl wait --for=condition=available \
        --timeout=300s \
        -n cnpg-system \
        deployment -l app.kubernetes.io/name=cloudnative-pg

    # Verify operator is running
    log_info "Verifying operator status..."
    kubectl get pods -n cnpg-system
    kubectl get deployment -n cnpg-system

    log_success "pgEdge CNPG operator deployed successfully"

    # Verify operator is configured with correct default PostgreSQL image
    verify_operator_default_image "${cluster_name}"
}

# Verify operator has correct default PostgreSQL image configured
verify_operator_default_image() {
    local cluster_name="$1"

    log_info "Verifying operator default PostgreSQL image configuration..."

    # Get the POSTGRES_IMAGE_NAME env var from operator deployment
    local operator_default_image=$(kubectl get deployment -n cnpg-system cnpg-operator-cloudnative-pg -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="POSTGRES_IMAGE_NAME")].value}' 2>/dev/null)

    if [[ -z "${operator_default_image}" ]]; then
        log_error "POSTGRES_IMAGE_NAME environment variable not set on operator"
        log_error "Operator will use hardcoded upstream image instead of pgEdge image"
        exit 1
    fi

    if [[ "${operator_default_image}" != "${POSTGRES_IMAGE}" ]]; then
        log_error "Operator default image mismatch!"
        log_error "Expected: ${POSTGRES_IMAGE}"
        log_error "Got:      ${operator_default_image}"
        exit 1
    fi

    log_success "Operator configured with correct default image: ${operator_default_image}"
}

# Run E2E tests
run_tests() {
    local cnpg_dir="$1"
    local cluster_name="$2"

    log_info "Running E2E tests..."
    log_info "Feature Type: ${FEATURE_TYPE}"
    log_info "Test Depth: ${TEST_DEPTH}"

    cd "${cnpg_dir}"

    # Set environment variables for CNPG tests
    export POSTGRES_IMG="${POSTGRES_IMAGE}"
    export CONTROLLER_IMG="${OPERATOR_IMAGE}"

    # Override default PostgreSQL image repository for tests that hardcode upstream images
    # This affects major upgrade tests that use OfficialStandardImageName/OfficialMinimalImageName
    local registry_base=$(echo "${POSTGRES_IMAGE}" | cut -d: -f1)
    export POSTGRES_IMG_REPOSITORY="${registry_base}"

    export E2E_DEFAULT_STORAGE_CLASS="csi-hostpath-sc"
    export E2E_CSI_STORAGE_CLASS="csi-hostpath-sc"
    export E2E_DEFAULT_VOLUMESNAPSHOT_CLASS="csi-hostpath-snapclass"
    export FEATURE_TYPE="${FEATURE_TYPE}"
    export TEST_DEPTH="${TEST_DEPTH}"
    export KUBECONFIG="${HOME}/.kube/config"

    # Tell CNPG E2E tests that operator is already installed
    # and which namespace it's in
    export E2E_PRE_ROLLING_UPDATE="false"
    export OPERATOR_NAMESPACE="cnpg-system"

    # Create results directory
    mkdir -p "${RESULTS_DIR}"
    local result_file="${RESULTS_DIR}/e2e-cnpg-${CNPG_VERSION}-pg${POSTGRES_VERSION}-${POSTGRES_VARIANT}.json"
    local junit_file="${RESULTS_DIR}/e2e-cnpg-${CNPG_VERSION}-pg${POSTGRES_VERSION}-${POSTGRES_VARIANT}.xml"

    log_info "Test results will be saved to:"
    log_info "  JSON: ${result_file}"
    log_info "  JUnit: ${junit_file}"

    # Run tests directly from tests/e2e directory
    cd "${cnpg_dir}/tests/e2e"

    # Run ginkgo tests with label filter
    set +e
    local ginkgo_args=(
        "--label-filter=${FEATURE_TYPE}"
        "-p"  # Run specs in parallel
        "--procs=4"  # Use 4 parallel processes
        "--show-node-events"  # Show node entry/exit events
        "--poll-progress-after=1200s"  # Show progress if test is quiet for 20 minutes
        "--timeout=3h"  # Overall test timeout
        "--json-report=${result_file}"
        "--junit-report=${junit_file}"
    )

    log_info "Running ginkgo tests in parallel (4 processes)..."
    log_info "Note: TEST_DEPTH=${TEST_DEPTH} is set as an environment variable (not a ginkgo flag)"
    go run github.com/onsi/ginkgo/v2/ginkgo "${ginkgo_args[@]}" . 2>&1 | tee "${RESULTS_DIR}/test-output.log"
    local exit_code=$?
    set -e

    if [[ ${exit_code} -eq 0 ]]; then
        log_success "Tests passed!"
    else
        log_error "Tests failed with exit code ${exit_code}"
    fi

    cd "${PROJECT_ROOT}"
    return ${exit_code}
}

# Cleanup resources
cleanup() {
    local cluster_name="$1"
    local cnpg_dir="$2"

    if [[ "${CLEANUP}" == "true" ]]; then
        log_info "Cleaning up resources..."

        if kind get clusters | grep -q "^${cluster_name}$"; then
            log_info "Deleting kind cluster: ${cluster_name}"
            kind delete cluster --name "${cluster_name}"
        fi
    else
        log_info "Skipping cleanup (--no-cleanup specified)"
        log_info "Cluster ${cluster_name} is still running"
        log_info "To delete manually: kind delete cluster --name ${cluster_name}"
    fi

    if [[ "${KEEP_CNPG_CLONE}" != "true" && -d "${cnpg_dir}" ]]; then
        log_info "Removing CNPG repository clone..."
        rm -rf "${cnpg_dir}"
    elif [[ "${KEEP_CNPG_CLONE}" == "true" ]]; then
        log_info "Keeping CNPG clone at: ${cnpg_dir}"
    fi
}

# Run single test configuration
run_single_test() {
    local pg_version="$1"
    local pg_variant="$2"

    log_info "=========================================="
    log_info "Running test configuration:"
    log_info "  CNPG: ${CNPG_VERSION}"
    log_info "  PostgreSQL: ${pg_version} (${pg_variant})"
    log_info "  Provider: ${PROVIDER}"
    log_info "=========================================="

    POSTGRES_VERSION="${pg_version}"
    POSTGRES_VARIANT="${pg_variant}"

    load_config
    local cnpg_dir=$(clone_cnpg_repo)
    local cluster_name=$(create_kind_cluster)

    # Deploy pgEdge operator from our Helm chart
    deploy_pgedge_operator "${cluster_name}"

    local exit_code=0
    run_tests "${cnpg_dir}" "${cluster_name}" || exit_code=$?

    cleanup "${cluster_name}" "${cnpg_dir}"

    return ${exit_code}
}

# Main function
main() {
    parse_args "$@"

    log_info "CNPG E2E Test Runner for pgEdge"
    log_info "================================"

    check_prerequisites

    # Handle --all-postgres-versions flag
    if [[ "${ALL_POSTGRES_VERSIONS:-false}" == "true" ]]; then
        log_info "Running tests for all PostgreSQL versions: 16, 17, 18"

        local pg_versions=(16 17 18)
        local variants=("${POSTGRES_VARIANT}")

        if [[ "${ALL_VARIANTS:-false}" == "true" ]]; then
            variants=(minimal standard)
        fi

        local failed=0

        if [[ "${PARALLEL}" == "true" ]]; then
            log_info "Running tests in parallel (max workers: ${MAX_WORKERS})"

            local pids=()
            local active=0

            for pg_ver in "${pg_versions[@]}"; do
                for variant in "${variants[@]}"; do
                    # Wait if we've hit max workers
                    while [[ ${active} -ge ${MAX_WORKERS} ]]; do
                        for pid in "${pids[@]}"; do
                            if ! kill -0 "${pid}" 2>/dev/null; then
                                wait "${pid}" || ((failed++))
                                ((active--))
                            fi
                        done
                        sleep 1
                    done

                    # Start test in background
                    run_single_test "${pg_ver}" "${variant}" &
                    pids+=($!)
                    ((active++))
                done
            done

            # Wait for remaining tests
            for pid in "${pids[@]}"; do
                wait "${pid}" || ((failed++))
            done
        else
            log_info "Running tests sequentially"

            for pg_ver in "${pg_versions[@]}"; do
                for variant in "${variants[@]}"; do
                    run_single_test "${pg_ver}" "${variant}" || ((failed++))
                done
            done
        fi

        log_info "=========================================="
        if [[ ${failed} -eq 0 ]]; then
            log_success "All tests passed!"
            exit 0
        else
            log_error "${failed} test configuration(s) failed"
            exit 1
        fi
    else
        # Single test run
        run_single_test "${POSTGRES_VERSION}" "${POSTGRES_VARIANT}"
    fi
}

# Run main function
main "$@"
