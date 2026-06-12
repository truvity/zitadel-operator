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
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/controller"
	"github.com/truvity/zitadel-operator/internal/zitadel"
	"github.com/zalando/go-keyring"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
)

var (
	// zitadelClient is the direct Zitadel API client (shared by all tests).
	zitadelClient *zitadel.Client

	// cfg is the operator config loaded from ~/.config/zitadel-operator/config.yaml.
	cfg *config.Config

	// k8sClient is the envtest K8s client (shared by reconciler tests).
	k8sClient client.Client

	// mgrCancel stops the shared manager.
	mgrCancel context.CancelFunc
)

func TestMain(m *testing.M) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Auto-detect KUBEBUILDER_ASSETS if not set.
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

	// Load JWT key from system keyring.
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

	// Start shared envtest environment.
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	restCfg, err := testEnv.Start()
	if err != nil {
		slog.Error("failed to start envtest", slog.Any("error", err))
		os.Exit(1)
	}

	// Register CRDs.
	if err := zitadelv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		slog.Error("failed to add scheme", slog.Any("error", err))
		os.Exit(1)
	}

	// Create shared manager with all controllers.
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	})
	if err != nil {
		slog.Error("failed to create manager", slog.Any("error", err))
		os.Exit(1)
	}

	// Register controllers.
	if err := (&controller.ProjectReconciler{Client: mgr.GetClient(), Zitadel: zitadelClient}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ProjectReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.OIDCAppReconciler{Client: mgr.GetClient(), Zitadel: zitadelClient}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup OIDCAppReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	// Start manager.
	var mgrCtx context.Context
	mgrCtx, mgrCancel = context.WithCancel(ctx)
	go func() {
		if err := mgr.Start(mgrCtx); err != nil {
			slog.Error("manager stopped with error", slog.Any("error", err))
		}
	}()

	// Wait for cache sync.
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		slog.Error("cache sync timeout")
		os.Exit(1)
	}

	// Create shared k8s client.
	k8sClient, err = client.New(restCfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		slog.Error("failed to create k8s client", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("test setup complete",
		slog.String("domain", cfg.Domain),
		slog.String("port", cfg.Port),
	)

	// Run tests.
	code := m.Run()

	// Cleanup.
	mgrCancel()
	if err := testEnv.Stop(); err != nil {
		slog.Error("failed to stop envtest", slog.Any("error", err))
	}

	os.Exit(code)
}
