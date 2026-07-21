// Package config loads the operator configuration from a YAML file.
package config

import (
	"fmt"
	"os"
	"slices"

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

	// InstanceAlias is a stable logical name for the bound Zitadel instance
	// (e.g. "prod-internal"). It is the operator's identity for spec.instance
	// pins, ScopeMap spec.instance assertions and the SSA field manager —
	// so a later domain migration does not orphan pins or managed fields.
	// Defaults to Domain when empty.
	// +optional
	InstanceAlias string `yaml:"instanceAlias"`

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

	// OperatorNamespace is the namespace holding ScopeMap CRs and
	// delegation Secrets (the operator's own namespace, v0.18 scope maps).
	// Falls back to the POD_NAMESPACE env var when empty; startup fails when
	// neither is set (a silently disabled routing surface would be
	// fail-permissive).
	OperatorNamespace string `yaml:"operatorNamespace"`
}

// InstanceIdentity returns the operator's stable instance identity:
// InstanceAlias when set, otherwise Domain. Used for spec.instance pins,
// ScopeMap instance assertions and the SSA field-manager name.
func (c *Config) InstanceIdentity() string {
	if c.InstanceAlias != "" {
		return c.InstanceAlias
	}
	return c.Domain
}

// ManagementIdentity returns the operator deployment's management identity
// for the v0.19 ForeignManager guard (the zitadel.truvity.io/managed-by
// annotation stamped at adoption). Two operators serving the same instance
// (identical InstanceIdentity, so identical SSA field managers) still get
// distinct management identities:
//
//   - org-owner binding: "<instance>/org/<boundOrganizationId>" — the fleet
//     shape runs one operator per org, so the bound org is the natural,
//     restart- and upgrade-stable discriminator.
//   - iam-owner binding (or org not yet verified): "<instance>/ns/<operatorNamespace>"
//     — same-instance iam-owner operators are expected to keep disjoint
//     namespace sets; the operator namespace disambiguates the usual layout
//     of one deployment per namespace.
func (c *Config) ManagementIdentity() string {
	if c.Binding == BindingOrgOwner && c.BoundOrganizationId != "" {
		return c.InstanceIdentity() + "/org/" + c.BoundOrganizationId
	}
	return c.InstanceIdentity() + "/ns/" + c.OperatorNamespace
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
		"defaultOrganizationId": "there is no default scope anymore — route namespaces to organizations via ScopeMap objects",
		"projectScopeLabel":     "label-based project routing is superseded by ScopeMap rules",
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
	if cfg.OperatorNamespace == "" {
		// Scope maps are a security-relevant routing surface: silently
		// disabling them because the namespace could not be determined would
		// be fail-permissive. Fail fast instead.
		return nil, fmt.Errorf("config %q: operatorNamespace could not be determined (key unset and POD_NAMESPACE empty); set operatorNamespace to the namespace holding ScopeMap objects and delegation Secrets", path)
	}
	if len(cfg.WatchNamespaces) > 0 && !slices.Contains(cfg.WatchNamespaces, cfg.OperatorNamespace) {
		// Without this the map informers can never sync and every scoped CR
		// sits at MapsNotSynced with a non-obvious cause.
		return nil, fmt.Errorf("config %q: watchNamespaces must include the operator namespace %q (ScopeMap objects and delegation Secrets live there)", path, cfg.OperatorNamespace)
	}

	return &cfg, nil
}
