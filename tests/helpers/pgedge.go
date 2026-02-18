package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/helm"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

// InstallCertManager deploys cert-manager from its release manifest and waits for it to be ready
func InstallCertManager(t *testing.T, kubeconfigPath, manifestURL string) {
	t.Helper()

	t.Logf("Installing cert-manager from %s", manifestURL)

	opts := k8s.NewKubectlOptions("", kubeconfigPath, "cert-manager")

	// Apply cert-manager manifest
	k8s.RunKubectl(t, k8s.NewKubectlOptions("", kubeconfigPath, ""), "apply", "-f", manifestURL)

	// Wait for cert-manager deployments to be ready
	deployments := []string{"cert-manager", "cert-manager-cainjector", "cert-manager-webhook"}

	for _, dep := range deployments {
		t.Logf("Waiting for cert-manager deployment %s to be ready...", dep)
		maxRetries := 60
		_, err := retry.DoWithRetryE(t, fmt.Sprintf("Wait for %s", dep), maxRetries, 5*time.Second, func() (string, error) {
			deployment, getErr := k8s.GetDeploymentE(t, opts, dep)
			if getErr != nil {
				return "", fmt.Errorf("failed to get deployment %s: %w", dep, getErr)
			}
			if deployment.Status.ReadyReplicas == 0 || deployment.Status.ReadyReplicas != *deployment.Spec.Replicas {
				return "", fmt.Errorf("deployment %s not ready: %d/%d", dep, deployment.Status.ReadyReplicas, *deployment.Spec.Replicas)
			}
			return "ready", nil
		})
		require.NoError(t, err, "cert-manager deployment %s not ready", dep)
	}

	// Register cleanup
	t.Cleanup(func() {
		t.Log("Cleaning up cert-manager")
		_ = k8s.RunKubectlE(t, k8s.NewKubectlOptions("", kubeconfigPath, ""), "delete", "-f", manifestURL, "--ignore-not-found")
	})

	t.Log("cert-manager installed successfully")
}

// BuildAndLoadImage builds the pgedge-helm-utils Docker image and loads it into the Kind cluster
func BuildAndLoadImage(t *testing.T, pgedgeHelmPath, kindClusterName string) {
	t.Helper()

	t.Logf("Building pgedge-helm-utils Docker image in %s", pgedgeHelmPath)

	// Build the dev image using docker buildx bake
	buildCmd := exec.Command("make", "docker-build-dev")
	buildCmd.Dir = pgedgeHelmPath
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	err := buildCmd.Run()
	require.NoError(t, err, "Failed to build pgedge-helm-utils Docker image")

	// Load image into Kind cluster
	t.Logf("Loading pgedge-helm-utils:dev image into Kind cluster %s", kindClusterName)
	loadCmd := exec.Command("kind", "load", "docker-image", "pgedge-helm-utils:dev", "--name", kindClusterName)
	loadCmd.Stdout = os.Stdout
	loadCmd.Stderr = os.Stderr
	err = loadCmd.Run()
	require.NoError(t, err, "Failed to load image into Kind cluster")

	t.Log("pgedge-helm-utils image built and loaded successfully")
}

// DeployPgedgeChart installs the pgEdge Helm chart with the given values
func DeployPgedgeChart(t *testing.T, kubeconfigPath, chartPath, valuesFile, namespace, initSpockImage string) {
	t.Helper()

	t.Logf("Installing pgEdge Helm chart from %s", chartPath)

	opts := k8s.NewKubectlOptions("", kubeconfigPath, namespace)

	// Create namespace if it doesn't exist
	_ = k8s.CreateNamespaceE(t, opts, namespace)

	helmOptions := &helm.Options{
		KubectlOptions: opts,
		ValuesFiles:    []string{valuesFile},
		SetValues:      map[string]string{},
		ExtraArgs: map[string][]string{
			"install": {
				"--create-namespace",
				"--timeout", "30m",
			},
		},
	}

	// Override init-spock image if specified
	if initSpockImage != "" {
		helmOptions.SetValues["pgEdge.initSpockImageName"] = initSpockImage
	}

	err := helm.InstallE(t, helmOptions, chartPath, "pgedge")
	require.NoError(t, err, "Failed to install pgEdge Helm chart")

	// Register cleanup
	t.Cleanup(func() {
		t.Log("Cleaning up pgEdge Helm release")
		uninstallOpts := &helm.Options{
			KubectlOptions: opts,
		}
		_ = helm.DeleteE(t, uninstallOpts, "pgedge", true)
	})

	t.Log("pgEdge Helm chart installed successfully")
}

// WaitForPgedgeClusters waits for all CNPG Cluster CRDs created by the pgedge chart to reach healthy state
func WaitForPgedgeClusters(t *testing.T, kubeconfigPath, namespace, appName string, nodeCount int, timeout time.Duration) {
	t.Helper()

	opts := k8s.NewKubectlOptions("", kubeconfigPath, namespace)
	maxRetries := int(timeout.Seconds() / 10)

	t.Logf("Waiting for %d pgEdge CNPG clusters to become healthy (timeout: %v)", nodeCount, timeout)

	_, err := retry.DoWithRetryE(t, "Wait for pgEdge clusters healthy", maxRetries, 10*time.Second, func() (string, error) {
		// Get all CNPG clusters in namespace
		output, getErr := k8s.RunKubectlAndGetOutputE(t, opts,
			"get", "clusters.postgresql.cnpg.io",
			"-o", "jsonpath={range .items[*]}{.metadata.name}={.status.phase}{\"\\n\"}{end}",
		)
		if getErr != nil {
			return "", fmt.Errorf("failed to get CNPG clusters: %w", getErr)
		}

		if output == "" {
			return "", fmt.Errorf("no CNPG clusters found")
		}

		// Parse cluster statuses
		lines := strings.Split(strings.TrimSpace(output), "\n")
		healthyCount := 0
		for _, line := range lines {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			name, phase := parts[0], parts[1]
			t.Logf("  Cluster %s: %s", name, phase)
			if phase == "Cluster in healthy state" {
				healthyCount++
			}
		}

		if healthyCount < nodeCount {
			return "", fmt.Errorf("only %d/%d clusters healthy", healthyCount, nodeCount)
		}

		return "All clusters healthy", nil
	})

	require.NoError(t, err, "pgEdge CNPG clusters did not become healthy within %v", timeout)
	t.Logf("All %d pgEdge CNPG clusters are healthy", nodeCount)
}

// WaitForInitSpockJob waits for the init-spock post-install job to complete
func WaitForInitSpockJob(t *testing.T, kubeconfigPath, namespace, appName string, timeout time.Duration) {
	t.Helper()

	opts := k8s.NewKubectlOptions("", kubeconfigPath, namespace)
	jobName := fmt.Sprintf("%s-init-spock", appName)
	maxRetries := int(timeout.Seconds() / 10)

	t.Logf("Waiting for init-spock job %s to complete (timeout: %v)", jobName, timeout)

	_, err := retry.DoWithRetryE(t, "Wait for init-spock job", maxRetries, 10*time.Second, func() (string, error) {
		output, getErr := k8s.RunKubectlAndGetOutputE(t, opts,
			"get", "job", jobName,
			"-o", "jsonpath={.status.succeeded}",
		)
		if getErr != nil {
			return "", fmt.Errorf("failed to get job %s: %w", jobName, getErr)
		}

		if output != "1" {
			// Check if job failed
			failedOutput, _ := k8s.RunKubectlAndGetOutputE(t, opts,
				"get", "job", jobName,
				"-o", "jsonpath={.status.failed}",
			)
			if failedOutput != "" && failedOutput != "0" {
				// Get job logs for debugging
				logs, _ := k8s.RunKubectlAndGetOutputE(t, opts,
					"logs", fmt.Sprintf("job/%s", jobName), "--all-containers=true",
				)
				return "", retry.FatalError{Underlying: fmt.Errorf("init-spock job failed. Logs:\n%s", logs)}
			}
			return "", fmt.Errorf("init-spock job not yet succeeded (succeeded=%s)", output)
		}

		return "Job completed", nil
	})

	require.NoError(t, err, "init-spock job did not complete within %v", timeout)
	t.Log("init-spock job completed successfully")
}

// RunHelmTest executes `helm test` for the pgedge release and returns the result
func RunHelmTest(t *testing.T, kubeconfigPath, releaseName, namespace string, timeout time.Duration) {
	t.Helper()

	t.Logf("Running helm test for release %s in namespace %s", releaseName, namespace)

	cmd := exec.Command("helm", "test", releaseName,
		"--namespace", namespace,
		"--kubeconfig", kubeconfigPath,
		"--timeout", fmt.Sprintf("%ds", int(timeout.Seconds())),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	require.NoError(t, err, "helm test failed for release %s", releaseName)

	t.Log("helm test passed successfully")
}

// GetPgedgeHelmPath returns the path to the pgedge-helm repository.
// Checks PGEDGE_HELM_PATH env var first, then clones the repo into a temp directory.
func GetPgedgeHelmPath(t *testing.T) string {
	t.Helper()

	if envPath := os.Getenv("PGEDGE_HELM_PATH"); envPath != "" {
		absPath, err := filepath.Abs(envPath)
		require.NoError(t, err, "Failed to resolve PGEDGE_HELM_PATH")
		return absPath
	}

	// Clone pgedge-helm repo into temp directory (similar to cloneCNPGRepo in cnpg_upstream_test.go)
	branch := os.Getenv("PGEDGE_HELM_BRANCH")
	if branch == "" {
		branch = "main"
	}

	repoDir := filepath.Join(os.TempDir(), "pgedge-helm")

	// Check if already cloned
	if _, err := os.Stat(filepath.Join(repoDir, "Chart.yaml")); err == nil {
		t.Logf("pgedge-helm repository already exists at %s", repoDir)
		return repoDir
	}

	t.Logf("Cloning pgedge-helm repository (branch: %s) to %s", branch, repoDir)

	cmd := exec.Command("git", "clone",
		"--depth", "1",
		"--branch", branch,
		"https://github.com/pgEdge/pgedge-helm.git",
		repoDir,
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to clone pgedge-helm repo: %s", string(output))

	// Register cleanup to remove cloned repo
	t.Cleanup(func() {
		t.Logf("Cleaning up cloned pgedge-helm repo at %s", repoDir)
		_ = os.RemoveAll(repoDir)
	})

	t.Log("pgedge-helm repository cloned successfully")
	return repoDir
}
