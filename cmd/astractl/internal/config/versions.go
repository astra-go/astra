package config

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed versions.yaml
var versionsFS embed.FS

// ServiceVersions holds the recommended versions for external services.
type ServiceVersions struct {
	Services map[string]string `yaml:"services"`
}

// LoadServiceVersions loads the service versions from the embedded versions.yaml file.
func LoadServiceVersions() (*ServiceVersions, error) {
	data, err := versionsFS.ReadFile("versions.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read versions.yaml: %w", err)
	}

	var versions ServiceVersions
	if err := yaml.Unmarshal(data, &versions); err != nil {
		return nil, fmt.Errorf("failed to parse versions.yaml: %w", err)
	}

	return &versions, nil
}

// GetVersion returns the recommended version for a service.
func (v *ServiceVersions) GetVersion(service string) (string, error) {
	version, ok := v.Services[service]
	if !ok {
		return "", fmt.Errorf("no version configured for service: %s", service)
	}
	return version, nil
}
