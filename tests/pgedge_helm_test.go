package tests

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pgedge/pgedge-cnpg-dist/tests/config"
	"github.com/pgedge/pgedge-cnpg-dist/tests/helpers"
	"github.com/pgedge/pgedge-cnpg-dist/tests/providers"
	"github.com/stretchr/testify/require"
)

// TestPgedgeHelm deploys the pgEdge Helm chart on a Kind cluster and runs the Helm test suite.
// It validates the full deployment flow: CNPG operator + cert-manager + pgEdge chart + Spock replication.
//
// Usage:
//
//	go test -v -run TestPgedgeHelm -timeout 1h ./tests/
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
	provider := providers.CreateFromEnv(t, clusterName)
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

	helpers.DeployCNPGOperator(t,
		kubeconfigPath,
		cnpgVersion.Version,
		cnpgVersion.ChartVersion,
		"cnpg-system",
		cnpgVersion.GetOperatorImageName(),
		postgresImage,
	)

	// Step 4: Install pgEdge Helm chart
	// Uses the published ghcr.io/pgedge/pgedge-helm-utils image (no local build needed).
	// helm install --timeout 30m blocks until the post-install init-spock hook completes,
	// which itself waits for all CNPG clusters to become healthy before initializing Spock.
	valuesFile := filepath.Join(pgedgeHelmPath, cfg.PgedgeHelm.DefaultValues)
	namespace := "default"

	helpers.DeployPgedgeChart(t, kubeconfigPath, pgedgeHelmPath, valuesFile, namespace, "")

	// Step 5: Run helm test (validates Spock replication across all nodes)
	helpers.RunHelmTest(t, kubeconfigPath, "pgedge", namespace, 5*time.Minute)

	t.Log("TestPgedgeHelm completed successfully - pgEdge chart deployed and Spock replication verified")
}
