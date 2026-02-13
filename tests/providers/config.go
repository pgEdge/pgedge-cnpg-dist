package providers

import (
	"os"
	"strconv"
	"testing"
)

// Lifecycle control environment variables
const (
	EnvClusterReuse   = "CLUSTER_REUSE"
	EnvClusterCleanup = "CLUSTER_CLEANUP"
	EnvClusterName    = "CLUSTER_NAME"
)

// GetProviderType returns the provider type from environment or defaults to "kind"
func GetProviderType() string {
	providerType := os.Getenv("CLUSTER_PROVIDER")
	if providerType == "" {
		providerType = "kind"
	}
	return providerType
}

// GetKubernetesVersion returns the Kubernetes version from environment or defaults to "1.32"
func GetKubernetesVersion() string {
	k8sVersion := os.Getenv("KUBERNETES_VERSION")
	if k8sVersion == "" {
		k8sVersion = "1.32"
	}
	return k8sVersion
}

// GetAWSRegion returns the AWS region from environment or defaults to us-west-2
func GetAWSRegion() string {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}
	return region
}

// GetUseSpotInstances returns whether to use spot instances for EKS
func GetUseSpotInstances() bool {
	useSpot := os.Getenv("EKS_USE_SPOT")
	// Default to true unless explicitly set to "false"
	return useSpot != "false"
}

// GetEKSNodeType returns the EC2 instance type for EKS nodes
func GetEKSNodeType() string {
	nodeType := os.Getenv("EKS_NODE_TYPE")
	if nodeType == "" {
		nodeType = "m5.large"
	}
	return nodeType
}

// GetNodeCount returns the number of nodes from environment or defaults to 3
func GetNodeCount() int {
	nodeCountStr := os.Getenv("NODE_COUNT")
	if nodeCountStr == "" {
		return 3
	}
	nodeCount, err := strconv.Atoi(nodeCountStr)
	if err != nil {
		return 3
	}
	return nodeCount
}

// GetClusterReuse returns whether to reuse existing clusters
func GetClusterReuse() bool {
	return os.Getenv(EnvClusterReuse) == "true"
}

// GetClusterCleanup returns whether to cleanup clusters after tests (default: true)
func GetClusterCleanup() bool {
	cleanup := os.Getenv(EnvClusterCleanup)
	return cleanup != "false" // Default to true unless explicitly set to "false"
}

// GetClusterName returns explicit cluster name from environment or empty string
func GetClusterName() string {
	return os.Getenv(EnvClusterName)
}

// CreateFromEnv creates a provider from environment variables
// If CLUSTER_NAME is set, it overrides the provided clusterName parameter
func CreateFromEnv(t *testing.T, clusterName string) Provider {
	t.Helper()

	providerType := GetProviderType()

	// Allow CLUSTER_NAME env var to override the provided name
	if envName := GetClusterName(); envName != "" {
		clusterName = envName
	}

	config := &Config{
		Name:              clusterName,
		KubernetesVersion: GetKubernetesVersion(),
		NodeCount:         GetNodeCount(),
	}

	// Log lifecycle settings
	reuse := GetClusterReuse()
	cleanup := GetClusterCleanup()

	// EKS-specific configuration
	if providerType == "eks" {
		config.Region = GetAWSRegion()
		t.Logf("Provider: %s, Cluster: %s, K8s: %s, Nodes: %d, Region: %s, Reuse: %v, Cleanup: %v",
			providerType, clusterName, config.KubernetesVersion, config.NodeCount, config.Region, reuse, cleanup)
	} else {
		t.Logf("Provider: %s, Cluster: %s, K8s: %s, Nodes: %d, Reuse: %v, Cleanup: %v",
			providerType, clusterName, config.KubernetesVersion, config.NodeCount, reuse, cleanup)
	}

	return Create(t, providerType, config)
}
