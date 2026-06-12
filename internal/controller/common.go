// Package controller implements Kubernetes controllers for Zitadel resources.
package controller

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/truvity/zitadel-operator/internal/zitadel"

	objectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object/v2"
	userv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
)

const (
	// finalizerName is the finalizer used by all Zitadel controllers.
	finalizerName = "zitadel.truvity.io/finalizer"

	// requeueInterval is the default requeue interval for periodic reconciliation.
	requeueInterval = 5 * time.Minute
)

// addFinalizer adds the finalizer to the object if not already present.
func addFinalizer(obj client.Object) bool {
	if !controllerutil.ContainsFinalizer(obj, finalizerName) {
		controllerutil.AddFinalizer(obj, finalizerName)
		return true
	}
	return false
}

// removeFinalizer removes the finalizer from the object.
func removeFinalizer(obj client.Object) bool {
	if controllerutil.ContainsFinalizer(obj, finalizerName) {
		controllerutil.RemoveFinalizer(obj, finalizerName)
		return true
	}
	return false
}

// generationChangedPredicate returns a predicate that filters out status-only
// updates. Only spec changes (generation bump) and deletion trigger reconciliation.
// This prevents hot-loops where status writes trigger re-reconciliation.
func generationChangedPredicate() predicate.Predicate {
	return predicate.GenerationChangedPredicate{}
}

// resolveUserIDByEmail resolves a user email address to a Zitadel user ID using the v2 User service.
func resolveUserIDByEmail(ctx context.Context, z *zitadel.Client, email string) (string, error) {
	resp, err := z.User().ListUsers(ctx, &userv2.ListUsersRequest{
		Queries: []*userv2.SearchQuery{
			{
				Query: &userv2.SearchQuery_EmailQuery{
					EmailQuery: &userv2.EmailQuery{
						EmailAddress: email,
						Method:       objectv2.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing users by email %q: %w", email, err)
	}

	if len(resp.GetResult()) == 0 {
		return "", fmt.Errorf("user with email %q not found", email)
	}

	return resp.GetResult()[0].GetUserId(), nil
}
