package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/controller"
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

func main() {
	var (
		configPath        string
		metricsAddr       string
		probeAddr         string
		enableLeaderElect bool
	)

	flag.StringVar(&configPath, "config", config.DefaultConfigPath(), "Path to operator config file.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElect, "leader-elect", false, "Enable leader election for controller manager.")

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
		"defaultOrganizationId", cfg.DefaultOrganizationId,
		"watchNamespaces", cfg.WatchNamespaces,
	)

	// Read JWT key from file.
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

	// Build manager options.
	mgrOpts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElect,
		LeaderElectionID:       "zitadel-operator.truvity.io",
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

	// Register controllers.
	if err := (&controller.OrganizationReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Organization")
		os.Exit(1)
	}

	if err := (&controller.ProjectReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Project")
		os.Exit(1)
	}

	if err := (&controller.OIDCAppReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OIDCApp")
		os.Exit(1)
	}

	if err := (&controller.MachineUserReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineUser")
		os.Exit(1)
	}

	if err := (&controller.UserGrantReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserGrant")
		os.Exit(1)
	}

	if err := (&controller.ActionTargetReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ActionTarget")
		os.Exit(1)
	}

	if err := (&controller.ActionExecutionReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ActionExecution")
		os.Exit(1)
	}

	if err := (&controller.ProjectMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProjectMember")
		os.Exit(1)
	}

	if err := (&controller.OrgMetadataReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OrgMetadata")
		os.Exit(1)
	}

	if err := (&controller.DomainReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Domain")
		os.Exit(1)
	}

	if err := (&controller.ProjectGrantReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProjectGrant")
		os.Exit(1)
	}

	if err := (&controller.IdentityProviderReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IdentityProvider")
		os.Exit(1)
	}

	if err := (&controller.APIAppReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "APIApp")
		os.Exit(1)
	}

	if err := (&controller.SAMLAppReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SAMLApp")
		os.Exit(1)
	}

	if err := (&controller.ApplicationKeyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ApplicationKey")
		os.Exit(1)
	}

	if err := (&controller.PersonalAccessTokenReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PersonalAccessToken")
		os.Exit(1)
	}

	if err := (&controller.ProjectGrantMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProjectGrantMember")
		os.Exit(1)
	}

	if err := (&controller.DefaultLoginPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultLoginPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultDomainPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultDomainPolicy")
		os.Exit(1)
	}

	if err := (&controller.GoogleIdPReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GoogleIdP")
		os.Exit(1)
	}

	if err := (&controller.LoginPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LoginPolicy")
		os.Exit(1)
	}

	if err := (&controller.PasswordComplexityPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PasswordComplexityPolicy")
		os.Exit(1)
	}

	if err := (&controller.LockoutPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LockoutPolicy")
		os.Exit(1)
	}

	if err := (&controller.EmailProviderReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EmailProvider")
		os.Exit(1)
	}

	if err := (&controller.HumanUserReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HumanUser")
		os.Exit(1)
	}

	if err := (&controller.OrgMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OrgMember")
		os.Exit(1)
	}

	if err := (&controller.InstanceMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "InstanceMember")
		os.Exit(1)
	}

	if err := (&controller.LabelPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LabelPolicy")
		os.Exit(1)
	}

	if err := (&controller.NotificationPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NotificationPolicy")
		os.Exit(1)
	}

	if err := (&controller.PasswordAgePolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PasswordAgePolicy")
		os.Exit(1)
	}

	if err := (&controller.SmsProviderReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SmsProvider")
		os.Exit(1)
	}

	if err := (&controller.GitHubIdPReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitHubIdP")
		os.Exit(1)
	}

	if err := (&controller.DefaultLockoutPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultLockoutPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultPasswordComplexityPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultPasswordComplexityPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultPasswordAgePolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultPasswordAgePolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultNotificationPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultNotificationPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultLabelPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultLabelPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultPrivacyPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultPrivacyPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultOIDCSettingsReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultOIDCSettings")
		os.Exit(1)
	}

	if err := (&controller.PrivacyPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PrivacyPolicy")
		os.Exit(1)
	}

	if err := (&controller.DefaultMessageTextReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DefaultMessageText")
		os.Exit(1)
	}

	if err := (&controller.MessageTextReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
		Config:  cfg,
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
