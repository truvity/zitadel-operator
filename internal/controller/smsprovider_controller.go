package controller

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
)

// SmsProviderReconciler reconciles a SmsProvider object.
type SmsProviderReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=smsproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=smsproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=smsproviders/finalizers,verbs=update

func (r *SmsProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cr zitadelv1alpha2.SmsProvider
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Validate: exactly one of twilio or http must be set.
	if cr.Spec.Twilio == nil && cr.Spec.Http == nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "InvalidSpec", "one of twilio or http must be set")
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{}, nil
	}
	if cr.Spec.Twilio != nil && cr.Spec.Http != nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "InvalidSpec", "twilio and http are mutually exclusive")
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{}, nil
	}

	// Handle deletion.
	if done, result, err := handleDeletionStrict(ctx, r.Client, &cr, func() error {
		return r.deleteProvider(ctx, cr.Status.ProviderId)
	}); done {
		return result, err
	}

	// Add finalizer.
	if err := ensureFinalizer(ctx, r.Client, &cr); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure SMS provider exists.
	providerID, err := r.ensureSmsProvider(ctx, &cr)
	if err != nil {
		if waiting, result := waitForRef(ctx, r.Client, &cr, &cr.Status.Conditions, "SecretNotFound", err); waiting {
			return result, nil
		}
		return ctrl.Result{}, err
	}

	// Activate the provider.
	r.activateProvider(ctx, providerID)

	// Update status.
	statusChanged := cr.Status.ProviderId != providerID
	cr.Status.ProviderId = providerID
	return ctrl.Result{RequeueAfter: requeueInterval}, markReady(ctx, r.Client, &cr, statusFields{
		conditions: &cr.Status.Conditions, ready: &cr.Status.Ready, lastSyncTime: &cr.Status.LastSyncTime,
	}, statusChanged)
}

func (r *SmsProviderReconciler) deleteProvider(ctx context.Context, providerID string) error {
	if providerID == "" {
		return nil
	}
	_, err := r.Zitadel.Admin().RemoveSMSProvider(ctx, &admin.RemoveSMSProviderRequest{Id: providerID})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("removing SMS provider: %w", err)
	}
	return nil
}

func (r *SmsProviderReconciler) activateProvider(ctx context.Context, providerID string) {
	if providerID == "" {
		return
	}
	_, err := r.Zitadel.Admin().ActivateSMSProvider(ctx, &admin.ActivateSMSProviderRequest{Id: providerID})
	if err != nil && status.Code(err) != codes.FailedPrecondition {
		log.FromContext(ctx).Info("could not activate SMS provider (may already be active)", "error", err)
	}
}

func (r *SmsProviderReconciler) ensureSmsProvider(ctx context.Context, cr *zitadelv1alpha2.SmsProvider) (string, error) {
	if cr.Spec.Twilio != nil {
		return r.ensureTwilioProvider(ctx, cr)
	}
	return r.ensureHttpProvider(ctx, cr)
}

func (r *SmsProviderReconciler) ensureTwilioProvider(ctx context.Context, cr *zitadelv1alpha2.SmsProvider) (string, error) {
	twilio := cr.Spec.Twilio

	// Resolve token from secret.
	token, err := r.resolveToken(ctx, cr.Namespace, &twilio.TokenSecretRef)
	if err != nil {
		return "", err
	}

	// If we have a provider ID, update it.
	if cr.Status.ProviderId != "" {
		_, err := r.Zitadel.Admin().UpdateSMSProviderTwilio(ctx, &admin.UpdateSMSProviderTwilioRequest{
			Id:           cr.Status.ProviderId,
			Sid:          twilio.SID,
			SenderNumber: twilio.SenderNumber,
			Description:  twilio.Description,
		})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// Deleted externally, recreate below.
			} else {
				return "", fmt.Errorf("updating twilio SMS provider: %w", err)
			}
		} else {
			// Update token separately.
			_, _ = r.Zitadel.Admin().UpdateSMSProviderTwilioToken(ctx, &admin.UpdateSMSProviderTwilioTokenRequest{
				Id:    cr.Status.ProviderId,
				Token: token,
			})
			return cr.Status.ProviderId, nil
		}
	}

	// Create new Twilio provider.
	resp, err := r.Zitadel.Admin().AddSMSProviderTwilio(ctx, &admin.AddSMSProviderTwilioRequest{
		Sid:          twilio.SID,
		Token:        token,
		SenderNumber: twilio.SenderNumber,
		Description:  twilio.Description,
	})
	if err != nil {
		return "", fmt.Errorf("adding twilio SMS provider: %w", err)
	}

	return resp.GetId(), nil
}

func (r *SmsProviderReconciler) ensureHttpProvider(ctx context.Context, cr *zitadelv1alpha2.SmsProvider) (string, error) {
	httpSpec := cr.Spec.Http

	// If we have a provider ID, update it.
	if cr.Status.ProviderId != "" {
		_, err := r.Zitadel.Admin().UpdateSMSProviderHTTP(ctx, &admin.UpdateSMSProviderHTTPRequest{
			Id:          cr.Status.ProviderId,
			Endpoint:    httpSpec.Endpoint,
			Description: httpSpec.Description,
		})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// Deleted externally, recreate below.
			} else {
				return "", fmt.Errorf("updating http SMS provider: %w", err)
			}
		} else {
			return cr.Status.ProviderId, nil
		}
	}

	// Create new HTTP provider.
	resp, err := r.Zitadel.Admin().AddSMSProviderHTTP(ctx, &admin.AddSMSProviderHTTPRequest{
		Endpoint:    httpSpec.Endpoint,
		Description: httpSpec.Description,
	})
	if err != nil {
		return "", fmt.Errorf("adding http SMS provider: %w", err)
	}

	return resp.GetId(), nil
}

func (r *SmsProviderReconciler) resolveToken(ctx context.Context, namespace string, ref *zitadelv1alpha2.SecretKeyRef) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      ref.Name,
		Namespace: namespace,
	}, secret); err != nil {
		return "", fmt.Errorf("getting token secret %s not yet ready: %w", ref.Name, err)
	}

	key := ref.Key
	if key == "" {
		key = "token"
	}

	data, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s not yet ready", key, ref.Name)
	}

	return string(data), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SmsProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.SmsProvider{}).
		Named("smsprovider").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
