package tests

import (
	"testing"

	"github.com/pgedge/cnpg-build/tests/config"
	"github.com/pgedge/cnpg-build/tests/helpers"
	"github.com/pgedge/cnpg-build/tests/providers"
	"github.com/stretchr/testify/require"
)

// TestKindClusterProvisioning validates that we can create a Kind cluster with CSI support
func TestKindClusterProvisioning(t *testing.T) {
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

// TestCNPGOperatorDeployment validates CNPG operator can be deployed
func TestCNPGOperatorDeployment(t *testing.T) {
	t.Parallel()

	// Load configuration
	cfg, err := config.LoadConfig()
	require.NoError(t, err, "Failed to load configuration")

	// Use first CNPG version from config
	cnpgVersion := cfg.CNPGVersions[0]

	// Create cluster using provider from environment
	provider := providers.CreateFromEnv(t, "cnpg-operator-test")
	providers.Setup(t, provider)

	// Get PostgreSQL image name
	postgresImage := cfg.GetPostgresImageName(
		cfg.PostgresImages.DefaultRegistry,
		cnpgVersion.PostgresVersions[0],
		"standard",
	)

	// Deploy CNPG operator
	operator := helpers.DeployCNPGOperator(t,
		provider.GetKubeConfigPath(),
		cnpgVersion.Version,
		"cnpg-system",
		cnpgVersion.GetOperatorImageName(),
		postgresImage,
	)

	t.Run("Verify operator is running", func(t *testing.T) {
		err := helpers.GetDeployment(t, operator.KubectlOptions, operator.ReleaseName)
		require.NoError(t, err, "Operator deployment should exist and be accessible")
	})

	t.Run("Verify operator CRDs are installed", func(t *testing.T) {
		crds := []string{
			"clusters.postgresql.cnpg.io",
			"backups.postgresql.cnpg.io",
			"scheduledbackups.postgresql.cnpg.io",
			"poolers.postgresql.cnpg.io",
		}

		opts := provider.GetKubectlOptions("")
		for _, crd := range crds {
			exists, err := helpers.CRDExists(t, opts, crd)
			require.NoError(t, err)
			require.True(t, exists, "CRD %s should exist", crd)
		}
	})
}

// TestMultiVersionCNPG validates we can test multiple CNPG versions
func TestMultiVersionCNPG(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multi-version test in short mode")
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	require.NoError(t, err, "Failed to load configuration")

	// Test each CNPG version
	for _, cnpgVersion := range cfg.CNPGVersions {
		cnpgVersion := cnpgVersion // capture range variable

		t.Run("CNPG-"+cnpgVersion.Version, func(t *testing.T) {
			t.Parallel()

			// Create cluster using provider from environment
			provider := providers.CreateFromEnv(t, "cnpg-"+cnpgVersion.Version)
			providers.Setup(t, provider)

			// Get PostgreSQL image
			postgresImage := cfg.GetPostgresImageName(
				cfg.PostgresImages.DefaultRegistry,
				cnpgVersion.PostgresVersions[0],
				"standard",
			)

			// Deploy operator
			operator := helpers.DeployCNPGOperator(t,
				provider.GetKubeConfigPath(),
				cnpgVersion.Version,
				"cnpg-system",
				cnpgVersion.GetOperatorImageName(),
				postgresImage,
			)

			// Verify deployment
			err := helpers.GetDeployment(t, operator.KubectlOptions, operator.ReleaseName)
			require.NoError(t, err, "Operator deployment should exist")
		})
	}
}
