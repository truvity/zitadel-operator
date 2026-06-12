package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	objectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object/v2"
	userv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
)

// MachineUserReconciler reconciles a MachineUser object.
type MachineUserReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=machineusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=machineusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=machineusers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *MachineUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the MachineUser CR.
	var cr zitadelv1alpha1.MachineUser
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.UserId != "" {
			_, err := r.Zitadel.User().DeleteUser(ctx, &userv2.DeleteUserRequest{
				UserId: cr.Status.UserId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting user: %w", err)
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

	// Create or get machine user by username.
	userID, err := r.ensureMachineUser(ctx, &cr)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Generate machine key if secret doesn't exist yet.
	if err := r.ensureMachineKey(ctx, &cr, userID); err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.UserId = userID
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("machineuser reconciled", "userId", userID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *MachineUserReconciler) ensureMachineUser(ctx context.Context, cr *zitadelv1alpha1.MachineUser) (string, error) {
	// If we already have a user ID, verify it still exists.
	if cr.Status.UserId != "" {
		_, err := r.Zitadel.User().GetUserByID(ctx, &userv2.GetUserByIDRequest{
			UserId: cr.Status.UserId,
		})
		if err == nil {
			return cr.Status.UserId, nil
		}
		if status.Code(err) != codes.NotFound {
			return "", fmt.Errorf("getting user: %w", err)
		}
		// User was deleted externally, recreate.
	}

	// Search by username.
	listResp, err := r.Zitadel.User().ListUsers(ctx, &userv2.ListUsersRequest{
		Queries: []*userv2.SearchQuery{
			{
				Query: &userv2.SearchQuery_UserNameQuery{
					UserNameQuery: &userv2.UserNameQuery{
						UserName: cr.Spec.Username,
						Method:   objectv2.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing users: %w", err)
	}

	for _, user := range listResp.GetResult() {
		if user.GetUsername() == cr.Spec.Username {
			return user.GetUserId(), nil
		}
	}

	// Create new machine user.
	username := cr.Spec.Username
	description := cr.Spec.Description
	createResp, err := r.Zitadel.User().CreateUser(ctx, &userv2.CreateUserRequest{
		Username: &username,
		UserType: &userv2.CreateUserRequest_Machine_{
			Machine: &userv2.CreateUserRequest_Machine{
				Name:        cr.Spec.Name,
				Description: &description,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("creating machine user: %w", err)
	}

	return createResp.GetId(), nil
}

func (r *MachineUserReconciler) ensureMachineKey(ctx context.Context, cr *zitadelv1alpha1.MachineUser, userID string) error {
	// Check if the key secret already exists.
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      cr.Spec.KeySecretRef.Name,
		Namespace: cr.Namespace,
	}, secret)
	if err == nil {
		// Secret already exists, key was already generated.
		return nil
	}
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("checking key secret: %w", err)
	}

	// Generate a new machine key.
	expirationDays := cr.Spec.KeyExpirationDays
	if expirationDays <= 0 {
		expirationDays = 3650 // Default: 10 years.
	}
	keyResp, err := r.Zitadel.User().AddKey(ctx, &userv2.AddKeyRequest{
		UserId:         userID,
		ExpirationDate: timestamppb.New(time.Now().AddDate(0, 0, expirationDays)),
	})
	if err != nil {
		return fmt.Errorf("adding machine key: %w", err)
	}

	// Write key JSON to K8s Secret.
	keySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.KeySecretRef.Name,
			Namespace: cr.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, keySecret, func() error {
		if keySecret.Data == nil {
			keySecret.Data = make(map[string][]byte)
		}
		keySecret.Data["key.json"] = keyResp.GetKeyContent()
		keySecret.Data["key_id"] = []byte(keyResp.GetKeyId())
		return nil
	})
	if err != nil {
		return fmt.Errorf("creating key secret: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachineUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.MachineUser{}).
		Named("machineuser").
		WithEventFilter(generationChangedPredicate()).
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
