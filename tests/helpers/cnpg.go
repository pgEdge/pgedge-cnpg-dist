package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/helm"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

// CNPGOperator represents a deployed CNPG operator
type CNPGOperator struct {
	Version        string
	Namespace      string
	ReleaseName    string
	ChartPath      string
	OperatorImage  string
	PostgresImage  string
	KubectlOptions *k8s.KubectlOptions
}

// CNPGOperatorConfig represents CNPG operator configuration
type CNPGOperatorConfig struct {
	Version       string
	Namespace     string
	ReleaseName   string
	OperatorImage string
	PostgresImage string
}

// NewCNPGOperator creates a new CNPG operator helper
func NewCNPGOperator(t *testing.T, config *CNPGOperatorConfig, kubeconfigPath string) *CNPGOperator {
	t.Helper()

	// Get project root
	projectRoot, err := os.Getwd()
	require.NoError(t, err, "Failed to get working directory")

	// Find project root by looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			require.Fail(t, "Could not find project root (go.mod not found)")
		}
		projectRoot = parent
	}

	chartPath := filepath.Join(projectRoot, "charts", "cloudnative-pg", fmt.Sprintf("v%s", config.Version))

	return &CNPGOperator{
		Version:        config.Version,
		Namespace:      config.Namespace,
		ReleaseName:    config.ReleaseName,
		ChartPath:      chartPath,
		OperatorImage:  config.OperatorImage,
		PostgresImage:  config.PostgresImage,
		KubectlOptions: k8s.NewKubectlOptions("", kubeconfigPath, config.Namespace),
	}
}

// Install deploys the CNPG operator using Helm
func (co *CNPGOperator) Install(t *testing.T) error {
	t.Helper()

	t.Logf("Installing CNPG operator %s in namespace %s", co.Version, co.Namespace)

	// Create namespace
	err := k8s.CreateNamespaceE(t, co.KubectlOptions, co.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Prepare Helm options
	helmOptions := &helm.Options{
		KubectlOptions: co.KubectlOptions,
		SetValues: map[string]string{
			"image.repository": getImageRepository(co.OperatorImage),
			"image.tag":        getImageTag(co.OperatorImage),
		},
		ExtraArgs: map[string][]string{
			"install": {
				"--create-namespace",
				"--wait",
				"--timeout", "5m",
			},
		},
	}

	// Add POSTGRES_IMAGE_NAME environment variable if PostgresImage is set
	if co.PostgresImage != "" {
		helmOptions.SetValues["config.data.POSTGRES_IMAGE_NAME"] = co.PostgresImage
	}

	// Install chart
	err = helm.InstallE(t, helmOptions, co.ChartPath, co.ReleaseName)
	if err != nil {
		return fmt.Errorf("failed to install Helm chart: %w", err)
	}

	// Wait for operator to be ready
	err = co.waitForOperatorReady(t, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("operator not ready: %w", err)
	}

	t.Logf("CNPG operator %s installed successfully", co.Version)
	return nil
}

// Uninstall removes the CNPG operator
func (co *CNPGOperator) Uninstall(t *testing.T) error {
	t.Helper()

	t.Logf("Uninstalling CNPG operator %s", co.ReleaseName)

	helmOptions := &helm.Options{
		KubectlOptions: co.KubectlOptions,
	}

	err := helm.DeleteE(t, helmOptions, co.ReleaseName, true)
	if err != nil {
		return fmt.Errorf("failed to uninstall Helm release: %w", err)
	}

	// Delete namespace
	err = k8s.DeleteNamespaceE(t, co.KubectlOptions, co.Namespace)
	if err != nil {
		t.Logf("Warning: failed to delete namespace: %v", err)
	}

	t.Logf("CNPG operator %s uninstalled successfully", co.ReleaseName)
	return nil
}

// waitForOperatorReady waits for the CNPG operator deployment to be ready
func (co *CNPGOperator) waitForOperatorReady(t *testing.T, timeout time.Duration) error {
	t.Helper()

	maxRetries := int(timeout.Seconds() / 5)

	_, err := retry.DoWithRetryE(t, "Wait for operator ready", maxRetries, 5*time.Second, func() (string, error) {
		// Check if deployment exists and is ready
		deployment, getErr := k8s.GetDeploymentE(t, co.KubectlOptions, co.ReleaseName)
		if getErr != nil {
			return "", fmt.Errorf("failed to get deployment: %w", getErr)
		}

		if deployment.Status.ReadyReplicas == 0 {
			return "", fmt.Errorf("no ready replicas")
		}

		if deployment.Status.ReadyReplicas != *deployment.Spec.Replicas {
			return "", fmt.Errorf("not all replicas ready: %d/%d", deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
		}

		return "Operator ready", nil
	})

	return err
}

// GetOperatorLogs retrieves the logs from the CNPG operator pod
func (co *CNPGOperator) GetOperatorLogs(t *testing.T) (string, error) {
	t.Helper()

	// Get pod name using kubectl
	output, err := k8s.RunKubectlAndGetOutputE(t, co.KubectlOptions,
		"get", "pods",
		"-l", "app.kubernetes.io/name=cloudnative-pg",
		"-o", "jsonpath={.items[0].metadata.name}",
	)
	if err != nil {
		return "", fmt.Errorf("failed to get operator pod: %w", err)
	}

	if output == "" {
		return "", fmt.Errorf("no operator pods found")
	}

	// Get logs
	logs, err := k8s.RunKubectlAndGetOutputE(t, co.KubectlOptions, "logs", output)
	if err != nil {
		return "", fmt.Errorf("failed to get pod logs: %w", err)
	}

	return logs, nil
}

// DeployCNPGOperator is a convenience function to deploy CNPG operator
func DeployCNPGOperator(t *testing.T, kubeconfigPath, version, namespace, operatorImage, postgresImage string) *CNPGOperator {
	t.Helper()

	config := &CNPGOperatorConfig{
		Version:       version,
		Namespace:     namespace,
		ReleaseName:   "cloudnative-pg",
		OperatorImage: operatorImage,
		PostgresImage: postgresImage,
	}

	operator := NewCNPGOperator(t, config, kubeconfigPath)

	err := operator.Install(t)
	require.NoError(t, err, "Failed to install CNPG operator")

	// Register cleanup
	t.Cleanup(func() {
		if err := operator.Uninstall(t); err != nil {
			t.Logf("Warning: failed to uninstall operator: %v", err)
		}
	})

	return operator
}

// Helper functions

func getImageRepository(fullImage string) string {
	// Split image:tag
	for i := len(fullImage) - 1; i >= 0; i-- {
		if fullImage[i] == ':' {
			return fullImage[:i]
		}
	}
	return fullImage
}

func getImageTag(fullImage string) string {
	// Split image:tag
	for i := len(fullImage) - 1; i >= 0; i-- {
		if fullImage[i] == ':' {
			return fullImage[i+1:]
		}
	}
	return "latest"
}
