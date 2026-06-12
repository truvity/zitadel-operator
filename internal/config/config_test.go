package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
domain: test.zitadel.cloud
port: "443"
insecure: false
externalDomain: auth.example.com
keyFile: /etc/zitadel/key.json
defaultOrganizationId: "12345"
watchNamespaces:
  - ns1
  - ns2
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Domain != "test.zitadel.cloud" {
		t.Errorf("domain: got %q", cfg.Domain)
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

	if cfg.DefaultOrganizationId != "12345" {
		t.Errorf("defaultOrganizationId: got %q", cfg.DefaultOrganizationId)
	}

	if len(cfg.WatchNamespaces) != 2 || cfg.WatchNamespaces[0] != "ns1" {
		t.Errorf("watchNamespaces: got %v", cfg.WatchNamespaces)
	}
}

func TestLoad_DefaultPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("domain: x.cloud\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Port != "443" {
		t.Errorf("expected default port '443', got %q", cfg.Port)
	}
}

func TestLoad_MissingDomain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("port: '443'\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing domain")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("invalid: [yaml: broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
