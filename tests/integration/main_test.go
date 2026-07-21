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
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/zalando/go-keyring"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"

	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/controller"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

// operatorNamespace hosts ScopeMaps and delegation Secrets in the
// v0.18 integration tests.
const operatorNamespace = "zitadel-operator-system"

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
	// v0.18 removed defaultOrganizationId, so tests that need a pre-existing
	// org reference it explicitly.
	testOrgID string

	// bindingUserID is the userId of the operator's binding credential
	// (from the JWT key), used by the actor-proof test.
	bindingUserID string

	// delegationMgr mints per-scope delegated clients (v0.18).
	delegationMgr *delegation.Manager

	// scopeResolver resolves namespaces to scopes (v0.18).
	scopeResolver *scopemap.Resolver

	// delegationGC sweeps delegates (v0.18).
	delegationGC *delegation.GC

	// testRestCfg is the envtest REST config (extra managers in tests).
	testRestCfg *rest.Config
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

	// v0.18 (INF-424): the harness verifies its binding like production does.
	if _, err := zitadel.VerifyBinding(ctx, zitadelClient, cfg.Binding); err != nil {
		slog.Error("binding verification failed", slog.Any("error", err), slog.String("binding", cfg.Binding))
		os.Exit(1)
	}

	// Resolve the binding credential's own org — used by tests that need a
	// pre-existing org ID (defaultOrganizationId was removed in v0.18).
	myOrg, err := zitadelClient.Management().GetMyOrg(ctx, &management.GetMyOrgRequest{}) //nolint:staticcheck // SA1019: v1 Management API
	if err != nil {
		slog.Error("failed to resolve binding org", slog.Any("error", err))
		os.Exit(1)
	}
	testOrgID = myOrg.GetOrg().GetId()
	slog.Info("resolved test org", slog.String("orgId", testOrgID))

	// Extract the binding credential's userId (for the actor-proof test).
	var keyMeta struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal([]byte(keyJSON), &keyMeta); err != nil {
		slog.Error("failed to parse JWT key JSON", slog.Any("error", err))
		os.Exit(1)
	}
	bindingUserID = keyMeta.UserID

	// Start shared envtest environment.
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	restCfg, err := testEnv.Start()
	testRestCfg = restCfg
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

	// v0.18 scope maps: resolver + delegation wired into all tenant
	// reconcilers. With zero ScopeMaps present this is passthrough,
	// so all pre-v0.18 tests behave unchanged.
	cacheSynced := &atomic.Bool{}
	if err := mgr.Add(manager.RunnableFunc(func(runCtx context.Context) error {
		if mgr.GetCache().WaitForCacheSync(runCtx) {
			cacheSynced.Store(true)
		}
		<-runCtx.Done()
		return nil
	})); err != nil {
		slog.Error("failed to add cache-sync tracker", slog.Any("error", err))
		os.Exit(1)
	}
	scopeResolver = &scopemap.Resolver{
		Reader:    mgr.GetClient(),
		Namespace: operatorNamespace,
		Instance:  cfg.InstanceIdentity(),
		Synced:    cacheSynced.Load,
		Recorder:  mgr.GetEventRecorderFor("scopemap-resolver"),
	}
	delegationMgr = &delegation.Manager{
		K8s:     mgr.GetClient(),
		Binding: zitadelClient,
		ClientCfg: zitadel.ClientConfig{
			Domain:            cfg.Domain,
			Port:              cfg.Port,
			InsecurePlaintext: cfg.Insecure,
		},
		Namespace: operatorNamespace,
		// Keep minted test resources identifiable on the shared test instance.
		UsernamePrefix: "v018-delegate-",
	}
	delegationGC = &delegation.GC{
		K8s:      mgr.GetClient(),
		Resolver: scopeResolver,
		Manager:  delegationMgr,
	}
	if err := (&controller.ScopeMapReconciler{
		Client:    mgr.GetClient(),
		Zitadel:   zitadelClient,
		Config:    cfg,
		Instance:  cfg.InstanceIdentity(),
		Namespace: operatorNamespace,
		Recorder:  mgr.GetEventRecorderFor("scopemap"),
		GC:        delegationGC,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ScopeMapReconciler", slog.Any("error", err))
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
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ProjectReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ProjectRoleReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ProjectRoleReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.OIDCAppReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup OIDCAppReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.MachineUserReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup MachineUserReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.UserGrantReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
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
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ProjectMemberReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.OrgMetadataReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup OrgMetadataReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.DomainReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup DomainReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ProjectGrantReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ProjectGrantReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.IdentityProviderReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup IdentityProviderReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.APIAppReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup APIAppReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.SAMLAppReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup SAMLAppReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ApplicationKeyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup ApplicationKeyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.PersonalAccessTokenReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup PersonalAccessTokenReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.ProjectGrantMemberReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
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
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup LoginPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.PasswordComplexityPolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup PasswordComplexityPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.LockoutPolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
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
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup HumanUserReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.OrgMemberReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
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
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup LabelPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.NotificationPolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup NotificationPolicyReconciler", slog.Any("error", err))
		os.Exit(1)
	}

	if err := (&controller.PasswordAgePolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
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
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
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
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
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

	// Create the operator namespace (holds scope maps + delegation Secrets).
	opNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: operatorNamespace}}
	if err := k8sClient.Create(ctx, opNs); err != nil {
		slog.Error("failed to create operator namespace", slog.Any("error", err))
		os.Exit(1)
	}

	slog.Info("test setup complete",
		slog.String("domain", cfg.Domain),
		slog.String("port", cfg.Port),
	)

	// Run tests.
	code := m.Run()

	// Test-instance hygiene: sweep leaked test resources ONLY after a fully
	// successful run. On any failure everything is preserved for debugging
	// (rerun with V018_HYGIENE=1 or a green run to clean up afterwards).
	if code == 0 {
		if summary, err := sweepStaleTestResources(ctx); err != nil {
			slog.Warn("post-run hygiene sweep had failures", slog.String("summary", summary), slog.Any("error", err))
		} else {
			slog.Info("post-run hygiene sweep", slog.String("summary", summary))
		}
	} else {
		slog.Info("suite failed: skipping hygiene sweep, test resources preserved for debugging")
	}

	// Cleanup.
	mgrCancel()
	if err := testEnv.Stop(); err != nil {
		slog.Error("failed to stop envtest", slog.Any("error", err))
	}

	os.Exit(code)
}
