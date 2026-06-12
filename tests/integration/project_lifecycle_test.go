//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project"
)

func TestProject_Lifecycle(t *testing.T) {
	ctx := context.Background()
	projectName := fmt.Sprintf("integration-test-%d", time.Now().UnixMilli())

	// Create project.
	createResp, err := zitadelClient.Management().AddProject(ctx, &management.AddProjectRequest{
		Name:                 projectName,
		ProjectRoleAssertion: true,
	})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	projectID := createResp.GetId()
	t.Logf("created project %q (id: %s)", projectName, projectID)

	// Cleanup on exit.
	t.Cleanup(func() {
		_, err := zitadelClient.Management().RemoveProject(ctx, &management.RemoveProjectRequest{
			Id: projectID,
		})
		if err != nil {
			t.Logf("cleanup RemoveProject: %v (may already be deleted)", err)
		}
	})

	// Verify project exists via list.
	listResp, err := zitadelClient.Management().ListProjects(ctx, &management.ListProjectsRequest{
		Queries: []*project.ProjectQuery{
			{
				Query: &project.ProjectQuery_NameQuery{
					NameQuery: &project.ProjectNameQuery{
						Name:   projectName,
						Method: object.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}

	if len(listResp.GetResult()) != 1 {
		t.Fatalf("expected 1 project, got %d", len(listResp.GetResult()))
	}

	found := listResp.GetResult()[0]
	if found.GetId() != projectID {
		t.Fatalf("expected project ID %s, got %s", projectID, found.GetId())
	}

	t.Logf("verified project exists: %s", found.GetName())

	// Update project (change name).
	updatedName := projectName + "-updated"

	_, err = zitadelClient.Management().UpdateProject(ctx, &management.UpdateProjectRequest{
		Id:   projectID,
		Name: updatedName,
	})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	t.Logf("updated project name to %q", updatedName)

	// Verify update.
	getResp, err := zitadelClient.Management().GetProjectByID(ctx, &management.GetProjectByIDRequest{
		Id: projectID,
	})
	if err != nil {
		t.Fatalf("GetProjectByID: %v", err)
	}

	if getResp.GetProject().GetName() != updatedName {
		t.Fatalf("expected name %q, got %q", updatedName, getResp.GetProject().GetName())
	}

	// Delete project.
	_, err = zitadelClient.Management().RemoveProject(ctx, &management.RemoveProjectRequest{
		Id: projectID,
	})
	if err != nil {
		t.Fatalf("RemoveProject: %v", err)
	}

	t.Logf("deleted project %s", projectID)

	// Verify deletion.
	listResp, err = zitadelClient.Management().ListProjects(ctx, &management.ListProjectsRequest{
		Queries: []*project.ProjectQuery{
			{
				Query: &project.ProjectQuery_NameQuery{
					NameQuery: &project.ProjectNameQuery{
						Name:   updatedName,
						Method: object.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ListProjects after delete: %v", err)
	}

	if len(listResp.GetResult()) != 0 {
		t.Fatalf("expected 0 projects after delete, got %d", len(listResp.GetResult()))
	}

	t.Log("verified project deleted")
}
