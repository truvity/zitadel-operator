package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeConfig(t, `
domain: test.zitadel.cloud
binding: iam-owner
port: "443"
insecure: false
externalDomain: auth.example.com
keyFile: /etc/zitadel/key.json
operatorNamespace: zitadel-operator
watchNamespaces:
  - ns1
  - ns2
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Domain != "test.zitadel.cloud" {
		t.Errorf("domain: got %q", cfg.Domain)
	}
	if cfg.Binding != BindingIAMOwner {
		t.Errorf("binding: got %q", cfg.Binding)
	}
	if cfg.Port != "443" {
		t.Errorf("port: got %q", cfg.Port)
	}
	if cfg.ExternalDomain != "auth.example.com" {
		t.Errorf("externalDomain: got %q", cfg.ExternalDomain)
	}
	if cfg.KeyFile != "/etc/zitadel/key.json" {
		t.Errorf("keyFile: got %q", cfg.KeyFile)
	}
	if cfg.OperatorNamespace != "zitadel-operator" {
		t.Errorf("operatorNamespace: got %q", cfg.OperatorNamespace)
	}
	if len(cfg.WatchNamespaces) != 2 || cfg.WatchNamespaces[0] != "ns1" {
		t.Errorf("watchNamespaces: got %v", cfg.WatchNamespaces)
	}
}

func TestLoad_DefaultPort(t *testing.T) {
	path := writeConfig(t, "domain: x.cloud\nbinding: org-owner\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "443" {
		t.Errorf("expected default port '443', got %q", cfg.Port)
	}
	if cfg.Binding != BindingOrgOwner {
		t.Errorf("binding: got %q", cfg.Binding)
	}
}

func TestLoad_MissingDomain(t *testing.T) {
	path := writeConfig(t, "port: '443'\nbinding: iam-owner\n")

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing domain")
	}
}

func TestLoad_MissingBinding(t *testing.T) {
	path := writeConfig(t, "domain: x.cloud\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing binding (required since v0.18)")
	}
	if !strings.Contains(err.Error(), "binding is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_InvalidBinding(t *testing.T) {
	path := writeConfig(t, "domain: x.cloud\nbinding: superuser\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid binding value")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLoad_RemovedKeysFailFast is the INF-428 breaking-config scenario:
// the operator refuses to start when a v0.17 key is still present, pointing
// at the migration guide instead of silently ignoring it.
func TestLoad_RemovedKeysFailFast(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"defaultOrganizationId", "domain: x.cloud\nbinding: iam-owner\ndefaultOrganizationId: \"123\"\n"},
		{"projectScopeLabel", "domain: x.cloud\nbinding: iam-owner\nprojectScopeLabel: team\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, tc.content)
			_, err := Load(path)
			if err == nil {
				t.Fatalf("expected fail-fast error for removed key %s", tc.name)
			}
			if !strings.Contains(err.Error(), "removed in v0.18") || !strings.Contains(err.Error(), "MIGRATION-0.18") {
				t.Fatalf("error must point at the migration guide, got: %v", err)
			}
		})
	}
}

func TestLoad_OperatorNamespaceFromEnv(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "pod-ns-fallback")
	path := writeConfig(t, "domain: x.cloud\nbinding: iam-owner\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OperatorNamespace != "pod-ns-fallback" {
		t.Errorf("operatorNamespace fallback: got %q", cfg.OperatorNamespace)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeConfig(t, "invalid: [yaml: broken")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
