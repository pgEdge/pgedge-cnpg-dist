package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/gruntwork-io/terratest/modules/terraform"
)

// Provider interfce for AWS EKS
type EKS struct {
	config         *Config
	kubeConfigPath string
	tfOptions      *terraform.Options
}

// NewEKS initializes detial/configuration required to create EKS cluster using Terraform
func NewEKS(config *Config) *EKS {
	if config.Region == "" {
		config.Region = "us-east-1"
	}
	if config.NodeCount == 0 {
		config.NodeCount = 3
	}

	// Kubectl configuration path, the file will be written after cluster creation
	kubeConfigPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s.kubeconfig", config.Name))
	fmt.Printf("EKS provider will use kubeconfig path: %s\n", kubeConfigPath)

	// eks relate terraform information
	tfDir := findTerraformDir("eks")

	tfOptions := terraform.WithDefaultRetryableErrors(nil, &terraform.Options{
		TerraformDir: tfDir,
		Vars: map[string]interface{}{
			"cluster_name":       config.Name,
			"region":             config.Region,
			"kubernetes_version": config.KubernetesVersion,
			"node_count":         config.NodeCount,
			"instance_type":      config.InstanceType,
			"node_arch":          config.NodeArch,
		},
		NoColor: true,
	})

	return &EKS{
		config:         config,
		kubeConfigPath: kubeConfigPath,
		tfOptions:      tfOptions,
	}
}

// findTerraformDir locates the terraform/<provider> directory relative to the project root
func findTerraformDir(provider string) string {
	dir, err := os.Getwd()
	if err != nil {
		return filepath.Join("terraform", provider)
	}

	for {
		candidate := filepath.Join(dir, "terraform", provider)
		if _, err := os.Stat(filepath.Join(candidate, "main.tf")); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Join("terraform", provider)
		}
		dir = parent
	}
}

// Name returns the provider name
func (e *EKS) Name() string {
	return "eks"
}

// Create provisions an EKS cluster using Terraform via Terratest
func (e *EKS) Create(t *testing.T) error {
	t.Helper()

	t.Logf("Creating EKS cluster: %s in region %s (via Terraform)", e.config.Name, e.config.Region)

	// Initialize and apply Terraform
	_, err := terraform.InitAndApplyE(t, e.tfOptions)
	if err != nil {
		return fmt.Errorf("terraform apply failed: %w", err)
	}

	// Extract kubeconfig from Terraform output and write to file
	kubeconfig := terraform.Output(t, e.tfOptions, "kubeconfig")
	if err := os.WriteFile(e.kubeConfigPath, []byte(kubeconfig), 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	// Wait for cluster to be ready
	if err := e.waitForClusterReady(t, 10*time.Minute); err != nil {
		return fmt.Errorf("cluster created but not ready: %w", err)
	}

	t.Logf("EKS cluster %s created successfully", e.config.Name)
	return nil
}

// Delete destroys the EKS cluster using Terraform via Terratest
func (e *EKS) Delete(t *testing.T) error {
	t.Helper()

	t.Logf("Deleting EKS cluster: %s (via Terraform destroy)", e.config.Name)

	_, err := terraform.DestroyE(t, e.tfOptions)
	if err != nil {
		return fmt.Errorf("terraform destroy failed: %w", err)
	}

	// Remove kubeconfig file
	if err := os.Remove(e.kubeConfigPath); err != nil && !os.IsNotExist(err) {
		t.Logf("Warning: failed to remove kubeconfig: %v", err)
	}

	t.Logf("EKS cluster %s deleted successfully", e.config.Name)
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

// InstallCSIDriver verifies the EBS CSI driver (already installed via Terraform addon)
// and creates the storage class and volume snapshot class
func (e *EKS) InstallCSIDriver(t *testing.T) error {
	t.Helper()

	t.Log("Verifying AWS EBS CSI driver (installed via Terraform)")

	opts := e.GetKubectlOptions("")

	// Wait for the EBS CSI driver pods to be ready (addon installed by Terraform)
	t.Log("Waiting for EBS CSI driver pods to be ready")
	for i := 0; i < 60; i++ {
		output, podErr := k8s.RunKubectlAndGetOutputE(t, opts, "get", "pods", "-n", "kube-system", "-l", "app.kubernetes.io/name=aws-ebs-csi-driver", "-o", "jsonpath={.items[*].status.phase}")
		if podErr == nil && output != "" && !strings.Contains(output, "Pending") {
			t.Logf("EBS CSI driver pods are running")
			break
		}
		if i < 59 {
			time.Sleep(5 * time.Second)
		} else {
			return fmt.Errorf("EBS CSI driver pods not ready after 5 minutes")
		}
	}

	// Install VolumeSnapshot CRDs (not included by default on EKS)
	t.Log("Installing VolumeSnapshot CRDs")
	snapshotCRDBaseURL := "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.2.0/client/config/crd"
	snapshotCRDs := []string{
		"snapshot.storage.k8s.io_volumesnapshotclasses.yaml",
		"snapshot.storage.k8s.io_volumesnapshotcontents.yaml",
		"snapshot.storage.k8s.io_volumesnapshots.yaml",
	}
	for _, crd := range snapshotCRDs {
		crdURL := fmt.Sprintf("%s/%s", snapshotCRDBaseURL, crd)
		if err := k8s.RunKubectlE(t, opts, "apply", "-f", crdURL); err != nil {
			return fmt.Errorf("failed to install snapshot CRD %s: %w", crd, err)
		}
	}

	// Create a gp3 storage class
	t.Log("Creating gp3 storage class")
	storageClass := `
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ebs-gp3
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
`
	if err := k8s.KubectlApplyFromStringE(t, opts, storageClass); err != nil {
		return fmt.Errorf("failed to create gp3 storage class: %w", err)
	}

	// Create volume snapshot class
	t.Log("Creating volume snapshot class")
	snapshotClass := `
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: ebs-snapshot-class
driver: ebs.csi.aws.com
deletionPolicy: Delete
`
	if err := k8s.KubectlApplyFromStringE(t, opts, snapshotClass); err != nil {
		return fmt.Errorf("failed to create volume snapshot class: %w", err)
	}

	t.Log("AWS EBS CSI driver verified and storage classes created successfully")
	return nil
}

// InstallImageValidationPolicy installs the ValidatingAdmissionPolicy to block non-pgEdge images
func (e *EKS) InstallImageValidationPolicy(t *testing.T) error {
	t.Helper()

	t.Log("Installing image validation policy to block non-pgEdge PostgreSQL images")

	opts := e.GetKubectlOptions("")

	// Find the manifests directory
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

// IsReady checks if the cluster is ready for use
func (e *EKS) IsReady(t *testing.T) bool {
	t.Helper()

	opts := e.GetKubectlOptions("")
	_, err := k8s.GetNodesE(t, opts)
	return err == nil
}

// GetClusterName returns the cluster name
func (e *EKS) GetClusterName() string {
	return e.config.Name
}

// waitForClusterReady waits for the EKS cluster to be fully ready
func (e *EKS) waitForClusterReady(t *testing.T, timeout time.Duration) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	opts := e.GetKubectlOptions("")

	maxRetries := int(timeout.Seconds() / 10)
	_, err := retry.DoWithRetryE(t, "Wait for EKS nodes ready", maxRetries, 10*time.Second, func() (string, error) {
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

	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for cluster ready")
	default:
		return nil
	}
}
