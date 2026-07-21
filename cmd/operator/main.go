package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync/atomic"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/controller"
	"github.com/truvity/zitadel-operator/internal/delegation"
	"github.com/truvity/zitadel-operator/internal/scopemap"
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(zitadelv1alpha2.AddToScheme(scheme))
}

func main() { //nolint:gocyclo // controller registration is inherently sequential
	var (
		configPath        string
		metricsAddr       string
		probeAddr         string
		enableLeaderElect bool
		leaderElectionID  string
	)

	flag.StringVar(&configPath, "config", config.DefaultConfigPath(), "Path to operator config file.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElect, "leader-elect", true,
		"Enable leader election (INF-427: on by default — even replicas=1 runs two pods during a rolling update).")
	flag.StringVar(&leaderElectionID, "leader-election-id", "zitadel-operator.truvity.io",
		"Leader election lease name. The Helm chart derives it from the release fullname so two deployments in one namespace get distinct leases.")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Load config.
	cfg, err := config.Load(configPath)
	if err != nil {
		setupLog.Error(err, "unable to load config", "path", configPath)
		os.Exit(1)
	}

	setupLog.Info("config loaded",
		"domain", cfg.Domain,
		"port", cfg.Port,
		"insecure", cfg.Insecure,
		"externalDomain", cfg.ExternalDomain,
		"binding", cfg.Binding,
		"watchNamespaces", cfg.WatchNamespaces,
		"operatorNamespace", cfg.OperatorNamespace,
	)

	// Read JWT key from file (fail fast on a missing path rather than a
	// confusing 'open : no such file' later).
	if cfg.KeyFile == "" {
		setupLog.Error(nil, "config keyFile is required (path to the mounted JWT key JSON)")
		os.Exit(1)
	}
	keyJSON, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		setupLog.Error(err, "unable to read key file", "path", cfg.KeyFile)
		os.Exit(1)
	}

	// Create Zitadel client.
	zitadelClient, err := zitadel.NewClient(context.Background(), &zitadel.ClientConfig{
		Domain:            cfg.Domain,
		Port:              cfg.Port,
		InsecurePlaintext: cfg.Insecure,
		KeyJSON:           keyJSON,
		ExternalDomain:    cfg.ExternalDomain,
	})
	if err != nil {
		setupLog.Error(err, "unable to create Zitadel client")
		os.Exit(1)
	}
	setupLog.Info("Zitadel client initialized successfully")

	// v0.18 (INF-424): verify the binding assertion against the credential's
	// actual memberships. Mismatch = crash early, before any reconcile.
	boundOrgID, err := zitadel.VerifyBinding(context.Background(), zitadelClient, cfg.Binding)
	if err != nil {
		setupLog.Error(err, "binding verification failed", "binding", cfg.Binding)
		os.Exit(1)
	}
	cfg.BoundOrganizationId = boundOrgID
	setupLog.Info("binding verified", "binding", cfg.Binding, "boundOrganizationId", boundOrgID)

	// Build manager options.
	mgrOpts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElect,
		LeaderElectionID:       leaderElectionID,
	}

	// Namespace-scoped watching.
	if len(cfg.WatchNamespaces) > 0 {
		defaultNs := make(map[string]cache.Config, len(cfg.WatchNamespaces))
		for _, ns := range cfg.WatchNamespaces {
			defaultNs[ns] = cache.Config{}
		}
		mgrOpts.Cache = cache.Options{
			DefaultNamespaces: defaultNs,
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// v0.18 scope maps (INF-423) + internal delegation (INF-425): active when
	// the operator namespace is known. With zero ScopeMap objects the
	// resolver is passthrough (legacy binding-client behavior).
	var scopeResolver *scopemap.Resolver
	var delegationMgr *delegation.Manager
	if cfg.OperatorNamespace != "" {
		synced := &atomic.Bool{}
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			if mgr.GetCache().WaitForCacheSync(ctx) {
				synced.Store(true)
			}
			<-ctx.Done()
			return nil
		})); err != nil {
			setupLog.Error(err, "unable to add cache-sync tracker")
			os.Exit(1)
		}
		scopeResolver = &scopemap.Resolver{
			Reader:    mgr.GetClient(),
			Namespace: cfg.OperatorNamespace,
			Instance:  cfg.InstanceIdentity(),
			Synced:    synced.Load,
			Recorder:  mgr.GetEventRecorderFor("scopemap-resolver"),
		}
		delegationMgr = &delegation.Manager{
			K8s:     mgr.GetClient(),
			Binding: zitadelClient,
			ClientCfg: zitadel.ClientConfig{
				Domain:            cfg.Domain,
				Port:              cfg.Port,
				InsecurePlaintext: cfg.Insecure,
				ExternalDomain:    cfg.ExternalDomain,
			},
			Namespace: cfg.OperatorNamespace,
		}
		gc := &delegation.GC{
			K8s:      mgr.GetClient(),
			Resolver: scopeResolver,
			Manager:  delegationMgr,
		}
		// Warm-restart (list delegation Secrets by label) then periodic
		// orphan GC. Leadership-gated like all controllers.
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			if err := delegationMgr.WarmFromSecrets(ctx); err != nil {
				setupLog.Error(err, "warming delegates from Secrets failed (lazy re-mint will recover)")
			}
			return gc.Run(ctx)
		})); err != nil {
			setupLog.Error(err, "unable to add delegation GC runnable")
			os.Exit(1)
		}
		if err := (&controller.ScopeMapReconciler{
			Client:    mgr.GetClient(),
			Zitadel:   zitadelClient,
			Config:    cfg,
			Instance:  cfg.InstanceIdentity(),
			Namespace: cfg.OperatorNamespace,
			Recorder:  mgr.GetEventRecorderFor("scopemap"),
			GC:        gc,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ScopeMap")
			os.Exit(1)
		}
		setupLog.Info("v0.18 scope maps enabled", "operatorNamespace", cfg.OperatorNamespace)
	} else {
		setupLog.Info("operatorNamespace not set; scope maps disabled (passthrough mode)")
	}

	// Register controllers.
	if err := (&controller.OrganizationReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Organization")
		os.Exit(1)
	}

	if err := (&controller.ProjectReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Project")
		os.Exit(1)
	}

	if err := (&controller.OIDCAppReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OIDCApp")
		os.Exit(1)
	}

	if err := (&controller.MachineUserReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineUser")
		os.Exit(1)
	}

	if err := (&controller.UserGrantReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserGrant")
		os.Exit(1)
	}

	if err := (&controller.ActionTargetReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ActionTarget")
		os.Exit(1)
	}

	if err := (&controller.ActionExecutionReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ActionExecution")
		os.Exit(1)
	}

	if err := (&controller.ProjectMemberReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProjectMember")
		os.Exit(1)
	}

	if err := (&controller.OrgMetadataReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OrgMetadata")
		os.Exit(1)
	}

	if err := (&controller.DomainReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Domain")
		os.Exit(1)
	}

	if err := (&controller.ProjectGrantReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProjectGrant")
		os.Exit(1)
	}

	if err := (&controller.IdentityProviderReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IdentityProvider")
		os.Exit(1)
	}

	if err := (&controller.APIAppReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "APIApp")
		os.Exit(1)
	}

	if err := (&controller.SAMLAppReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SAMLApp")
		os.Exit(1)
	}

	if err := (&controller.ApplicationKeyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ApplicationKey")
		os.Exit(1)
	}

	if err := (&controller.PersonalAccessTokenReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PersonalAccessToken")
		os.Exit(1)
	}

	if err := (&controller.ProjectGrantMemberReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProjectGrantMember")
		os.Exit(1)
	}

	if err := (&controller.DefaultLoginPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultLoginPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultDomainPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultDomainPolicy")
		os.Exit(1)
	}

	if err := (&controller.GoogleIdPReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GoogleIdP")
		os.Exit(1)
	}

	if err := (&controller.LoginPolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LoginPolicy")
		os.Exit(1)
	}

	if err := (&controller.PasswordComplexityPolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PasswordComplexityPolicy")
		os.Exit(1)
	}

	if err := (&controller.LockoutPolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LockoutPolicy")
		os.Exit(1)
	}

	if err := (&controller.EmailProviderReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EmailProvider")
		os.Exit(1)
	}

	if err := (&controller.HumanUserReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HumanUser")
		os.Exit(1)
	}

	if err := (&controller.OrgMemberReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OrgMember")
		os.Exit(1)
	}

	if err := (&controller.InstanceMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "InstanceMember")
		os.Exit(1)
	}

	if err := (&controller.LabelPolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LabelPolicy")
		os.Exit(1)
	}

	if err := (&controller.NotificationPolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NotificationPolicy")
		os.Exit(1)
	}

	if err := (&controller.PasswordAgePolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PasswordAgePolicy")
		os.Exit(1)
	}

	if err := (&controller.SmsProviderReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SmsProvider")
		os.Exit(1)
	}

	if err := (&controller.GitHubIdPReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitHubIdP")
		os.Exit(1)
	}

	if err := (&controller.DefaultLockoutPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultLockoutPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultPasswordComplexityPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultPasswordComplexityPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultPasswordAgePolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultPasswordAgePolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultNotificationPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultNotificationPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultLabelPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultLabelPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultPrivacyPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultPrivacyPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultOIDCSettingsReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultOIDCSettings")
		os.Exit(1)
	}

	if err := (&controller.PrivacyPolicyReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PrivacyPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultMessageTextReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultMessageText")
		os.Exit(1)
	}

	if err := (&controller.MessageTextReconciler{
		Client:     mgr.GetClient(),
		Zitadel:    zitadelClient,
		Config:     cfg,
		Resolver:   scopeResolver,
		Delegation: delegationMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MessageText")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// Version is set via ldflags at build time.
var Version = "dev"

func init() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("zitadel-operator %s\n", Version)
			os.Exit(0)
		}
	}
}
