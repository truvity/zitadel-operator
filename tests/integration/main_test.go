//go:build integration

// Package integration provides end-to-end tests for the zitadel-operator against a real Zitadel instance.
//
// Prerequisites:
//   - JWT key stored in system keyring: service="zitadel-operator", username="jwt-key"
//     (go-keyring uses service + username attributes)
//   - Config at ~/.config/zitadel-operator/config.yaml with domain, port, insecure
//
// Store key:
//
//	secret-tool store --label='zitadel-operator jwt-key' service zitadel-operator username jwt-key < /path/to/key.json
//
// Run: go test -tags=integration -v ./tests/integration/...
package integration

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/truvity/zitadel-operator/internal/zitadel"
	"github.com/zalando/go-keyring"
)

type testConfig struct {
	Domain   string `yaml:"domain"`
	Port     string `yaml:"port"`
	Insecure bool   `yaml:"insecure"`
}

var (
	zitadelClient *zitadel.Client
	cfg           testConfig
)

func TestMain(m *testing.M) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Load config from XDG path.
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("failed to get home dir", slog.Any("error", err))
		os.Exit(1)
	}

	configPath := home + "/.config/zitadel-operator/config.yaml"

	data, err := os.ReadFile(configPath)
	if err != nil {
		slog.Error("failed to read config", slog.String("path", configPath), slog.Any("error", err))
		slog.Error("create ~/.config/zitadel-operator/config.yaml with domain, port, insecure")
		os.Exit(1)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		slog.Error("failed to parse config", slog.Any("error", err))
		os.Exit(1)
	}

	if cfg.Domain == "" {
		slog.Error("config.domain is required")
		os.Exit(1)
	}

	if cfg.Port == "" {
		cfg.Port = "443"
	}

	// Load JWT key from system keyring via go-keyring.
	keyJSON, err := keyring.Get("zitadel-operator", "jwt-key")
	if err != nil {
		slog.Error("failed to read JWT key from keyring",
			slog.Any("error", err),
			slog.String("hint", "store with: secret-tool store --label='zitadel-operator jwt-key' service zitadel-operator username jwt-key < /path/to/key.json"),
		)
		os.Exit(1)
	}

	// Create the Zitadel client.
	ctx := context.Background()

	zitadelClient, err = zitadel.NewClient(ctx, &zitadel.ClientConfig{
		Domain:            cfg.Domain,
		Port:              cfg.Port,
		InsecurePlaintext: cfg.Insecure,
		KeyJSON:           []byte(keyJSON),
	})
	if err != nil {
		slog.Error("failed to create Zitadel client", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("test setup complete",
		slog.String("domain", cfg.Domain),
		slog.String("port", cfg.Port),
	)

	os.Exit(m.Run())
}
