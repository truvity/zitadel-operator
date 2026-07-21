package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/settings"
)

// DefaultOIDCSettingsReconciler reconciles a DefaultOIDCSettings object.
type DefaultOIDCSettingsReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultoidcsettings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultoidcsettings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=defaultoidcsettings/finalizers,verbs=update

func (r *DefaultOIDCSettingsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.DefaultOIDCSettings
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// INF-424 degradation matrix: instance-level resources are not supported
	// under an org-owner binding (no-op during deletion so finalizers complete).
	if done, result, err := checkBindingLevel(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, &cr.Status.Ready); done {
		return result, err
	}

	// Singleton conflict detection.
	if conflict, err := r.checkConflict(ctx, &cr); err != nil || conflict {
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	}

	// Deletion.
	if done, result, err := handleSingletonDeletion(ctx, r.Client, &cr, func() {
		_, _ = r.Zitadel.Admin().UpdateOIDCSettings(ctx, &admin.UpdateOIDCSettingsRequest{
			AccessTokenLifetime:        durationpb.New(12 * time.Hour),
			IdTokenLifetime:            durationpb.New(12 * time.Hour),
			RefreshTokenIdleExpiration: durationpb.New(720 * time.Hour),
			RefreshTokenExpiration:     durationpb.New(2160 * time.Hour),
		})
		logger.Info("reset instance OIDC settings to defaults (reset-on-delete annotation present)")
	}); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Business logic.
	if err := r.reconcileSpec(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Status.
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, false); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("defaultoidcsettings reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DefaultOIDCSettingsReconciler) checkConflict(ctx context.Context, cr *zitadelv1alpha2.DefaultOIDCSettings) (bool, error) {
	var list zitadelv1alpha2.DefaultOIDCSettingsList
	if err := r.List(ctx, &list); err != nil {
		return false, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultOIDCSettings") {
		_ = applyStatus(ctx, r.Client, r.Config, cr)
		return true, nil
	}
	return false, nil
}

func (r *DefaultOIDCSettingsReconciler) reconcileSpec(ctx context.Context, cr *zitadelv1alpha2.DefaultOIDCSettings) error {
	logger := log.FromContext(ctx)

	accessTokenLifetime, err := parseDurationOrDefault(cr.Spec.AccessTokenLifetime, 12*time.Hour)
	if err != nil {
		return fmt.Errorf("parsing accessTokenLifetime: %w", err)
	}
	idTokenLifetime, err := parseDurationOrDefault(cr.Spec.IdTokenLifetime, 12*time.Hour)
	if err != nil {
		return fmt.Errorf("parsing idTokenLifetime: %w", err)
	}
	refreshTokenIdleExpiration, err := parseDurationOrDefault(cr.Spec.RefreshTokenIdleExpiration, 720*time.Hour)
	if err != nil {
		return fmt.Errorf("parsing refreshTokenIdleExpiration: %w", err)
	}
	refreshTokenExpiration, err := parseDurationOrDefault(cr.Spec.RefreshTokenExpiration, 2160*time.Hour)
	if err != nil {
		return fmt.Errorf("parsing refreshTokenExpiration: %w", err)
	}

	current, err := r.Zitadel.Admin().GetOIDCSettings(ctx, &admin.GetOIDCSettingsRequest{})
	if err != nil {
		return fmt.Errorf("getting OIDC settings: %w", err)
	}

	if r.hasDrift(current.GetSettings(), accessTokenLifetime, idTokenLifetime, refreshTokenIdleExpiration, refreshTokenExpiration) {
		_, err := r.Zitadel.Admin().UpdateOIDCSettings(ctx, &admin.UpdateOIDCSettingsRequest{
			AccessTokenLifetime:        durationpb.New(accessTokenLifetime),
			IdTokenLifetime:            durationpb.New(idTokenLifetime),
			RefreshTokenIdleExpiration: durationpb.New(refreshTokenIdleExpiration),
			RefreshTokenExpiration:     durationpb.New(refreshTokenExpiration),
		})
		if err != nil {
			return fmt.Errorf("updating OIDC settings: %w", err)
		}
		logger.Info("default OIDC settings updated (drift detected)")
	}
	return nil
}

// hasDrift checks if the current OIDC settings differ from the desired state.
func (r *DefaultOIDCSettingsReconciler) hasDrift(s *settings.OIDCSettings, accessTokenLifetime, idTokenLifetime, refreshTokenIdleExpiration, refreshTokenExpiration time.Duration) bool {
	if s == nil {
		return true
	}
	if s.GetAccessTokenLifetime() == nil || s.GetAccessTokenLifetime().AsDuration() != accessTokenLifetime {
		return true
	}
	if s.GetIdTokenLifetime() == nil || s.GetIdTokenLifetime().AsDuration() != idTokenLifetime {
		return true
	}
	if s.GetRefreshTokenIdleExpiration() == nil || s.GetRefreshTokenIdleExpiration().AsDuration() != refreshTokenIdleExpiration {
		return true
	}
	if s.GetRefreshTokenExpiration() == nil || s.GetRefreshTokenExpiration().AsDuration() != refreshTokenExpiration {
		return true
	}
	return false
}

// parseDurationOrDefault parses a duration string, returning a default if the string is empty.
func parseDurationOrDefault(s string, defaultVal time.Duration) (time.Duration, error) {
	if s == "" {
		return defaultVal, nil
	}
	return time.ParseDuration(s)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DefaultOIDCSettingsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.DefaultOIDCSettings{}).
		Named("defaultoidcsettings").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
