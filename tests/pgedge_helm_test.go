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

// TestPgedgeHelm sets up a Kind cluster with CNPG and cert-manager, then runs the full
// pgedge-helm integration test suite against it.
//
// Usage:
//
//	go test -v -run TestPgedgeHelm -timeout 3h ./tests/
//
// Environment variables:
//
//	PGEDGE_HELM_PATH   - Path to the pgedge-helm repository (default: clones from GitHub)
//	PGEDGE_HELM_BRANCH - Branch to clone (default: main)
//	CNPG_VERSION       - CNPG version to deploy (default: first in versions.yaml)
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

	// Locate pgedge-helm repository (clones from GitHub if not provided)
	pgedgeHelmPath := helpers.GetPgedgeHelmPath(t)
	t.Logf("Using pgedge-helm repo at: %s", pgedgeHelmPath)

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

	// Step 4: Build pgedge-helm-utils:dev and load it into the Kind cluster.
	// The integration tests require the locally-built image — the published image
	// may be outdated and will silently skip Spock initialization.
	helpers.BuildAndLoadImage(t, pgedgeHelmPath, clusterName)

	// Step 5: Run the full pgedge-helm integration test suite.
	// Each integration test manages its own helm install/uninstall cycle against
	// the cluster we just provisioned (CNPG + cert-manager already installed above).
	kubeContext := fmt.Sprintf("kind-%s", clusterName)
	helpers.RunPgedgeHelmIntegrationTests(t, kubeconfigPath, kubeContext, pgedgeHelmPath, postgresImage, "pgedge-helm-utils:dev", 2*time.Hour)

	t.Log("TestPgedgeHelm completed successfully - all pgedge-helm integration tests passed")
}
