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

// GetKubernetesVersion returns the Kubernetes version from environment or defaults to "1.32"
func GetKubernetesVersion() string {
	k8sVersion := os.Getenv("KUBERNETES_VERSION")
	if k8sVersion == "" {
		k8sVersion = "1.32"
	}
	return k8sVersion
}

// GetRegion returns the cloud region from environment, falling back to versions.yaml default
func GetRegion() string {
	if v := os.Getenv("CLOUD_REGION"); v != "" {
		return v
	}
	if cfg, err := config.LoadConfig(); err == nil && cfg.EKSDefaults.Region != "" {
		return cfg.EKSDefaults.Region
	}
	return "us-east-1"
}

// GetNodeCount returns the number of nodes from environment, falling back to versions.yaml default
func GetNodeCount() int {
	if v := os.Getenv("NODE_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	if cfg, err := config.LoadConfig(); err == nil {
		switch GetProviderType() {
		case "eks":
			if cfg.EKSDefaults.NodeCount > 0 {
				return cfg.EKSDefaults.NodeCount
			}
		default:
			if cfg.KindDefaults.Nodes > 0 {
				return cfg.KindDefaults.Nodes
			}
		}
	}
	return 3
}

// GetInstanceType returns the instance type from environment, falling back to versions.yaml default
func GetInstanceType() string {
	if v := os.Getenv("INSTANCE_TYPE"); v != "" {
		return v
	}
	if cfg, err := config.LoadConfig(); err == nil && cfg.EKSDefaults.InstanceType != "" {
		return cfg.EKSDefaults.InstanceType
	}
	return "m5.large"
}

// GetNodeArch returns the node architecture from environment, falling back to versions.yaml default
func GetNodeArch() string {
	if v := os.Getenv("NODE_ARCH"); v != "" {
		return v
	}
	if cfg, err := config.LoadConfig(); err == nil && cfg.EKSDefaults.NodeArch != "" {
		return cfg.EKSDefaults.NodeArch
	}
	return "amd64"
}

// NewProvider creates a provider from environment variables
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
