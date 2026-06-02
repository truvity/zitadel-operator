package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/controller"
	"github.com/truvity/zitadel-operator/internal/zitadel"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(zitadelv1alpha1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr        string
		probeAddr          string
		enableLeaderElect  bool
		zitadelDomain      string
		zitadelPort        string
		zitadelInsecure    bool
		externalDomain     string
		jwtSecretName      string
		jwtSecretNamespace string
		jwtSecretKey       string
		mode               string
		project            string
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElect, "leader-elect", false, "Enable leader election for controller manager.")
	flag.StringVar(&zitadelDomain, "zitadel-domain", "", "The internal address of the Zitadel instance (e.g., zitadel.zitadel.svc.cluster.kernel).")
	flag.StringVar(&zitadelPort, "zitadel-port", "8080", "The port of the Zitadel instance.")
	flag.BoolVar(&zitadelInsecure, "zitadel-insecure", true, "Connect to Zitadel over plain HTTP (no TLS).")
	flag.StringVar(&externalDomain, "external-domain", "", "The external domain Zitadel is configured with (e.g., zitadel.truvity.xyz). When set, used as Host header while routing to zitadel-domain internally.")
	flag.StringVar(&jwtSecretName, "jwt-secret-name", "zitadel-admin-sa", "The name of the Secret containing the JWT key.")
	flag.StringVar(&jwtSecretNamespace, "jwt-secret-namespace", "zitadel", "The namespace of the Secret containing the JWT key.")
	flag.StringVar(&jwtSecretKey, "jwt-secret-key", "", "The data key within the Secret (default: <secretName>.json, matching Zitadel convention).")
	flag.StringVar(&mode, "mode", "iam-owner", "Operator mode: iam-owner or project-owner.")
	flag.StringVar(&project, "project", "", "Project name (required for project-owner mode).")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if zitadelDomain == "" {
		setupLog.Error(nil, "zitadel-domain is required")
		os.Exit(1)
	}

	if mode == "project-owner" && project == "" {
		setupLog.Error(nil, "project is required when mode is project-owner")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElect,
		LeaderElectionID:       "zitadel-operator.truvity.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Default jwt-secret-key to <secretName>.json (Zitadel convention).
	if jwtSecretKey == "" {
		jwtSecretKey = jwtSecretName + ".json"
	}

	setupLog.Info("connecting to Zitadel",
		"domain", zitadelDomain,
		"port", zitadelPort,
		"insecure", zitadelInsecure,
		"externalDomain", externalDomain,
		"mode", mode,
		"jwtSecretName", jwtSecretName,
		"jwtSecretNamespace", jwtSecretNamespace,
		"jwtSecretKey", jwtSecretKey,
	)

	// Read JWT key from Kubernetes Secret.
	zitadelClient, err := initZitadelClient(mgr, zitadelDomain, zitadelPort, zitadelInsecure, externalDomain, jwtSecretName, jwtSecretNamespace, jwtSecretKey)
	if err != nil {
		setupLog.Error(err, "unable to initialize Zitadel client",
			"domain", zitadelDomain,
			"port", zitadelPort,
			"insecure", zitadelInsecure,
			"externalDomain", externalDomain,
			"hint", "check CNP allows egress from zitadel-operator to zitadel namespace on port 8080, and ingress on the zitadel side",
		)
		os.Exit(1)
	}
	setupLog.Info("Zitadel client initialized successfully")

	// Register controllers.
	if err := (&controller.ProjectReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Project")
		os.Exit(1)
	}

	if err := (&controller.OIDCAppReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OIDCApp")
		os.Exit(1)
	}

	if err := (&controller.IdentityProviderReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "IdentityProvider")
		os.Exit(1)
	}

	if err := (&controller.MachineUserReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MachineUser")
		os.Exit(1)
	}

	if err := (&controller.OrganizationReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Organization")
		os.Exit(1)
	}

	if err := (&controller.LoginPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LoginPolicy")
		os.Exit(1)
	}

	if err := (&controller.ProjectGrantReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProjectGrant")
		os.Exit(1)
	}

	if err := (&controller.UserGrantReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "UserGrant")
		os.Exit(1)
	}

	if err := (&controller.ProjectMemberReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProjectMember")
		os.Exit(1)
	}

	if err := (&controller.ApplicationKeyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ApplicationKey")
		os.Exit(1)
	}

	if err := (&controller.PersonalAccessTokenReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PersonalAccessToken")
		os.Exit(1)
	}

	if err := (&controller.PasswordComplexityPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PasswordComplexityPolicy")
		os.Exit(1)
	}

	if err := (&controller.LockoutPolicyReconciler{
		Client:  mgr.GetClient(),
		Zitadel: zitadelClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LockoutPolicy")
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

// initZitadelClient reads the JWT key from a Kubernetes Secret and creates the Zitadel client.
func initZitadelClient(mgr ctrl.Manager, domain, port string, insecurePlaintext bool, externalDomain, secretName, secretNamespace, secretKey string) (*zitadel.Client, error) {
	// Use a direct client (not cached) to read the secret at startup.
	directClient, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return nil, fmt.Errorf("creating direct client: %w", err)
	}

	secret := &corev1.Secret{}
	if err := directClient.Get(context.Background(), types.NamespacedName{
		Name:      secretName,
		Namespace: secretNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("reading JWT secret %s/%s: %w", secretNamespace, secretName, err)
	}

	keyJSON, ok := secret.Data[secretKey]
	if !ok {
		return nil, fmt.Errorf("%q not found in secret %s/%s", secretKey, secretNamespace, secretName)
	}

	cfg := zitadel.ClientConfig{
		Domain:            domain,
		Port:              port,
		InsecurePlaintext: insecurePlaintext,
		KeyJSON:           keyJSON,
		ExternalDomain:    externalDomain,
	}

	zitadelClient, err := zitadel.NewClient(context.Background(), &cfg)
	if err != nil {
		return nil, fmt.Errorf("creating Zitadel client: %w", err)
	}

	return zitadelClient, nil
}
