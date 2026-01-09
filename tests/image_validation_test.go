package tests

import (
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/pgedge/cnpg-build/tests/config"
	"github.com/pgedge/cnpg-build/tests/helpers"
	"github.com/stretchr/testify/require"
)

// TestImageValidationPolicy verifies that non-pgEdge PostgreSQL images are blocked
func TestImageValidationPolicy(t *testing.T) {
	t.Parallel()

	// Load configuration
	cfg, err := config.LoadConfig()
	require.NoError(t, err, "Failed to load configuration")

	// Create Kind cluster (this will install the policy)
	cluster := helpers.CreateKindCluster(t,
		"cnpg-image-validation-test",
		cfg.KindDefaults.Image,
		cfg.KindDefaults.Nodes,
	)

	cnpgVersion := cfg.CNPGVersions[0]
	postgresVersion := cnpgVersion.PostgresVersions[0]

	// Get pgEdge PostgreSQL image
	pgEdgeImage := cfg.GetPostgresImageName(
		cfg.PostgresImages.DefaultRegistry,
		postgresVersion,
		"standard",
	)

	// Deploy CNPG operator
	helpers.DeployCNPGOperator(t,
		cluster.KubeConfigPath,
		cnpgVersion.Version,
		"cnpg-system",
		cnpgVersion.GetOperatorImageName(),
		pgEdgeImage,
	)

	opts := cluster.GetKubectlOptions("default")

	t.Run("Allow pgEdge public registry image", func(t *testing.T) {
		// This should succeed - pgEdge public image
		validCluster := `
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: valid-pgedge-cluster
spec:
  instances: 1
  imageName: ghcr.io/pgedge/pgedge-postgres:17-spock5-standard
  storage:
    size: 1Gi
`
		err := k8s.KubectlApplyFromStringE(t, opts, validCluster)
		require.NoError(t, err, "pgEdge public registry image should be allowed")

		// Cleanup
		_ = k8s.RunKubectlE(t, opts, "delete", "cluster", "valid-pgedge-cluster", "--ignore-not-found=true")
	})

	t.Run("Allow pgEdge internal registry image", func(t *testing.T) {
		// This should succeed - pgEdge internal image
		validInternalCluster := `
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: valid-pgedge-internal-cluster
spec:
  instances: 1
  imageName: ghcr.io/pgedge/pgedge-postgres-internal:17-spock5-standard
  storage:
    size: 1Gi
`
		err := k8s.KubectlApplyFromStringE(t, opts, validInternalCluster)
		require.NoError(t, err, "pgEdge internal registry image should be allowed")

		// Cleanup
		_ = k8s.RunKubectlE(t, opts, "delete", "cluster", "valid-pgedge-internal-cluster", "--ignore-not-found=true")
	})

	t.Run("Block upstream CNPG image", func(t *testing.T) {
		// This should fail - upstream CNPG image
		invalidCluster := `
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: invalid-upstream-cluster
spec:
  instances: 1
  imageName: ghcr.io/cloudnative-pg/postgresql:17
  storage:
    size: 1Gi
`
		err := k8s.KubectlApplyFromStringE(t, opts, invalidCluster)
		require.Error(t, err, "Upstream CNPG image should be blocked")
		require.Contains(t, err.Error(), "must use pgEdge PostgreSQL images",
			"Error message should indicate pgEdge images are required")
	})

	t.Run("Block Docker Hub PostgreSQL image", func(t *testing.T) {
		// This should fail - Docker Hub postgres image
		invalidDockerCluster := `
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: invalid-docker-cluster
spec:
  instances: 1
  imageName: postgres:17
  storage:
    size: 1Gi
`
		err := k8s.KubectlApplyFromStringE(t, opts, invalidDockerCluster)
		require.Error(t, err, "Docker Hub postgres image should be blocked")
		require.Contains(t, err.Error(), "must use pgEdge PostgreSQL images",
			"Error message should indicate pgEdge images are required")
	})

	t.Run("Allow cluster without explicit imageName", func(t *testing.T) {
		// This should succeed - no imageName means it will use operator's default
		// (which we configured to be pgEdge)
		defaultImageCluster := `
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: default-image-cluster
spec:
  instances: 1
  storage:
    size: 1Gi
`
		err := k8s.KubectlApplyFromStringE(t, opts, defaultImageCluster)
		require.NoError(t, err, "Cluster without explicit imageName should be allowed (uses operator default)")

		// Cleanup
		_ = k8s.RunKubectlE(t, opts, "delete", "cluster", "default-image-cluster", "--ignore-not-found=true")
	})
}
