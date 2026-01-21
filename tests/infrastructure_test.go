package tests

import (
	"testing"

	"github.com/pgedge/cnpg-build/tests/config"
	"github.com/pgedge/cnpg-build/tests/helpers"
	"github.com/pgedge/cnpg-build/tests/providers"
	"github.com/stretchr/testify/require"
)

// TestInfra validates that we can create a Kind cluster with CSI support
func TestInfra(t *testing.T) {
	t.Parallel()

	// Load configuration
	cfg, err := config.LoadConfig()
	require.NoError(t, err, "Failed to load configuration")

	// Create cluster using provider from environment
	provider := providers.CreateFromEnv(t, "cnpg-infra-test")
	providers.Setup(t, provider)

	// Get expected node count from environment or use default
	expectedNodes := providers.GetNodeCount()

	// Verify cluster is functional
	t.Run("Verify cluster has correct number of nodes", func(t *testing.T) {
		opts := provider.GetKubectlOptions("")
		nodes, err := helpers.GetNodes(t, opts)
		require.NoError(t, err)
		require.Len(t, nodes, expectedNodes, "Expected %d nodes", expectedNodes)
	})

	t.Run("Verify CSI storage class exists", func(t *testing.T) {
		opts := provider.GetKubectlOptions("")
		storageClasses, err := helpers.GetStorageClasses(t, opts)
		require.NoError(t, err)

		found := false
		for _, sc := range storageClasses {
			if sc == cfg.KindDefaults.Storage.CSIClass {
				found = true
				break
			}
		}
		require.True(t, found, "CSI storage class %s not found", cfg.KindDefaults.Storage.CSIClass)
	})

	t.Run("Verify volume snapshot class exists", func(t *testing.T) {
		opts := provider.GetKubectlOptions("")
		snapshotClasses, err := helpers.GetVolumeSnapshotClasses(t, opts)
		require.NoError(t, err)

		found := false
		for _, vsc := range snapshotClasses {
			if vsc == cfg.KindDefaults.Storage.SnapshotClass {
				found = true
				break
			}
		}
		require.True(t, found, "Volume snapshot class %s not found", cfg.KindDefaults.Storage.SnapshotClass)
	})
}

// TestOperator validates CNPG operator can be deployed using the static manifest
func TestOperator(t *testing.T) {
	t.Parallel()

	// Load configuration
	cfg, err := config.LoadConfig()
	require.NoError(t, err, "Failed to load configuration")

	// Get CNPG version from environment or use default
	cnpgVersion, err := cfg.GetCNPGVersionFromEnv()
	require.NoError(t, err, "Failed to get CNPG version")

	// Create cluster using provider from environment
	provider := providers.CreateFromEnv(t, "cnpg-operator-test")
	providers.Setup(t, provider)

	// Deploy CNPG operator from manifest (no Helm, tests the static YAML)
	operator := helpers.DeployCNPGOperatorFromManifest(t,
		provider.GetKubeConfigPath(),
		cnpgVersion.Version,
		"cnpg-system",
	)

	t.Run("Verify operator is running", func(t *testing.T) {
		err := helpers.GetDeployment(t, operator.KubectlOptions, operator.ReleaseName)
		require.NoError(t, err, "Operator deployment should exist and be accessible")
	})

	t.Run("Verify operator CRDs are installed", func(t *testing.T) {
		crds := []string{
			"backups.postgresql.cnpg.io",
			"clusterimagecatalogs.postgresql.cnpg.io",
			"clusters.postgresql.cnpg.io",
			"databases.postgresql.cnpg.io",
			"imagecatalogs.postgresql.cnpg.io",
			"poolers.postgresql.cnpg.io",
			"publications.postgresql.cnpg.io",
			"scheduledbackups.postgresql.cnpg.io",
			"subscriptions.postgresql.cnpg.io",
		}

		opts := provider.GetKubectlOptions("")
		for _, crd := range crds {
			exists, err := helpers.CRDExists(t, opts, crd)
			require.NoError(t, err)
			require.True(t, exists, "CRD %s should exist", crd)
		}
	})
}
