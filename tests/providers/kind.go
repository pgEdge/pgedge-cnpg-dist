package providers

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
	"github.com/pgedge/pgedge-cnpg-dist/tests/config"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"
)

// kindCluster represents a Kind cluster configuration and lifecycle
type kindCluster struct {
	Name           string
	KubeConfigPath string
	Provider       *cluster.Provider
	Config         *kindConfig
}

// kindConfig represents Kind cluster configuration
type kindConfig struct {
	Name          string
	Image         string
	Nodes         int
	ServiceSubnet string
	PodSubnet     string
	ConfigPath    string
}

// newKindCluster creates a new Kind cluster
func newKindCluster(t *testing.T, config *kindConfig) *kindCluster {
	if t != nil {
		t.Helper()
	}

	// Create Kind provider
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(cmd.NewLogger()),
	)

	kc := &kindCluster{
		Name:     config.Name,
		Provider: provider,
		Config:   config,
	}

	// Set kubeconfig path
	kc.KubeConfigPath = filepath.Join(os.TempDir(), fmt.Sprintf("%s.kubeconfig", config.Name))

	return kc
}

// Create provisions a new Kind cluster
func (kc *kindCluster) Create(t *testing.T) error {
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
		// Build Kind cluster configuration with multiple nodes
		kindConfig := &v1alpha4.Cluster{
			Networking: v1alpha4.Networking{
				ServiceSubnet: kc.Config.ServiceSubnet,
				PodSubnet:     kc.Config.PodSubnet,
			},
		}

		// Add control plane node
		kindConfig.Nodes = append(kindConfig.Nodes, v1alpha4.Node{
			Role:  v1alpha4.ControlPlaneRole,
			Image: kc.Config.Image,
		})

		// Add worker nodes (NodeCount - 1 since we already have control plane)
		for i := 1; i < kc.Config.Nodes; i++ {
			kindConfig.Nodes = append(kindConfig.Nodes, v1alpha4.Node{
				Role:  v1alpha4.WorkerRole,
				Image: kc.Config.Image,
			})
		}

		// Create cluster with retry logic
		createErr := kc.Provider.Create(
			kc.Name,
			cluster.CreateWithV1Alpha4Config(kindConfig),
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
func (kc *kindCluster) Delete(t *testing.T) error {
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
func (kc *kindCluster) GetKubectlOptions(namespace string) *k8s.KubectlOptions {
	return k8s.NewKubectlOptions("", kc.KubeConfigPath, namespace)
}

// waitForClusterReady waits for the cluster to be fully ready
func (kc *kindCluster) waitForClusterReady(t *testing.T, timeout time.Duration) error {
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
func (kc *kindCluster) InstallCSIDriver(t *testing.T) error {
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
func (kc *kindCluster) InstallImageValidationPolicy(t *testing.T) error {
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

// Kind implements the Provider interface for Kind clusters
type Kind struct {
	cluster *kindCluster
	config  *Config
}

// NewKind creates a new Kind provider
func NewKind(config *Config) *Kind {
	// Determine Kind node image based on K8s version
	kindImage := fmt.Sprintf("kindest/node:v%s.0", config.KubernetesVersion)
	if config.KubernetesVersion == "" {
		kindImage = "kindest/node:v1.32.0" // Default
	}

	kindConfig := &kindConfig{
		Name:          config.Name,
		Image:         kindImage,
		Nodes:         config.NodeCount,
		ServiceSubnet: "10.21.0.0/16",
		PodSubnet:     "10.20.0.0/16",
	}

	return &Kind{
		cluster: newKindCluster(nil, kindConfig),
		config:  config,
	}
}

// Name returns the provider name
func (p *Kind) Name() string {
	return "kind"
}

// Create provisions the Kind cluster
func (p *Kind) Create(t *testing.T) error {
	t.Helper()
	return p.cluster.Create(t)
}

// Delete destroys the Kind cluster
func (p *Kind) Delete(t *testing.T) error {
	t.Helper()
	return p.cluster.Delete(t)
}

// GetKubeConfigPath returns the path to the kubeconfig file
func (p *Kind) GetKubeConfigPath() string {
	return p.cluster.KubeConfigPath
}

// GetKubectlOptions returns kubectl options for the cluster
func (p *Kind) GetKubectlOptions(namespace string) *k8s.KubectlOptions {
	return p.cluster.GetKubectlOptions(namespace)
}

// InstallCSIDriver installs the CSI hostpath driver for Kind
func (p *Kind) InstallCSIDriver(t *testing.T) error {
	t.Helper()
	return p.cluster.InstallCSIDriver(t)
}

// InstallImageValidationPolicy installs the pgEdge image validation policy
func (p *Kind) InstallImageValidationPolicy(t *testing.T) error {
	t.Helper()
	return p.cluster.InstallImageValidationPolicy(t)
}

// IsReady checks if the cluster is ready
func (p *Kind) IsReady(t *testing.T) bool {
	t.Helper()

	opts := p.GetKubectlOptions("")
	_, err := k8s.GetNodesE(t, opts)
	return err == nil
}

// GetClusterName returns the cluster name
func (p *Kind) GetClusterName() string {
	return p.cluster.Name
}
