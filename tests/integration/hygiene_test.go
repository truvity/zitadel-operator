//go:build integration

package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"

	"google.golang.org/grpc/metadata"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	objectv1 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	objectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object/v2"
	orgv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/org/v2"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
)

// Test-instance hygiene: every integration run mints uniquely named orgs,
// projects, users and IdP providers on the dedicated test instance. Cleanup
// paths cover the happy case, but aborted runs (Ctrl-C, t.Fatal before
// t.Cleanup registration, crashes) leak — and some kinds (instance-level IdP
// providers) are only deactivated, never deleted, by their controllers.
//
// staleTestName matches the naming conventions all test resources share:
// either a known test prefix or a unix-milli timestamp suffix segment.
// Real resources (the binding SA, the default org, "Truvity Testing", any
// human-named object) never match.
var (
	staleTimestamp = regexp.MustCompile(`(^|[ \-])1[5-9][0-9]{11}([^0-9]|$)`)
	stalePrefixes  = []string{"proto018-", "v018-"}
)

func staleTestName(name string) bool {
	for _, p := range stalePrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return staleTimestamp.MatchString(name)
}

// protectedOrgNames are never swept regardless of pattern matches.
var protectedOrgNames = map[string]bool{
	"ZITADEL":         true,
	"Truvity Testing": true,
}

// sweepStaleTestResources removes leaked test resources across the instance:
// stale orgs (cascade), then stale projects/users inside surviving orgs, then
// stale instance-level IdP providers. Best-effort: individual failures are
// reported but do not stop the sweep.
func sweepStaleTestResources(ctx context.Context) (summary string, firstErr error) {
	var orgsDeleted, projectsDeleted, usersDeleted, providersDeleted, failures int
	note := func(err error) {
		if err != nil {
			failures++
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("hygiene sweep item failed", slog.Any("error", err))
		}
	}

	// 1. Organizations (stale ones cascade everything inside them).
	surviving := []string{}
	for offset := uint64(0); ; {
		resp, err := zitadelClient.Organization().ListOrganizations(ctx, &orgv2.ListOrganizationsRequest{
			Query: &objectv2.ListQuery{Offset: offset, Limit: 250},
		})
		if err != nil {
			note(fmt.Errorf("listing organizations: %w", err))
			break
		}
		for _, o := range resp.GetResult() {
			switch {
			case o.GetId() == testOrgID || protectedOrgNames[o.GetName()]:
				surviving = append(surviving, o.GetId())
			case staleTestName(o.GetName()):
				if _, err := zitadelClient.Organization().DeleteOrganization(ctx, &orgv2.DeleteOrganizationRequest{
					OrganizationId: o.GetId(),
				}); err != nil {
					note(fmt.Errorf("deleting org %s (%s): %w", o.GetName(), o.GetId(), err))
				} else {
					orgsDeleted++
				}
			default:
				surviving = append(surviving, o.GetId())
			}
		}
		if uint64(len(resp.GetResult())) < 250 {
			break
		}
		offset += 250
	}

	// 2. Projects and users inside surviving orgs.
	for _, orgID := range surviving {
		orgCtx := metadata.AppendToOutgoingContext(ctx, "x-zitadel-orgid", orgID)

		for offset := uint64(0); ; {
			resp, err := zitadelClient.Project().ListProjects(orgCtx, &projectv2.ListProjectsRequest{
				Pagination: &filterv2.PaginationRequest{Offset: offset, Limit: 250},
				Filters: []*projectv2.ProjectSearchFilter{{
					Filter: &projectv2.ProjectSearchFilter_OrganizationIdFilter{
						OrganizationIdFilter: &projectv2.ProjectOrganizationIDFilter{OrganizationId: orgID},
					},
				}},
			})
			if err != nil {
				note(fmt.Errorf("listing projects in org %s: %w", orgID, err))
				break
			}
			for _, p := range resp.GetProjects() {
				if staleTestName(p.GetName()) {
					if _, err := zitadelClient.Project().DeleteProject(orgCtx, &projectv2.DeleteProjectRequest{ProjectId: p.GetProjectId()}); err != nil {
						note(fmt.Errorf("deleting project %s: %w", p.GetName(), err))
					} else {
						projectsDeleted++
					}
				}
			}
			if uint64(len(resp.GetProjects())) < 250 {
				break
			}
			offset += 250
		}

		for offset := uint64(0); ; {
			resp, err := zitadelClient.Management().ListUsers(orgCtx, &management.ListUsersRequest{ //nolint:staticcheck // SA1019: v1 Management API
				Query: &objectv1.ListQuery{Offset: offset, Limit: 250},
			})
			if err != nil {
				note(fmt.Errorf("listing users in org %s: %w", orgID, err))
				break
			}
			for _, u := range resp.GetResult() {
				if u.GetId() == bindingUserID || !staleTestName(u.GetUserName()) {
					continue
				}
				if _, err := zitadelClient.Management().RemoveUser(orgCtx, &management.RemoveUserRequest{Id: u.GetId()}); err != nil { //nolint:staticcheck // SA1019
					note(fmt.Errorf("deleting user %s: %w", u.GetUserName(), err))
				} else {
					usersDeleted++
				}
			}
			if uint64(len(resp.GetResult())) < 250 {
				break
			}
			offset += 250
		}
	}

	// 3. Instance-level IdP providers (controllers deactivate but cannot
	// delete them, so aborted IdP tests leak providers).
	if resp, err := zitadelClient.Admin().ListProviders(ctx, &admin.ListProvidersRequest{}); err != nil {
		note(fmt.Errorf("listing instance IdP providers: %w", err))
	} else {
		for _, p := range resp.GetResult() {
			if !staleTestName(p.GetName()) {
				continue
			}
			if _, err := zitadelClient.Admin().DeleteProvider(ctx, &admin.DeleteProviderRequest{Id: p.GetId()}); err != nil {
				note(fmt.Errorf("deleting provider %s: %w", p.GetName(), err))
			} else {
				providersDeleted++
			}
		}
	}

	return fmt.Sprintf("orgs=%d projects=%d users=%d idpProviders=%d failures=%d",
		orgsDeleted, projectsDeleted, usersDeleted, providersDeleted, failures), firstErr
}

// TestHygiene_SweepStaleResources runs the sweep on demand:
//
//	V018_HYGIENE=1 go test -tags=integration -run TestHygiene_Sweep ./tests/integration/
//
// It is skipped in normal runs — TestMain triggers the same sweep
// automatically after a fully successful suite (and preserves everything on
// failure for debugging); this entry point exists for manual cleanup of
// historical bloat.
func TestHygiene_SweepStaleResources(t *testing.T) {
	if os.Getenv("V018_HYGIENE") != "1" {
		t.Skip("set V018_HYGIENE=1 to run the manual hygiene sweep")
	}
	summary, err := sweepStaleTestResources(context.Background())
	t.Logf("hygiene sweep: %s", summary)
	if err != nil {
		t.Fatalf("sweep had failures (first: %v)", err)
	}
}
