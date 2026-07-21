//go:build integration

// Package integration provides end-to-end tests for the zitadel-operator v1alpha2
// against a real Zitadel instance using envtest.
//
// Prerequisites:
//   - JWT key stored in system keyring: service="zitadel-operator", username="jwt-key"
//     Store:  secret-tool store --label='zitadel-operator jwt-key' service zitadel-operator username jwt-key < /path/to/key.json
//     Clean:  rm /path/to/key.json  # delete the file after storing to keyring
//     Verify: secret-tool lookup service zitadel-operator username jwt-key | head -c 20
//   - Config at ~/.config/zitadel-operator/config.yaml with domain, port, insecure
//
// Run: go test -tags=integration -v ./tests/integration/... -count=1
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
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/zalando/go-keyring"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"

	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/controller"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
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

	// testOrgID is the Zitadel org the binding credential belongs to.
	// v0.18 removes defaultOrganizationId, so tests that need a pre-existing
	// org reference it explicitly.
	testOrgID string
)

func TestMain(m *testing.M) {
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

	// Resolve the binding credential's own org — used by tests that need a
	// pre-existing org ID (defaultOrganizationId is removed in v0.18).
	myOrg, err := zitadelClient.Management().GetMyOrg(ctx, &management.GetMyOrgRequest{}) //nolint:staticcheck // SA1019: v1 Management API
	if err != nil {
		slog.Error("failed to resolve binding org", slog.Any("error", err))
		os.Exit(1)
	}
	testOrgID = myOrg.GetOrg().GetId()
	slog.Info("resolved test org", slog.String("orgId", testOrgID))

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
	if err := zitadelv1alpha2.AddToScheme(scheme.Scheme); err != nil {
		slog.Error("failed to add scheme", slog.Any("error", err))
		os.Exit(1)
	}

	// Create shared manager with all controllers.
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))

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
	if err := (&controller.OrganizationReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup OrganizationReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ProjectReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ProjectReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.OIDCAppReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup OIDCAppReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.MachineUserReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup MachineUserReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.UserGrantReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup UserGrantReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ActionTargetReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ActionTargetReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ActionExecutionReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ActionExecutionReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ProjectMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ProjectMemberReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.OrgMetadataReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup OrgMetadataReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DomainReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DomainReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ProjectGrantReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ProjectGrantReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.IdentityProviderReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup IdentityProviderReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.APIAppReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup APIAppReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.SAMLAppReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup SAMLAppReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ApplicationKeyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ApplicationKeyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.PersonalAccessTokenReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup PersonalAccessTokenReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ProjectGrantMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ProjectGrantMemberReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultLoginPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultLoginPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultDomainPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultDomainPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.GoogleIdPReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup GoogleIdPReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.LoginPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup LoginPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.PasswordComplexityPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup PasswordComplexityPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.LockoutPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup LockoutPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.EmailProviderReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup EmailProviderReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.HumanUserReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup HumanUserReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.OrgMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup OrgMemberReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.InstanceMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup InstanceMemberReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.LabelPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup LabelPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.NotificationPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup NotificationPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.PasswordAgePolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup PasswordAgePolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.SmsProviderReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup SmsProviderReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.GitHubIdPReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup GitHubIdPReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultLockoutPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultLockoutPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultPasswordComplexityPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultPasswordComplexityPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultPasswordAgePolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultPasswordAgePolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultNotificationPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultNotificationPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultLabelPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultLabelPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultPrivacyPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultPrivacyPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultOIDCSettingsReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultOIDCSettingsReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.PrivacyPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup PrivacyPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DefaultMessageTextReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DefaultMessageTextReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.MessageTextReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup MessageTextReconciler", slog.Any("error", err))
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

	slog.Info("test setup complete",
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
