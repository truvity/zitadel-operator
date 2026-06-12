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
	"os/exec"
	"strings"
	"testing"

	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"
	"github.com/zalando/go-keyring"
)

var (
	zitadelClient *zitadel.Client
	cfg           *config.Config
)

func TestMain(m *testing.M) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Auto-detect KUBEBUILDER_ASSETS if not set (requires setup-envtest in PATH via devbox).
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		out, err := exec.Command("setup-envtest", "use", "--print", "path", "-p", "path").Output()
		if err != nil {
			slog.Error("KUBEBUILDER_ASSETS not set and setup-envtest failed",
				slog.Any("error", err),
				slog.String("hint", "run 'devbox shell' to get setup-envtest in PATH"),
			)
			os.Exit(1)
		}

		path := strings.TrimSpace(string(out))
		os.Setenv("KUBEBUILDER_ASSETS", path)
		slog.Info("auto-detected KUBEBUILDER_ASSETS", slog.String("path", path))
	}

	// Load config.
	configPath := config.DefaultConfigPath()
	var err error
	cfg, err = config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		os.Exit(1)
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
