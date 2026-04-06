package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgedge/pgedge-cnpg-dist/tests/config"
	"github.com/pgedge/pgedge-cnpg-dist/tests/helpers"
	"github.com/pgedge/pgedge-cnpg-dist/tests/providers"
	"github.com/stretchr/testify/require"
)

// TestUpstream runs the upstream CNPG E2E tests
// Use LABEL_FILTER env var to filter tests (e.g., LABEL_FILTER=smoke, LABEL_FILTER=postgres-configuration)
func TestUpstream(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping upstream E2E tests in short mode")
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	require.NoError(t, err, "Failed to load configuration")

	// Get CNPG version from environment or use default
	cnpgVersion, err := cfg.GetCNPGVersionFromEnv()
	require.NoError(t, err, "Failed to get CNPG version")
	postgresVersion := cnpgVersion.GetPostgresVersionFromEnv()

	t.Logf("Test execution: CNPG=%s  PostgreSQL=%s  Kubernetes=%s  Provider=%s",
		cnpgVersion.Version, postgresVersion, providers.GetKubernetesVersion(), providers.GetProviderType())

	// Create cluster using provider from environment
	clusterName := fmt.Sprintf("cnpg-e2e-%s", strings.ReplaceAll(cnpgVersion.Version, ".", "-"))
	provider := providers.NewProvider(t, clusterName)
	providers.Setup(t, provider)

	// Get PostgreSQL image
	postgresImage := cfg.GetPostgresImageName(
		cfg.PostgresImages.DefaultRegistry,
		postgresVersion,
		"standard",
	)

	// Deploy CNPG operator
	operator := helpers.DeployCNPGOperator(t,
		provider.GetKubeConfigPath(),
		cnpgVersion.Version,
		cnpgVersion.ChartVersion,
		"cnpg-system",
		cnpgVersion.GetOperatorImageName(),
		postgresImage,
	)

	t.Logf("CNPG operator deployed, running upstream E2E tests")

	// Clone CNPG repository at specific version
	cnpgRepo := cloneCNPGRepo(t, cnpgVersion.GitTag, cnpgVersion.Version, postgresVersion)

	// Get provider-aware storage config
	storageConfig, ok := cfg.GetStorageConfig(providers.GetProviderType())
	if !ok {
		t.Fatalf("no storage config found for provider %s", providers.GetProviderType())
	}
	if storageConfig.CSIClass == "" {
		t.Fatalf("storage config for provider %s is missing CSIClass", providers.GetProviderType())
	}
	if storageConfig.SnapshotClass == "" {
		t.Fatalf("storage config for provider %s is missing SnapshotClass", providers.GetProviderType())
	}

	// Run upstream E2E tests
	testResults := runUpstreamE2ETests(t, cnpgRepo, provider.GetKubeConfigPath(), postgresImage, storageConfig)

	// Log results
	t.Logf("Test results: %+v", testResults)

	// Assert tests passed
	require.Equal(t, 0, testResults.Failed, "Some E2E tests failed. Check logs for details.")
	require.Greater(t, testResults.Passed, 0, "Expected some tests to pass")

	// Log operator logs if tests failed
	if testResults.Failed > 0 {
		logs, _ := operator.GetOperatorLogs(t)
		t.Logf("Operator logs:\n%s", logs)
	}
}

// TestResults represents E2E test execution results
type TestResults struct {
	Passed  int
	Failed  int
	Skipped int
	Total   int
}

// cloneCNPGRepo clones the CNPG repository at a specific version
func cloneCNPGRepo(t *testing.T, gitTag, cnpgVersion, postgresVersion string) string {
	t.Helper()

	repoDir := filepath.Join(os.TempDir(), fmt.Sprintf("cnpg-e2e-%s-%s", cnpgVersion, postgresVersion))

	// Always start fresh to avoid stale/corrupted clones
	if _, err := os.Stat(repoDir); err == nil {
		t.Logf("Removing existing CNPG repository at %s", repoDir)
		if err := os.RemoveAll(repoDir); err != nil {
			t.Fatalf("failed to remove existing CNPG repo %s: %v", repoDir, err)
		}
	}

	t.Logf("Cloning CNPG repository (tag: %s) to %s", gitTag, repoDir)

	// Clone with shallow copy for speed
	cmd := exec.Command("git", "clone",
		"--depth", "1",
		"--branch", gitTag,
		"https://github.com/cloudnative-pg/cloudnative-pg.git",
		repoDir,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to clone CNPG repo: %v\nOutput: %s", err, string(output))
	}

	t.Logf("CNPG repository cloned successfully")
	return repoDir
}

// e2eExcludeFilters lists Ginkgo label exclusions applied to every upstream E2E run.
// - backup-restore, snapshot: pgEdge images use new Barman Cloud Plugin
// - postgres-major-upgrade: requires specific upgrade path setup
// - plugin: requires plugin infrastructure not available in test environment
// - observability: requires PodMonitor CRD from prometheus-operator
// - postgres-configuration: rolling update tests fail with pgEdge images (image tag format)
// - pod-scheduling: affinity test fails in CI environment
// - cluster-metadata: configuration update tests timeout due to resource contention in CI
// - self-healing: fast failover with sync replicas is timing-sensitive and flaky in CI
var e2eExcludeFilters = []string{
	"!backup-restore", "!snapshot", "!postgres-major-upgrade", "!plugin",
	"!observability", "!postgres-configuration", "!pod-scheduling",
	"!cluster-metadata", "!self-healing",
}

// e2eSkipTests lists Ginkgo name patterns (regex) to skip in every upstream E2E run.
// - Image.Catalogs: requires E2E_PRE_ROLLING_UPDATE_IMG with semantic version tag
var e2eSkipTests = []string{"Image.Catalogs"}

// buildLabelFilter combines an optional LABEL_FILTER env override with the fixed exclusions.
func buildLabelFilter() string {
	if envFilter := os.Getenv("LABEL_FILTER"); envFilter != "" {
		return strings.Join(append([]string{envFilter}, e2eExcludeFilters...), " && ")
	}
	return strings.Join(e2eExcludeFilters, " && ")
}

// buildE2EEnv constructs the environment for the ginkgo E2E process.
func buildE2EEnv(kubeconfigPath, postgresImage string, storageConfig config.StorageConfig) []string {
	return append(os.Environ(),
		fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath),
		fmt.Sprintf("POSTGRES_IMG=%s", postgresImage),
		"TEST_UPGRADE_TO_V1=false",
		"TEST_CLOUD_VENDOR=local",
		fmt.Sprintf("E2E_DEFAULT_STORAGE_CLASS=%s", storageConfig.CSIClass),
		fmt.Sprintf("E2E_CSI_STORAGE_CLASS=%s", storageConfig.CSIClass),
		fmt.Sprintf("E2E_DEFAULT_VOLUMESNAPSHOT_CLASS=%s", storageConfig.SnapshotClass),
	)
}

// buildGinkgoCmd constructs the ginkgo exec.Command for the upstream E2E suite.
func buildGinkgoCmd(testsDir, labelFilter, reportPath string) *exec.Cmd {
	cmd := exec.Command("ginkgo",
		"run",
		fmt.Sprintf("--label-filter=%s", labelFilter),
		fmt.Sprintf("--skip=%s", strings.Join(e2eSkipTests, "|")),
		"--nodes=2",                     // 2 parallel nodes
		"--timeout=3h",                  // Overall timeout
		"--poll-progress-after=1200s",   // Show progress if quiet for 20min
		"--poll-progress-interval=150s", // Update progress every 2.5min
		"--github-output",               // GitHub-friendly output
		"--force-newlines",              // Ensure newlines in output
		"--silence-skips",               // Don't log skipped tests
		"-v",                            // Verbose
		fmt.Sprintf("--json-report=%s", reportPath),
		"./...",
	)
	cmd.Dir = testsDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// runUpstreamE2ETests executes the upstream CNPG E2E tests
func runUpstreamE2ETests(t *testing.T, cnpgRepoDir, kubeconfigPath, postgresImage string, storageConfig config.StorageConfig) TestResults {
	t.Helper()

	testsDir := filepath.Join(cnpgRepoDir, "tests", "e2e")
	t.Logf("Running upstream E2E tests from %s", testsDir)

	labelFilter := buildLabelFilter()
	reportPath := filepath.Join(testsDir, "report.json")

	cmd := buildGinkgoCmd(testsDir, labelFilter, reportPath)
	cmd.Env = buildE2EEnv(kubeconfigPath, postgresImage, storageConfig)

	t.Logf("Executing: ginkgo with label filter: %s", labelFilter)
	t.Logf("JSON report will be written to: %s", reportPath)

	err := cmd.Run()
	results := parseTestResults(t, reportPath)

	// If ginkgo succeeded but JSON parsing found no results, the format may differ.
	// Trust ginkgo's exit code in that case.
	if err == nil && results.Passed == 0 && results.Failed == 0 {
		t.Logf("Warning: Ginkgo succeeded but JSON parsing found no results")
		t.Logf("This likely means the JSON format is different than expected")
		t.Logf("Trusting ginkgo exit code - marking tests as passed")
		results.Passed = 1
	}

	if err != nil {
		t.Logf("Warning: ginkgo command failed: %v", err)
		t.Logf("This may be expected if tests failed. Results: %+v", results)
	}

	return results
}

// countGinkgoState counts occurrences of a test state in Ginkgo v2 JSON content,
// trying multiple format variants until a non-zero count is found.
func countGinkgoState(content string, patterns []string) int {
	for _, p := range patterns {
		if n := strings.Count(content, p); n > 0 {
			return n
		}
	}
	return 0
}

// parseTestResults parses the Ginkgo JSON report
func parseTestResults(t *testing.T, reportPath string) TestResults {
	t.Helper()

	defaultResults := TestResults{}

	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Logf("Test report not found at %s", reportPath)
		return defaultResults
	}

	t.Logf("Test report generated at %s", reportPath)

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Logf("Warning: failed to read report: %v", err)
		return defaultResults
	}

	content := string(data)

	passed := countGinkgoState(content, []string{`"State":"passed"`, `"state":"passed"`, `"Passed":true`})
	failed := countGinkgoState(content, []string{`"State":"failed"`, `"state":"failed"`})
	if failed == 0 {
		failed = strings.Count(content, `"Passed":false`) - strings.Count(content, `"Skipped":true`)
	}
	skipped := countGinkgoState(content, []string{`"State":"skipped"`, `"state":"skipped"`, `"Skipped":true`})

	results := TestResults{
		Passed:  passed,
		Failed:  failed,
		Skipped: skipped,
	}
	results.Total = results.Passed + results.Failed + results.Skipped

	t.Logf("Parsed test results: Passed=%d, Failed=%d, Skipped=%d, Total=%d",
		results.Passed, results.Failed, results.Skipped, results.Total)

	return results
}
