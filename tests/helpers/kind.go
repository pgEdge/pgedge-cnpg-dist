package helpers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/pgedge/cnpg-build/tests/config"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"
)

// KindCluster represents a Kind cluster configuration and lifecycle
type KindCluster struct {
	Name           string
	KubeConfigPath string
	Provider       *cluster.Provider
	Config         *KindConfig
}

// KindConfig represents Kind cluster configuration
type KindConfig struct {
	Name          string
	Image         string
	Nodes         int
	ServiceSubnet string
	PodSubnet     string
	ConfigPath    string
}

// NewKindCluster creates a new Kind cluster
func NewKindCluster(t *testing.T, config *KindConfig) *KindCluster {
	t.Helper()

	// Create Kind provider
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(cmd.NewLogger()),
	)

	kc := &KindCluster{
		Name:     config.Name,
		Provider: provider,
		Config:   config,
	}

	// Set kubeconfig path
	kc.KubeConfigPath = filepath.Join(os.TempDir(), fmt.Sprintf("%s.kubeconfig", config.Name))

	return kc
}

// Create provisions a new Kind cluster
func (kc *KindCluster) Create(t *testing.T) error {
	t.Helper()

	t.Logf("Creating Kind cluster: %s", kc.Name)

	// Check if cluster already exists
	clusters, err := kc.Provider.List()
	if err != nil {
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	for _, c := range clusters {
		if c == kc.Name {
			t.Logf("Kind cluster %s already exists, deleting first", kc.Name)
			if err := kc.Delete(t); err != nil {
				return fmt.Errorf("failed to delete existing cluster: %w", err)
			}
			break
		}
	}

	// Retry cluster creation with backoff
	maxRetries := 3
	timeBetweenRetries := 10 * time.Second

	_, err = retry.DoWithRetryE(t, "Create Kind cluster", maxRetries, timeBetweenRetries, func() (string, error) {
		// Create cluster with retry logic
		createErr := kc.Provider.Create(
			kc.Name,
			cluster.CreateWithNodeImage(kc.Config.Image),
			cluster.CreateWithKubeconfigPath(kc.KubeConfigPath),
			cluster.CreateWithDisplayUsage(false),
			cluster.CreateWithDisplaySalutation(false),
		)

		if createErr != nil {
			return "", fmt.Errorf("failed to create cluster: %w", createErr)
		}

		// Wait for cluster to be ready
		waitErr := kc.waitForClusterReady(t, 5*time.Minute)
		if waitErr != nil {
			// Cleanup failed cluster
			_ = kc.Delete(t)
			return "", fmt.Errorf("cluster creation succeeded but not ready: %w", waitErr)
		}

		return "Cluster created successfully", nil
	})

	if err != nil {
		return fmt.Errorf("failed to create Kind cluster after %d retries: %w", maxRetries, err)
	}

	t.Logf("Kind cluster %s created successfully", kc.Name)
	return nil
}

// Delete removes the Kind cluster
func (kc *KindCluster) Delete(t *testing.T) error {
	t.Helper()

	t.Logf("Deleting Kind cluster: %s", kc.Name)

	err := kc.Provider.Delete(kc.Name, kc.KubeConfigPath)
	if err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}

	// Remove kubeconfig file
	if err := os.Remove(kc.KubeConfigPath); err != nil && !os.IsNotExist(err) {
		t.Logf("Warning: failed to remove kubeconfig: %v", err)
	}

	t.Logf("Kind cluster %s deleted successfully", kc.Name)
	return nil
}

// GetKubectlOptions returns kubectl options for this cluster
func (kc *KindCluster) GetKubectlOptions(namespace string) *k8s.KubectlOptions {
	return k8s.NewKubectlOptions("", kc.KubeConfigPath, namespace)
}

// waitForClusterReady waits for the cluster to be fully ready
func (kc *KindCluster) waitForClusterReady(t *testing.T, timeout time.Duration) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	opts := kc.GetKubectlOptions("")

	// Wait for all nodes to be ready
	maxRetries := int(timeout.Seconds() / 5)
	_, err := retry.DoWithRetryE(t, "Wait for nodes ready", maxRetries, 5*time.Second, func() (string, error) {
		nodes, getErr := k8s.GetNodesE(t, opts)
		if getErr != nil {
			return "", fmt.Errorf("failed to get nodes: %w", getErr)
		}

		if len(nodes) == 0 {
			return "", fmt.Errorf("no nodes found")
		}

		for _, node := range nodes {
			if !k8s.IsNodeReady(node) {
				return "", fmt.Errorf("node %s not ready", node.Name)
			}
		}

		return "All nodes ready", nil
	})

	if err != nil {
		return err
	}

	// Wait for system pods by checking with kubectl
	for i := 0; i < maxRetries; i++ {
		output, podErr := k8s.RunKubectlAndGetOutputE(t, opts, "get", "pods", "-n", "kube-system", "-o", "jsonpath={.items[*].metadata.name}")
		if podErr == nil && output != "" {
			break
		}
		if i < maxRetries-1 {
			time.Sleep(5 * time.Second)
		}
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for cluster ready")
	default:
		return nil
	}
}

// InstallCSIDriver installs the CSI hostpath driver for storage support
func (kc *KindCluster) InstallCSIDriver(t *testing.T) error {
	t.Helper()

	t.Log("Installing CSI hostpath driver")

	opts := kc.GetKubectlOptions("")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Extract K8s version from Kind node image (e.g., "kindest/node:v1.32.0" -> "1.32")
	k8sVersion := extractK8sVersion(kc.Config.Image)
	t.Logf("Using K8s version %s", k8sVersion)

	// Get manifests for this K8s version
	manifests := cfg.GetManifests(k8sVersion)
	if len(manifests) == 0 {
		return fmt.Errorf("no manifests found for K8s version %s", k8sVersion)
	}

	// Apply each manifest
	for _, m := range manifests {
		t.Logf("Applying %s", m.Name)
		err = k8s.RunKubectlE(t, opts, "apply", "-f", m.URL)
		if err != nil {
			return fmt.Errorf("failed to apply %s: %w", m.Name, err)
		}
	}

	// Create storage class
	t.Log("Creating storage class")
	storageClass := `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-hostpath-sc
provisioner: hostpath.csi.k8s.io
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
`
	err = k8s.KubectlApplyFromStringE(t, opts, storageClass)
	if err != nil {
		return fmt.Errorf("failed to create storage class: %w", err)
	}

	// Create volume snapshot class
	t.Log("Creating volume snapshot class")
	snapshotClass := `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: csi-hostpath-snapclass
driver: hostpath.csi.k8s.io
deletionPolicy: Delete
parameters:
  ignoreFailedRead: "true"
`
	err = k8s.KubectlApplyFromStringE(t, opts, snapshotClass)
	if err != nil {
		return fmt.Errorf("failed to create snapshot class: %w", err)
	}

	// Wait for CSI driver to be ready
	t.Log("Waiting for CSI driver pods to be ready")
	for i := 0; i < 60; i++ {
		output, podErr := k8s.RunKubectlAndGetOutputE(t, opts, "get", "pods", "-n", "default", "-l", "app.kubernetes.io/name=csi-hostpathplugin", "-o", "jsonpath={.items[*].metadata.name}")
		if podErr == nil && output != "" {
			t.Logf("CSI driver pods found: %s", output)
			break
		}
		if i < 59 {
			time.Sleep(5 * time.Second)
		} else {
			return fmt.Errorf("CSI driver pods not created after 5 minutes")
		}
	}

	t.Log("CSI hostpath driver installed successfully")
	return nil
}

// InstallImageValidationPolicy installs the ValidatingAdmissionPolicy to block non-pgEdge images
func (kc *KindCluster) InstallImageValidationPolicy(t *testing.T) error {
	t.Helper()

	t.Log("Installing image validation policy to block non-pgEdge PostgreSQL images")

	opts := kc.GetKubectlOptions("")

	// Find the manifests directory
	// Look for project root by finding go.mod
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

	// Check if policy file exists
	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		return fmt.Errorf("image validation policy not found at %s", policyPath)
	}

	// Apply the policy
	err = k8s.RunKubectlE(t, opts, "apply", "-f", policyPath)
	if err != nil {
		return fmt.Errorf("failed to apply image validation policy: %w", err)
	}

	t.Log("Image validation policy installed - only pgEdge PostgreSQL images will be allowed")
	return nil
}

// CreateKindCluster is a convenience function to create and setup a complete Kind cluster
func CreateKindCluster(t *testing.T, name string, image string, nodes int) *KindCluster {
	t.Helper()

	config := &KindConfig{
		Name:          name,
		Image:         image,
		Nodes:         nodes,
		ServiceSubnet: "10.21.0.0/16",
		PodSubnet:     "10.20.0.0/16",
	}

	kc := NewKindCluster(t, config)

	// Create cluster
	err := kc.Create(t)
	require.NoError(t, err, "Failed to create Kind cluster")

	// Install CSI driver
	err = kc.InstallCSIDriver(t)
	require.NoError(t, err, "Failed to install CSI driver")

	// Install image validation policy to block non-pgEdge images
	err = kc.InstallImageValidationPolicy(t)
	require.NoError(t, err, "Failed to install image validation policy")

	// Register cleanup
	t.Cleanup(func() {
		if err := kc.Delete(t); err != nil {
			t.Logf("Warning: failed to cleanup cluster: %v", err)
		}
	})

	return kc
}

// extractK8sVersion extracts the major.minor version from a Kind node image name
// Examples: "kindest/node:v1.32.0" -> "1.32", "kindest/node:v1.33" -> "1.33"
func extractK8sVersion(image string) string {
	re := regexp.MustCompile(`v?(\d+\.\d+)`)
	matches := re.FindStringSubmatch(image)
	if len(matches) >= 2 {
		return matches[1]
	}
	// Fallback to a default version if parsing fails
	return "1.32"
}
