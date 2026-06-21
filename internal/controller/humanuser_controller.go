package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	objectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object/v2"
	userv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
)

// HumanUserReconciler reconciles a HumanUser object.
type HumanUserReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=humanusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=humanusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=humanusers/finalizers,verbs=update

func (r *HumanUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.HumanUser
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve organization.
	orgID, err := resolveOrganizationId(ctx, r.Client, r.Config, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.UserId != "" {
			_, err := r.Zitadel.User().DeleteUser(ctx, &userv2.DeleteUserRequest{
				UserId: cr.Status.UserId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting human user: %w", err)
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

	// Resolve initial password if set.
	initialPassword := ""
	if cr.Spec.InitialPasswordSecretRef != nil {
		pw, err := r.resolveInitialPassword(ctx, &cr)
		if err != nil {
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "SecretNotFound", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		initialPassword = pw
	}

	// Ensure human user exists.
	userID, err := r.ensureHumanUser(ctx, &cr, orgID, initialPassword)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	if cr.Status.UserId != userID || cr.Status.OrganizationId != orgID || !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.UserId = userID
		cr.Status.OrganizationId = orgID
		cr.Status.Ready = true
		cr.Status.LastSyncTime = &now
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("humanuser reconciled", "userId", userID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *HumanUserReconciler) resolveInitialPassword(ctx context.Context, cr *zitadelv1alpha2.HumanUser) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cr.Spec.InitialPasswordSecretRef.Name,
		Namespace: cr.Namespace,
	}, secret); err != nil {
		return "", fmt.Errorf("getting initialPasswordSecretRef secret %s: %w", cr.Spec.InitialPasswordSecretRef.Name, err)
	}

	key := cr.Spec.InitialPasswordSecretRef.Key
	if key == "" {
		key = "password"
	}

	data, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s", key, cr.Spec.InitialPasswordSecretRef.Name)
	}

	return string(data), nil
}

func (r *HumanUserReconciler) ensureHumanUser(ctx context.Context, cr *zitadelv1alpha2.HumanUser, orgID, initialPassword string) (string, error) {
	// If we already have a user ID, verify it still exists.
	if cr.Status.UserId != "" {
		_, err := r.Zitadel.User().GetUserByID(ctx, &userv2.GetUserByIDRequest{
			UserId: cr.Status.UserId,
		})
		if err == nil {
			// User exists — update is not directly supported for all fields via v2 API,
			// so we just confirm existence.
			return cr.Status.UserId, nil
		}
		if status.Code(err) != codes.NotFound {
			return "", fmt.Errorf("getting human user: %w", err)
		}
		// User deleted externally, recreate below.
	}

	// Search by userName to avoid duplicates.
	listResp, err := r.Zitadel.User().ListUsers(ctx, &userv2.ListUsersRequest{
		Queries: []*userv2.SearchQuery{
			{
				Query: &userv2.SearchQuery_UserNameQuery{
					UserNameQuery: &userv2.UserNameQuery{
						UserName: cr.Spec.UserName,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing users: %w", err)
	}

	for _, u := range listResp.GetResult() {
		if u.GetUsername() == cr.Spec.UserName {
			return u.GetUserId(), nil
		}
	}

	// Create new human user.
	username := cr.Spec.UserName
	addReq := &userv2.AddHumanUserRequest{
		Organization: &objectv2.Organization{
			Org: &objectv2.Organization_OrgId{
				OrgId: orgID,
			},
		},
		Username: &username,
		Profile: &userv2.SetHumanProfile{
			GivenName:  cr.Spec.FirstName,
			FamilyName: cr.Spec.LastName,
		},
		Email: &userv2.SetHumanEmail{
			Email: cr.Spec.Email,
		},
	}

	if cr.Spec.DisplayName != "" {
		addReq.Profile.DisplayName = strPtr(cr.Spec.DisplayName)
	}
	if cr.Spec.NickName != "" {
		addReq.Profile.NickName = strPtr(cr.Spec.NickName)
	}
	if cr.Spec.PreferredLanguage != "" {
		addReq.Profile.PreferredLanguage = strPtr(cr.Spec.PreferredLanguage)
	}

	if cr.Spec.IsEmailVerified {
		addReq.Email.Verification = &userv2.SetHumanEmail_IsVerified{
			IsVerified: true,
		}
	}

	if cr.Spec.Phone != "" {
		addReq.Phone = &userv2.SetHumanPhone{
			Phone: cr.Spec.Phone,
		}
		if cr.Spec.IsPhoneVerified {
			addReq.Phone.Verification = &userv2.SetHumanPhone_IsVerified{
				IsVerified: true,
			}
		}
	}

	if initialPassword != "" {
		addReq.PasswordType = &userv2.AddHumanUserRequest_Password{
			Password: &userv2.Password{
				Password:       initialPassword,
				ChangeRequired: true,
			},
		}
	}

	resp, err := r.Zitadel.User().AddHumanUser(ctx, addReq)
	if err != nil {
		return "", fmt.Errorf("adding human user: %w", err)
	}

	return resp.GetUserId(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HumanUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.HumanUser{}).
		Named("humanuser").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
