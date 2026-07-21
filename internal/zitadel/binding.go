package zitadel

import (
	"context"
	"fmt"
	"slices"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/auth"
)

// Binding levels (mirrors internal/config to avoid an import cycle).
const (
	bindingIAMOwner = "iam-owner"
	bindingOrgOwner = "org-owner"
)

// VerifyBinding checks the config's binding assertion against the
// credential's actual memberships (AuthService.ListMyMemberships) and returns
// the bound org ID for org-owner bindings (empty for iam-owner).
//
// A mismatch in either direction is fatal (INF-424): asserting iam-owner with
// a lesser credential would fail at runtime in confusing places; asserting
// org-owner with an IAM_OWNER credential would silently run with far more
// privilege than declared.
func VerifyBinding(ctx context.Context, c *Client, binding string) (boundOrgID string, err error) {
	resp, err := c.Auth().ListMyMemberships(ctx, &auth.ListMyMembershipsRequest{})
	if err != nil {
		return "", fmt.Errorf("listing own memberships for binding verification: %w", err)
	}

	hasIAMOwner := false
	orgOwnerOrgs := []string{}
	for _, m := range resp.GetResult() {
		switch {
		case m.GetIam() && slices.Contains(m.GetRoles(), "IAM_OWNER"):
			hasIAMOwner = true
		case m.GetOrgId() != "" && slices.Contains(m.GetRoles(), "ORG_OWNER"):
			orgOwnerOrgs = append(orgOwnerOrgs, m.GetOrgId())
		}
	}

	switch binding {
	case bindingIAMOwner:
		if !hasIAMOwner {
			return "", fmt.Errorf("binding asserted iam-owner but the credential has no IAM_OWNER membership (org-owner orgs: %v)", orgOwnerOrgs)
		}
		return "", nil
	case bindingOrgOwner:
		if hasIAMOwner {
			return "", fmt.Errorf("binding asserted org-owner but the credential holds instance-level IAM_OWNER; fix the assertion or narrow the credential")
		}
		if len(orgOwnerOrgs) == 0 {
			return "", fmt.Errorf("binding asserted org-owner but the credential has no ORG_OWNER membership")
		}
		if len(orgOwnerOrgs) > 1 {
			return "", fmt.Errorf("binding asserted org-owner but the credential is ORG_OWNER in %d orgs (%v); exactly one is required", len(orgOwnerOrgs), orgOwnerOrgs)
		}
		return orgOwnerOrgs[0], nil
	default:
		return "", fmt.Errorf("unknown binding level %q", binding)
	}
}
