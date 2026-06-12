package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/truvity/zitadel-operator/api/v1alpha1"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	objectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object/v2"
	orgv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/org/v2"
)

// OrganizationReconciler reconciles an Organization object.
type OrganizationReconciler struct {
	client.Client
	Zitadel *zitadel.Client
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=organizations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=organizations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=organizations/finalizers,verbs=update

func (r *OrganizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Organization CR.
	var cr zitadelv1alpha1.Organization
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		if cr.Status.OrgId != "" {
			_, err := r.Zitadel.Organization().DeleteOrganization(ctx, &orgv2.DeleteOrganizationRequest{
				OrganizationId: cr.Status.OrgId,
			})
			if err != nil && status.Code(err) != codes.NotFound {
				return ctrl.Result{}, fmt.Errorf("deleting organization: %w", err)
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

	// Create or get org by name.
	orgID, err := r.ensureOrganization(ctx, &cr)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	cr.Status.OrgId = orgID
	cr.Status.Ready = true
	cr.Status.LastSyncTime = &now
	if err := r.Status().Update(ctx, &cr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("organization reconciled", "orgId", orgID)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *OrganizationReconciler) ensureOrganization(ctx context.Context, cr *zitadelv1alpha1.Organization) (string, error) {
	// If we already have an org ID, verify it still exists.
	if cr.Status.OrgId != "" {
		listResp, err := r.Zitadel.Organization().ListOrganizations(ctx, &orgv2.ListOrganizationsRequest{
			Queries: []*orgv2.SearchQuery{
				{
					Query: &orgv2.SearchQuery_NameQuery{
						NameQuery: &orgv2.OrganizationNameQuery{
							Name:   cr.Spec.DisplayName,
							Method: objectv2.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
						},
					},
				},
			},
		})
		if err == nil {
			for _, org := range listResp.GetResult() {
				if org.GetId() == cr.Status.OrgId {
					return cr.Status.OrgId, nil
				}
			}
		}
		// Org was deleted externally or not found, recreate.
	}

	// Search by name.
	listResp, err := r.Zitadel.Organization().ListOrganizations(ctx, &orgv2.ListOrganizationsRequest{
		Queries: []*orgv2.SearchQuery{
			{
				Query: &orgv2.SearchQuery_NameQuery{
					NameQuery: &orgv2.OrganizationNameQuery{
						Name:   cr.Spec.DisplayName,
						Method: objectv2.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing organizations: %w", err)
	}

	for _, org := range listResp.GetResult() {
		if org.GetName() == cr.Spec.DisplayName {
			return org.GetId(), nil
		}
	}

	// Create new organization.
	createResp, err := r.Zitadel.Organization().AddOrganization(ctx, &orgv2.AddOrganizationRequest{
		Name: cr.Spec.DisplayName,
	})
	if err != nil {
		return "", fmt.Errorf("creating organization: %w", err)
	}

	return createResp.GetOrganizationId(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrganizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.Organization{}).
		Named("organization").
		WithEventFilter(generationChangedPredicate()).
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
