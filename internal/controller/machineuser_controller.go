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
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/authn"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	objectv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	userv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user"
)

// MachineUserReconciler reconciles a MachineUser object.
type MachineUserReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=machineusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=machineusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=machineusers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *MachineUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.MachineUser
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Resolve organization.
	orgID, err := resolveOrganizationId(ctx, r.Client, r.Config, cr.Spec.OrganizationRef, cr.Spec.OrganizationId, cr.Namespace)
	if err != nil {
		if isRefNotReady(err) {
			logger.Info("waiting for organization ref to become ready", "error", err)
			setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionFalse, "OrgNotReady", err.Error())
			_ = r.Status().Update(ctx, &cr)
			return ctrl.Result{RequeueAfter: requeueOnError}, nil
		}
		return ctrl.Result{}, fmt.Errorf("resolving organization: %w", err)
	}

	// Set org context for Management API calls.
	ctx = withOrgID(ctx, orgID)

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.UserId != "" {
			_, err := r.Zitadel.Management().RemoveUser(ctx, &management.RemoveUserRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
				Id: cr.Status.UserId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting machine user: %w", err)
			}
		}
		if removeFinalizer(&cr) {
			if err := r.Update(ctx, &cr); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present.
	if addFinalizer(&cr) {
		if err := r.Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Ensure machine user exists.
	userID, err := r.ensureMachineUser(ctx, &cr, orgID)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Ensure key exists and is stored in Secret.
	if err := r.ensureKey(ctx, &cr, userID); err != nil {
		return ctrl.Result{}, err
	}

	// Update status only if changed.
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

	logger.Info("machineuser reconciled", "userId", userID, "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *MachineUserReconciler) ensureMachineUser(ctx context.Context, cr *zitadelv1alpha2.MachineUser, _ string) (string, error) {
	// If we already have a user ID, verify it still exists.
	if cr.Status.UserId != "" {
		_, err := r.Zitadel.Management().GetUserByID(ctx, &management.GetUserByIDRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
			Id: cr.Status.UserId,
		})
		if err == nil {
			return cr.Status.UserId, nil
		}
		if status.Code(err) != codes.NotFound {
			return "", fmt.Errorf("getting user by ID: %w", err)
		}
		// User was deleted externally, recreate.
	}

	// Search by username via Management API.
	listResp, err := r.Zitadel.Management().ListUsers(ctx, &management.ListUsersRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		Queries: []*userv1.SearchQuery{
			{
				Query: &userv1.SearchQuery_UserNameQuery{
					UserNameQuery: &userv1.UserNameQuery{
						UserName: cr.Spec.UserName,
						Method:   objectv1.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing users: %w", err)
	}

	for _, u := range listResp.GetResult() {
		if u.GetUserName() == cr.Spec.UserName {
			return u.GetId(), nil
		}
	}

	// Resolve access token type.
	accessTokenType := userv1.AccessTokenType_ACCESS_TOKEN_TYPE_BEARER
	if cr.Spec.AccessTokenType == "jwt" {
		accessTokenType = userv1.AccessTokenType_ACCESS_TOKEN_TYPE_JWT
	}

	// Create machine user via Management API.
	createResp, err := r.Zitadel.Management().AddMachineUser(ctx, &management.AddMachineUserRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		UserName:        cr.Spec.UserName,
		Name:            cr.DisplayName(),
		Description:     cr.Spec.Description,
		AccessTokenType: accessTokenType,
	})
	if err != nil {
		return "", fmt.Errorf("creating machine user: %w", err)
	}

	return createResp.GetUserId(), nil
}

func (r *MachineUserReconciler) ensureKey(ctx context.Context, cr *zitadelv1alpha2.MachineUser, userID string) error {
	secretKey := cr.Spec.KeySecretRef.Key
	if secretKey == "" {
		secretKey = "key.json"
	}

	// Check if Secret already exists with key data.
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cr.Spec.KeySecretRef.Name, Namespace: cr.Namespace}, secret)
	if err == nil && len(secret.Data[secretKey]) > 0 {
		return nil // Key already stored.
	}

	// Create a new key via Management API.
	expiration := time.Now().Add(365 * 10 * 24 * time.Hour)                                     // 10 years
	keyResp, err := r.Zitadel.Management().AddMachineKey(ctx, &management.AddMachineKeyRequest{ //nolint:staticcheck // SA1019: deprecated SDK v1 method, migrate to v2 when stable
		UserId:         userID,
		Type:           authn.KeyType_KEY_TYPE_JSON,
		ExpirationDate: timestamppb.New(expiration),
	})
	if err != nil {
		return fmt.Errorf("creating machine key: %w", err)
	}

	// keyDetails is the full key JSON from Zitadel.
	keyJSON := keyResp.GetKeyDetails()

	// Store in Secret.
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

// withOrgID adds x-zitadel-orgid metadata to the context for Management API calls.
func withOrgID(ctx context.Context, orgID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachineUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.MachineUser{}).
		Named("machineuser").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
