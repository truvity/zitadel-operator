package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/settings"
)

// DefaultOIDCSettingsReconciler reconciles a DefaultOIDCSettings object.
type DefaultOIDCSettingsReconciler struct {
	client.Client
	Zitadel *zitadel.Client
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

	// Singleton conflict detection: only the earliest-created CR manages the instance.
	var list zitadelv1alpha2.DefaultOIDCSettingsList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}
	candidates := make([]singletonCandidate, len(list.Items))
	for i := range list.Items {
		candidates[i] = singletonCandidate{UID: list.Items[i].UID, Name: list.Items[i].Name, Namespace: list.Items[i].Namespace, CreationTimestamp: list.Items[i].CreationTimestamp, IsDeleting: !list.Items[i].DeletionTimestamp.IsZero()}
	}
	if checkSingletonConflict(&cr, candidates, &cr.Status.Conditions, &cr.Status.Ready, "DefaultOIDCSettings") {
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if shouldResetOnDelete(&cr) {
			// Zitadel documented instance defaults: 12h/12h/720h/2160h.
			_, _ = r.Zitadel.Admin().UpdateOIDCSettings(ctx, &admin.UpdateOIDCSettingsRequest{
				AccessTokenLifetime:        durationpb.New(12 * time.Hour),
				IdTokenLifetime:            durationpb.New(12 * time.Hour),
				RefreshTokenIdleExpiration: durationpb.New(720 * time.Hour),
				RefreshTokenExpiration:     durationpb.New(2160 * time.Hour),
			})
			logger.Info("reset instance OIDC settings to defaults (reset-on-delete annotation present)")
		}
		if removeFinalizer(&cr) {
			if err := r.Update(ctx, &cr); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer.
	if addFinalizer(&cr) {
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Parse desired durations from spec.
	accessTokenLifetime, err := parseDurationOrDefault(cr.Spec.AccessTokenLifetime, 12*time.Hour)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("parsing accessTokenLifetime: %w", err)
	}
	idTokenLifetime, err := parseDurationOrDefault(cr.Spec.IdTokenLifetime, 12*time.Hour)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("parsing idTokenLifetime: %w", err)
	}
	refreshTokenIdleExpiration, err := parseDurationOrDefault(cr.Spec.RefreshTokenIdleExpiration, 720*time.Hour)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("parsing refreshTokenIdleExpiration: %w", err)
	}
	refreshTokenExpiration, err := parseDurationOrDefault(cr.Spec.RefreshTokenExpiration, 2160*time.Hour)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("parsing refreshTokenExpiration: %w", err)
	}

	// Read current OIDC settings from Zitadel.
	current, err := r.Zitadel.Admin().GetOIDCSettings(ctx, &admin.GetOIDCSettingsRequest{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting OIDC settings: %w", err)
	}

	// Detect drift and update if needed.
	if r.hasDrift(current.GetSettings(), accessTokenLifetime, idTokenLifetime, refreshTokenIdleExpiration, refreshTokenExpiration) {
		_, err := r.Zitadel.Admin().UpdateOIDCSettings(ctx, &admin.UpdateOIDCSettingsRequest{
			AccessTokenLifetime:        durationpb.New(accessTokenLifetime),
			IdTokenLifetime:            durationpb.New(idTokenLifetime),
			RefreshTokenIdleExpiration: durationpb.New(refreshTokenIdleExpiration),
			RefreshTokenExpiration:     durationpb.New(refreshTokenExpiration),
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("updating OIDC settings: %w", err)
		}
		logger.Info("default OIDC settings updated (drift detected)")
	}

	// Update status.
	if !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("defaultoidcsettings reconciled")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
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
