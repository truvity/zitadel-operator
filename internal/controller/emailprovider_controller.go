package controller

import (
	"context"
	"fmt"
	"time"

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

// EmailProviderReconciler reconciles an EmailProvider object.
type EmailProviderReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=emailproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=emailproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=emailproviders/finalizers,verbs=update

func (r *EmailProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.EmailProvider
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Validate: exactly one of smtp or http must be set.
	if cr.Spec.Smtp == nil && cr.Spec.Http == nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "InvalidSpec", "one of smtp or http must be set")
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{}, nil
	}
	if cr.Spec.Smtp != nil && cr.Spec.Http != nil {
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "InvalidSpec", "smtp and http are mutually exclusive")
		_ = r.Status().Update(ctx, &cr)
		return ctrl.Result{}, nil
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.ProviderId != "" {
			_, err := r.Zitadel.Admin().RemoveEmailProvider(ctx, &admin.RemoveEmailProviderRequest{
				Id: cr.Status.ProviderId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("removing email provider: %w", err)
			}
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

	// Ensure email provider exists.
	providerID, err := r.ensureEmailProvider(ctx, &cr)
	if err != nil {
		if isRefNotReady(err) {
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "SecretNotFound", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, err
	}

	// Activate the provider.
	if providerID != "" {
		_, err := r.Zitadel.Admin().ActivateEmailProvider(ctx, &admin.ActivateEmailProviderRequest{
			Id: providerID,
		})
		if err != nil && status.Code(err) != codes.FailedPrecondition {
			// FailedPrecondition means already active — that's fine.
			logger.Info("could not activate email provider (may already be active)", "error", err)
		}
	}

	// Update status.
	if cr.Status.ProviderId != providerID || !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.ProviderId = providerID
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("emailprovider reconciled", "providerId", providerID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *EmailProviderReconciler) ensureEmailProvider(ctx context.Context, cr *zitadelv1alpha2.EmailProvider) (string, error) {
	if cr.Spec.Smtp != nil {
		return r.ensureSmtpProvider(ctx, cr)
	}
	return r.ensureHttpProvider(ctx, cr)
}

func (r *EmailProviderReconciler) ensureSmtpProvider(ctx context.Context, cr *zitadelv1alpha2.EmailProvider) (string, error) {
	smtp := cr.Spec.Smtp

	// Resolve password from secret if provided.
	password := ""
	if smtp.PasswordSecretRef != nil {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      smtp.PasswordSecretRef.Name,
			Namespace: cr.Namespace,
		}, secret); err != nil {
			return "", fmt.Errorf("getting smtp password secret %s not yet ready: %w", smtp.PasswordSecretRef.Name, err)
		}
		key := smtp.PasswordSecretRef.Key
		if key == "" {
			key = "password"
		}
		data, ok := secret.Data[key]
		if !ok {
			return "", fmt.Errorf("key %q not found in secret %s not yet ready", key, smtp.PasswordSecretRef.Name)
		}
		password = string(data)
	}

	// If we have a provider ID, update it.
	if cr.Status.ProviderId != "" {
		_, err := r.Zitadel.Admin().UpdateEmailProviderSMTP(ctx, &admin.UpdateEmailProviderSMTPRequest{
			Id:             cr.Status.ProviderId,
			Description:    smtp.Description,
			SenderAddress:  smtp.SenderAddress,
			SenderName:     smtp.SenderName,
			ReplyToAddress: smtp.ReplyToAddress,
			Tls:            smtp.Tls,
			Host:           smtp.Host,
			User:           smtp.User,
		})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// Deleted externally, recreate below.
			} else {
				return "", fmt.Errorf("updating smtp email provider: %w", err)
			}
		} else {
			// Update password separately if set.
			if password != "" {
				_, _ = r.Zitadel.Admin().UpdateEmailProviderSMTPPassword(ctx, &admin.UpdateEmailProviderSMTPPasswordRequest{
					Id:       cr.Status.ProviderId,
					Password: password,
				})
			}
			return cr.Status.ProviderId, nil
		}
	}

	// Create new SMTP provider.
	resp, err := r.Zitadel.Admin().AddEmailProviderSMTP(ctx, &admin.AddEmailProviderSMTPRequest{
		Description:    smtp.Description,
		SenderAddress:  smtp.SenderAddress,
		SenderName:     smtp.SenderName,
		ReplyToAddress: smtp.ReplyToAddress,
		Tls:            smtp.Tls,
		Host:           smtp.Host,
		User:           smtp.User,
		Password:       password,
	})
	if err != nil {
		return "", fmt.Errorf("adding smtp email provider: %w", err)
	}

	return resp.GetId(), nil
}

func (r *EmailProviderReconciler) ensureHttpProvider(ctx context.Context, cr *zitadelv1alpha2.EmailProvider) (string, error) {
	httpSpec := cr.Spec.Http

	// If we have a provider ID, update it.
	if cr.Status.ProviderId != "" {
		_, err := r.Zitadel.Admin().UpdateEmailProviderHTTP(ctx, &admin.UpdateEmailProviderHTTPRequest{
			Id:       cr.Status.ProviderId,
			Endpoint: httpSpec.Endpoint,
		})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// Deleted externally, recreate below.
			} else {
				return "", fmt.Errorf("updating http email provider: %w", err)
			}
		} else {
			return cr.Status.ProviderId, nil
		}
	}

	// Create new HTTP provider.
	resp, err := r.Zitadel.Admin().AddEmailProviderHTTP(ctx, &admin.AddEmailProviderHTTPRequest{
		Endpoint: httpSpec.Endpoint,
	})
	if err != nil {
		return "", fmt.Errorf("adding http email provider: %w", err)
	}

	return resp.GetId(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EmailProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.EmailProvider{}).
		Named("emailprovider").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
