package providers

import (
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
)

// Provider represents a Kubernetes cluster provider (Kind, EKS, AKS, GKE, etc.)
type Provider interface {
	// Name returns the provider name (e.g., "kind", "eks", "aks", "gke")
	Name() string

	// Create provisions the Kubernetes cluster
	Create(t *testing.T) error

	// Delete destroys the Kubernetes cluster
	Delete(t *testing.T) error

	// GetKubeConfigPath returns the path to the kubeconfig file
	GetKubeConfigPath() string

	// GetKubectlOptions returns kubectl options for the cluster
	GetKubectlOptions(namespace string) *k8s.KubectlOptions

	// InstallCSIDriver installs CSI storage driver (implementation varies by provider)
	InstallCSIDriver(t *testing.T) error

	// InstallImageValidationPolicy installs the pgEdge image validation policy
	InstallImageValidationPolicy(t *testing.T) error

	// IsReady checks if the cluster is ready for use
	IsReady(t *testing.T) bool

	// GetClusterName returns the cluster name
	GetClusterName() string

	// Exists checks if the cluster already exists
	Exists(t *testing.T) bool

	// Connect connects to an existing cluster (updates kubeconfig without creating)
	Connect(t *testing.T) error
}

// Config represents common configuration for all providers
type Config struct {
	Name              string // Cluster name
	KubernetesVersion string // K8s version (e.g., "1.32")
	NodeCount         int    // Number of nodes
	Region            string // Cloud region (for cloud providers)
}

// Create creates a provider based on the provider type
func Create(t *testing.T, providerType string, config *Config) Provider {
	t.Helper()

	switch providerType {
	case "kind":
		return NewKind(config)
	case "eks":
		return NewEKS(config)
	case "aks":
		// TODO: Implement AKS provider
		t.Fatalf("AKS provider not yet implemented")
		return nil
	case "gke":
		// TODO: Implement GKE provider
		t.Fatalf("GKE provider not yet implemented")
		return nil
	default:
		t.Fatalf("Unknown provider type: %s", providerType)
		return nil
	}
}

// Setup provisions a cluster with all required components
// Respects CLUSTER_REUSE and CLUSTER_CLEANUP environment variables:
//   - CLUSTER_REUSE=true: Reuse existing cluster if found
//   - CLUSTER_CLEANUP=false: Skip cluster deletion after tests
func Setup(t *testing.T, provider Provider) {
	t.Helper()

	reuse := GetClusterReuse()
	cleanup := GetClusterCleanup()

	// Check if we should reuse existing cluster
	if reuse && provider.Exists(t) {
		t.Logf("Reusing existing cluster: %s", provider.GetClusterName())
		err := provider.Connect(t)
		if err != nil {
			t.Fatalf("Failed to connect to existing cluster: %v", err)
		}
	} else {
		// Create cluster
		err := provider.Create(t)
		if err != nil {
			t.Fatalf("Failed to create cluster: %v", err)
		}

		// Install CSI driver
		err = provider.InstallCSIDriver(t)
		if err != nil {
			t.Fatalf("Failed to install CSI driver: %v", err)
		}

		// Install image validation policy
		err = provider.InstallImageValidationPolicy(t)
		if err != nil {
			t.Fatalf("Failed to install image validation policy: %v", err)
		}
	}

	// Register cleanup (unless disabled)
	if cleanup {
		t.Cleanup(func() {
			if err := provider.Delete(t); err != nil {
				t.Logf("Warning: failed to cleanup cluster: %v", err)
			}
		})
	} else {
		t.Logf("Cluster cleanup disabled (CLUSTER_CLEANUP=false), cluster %s will remain running", provider.GetClusterName())
	}
}
