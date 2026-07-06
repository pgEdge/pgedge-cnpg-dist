package helpers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/pgedge/pgedge-cnpg-dist/tests/config"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
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
		maxRetries := 120
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

// GetLatestPgedgeHelmVersion resolves the pgedge chart version to test against.
// Priority order:
//  1. PGEDGE_HELM_VERSION env var (no validation — for advanced overrides)
//  2. version_override in versions.yaml (validated against Helm index)
//  3. Latest stable inferred from Helm index
func GetLatestPgedgeHelmVersion(t *testing.T, helmCfg config.PgedgeHelmDefaults) string {
	t.Helper()

	chartRepo := helmCfg.ChartRepo
	if chartRepo == "" {
		chartRepo = "https://pgedge.github.io/charts"
	}

	// 1. Env var override — highest priority, no index validation
	if v := os.Getenv("PGEDGE_HELM_VERSION"); v != "" {
		t.Logf("Chart version source : PGEDGE_HELM_VERSION env var")
		t.Logf("Chart repo           : %s", chartRepo)
		t.Logf("Chart reference      : pgedge/pgedge")
		t.Logf("Chart version        : %s", v)
		return v
	}

	// Fetch Helm index (needed for config validation and inference)
	indexURL := strings.TrimRight(chartRepo, "/") + "/index.yaml"
	available := fetchPgedgeChartVersions(t, indexURL)

	// 2. Config version_override — validated against Helm index
	if helmCfg.VersionOverride != "" {
		for _, v := range available {
			if v == helmCfg.VersionOverride {
				t.Logf("Chart version source : versions.yaml version_override")
				t.Logf("Chart repo           : %s", chartRepo)
				t.Logf("Chart reference      : pgedge/pgedge")
				t.Logf("Chart version        : %s", helmCfg.VersionOverride)
				return helmCfg.VersionOverride
			}
		}
		require.Failf(t, "chart version not found in Helm repo",
			"version_override %q from versions.yaml does not exist in Helm repo %s\nAvailable versions: %s",
			helmCfg.VersionOverride, chartRepo, strings.Join(available, ", "))
		return ""
	}

	// 3. Infer highest stable version (no "-" prerelease suffix) using semver comparison
	latest := ""
	for _, v := range available {
		if strings.Contains(v, "-") {
			continue
		}
		sv := "v" + v
		if !semver.IsValid(sv) {
			continue
		}
		if latest == "" || semver.Compare(sv, "v"+latest) > 0 {
			latest = v
		}
	}

	if latest == "" {
		require.Fail(t, "no stable pgedge chart version found in Helm index at "+indexURL)
		return ""
	}

	t.Logf("Chart version source : inferred from Helm index (latest stable)")
	t.Logf("Chart repo           : %s", chartRepo)
	t.Logf("Chart reference      : pgedge/pgedge")
	t.Logf("Chart version        : %s", latest)
	return latest
}

// fetchPgedgeChartVersions fetches all available pgedge chart versions from the Helm index.
func fetchPgedgeChartVersions(t *testing.T, indexURL string) []string {
	t.Helper()

	resp, err := http.Get(indexURL) //nolint:noctx
	require.NoError(t, err, "Failed to fetch Helm index from %s", indexURL)
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read Helm index response")

	var index struct {
		Entries map[string][]struct {
			Version string `yaml:"version"`
		} `yaml:"entries"`
	}
	require.NoError(t, yaml.Unmarshal(data, &index), "Failed to parse Helm index")

	entries, ok := index.Entries["pgedge"]
	require.True(t, ok, "pgedge chart not found in Helm index at %s", indexURL)

	versions := make([]string, 0, len(entries))
	for _, e := range entries {
		versions = append(versions, e.Version)
	}
	return versions
}

// GetPgedgeHelmPath returns the path to the pgedge-helm repository.
// If PGEDGE_HELM_PATH is set, that directory is used directly.
// Otherwise the repository is cloned at the release tag matching version.
func GetPgedgeHelmPath(t *testing.T, version string) string {
	t.Helper()

	if envPath := os.Getenv("PGEDGE_HELM_PATH"); envPath != "" {
		absPath, err := filepath.Abs(envPath)
		require.NoError(t, err, "Failed to resolve PGEDGE_HELM_PATH")
		return absPath
	}

	tag := fmt.Sprintf("v%s", version)
	repoDir, err := os.MkdirTemp("", "pgedge-helm-*")
	require.NoError(t, err, "Failed to create temp directory for pgedge-helm clone")

	t.Logf("Cloning pgedge-helm repository (tag: %s) to %s", tag, repoDir)

	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "git", "clone",
		"--depth", "1",
		"--branch", tag,
		"https://github.com/pgEdge/pgedge-helm.git",
		repoDir,
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to clone pgedge-helm repo at tag %s: %s", tag, string(output))

	t.Cleanup(func() {
		t.Logf("Cleaning up cloned pgedge-helm repo at %s", repoDir)
		_ = os.RemoveAll(repoDir)
	})

	t.Log("pgedge-helm repository cloned successfully")
	return repoDir
}

// RunPgedgeHelmIntegrationTests runs the full pgedge-helm integration test suite against a
// cluster that already has CNPG and cert-manager installed. The chart is installed from the
// published Helm repository (helmRepo/chartRef@chartVersion) so no local image build is required.
func RunPgedgeHelmIntegrationTests(t *testing.T, kubeconfigPath, kubeContext, pgedgeHelmPath, postgresImage, helmRepo, chartRef, chartVersion string, totalTimeout time.Duration) {
	t.Helper()

	t.Logf("Running pgedge-helm integration tests (context: %s, chart: %s@%s)", kubeContext, chartRef, chartVersion)

	ensureGotestsum(t)

	perTestTimeout := totalTimeout - 10*time.Minute
	if perTestTimeout <= 0 {
		perTestTimeout = totalTimeout
	}

	cmd := exec.Command("gotestsum", "--format", "testname", "--",
		"-tags", "integration",
		"-v",
		fmt.Sprintf("-timeout=%s", totalTimeout.String()),
		"./test/integration/...",
	)
	cmd.Dir = pgedgeHelmPath
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath),
		fmt.Sprintf("KUBECONTEXT=%s", kubeContext),
		"NAMESPACE=default",
		fmt.Sprintf("CHART_REF=%s", chartRef),
		fmt.Sprintf("CHART_VERSION=%s", chartVersion),
		fmt.Sprintf("HELM_REPO=%s", helmRepo),
		fmt.Sprintf("TIMEOUT=%s", perTestTimeout.String()),
	)
	if postgresImage != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("POSTGRES_IMAGE=%s", postgresImage))
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Run(), "pgedge-helm integration tests failed")
	t.Log("pgedge-helm integration tests completed successfully")
}

// ensureGotestsum installs gotestsum if it is not already on PATH.
func ensureGotestsum(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("gotestsum"); err == nil {
		return
	}

	t.Log("gotestsum not found, installing gotest.tools/gotestsum@v1.13.0")
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "go", "install", "gotest.tools/gotestsum@v1.13.0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "failed to install gotestsum")
}
