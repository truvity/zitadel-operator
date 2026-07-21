// Package config loads the operator configuration from a YAML file.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Binding levels assert what the operator's credential is (v0.18, INF-424).
const (
	// BindingIAMOwner is a credential with instance-level IAM_OWNER.
	BindingIAMOwner = "iam-owner"
	// BindingOrgOwner is a credential with ORG_OWNER on exactly one org.
	BindingOrgOwner = "org-owner"
)

// Config holds all operator configuration loaded from the config file.
type Config struct {
	// Domain is the Zitadel instance domain (e.g., "zitadel.truvity.xyz").
	Domain string `yaml:"domain"`

	// Binding asserts the credential's privilege level: "iam-owner" or
	// "org-owner" (required since v0.18). Verified at startup against
	// AuthService.ListMyMemberships; mismatch = crash before any reconcile.
	Binding string `yaml:"binding"`

	// BoundOrganizationId is the org of an org-owner binding, discovered at
	// startup verification. Not read from the config file.
	BoundOrganizationId string `yaml:"-"`

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

	// WatchNamespaces limits the operator to watch only these namespaces.
	// If empty, the operator watches all namespaces.
	WatchNamespaces []string `yaml:"watchNamespaces"`

	// OperatorNamespace is the namespace holding ZitadelScopeMap CRs and
	// delegation Secrets (the operator's own namespace, v0.18 scope maps).
	// Falls back to the POD_NAMESPACE env var when empty; if neither is set,
	// scope maps are disabled and the operator runs in passthrough mode.
	OperatorNamespace string `yaml:"operatorNamespace"`
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
	data, err := os.ReadFile(path) //nolint:gosec // config path from trusted --config flag
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	// v0.18 (INF-428): removed keys fail fast with a migration pointer
	// instead of being silently ignored.
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	removed := map[string]string{
		"defaultOrganizationId": "there is no default scope anymore — route namespaces to organizations via ZitadelScopeMap objects",
		"projectScopeLabel":     "label-based project routing is superseded by ZitadelScopeMap rules",
	}
	for key, hint := range removed {
		if _, ok := raw[key]; ok {
			return nil, fmt.Errorf("config %q: %q was removed in v0.18: %s (see docs/MIGRATION-0.18.md)", path, key, hint)
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	if cfg.Domain == "" {
		return nil, fmt.Errorf("config %q: domain is required", path)
	}

	switch cfg.Binding {
	case BindingIAMOwner, BindingOrgOwner:
	case "":
		return nil, fmt.Errorf("config %q: binding is required since v0.18 (set binding: iam-owner or binding: org-owner matching the mounted credential)", path)
	default:
		return nil, fmt.Errorf("config %q: binding %q is invalid (must be %q or %q)", path, cfg.Binding, BindingIAMOwner, BindingOrgOwner)
	}

	if cfg.Port == "" {
		cfg.Port = "443"
	}

	if cfg.OperatorNamespace == "" {
		cfg.OperatorNamespace = os.Getenv("POD_NAMESPACE")
	}

	return &cfg, nil
}
