package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/authn"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

// ApplicationKeyReconciler reconciles an ApplicationKey object.
type ApplicationKeyReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=applicationkeys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=applicationkeys/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=applicationkeys/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *ApplicationKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cr zitadelv1alpha2.ApplicationKey
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve project ID (and inherited org ID).
	projectID, inheritedOrgID, err := resolveProjectId(ctx, r.Client, cr.Spec.ProjectRef, cr.Spec.ProjectId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "ProjectNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving project: %w", err)
	}

	// Resolve app ID.
	appID, err := resolveAppId(ctx, r.Client, cr.Spec.AppRef, cr.Spec.AppId, cr.Namespace)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, r.Config, &cr, &cr.Status.Conditions, "AppNotReady", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving app: %w", err)
	}

	// Set org context for Management API calls.
	if inheritedOrgID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", inheritedOrgID)
	}

	// Deletion.
	if done, result, err := handleDeletionStrict(ctx, r.Client, &cr, func() error {
		return r.deleteKey(ctx, projectID, appID, cr.Status.KeyId)
	}); done {
		return result, err
	}

	// Finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure key exists and is stored in Secret.
	if err := r.ensureKey(ctx, &cr, projectID, appID); err != nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "KeyError", err.Error())
		_ = applyStatus(ctx, r.Client, r.Config, &cr)
		return ctrl.Result{}, err
	}

	// Status.
	statusChanged := cr.Status.ProjectId != projectID || cr.Status.AppId != appID
	cr.Status.ProjectId = projectID
	cr.Status.AppId = appID
	if err := markReady(ctx, r.Client, r.Config, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *ApplicationKeyReconciler) deleteKey(ctx context.Context, projectID, appID, keyID string) error {
	if keyID == "" {
		return nil
	}
	_, err := r.Zitadel.Management().RemoveAppKey(ctx, &management.RemoveAppKeyRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		ProjectId: projectID,
		AppId:     appID,
		KeyId:     keyID,
	})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("removing app key: %w", err)
	}
	return nil
}

func (r *ApplicationKeyReconciler) ensureKey(ctx context.Context, cr *zitadelv1alpha2.ApplicationKey, projectID, appID string) error {
	secretKey := cr.Spec.KeySecretRef.Key
	if secretKey == "" {
		secretKey = "key.json"
	}

	// Check if Secret already exists with key data.
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cr.Spec.KeySecretRef.Name, Namespace: cr.Namespace}, secret)
	if err == nil && len(secret.Data[secretKey]) > 0 && cr.Status.KeyId != "" {
		return nil // Key already stored.
	}

	// Determine expiration.
	expiration := time.Now().Add(365 * 10 * 24 * time.Hour) // 10 years
	if cr.Spec.ExpirationDate != nil {
		expiration = cr.Spec.ExpirationDate.Time
	}

	// Create a new key via Management API.
	keyResp, err := r.Zitadel.Management().AddAppKey(ctx, &management.AddAppKeyRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		ProjectId:      projectID,
		AppId:          appID,
		Type:           authn.KeyType_KEY_TYPE_JSON,
		ExpirationDate: timestamppb.New(expiration),
	})
	if err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			// App may not be fully propagated in Zitadel yet — requeue.
			return fmt.Errorf("creating app key (will retry): %w", err)
		}
		return fmt.Errorf("creating app key: %w", err)
	}

	cr.Status.KeyId = keyResp.GetId()

	// Store in Secret.
	keyJSON := keyResp.GetKeyDetails()
	if secret.Name == "" {
		// Create new secret.
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Spec.KeySecretRef.Name,
				Namespace: cr.Namespace,
			},
			Data: map[string][]byte{
				secretKey: keyJSON,
			},
		}
		return r.Create(ctx, newSecret)
	}

	// Update existing secret.
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[secretKey] = keyJSON
	return r.Update(ctx, secret)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.ApplicationKey{}).
		Named("applicationkey").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
