package providers

import (
	"os"
	"strconv"
	"testing"
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

// GetRegion returns the cloud region from environment (for cloud providers)
func GetRegion() string {
	return os.Getenv("CLOUD_REGION")
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

// CreateFromEnv creates a provider from environment variables
func CreateFromEnv(t *testing.T, clusterName string) Provider {
	t.Helper()

	config := &Config{
		Name:              clusterName,
		KubernetesVersion: GetKubernetesVersion(),
		NodeCount:         GetNodeCount(),
		Region:            GetRegion(),
	}

	providerType := GetProviderType()
	t.Logf("Creating cluster %s using provider: %s (K8s: %s, Nodes: %d)",
		clusterName, providerType, config.KubernetesVersion, config.NodeCount)

	return Create(t, providerType, config)
}
