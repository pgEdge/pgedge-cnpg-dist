package providers

import (
	"os"
	"strconv"
	"testing"

	"github.com/pgedge/pgedge-cnpg-dist/tests/config"
)

// GetProviderType returns the provider type from environment or defaults to "kind"
func GetProviderType() string {
	providerType := os.Getenv("CLUSTER_PROVIDER")
	if providerType == "" {
		providerType = "kind"
	}
	return providerType
}

// getProviderDefaults returns the ProviderDefaults for the active provider from versions.yaml
func getProviderDefaults() *config.ProviderDefaults {
	if cfg, err := config.LoadConfig(); err == nil {
		if defaults, ok := cfg.ProviderDefaults[GetProviderType()]; ok {
			return &defaults
		}
	}
	return nil
}

// GetKubernetesVersion returns the Kubernetes version from environment, falling back to versions.yaml default
func GetKubernetesVersion() string {
	if v := os.Getenv("KUBERNETES_VERSION"); v != "" {
		return v
	}
	if d := getProviderDefaults(); d != nil && d.KubernetesVersion != "" {
		return d.KubernetesVersion
	}
	return "1.32"
}

// GetRegion returns the cloud region from environment, falling back to versions.yaml default
func GetRegion() string {
	if v := os.Getenv("CLOUD_REGION"); v != "" {
		return v
	}
	if d := getProviderDefaults(); d != nil && d.Region != "" {
		return d.Region
	}
	return "us-east-1"
}

// GetNodeCount returns the number of nodes from environment, falling back to versions.yaml default
func GetNodeCount() int {
	if v := os.Getenv("NODE_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if d := getProviderDefaults(); d != nil && d.NodeCount > 0 {
		return d.NodeCount
	}
	return 3
}

// GetInstanceType returns the instance type from environment, falling back to versions.yaml default
func GetInstanceType() string {
	if v := os.Getenv("INSTANCE_TYPE"); v != "" {
		return v
	}
	if d := getProviderDefaults(); d != nil && d.InstanceType != "" {
		return d.InstanceType
	}
	return "m5.large"
}

// GetNodeArch returns the node architecture from environment, falling back to versions.yaml default
func GetNodeArch() string {
	if v := os.Getenv("NODE_ARCH"); v != "" {
		return v
	}
	if d := getProviderDefaults(); d != nil && d.NodeArch != "" {
		return d.NodeArch
	}
	return "amd64"
}

// NewProvider creates a provider
func NewProvider(t *testing.T, clusterName string) Provider {
	t.Helper()

	config := &Config{
		Name:              clusterName,
		KubernetesVersion: GetKubernetesVersion(),
		NodeCount:         GetNodeCount(),
		Region:            GetRegion(),
		InstanceType:      GetInstanceType(),
		NodeArch:          GetNodeArch(),
	}

	providerType := GetProviderType()
	t.Logf("Creating cluster %s using provider: %s (K8s: %s, Nodes: %d, Arch: %s, Instance: %s)",
		clusterName, providerType, config.KubernetesVersion, config.NodeCount, config.NodeArch, config.InstanceType)

	return Create(t, providerType, config)
}
