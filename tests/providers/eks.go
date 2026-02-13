package providers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/gruntwork-io/terratest/modules/terraform"
)

// EKS implements the Provider interface for AWS EKS clusters
type EKS struct {
	config         *Config
	terraformOpts  *terraform.Options
	kubeConfigPath string
	clusterName    string
}

// NewEKS creates a new EKS provider
func NewEKS(config *Config) *EKS {
	// Find terraform directory
	terraformDir := findTerraformDir()

	// Generate unique cluster name
	clusterName := config.Name
	if len(clusterName) > 40 {
		clusterName = clusterName[:40]
	}

	// Default region if not specified
	region := config.Region
	if region == "" {
		region = GetAWSRegion()
	}

	// Map K8s version to EKS supported version format
	k8sVersion := config.KubernetesVersion
	if k8sVersion == "" {
		k8sVersion = "1.31"
	}

	terraformOpts := &terraform.Options{
		TerraformDir: terraformDir,
		Vars: map[string]interface{}{
			"cluster_name":       clusterName,
			"kubernetes_version": k8sVersion,
			"region":             region,
			"node_count":         config.NodeCount,
			"use_spot_instances": GetUseSpotInstances(),
			"node_instance_type": GetEKSNodeType(),
			"tags": map[string]string{
				"Environment": "e2e-test",
				"ManagedBy":   "terratest",
			},
		},
		NoColor: true,
	}

	return &EKS{
		config:        config,
		terraformOpts: terraformOpts,
		clusterName:   clusterName,
	}
}

// Name returns the provider name
func (e *EKS) Name() string {
	return "eks"
}

// Create provisions the EKS cluster using Terraform
func (e *EKS) Create(t *testing.T) error {
	t.Helper()

	t.Logf("Creating EKS cluster: %s in region %s", e.clusterName, e.config.Region)

	// Initialize and apply Terraform
	_, err := terraform.InitAndApplyE(t, e.terraformOpts)
	if err != nil {
		return fmt.Errorf("failed to create EKS cluster: %w", err)
	}

	// Get kubeconfig
	err = e.updateKubeconfig(t)
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Wait for cluster to be ready
	err = e.waitForClusterReady(t, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("cluster not ready: %w", err)
	}

	t.Logf("EKS cluster %s created successfully", e.clusterName)
	return nil
}

// Delete destroys the EKS cluster using Terraform
func (e *EKS) Delete(t *testing.T) error {
	t.Helper()

	t.Logf("Deleting EKS cluster: %s", e.clusterName)

	_, err := terraform.DestroyE(t, e.terraformOpts)
	if err != nil {
		return fmt.Errorf("failed to destroy EKS cluster: %w", err)
	}

	// Remove kubeconfig file
	if e.kubeConfigPath != "" {
		os.Remove(e.kubeConfigPath)
	}

	t.Logf("EKS cluster %s deleted successfully", e.clusterName)
	return nil
}

// GetKubeConfigPath returns the path to the kubeconfig file
func (e *EKS) GetKubeConfigPath() string {
	return e.kubeConfigPath
}

// GetKubectlOptions returns kubectl options for the cluster
func (e *EKS) GetKubectlOptions(namespace string) *k8s.KubectlOptions {
	return k8s.NewKubectlOptions("", e.kubeConfigPath, namespace)
}

// InstallCSIDriver configures EBS CSI storage classes for the cluster
// Note: The EBS CSI driver addon is installed via Terraform, this creates the storage classes
func (e *EKS) InstallCSIDriver(t *testing.T) error {
	t.Helper()

	t.Log("Configuring EBS CSI storage classes")

	opts := e.GetKubectlOptions("")

	// Wait for EBS CSI driver to be ready
	t.Log("Waiting for EBS CSI driver to be ready")
	_, err := retry.DoWithRetryE(t, "Wait for EBS CSI driver", 30, 10*time.Second, func() (string, error) {
		output, runErr := k8s.RunKubectlAndGetOutputE(t, opts, "get", "pods", "-n", "kube-system", "-l", "app.kubernetes.io/name=aws-ebs-csi-driver", "-o", "jsonpath={.items[*].status.phase}")
		if runErr != nil {
			return "", runErr
		}
		if output == "" {
			return "", fmt.Errorf("EBS CSI driver pods not found")
		}
		return output, nil
	})
	if err != nil {
		return fmt.Errorf("EBS CSI driver not ready: %w", err)
	}

	// Create gp3 StorageClass with name matching test expectations
	storageClass := `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-hostpath-sc
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  encrypted: "true"
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
`
	err = k8s.KubectlApplyFromStringE(t, opts, storageClass)
	if err != nil {
		return fmt.Errorf("failed to create storage class: %w", err)
	}

	// Install snapshot CRDs and controller
	snapshotCRDs := []string{
		"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.0.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml",
		"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.0.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml",
		"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.0.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml",
		"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.0.0/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml",
		"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.0.0/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml",
	}

	for _, url := range snapshotCRDs {
		err = k8s.RunKubectlE(t, opts, "apply", "-f", url)
		if err != nil {
			return fmt.Errorf("failed to apply snapshot CRD %s: %w", url, err)
		}
	}

	// Create VolumeSnapshotClass with name matching test expectations
	snapshotClass := `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: csi-hostpath-snapclass
driver: ebs.csi.aws.com
deletionPolicy: Delete
`
	err = k8s.KubectlApplyFromStringE(t, opts, snapshotClass)
	if err != nil {
		return fmt.Errorf("failed to create snapshot class: %w", err)
	}

	t.Log("EBS CSI storage configuration complete")
	return nil
}

// InstallImageValidationPolicy installs the pgEdge image validation policy
func (e *EKS) InstallImageValidationPolicy(t *testing.T) error {
	t.Helper()

	t.Log("Installing image validation policy to block non-pgEdge PostgreSQL images")

	projectRoot, err := findProjectRoot()
	if err != nil {
		return err
	}

	policyPath := filepath.Join(projectRoot, "tests", "manifests", "image-validation-policy.yaml")

	// Check if policy file exists
	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		return fmt.Errorf("image validation policy not found at %s", policyPath)
	}

	opts := e.GetKubectlOptions("")
	err = k8s.RunKubectlE(t, opts, "apply", "-f", policyPath)
	if err != nil {
		return fmt.Errorf("failed to apply image validation policy: %w", err)
	}

	t.Log("Image validation policy installed - only pgEdge PostgreSQL images will be allowed")
	return nil
}

// IsReady checks if the cluster is ready
func (e *EKS) IsReady(t *testing.T) bool {
	t.Helper()

	opts := e.GetKubectlOptions("")
	_, err := k8s.GetNodesE(t, opts)
	return err == nil
}

// GetClusterName returns the cluster name
func (e *EKS) GetClusterName() string {
	return e.clusterName
}

// updateKubeconfig runs aws eks update-kubeconfig
func (e *EKS) updateKubeconfig(t *testing.T) error {
	t.Helper()

	// Create kubeconfig path
	e.kubeConfigPath = filepath.Join(os.TempDir(), fmt.Sprintf("%s.kubeconfig", e.clusterName))

	region := terraform.Output(t, e.terraformOpts, "region")
	clusterName := terraform.Output(t, e.terraformOpts, "cluster_name")

	cmd := exec.Command("aws", "eks", "update-kubeconfig",
		"--region", region,
		"--name", clusterName,
		"--kubeconfig", e.kubeConfigPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w, output: %s", err, string(output))
	}

	t.Logf("Kubeconfig written to %s", e.kubeConfigPath)
	return nil
}

// waitForClusterReady waits for all nodes to be ready
func (e *EKS) waitForClusterReady(t *testing.T, timeout time.Duration) error {
	t.Helper()

	maxRetries := int(timeout.Seconds() / 10)
	opts := e.GetKubectlOptions("")

	_, err := retry.DoWithRetryE(t, "Wait for EKS nodes ready", maxRetries, 10*time.Second, func() (string, error) {
		nodes, getErr := k8s.GetNodesE(t, opts)
		if getErr != nil {
			return "", fmt.Errorf("failed to get nodes: %w", getErr)
		}

		if len(nodes) < e.config.NodeCount {
			return "", fmt.Errorf("expected %d nodes, got %d", e.config.NodeCount, len(nodes))
		}

		for _, node := range nodes {
			if !k8s.IsNodeReady(node) {
				return "", fmt.Errorf("node %s not ready", node.Name)
			}
		}

		return "All nodes ready", nil
	})

	return err
}

// findTerraformDir locates the terraform/eks directory
func findTerraformDir() string {
	projectRoot, _ := findProjectRoot()
	return filepath.Join(projectRoot, "tests", "terraform", "eks")
}

// findProjectRoot finds the project root by locating go.mod
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("project root not found")
		}
		dir = parent
	}
}

// Exists checks if the EKS cluster already exists
func (e *EKS) Exists(t *testing.T) bool {
	t.Helper()

	// Check if terraform state exists and has resources
	tfStateFile := filepath.Join(e.terraformOpts.TerraformDir, "terraform.tfstate")
	if _, err := os.Stat(tfStateFile); os.IsNotExist(err) {
		return false
	}

	// Also verify via AWS CLI that cluster actually exists
	region := e.config.Region
	if region == "" {
		region = GetAWSRegion()
	}

	cmd := exec.Command("aws", "eks", "describe-cluster",
		"--region", region,
		"--name", e.clusterName,
	)
	err := cmd.Run()
	return err == nil
}

// Connect connects to an existing EKS cluster
func (e *EKS) Connect(t *testing.T) error {
	t.Helper()

	t.Logf("Connecting to existing EKS cluster: %s", e.clusterName)

	// Initialize terraform to read state
	terraform.Init(t, e.terraformOpts)

	// Update kubeconfig
	err := e.updateKubeconfig(t)
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w", err)
	}

	// Verify cluster is accessible
	if !e.IsReady(t) {
		return fmt.Errorf("cluster exists but is not ready")
	}

	t.Logf("Connected to EKS cluster %s", e.clusterName)
	return nil
}
