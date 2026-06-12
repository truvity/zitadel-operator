// Package config loads the operator configuration from a YAML file.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all operator configuration loaded from the config file.
type Config struct {
	// Domain is the Zitadel instance domain (e.g., "zitadel.truvity.xyz").
	Domain string `yaml:"domain"`

	// Port is the Zitadel API port (default: "443").
	Port string `yaml:"port"`

	// Insecure connects to Zitadel over plain HTTP (no TLS).
	Insecure bool `yaml:"insecure"`

	// ExternalDomain is the canonical external domain Zitadel is configured with.
	// When set, enables split-horizon mode: connects to Domain:Port internally,
	// sends x-zitadel-instance-host header, signs JWT with https://ExternalDomain audience.
	ExternalDomain string `yaml:"externalDomain"`

	// KeyFile is the path to the JWT key JSON file for service account authentication.
	KeyFile string `yaml:"keyFile"`

	// DefaultOrganizationId is the default org for resources that omit organizationId.
	DefaultOrganizationId string `yaml:"defaultOrganizationId"`

	// WatchNamespaces limits the operator to watch only these namespaces.
	// If empty, the operator watches all namespaces.
	WatchNamespaces []string `yaml:"watchNamespaces"`
}

// DefaultConfigPath returns the default config file path (~/.config/zitadel-operator/config.yaml).
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/etc/zitadel-operator/config.yaml"
	}

	return home + "/.config/zitadel-operator/config.yaml"
}

// Load reads and parses the config file at the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if cfg.Domain == "" {
		return nil, fmt.Errorf("config %q: domain is required", path)
	}

	if cfg.Port == "" {
		cfg.Port = "443"
	}

	return &cfg, nil
}
