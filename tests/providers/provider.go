package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
)

// installImageValidationPolicy is shared across providers: finds the project root,
// locates the policy YAML, and applies it via kubectl.
func installImageValidationPolicy(t *testing.T, opts *k8s.KubectlOptions) error {
	t.Helper()

	t.Log("Installing image validation policy to block non-pgEdge PostgreSQL images")

	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			return fmt.Errorf("could not find project root (go.mod not found)")
		}
		projectRoot = parent
	}

	policyPath := filepath.Join(projectRoot, "tests", "manifests", "image-validation-policy.yaml")

	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		return fmt.Errorf("image validation policy not found at %s", policyPath)
	}

	if err := k8s.RunKubectlE(t, opts, "apply", "-f", policyPath); err != nil {
		return fmt.Errorf("failed to apply image validation policy: %w", err)
	}

	t.Log("Image validation policy installed - only pgEdge PostgreSQL images will be allowed")
	return nil
}

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
}

// Config represents common configuration for all providers
type Config struct {
	Name              string // Cluster name
	KubernetesVersion string // K8s version (e.g., "1.32")
	NodeCount         int    // Number of nodes
	Region            string // Cloud region (for cloud providers)
	InstanceType      string // Instance type (for cloud providers, e.g., "m5.large", "m7g.large")
	NodeArch          string // Node architecture: "amd64" or "arm64"
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
func Setup(t *testing.T, provider Provider) {
	t.Helper()

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

	// Register cleanup
	t.Cleanup(func() {
		if err := provider.Delete(t); err != nil {
			t.Logf("Warning: failed to cleanup cluster: %v", err)
		}
	})
}
