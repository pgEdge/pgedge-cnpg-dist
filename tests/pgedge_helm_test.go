package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pgedge/pgedge-cnpg-dist/tests/config"
	"github.com/pgedge/pgedge-cnpg-dist/tests/helpers"
	"github.com/pgedge/pgedge-cnpg-dist/tests/providers"
	"github.com/stretchr/testify/require"
)

// TestPgedgeHelm sets up a Kubernetes cluster with CNPG and cert-manager, then runs the full
// pgedge-helm integration test suite against it.
//
// Usage:
//
//	go test -v -run TestPgedgeHelm -timeout 3h ./tests/
//
// Environment variables:
//
//	PGEDGE_HELM_PATH    - Path to the pgedge-helm repository (default: clones from GitHub at release tag)
//	PGEDGE_HELM_VERSION - Chart version to test (default: inferred from Helm index)
//	CNPG_VERSION        - CNPG version to deploy (default: first in versions.yaml)
//	PROVIDER            - Cluster provider: "kind" (default) or "eks"
func TestPgedgeHelm(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping pgEdge Helm test in short mode")
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	require.NoError(t, err, "Failed to load configuration")

	// Get CNPG version
	cnpgVersion, err := cfg.GetCNPGVersionFromEnv()
	require.NoError(t, err, "Failed to get CNPG version")

	// Resolve pgedge chart version (env var → config override → latest stable from Helm index)
	chartVersion := helpers.GetLatestPgedgeHelmVersion(t, cfg.PgedgeHelm)

	// Clone pgedge-helm at the matching release tag to run the integration test suite
	pgedgeHelmPath := helpers.GetPgedgeHelmPath(t, chartVersion)
	t.Logf("Using pgedge-helm repo at: %s (version: %s)", pgedgeHelmPath, chartVersion)

	// Step 1: Create Kind cluster with CSI driver and image validation policy
	clusterName := fmt.Sprintf("pgedge-helm-%s", strings.ReplaceAll(cnpgVersion.Version, ".", "-"))
	provider := providers.NewProvider(t, clusterName)
	providers.Setup(t, provider)

	kubeconfigPath := provider.GetKubeConfigPath()

	// Step 2: Install cert-manager
	helpers.InstallCertManager(t, kubeconfigPath, cfg.PgedgeHelm.CertManagerManifest)

	// Step 3: Deploy CNPG operator
	postgresVersion := cnpgVersion.GetPostgresVersionFromEnv()
	postgresImage := cfg.GetPostgresImageName(
		cfg.PostgresImages.DefaultRegistry,
		postgresVersion,
		"standard",
	)

	// "manifest" creates deployment "cnpg-controller-manager", which pgedge-helm's
	// checkPrerequisites() expects. Change to "helm" if pgedge-helm switches to Helm-based install.
	helpers.DeployCNPGOperatorWithMethod(t,
		kubeconfigPath,
		cnpgVersion.Version,
		cnpgVersion.ChartVersion,
		"cnpg-system",
		cnpgVersion.GetOperatorImageName(),
		postgresImage,
		"manifest",
	)

	// Step 4: Run the full pgedge-helm integration test suite.
	// The tests install the chart from the published Helm repository, so no local
	// image build is needed.
	kubeContext := provider.GetKubeContext()
	helpers.RunPgedgeHelmIntegrationTests(t, kubeconfigPath, kubeContext, pgedgeHelmPath, postgresImage,
		cfg.PgedgeHelm.ChartRepo, "pgedge/pgedge", chartVersion, 2*time.Hour)

	t.Log("TestPgedgeHelm completed successfully - all pgedge-helm integration tests passed")
}
