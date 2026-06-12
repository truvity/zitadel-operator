//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
)

func TestClient_GetMyOrg(t *testing.T) {
	ctx := context.Background()

	resp, err := zitadelClient.Management().GetMyOrg(ctx, &management.GetMyOrgRequest{})
	if err != nil {
		t.Fatalf("GetMyOrg: %v", err)
	}

	org := resp.GetOrg()
	if org == nil {
		t.Fatal("expected non-nil org")
	}

	t.Logf("connected to Zitadel org: %s (id: %s)", org.GetName(), org.GetId())
}

func TestClient_ListProjects(t *testing.T) {
	ctx := context.Background()

	resp, err := zitadelClient.Management().ListProjects(ctx, &management.ListProjectsRequest{})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}

	t.Logf("found %d projects", len(resp.GetResult()))
	for _, p := range resp.GetResult() {
		t.Logf("  project: %s (id: %s)", p.GetName(), p.GetId())
	}
}
