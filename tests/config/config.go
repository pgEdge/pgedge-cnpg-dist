package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the complete test configuration loaded from versions.yaml
type Config struct {
	CNPGVersions             []CNPGVersion                   `yaml:"cnpg_versions"`
	PostgresImages           PostgresImages                  `yaml:"postgres_images"`
	TestDefaults             TestDefaults                    `yaml:"test_defaults"`
	KindDefaults             KindDefaults                    `yaml:"kind_defaults"`
	KubernetesVersions       map[string]KubernetesVersion    `yaml:"kubernetes_versions"`
	DefaultKubernetesVersion string                          `yaml:"default_kubernetes_version"`
}

// CNPGVersion represents a specific CNPG version configuration
type CNPGVersion struct {
	Version          string                    `yaml:"version"`
	GitTag           string                    `yaml:"git_tag"`
	OperatorImage    string                    `yaml:"operator_image"`
	PostgresVersions []string                  `yaml:"postgres_versions"`
	Providers        map[string]ProviderConfig `yaml:"providers"`
}

// ProviderConfig represents provider-specific configuration
type ProviderConfig struct {
	KubernetesVersions []string `yaml:"kubernetes_versions"`
}

// PostgresImages represents PostgreSQL image configuration
type PostgresImages struct {
	Registries      map[string]Registry `yaml:"registries"`
	DefaultRegistry string              `yaml:"default_registry"`
	SpockVersion    string              `yaml:"spock_version"`
	Variants        []ImageVariant      `yaml:"variants"`
}

// Registry represents a container registry configuration
type Registry struct {
	Name        string `yaml:"name"`
	Base        string `yaml:"base"`
	Description string `yaml:"description"`
	URL         string `yaml:"url"`
}

// ImageVariant represents a PostgreSQL image variant
type ImageVariant struct {
	Name        string `yaml:"name"`
	TagSuffix   string `yaml:"tag_suffix"`
	Description string `yaml:"description"`
}

// TestDefaults represents default test execution settings
type TestDefaults struct {
	FeatureType string `yaml:"feature_type"`
	TestDepth   int    `yaml:"test_depth"`
	Provider    string `yaml:"provider"`
}

// KindDefaults represents Kind cluster defaults
type KindDefaults struct {
	Image      string         `yaml:"image"`
	Nodes      int            `yaml:"nodes"`
	Networking KindNetworking `yaml:"networking"`
	Storage    StorageConfig  `yaml:"storage"`
}

// KindNetworking represents Kind networking configuration
type KindNetworking struct {
	ServiceSubnet string `yaml:"service_subnet"`
	PodSubnet     string `yaml:"pod_subnet"`
}

// StorageConfig represents storage configuration
type StorageConfig struct {
	DefaultClass  string `yaml:"default_class"`
	CSIClass      string `yaml:"csi_class"`
	SnapshotClass string `yaml:"snapshot_class"`
}

// KubernetesVersion represents K8s version-specific configuration
type KubernetesVersion struct {
	Manifests []Manifest `yaml:"manifests"`
}

// Manifest represents a single manifest to apply for cluster setup
type Manifest struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// GetManifests returns the list of manifests for a given K8s version
func (c *Config) GetManifests(k8sVersion string) []Manifest {
	// Extract major.minor from version like "v1.32.0" or "1.32"
	var majorMinor string
	for i := 0; i < len(k8sVersion); i++ {
		if k8sVersion[i] >= '0' && k8sVersion[i] <= '9' {
			// Found start of version number
			end := i
			dotCount := 0
			for end < len(k8sVersion) && dotCount < 2 {
				if k8sVersion[end] == '.' {
					dotCount++
					if dotCount == 2 {
						break
					}
				}
				end++
			}
			majorMinor = k8sVersion[i:end]
			break
		}
	}

	// Look up manifests for this K8s version
	if versionConfig, ok := c.KubernetesVersions[majorMinor]; ok {
		return versionConfig.Manifests
	}

	// Fallback to default version
	if versionConfig, ok := c.KubernetesVersions[c.DefaultKubernetesVersion]; ok {
		return versionConfig.Manifests
	}

	// Return empty list if nothing found
	return []Manifest{}
}

// LoadConfig loads the configuration from versions.yaml
func LoadConfig() (*Config, error) {
	// Try to find versions.yaml from current directory or parent
	configPath := "config/versions.yaml"

	// Check if running from tests directory
	if _, err := os.Stat(configPath); err != nil {
		// Try from parent (project root)
		configPath = "tests/config/versions.yaml"
		if _, err := os.Stat(configPath); err != nil {
			return nil, fmt.Errorf("failed to find config file: %w", err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// GetPostgresImageName constructs the full PostgreSQL image name
func (c *Config) GetPostgresImageName(registry, version, variant string) string {
	reg, ok := c.PostgresImages.Registries[registry]
	if !ok {
		reg = c.PostgresImages.Registries[c.PostgresImages.DefaultRegistry]
	}

	variantSuffix := ""
	for _, v := range c.PostgresImages.Variants {
		if v.Name == variant {
			variantSuffix = v.TagSuffix
			break
		}
	}

	return fmt.Sprintf("%s:%s-%s%s",
		reg.Base,
		version,
		c.PostgresImages.SpockVersion,
		variantSuffix,
	)
}

// GetCNPGVersion returns the configuration for a specific CNPG version
func (c *Config) GetCNPGVersion(version string) (*CNPGVersion, error) {
	for _, v := range c.CNPGVersions {
		if v.Version == version {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("CNPG version %s not found in configuration", version)
}

// GetOperatorImageName returns the full operator image name
func (v *CNPGVersion) GetOperatorImageName() string {
	return v.OperatorImage
}

// GetCNPGVersionFromEnv returns the CNPG version from environment or the first one as default
func (c *Config) GetCNPGVersionFromEnv() (*CNPGVersion, error) {
	version := os.Getenv("CNPG_VERSION")
	if version == "" {
		// Return first version as default
		if len(c.CNPGVersions) == 0 {
			return nil, fmt.Errorf("no CNPG versions defined in configuration")
		}
		return &c.CNPGVersions[0], nil
	}

	return c.GetCNPGVersion(version)
}
