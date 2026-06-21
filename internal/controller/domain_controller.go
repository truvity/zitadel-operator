package controller

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
	"github.com/truvity/zitadel-operator/internal/config"
	"github.com/truvity/zitadel-operator/internal/zitadel"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/org"
)

// DomainReconciler reconciles a Domain object.
type DomainReconciler struct {
	client.Client
	Zitadel *zitadel.Client
	Config  *config.Config
}

// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=domains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=domains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.truvity.io,resources=domains/finalizers,verbs=update

func (r *DomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cr zitadelv1alpha2.Domain
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

	// Set org context for Management API calls.
	ctx = metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

	// Handle deletion.
	if !cr.DeletionTimestamp.IsZero() {
		_, err := r.Zitadel.Management().RemoveOrgDomain(ctx, &management.RemoveOrgDomainRequest{
			Domain: cr.Spec.DomainName,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			return ctrl.Result{}, fmt.Errorf("removing org domain: %w", err)
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

	// Ensure domain exists.
	if err := r.ensureDomain(ctx, cr.Spec.DomainName); err != nil {
		return ctrl.Result{}, err
	}

	// Update status.
	if !cr.Status.Ready {
		now := metav1.NewTime(time.Now())
		cr.Status.Ready = true
		setCondition(&cr.Status.Conditions, ConditionTypeReady, metav1.ConditionTrue, "Reconciled", "Successfully synced with Zitadel")
		cr.Status.LastSyncTime = &now
		if err := r.Status().Update(ctx, &cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("domain reconciled", "domain", cr.Spec.DomainName)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *DomainReconciler) ensureDomain(ctx context.Context, domainName string) error {
	// Check if domain already exists.
	listResp, err := r.Zitadel.Management().ListOrgDomains(ctx, &management.ListOrgDomainsRequest{
		Query: &object.ListQuery{Limit: 100},
		Queries: []*org.DomainSearchQuery{
			{
				Query: &org.DomainSearchQuery_DomainNameQuery{
					DomainNameQuery: &org.DomainNameQuery{
						Name:   domainName,
						Method: object.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("listing org domains: %w", err)
	}

	for _, d := range listResp.GetResult() {
		if d.GetDomainName() == domainName {
			return nil // Domain already exists.
		}
	}

	// Add domain.
	_, err = r.Zitadel.Management().AddOrgDomain(ctx, &management.AddOrgDomainRequest{
		Domain: domainName,
	})
	if err != nil {
		// If already exists (race condition), treat as success.
		if status.Code(err) == codes.AlreadyExists {
			return nil
		}
		return fmt.Errorf("adding org domain: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha2.Domain{}).
		Named("domain").
		WithEventFilter(generationChangedPredicate()).
		Complete(r)
}
