// Package chartcheck cross-checks the Helm charts against each other —
// the assertions that keep hand-maintained enumerations honest.
package chartcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestRBACCoversEveryCRD asserts that BOTH RBAC templates (the
// cluster-wide ClusterRole and the namespaced Role) grant the operator
// every resource kind the CRD chart ships. This has now failed twice in
// production the same way: a new CRD lands, ONE of the two enumerations
// gains it, the other doesn't, and the operator crashloops on a
// forbidden watch (#89: projectroles missing from the ClusterRole while
// the Role had it). The enumerations stay hand-written — RBAC is policy
// and should read as policy — but they no longer get to drift.
//
// scopemaps are the one deliberate exception: admin-owned routing
// config, granted read-only + status, never finalizers.
func TestRBACCoversEveryCRD(t *testing.T) {
	root := repoRoot(t)

	plurals := crdPlurals(t, filepath.Join(root, "charts", "zitadel-operator-crds", "templates"))
	if len(plurals) < 30 {
		t.Fatalf("suspiciously few CRDs found (%d) — did the CRD chart move?", len(plurals))
	}

	readOnly := map[string]bool{"scopemaps": true}

	for _, tmpl := range []string{"clusterrole.yaml", "role.yaml"} {
		raw, err := os.ReadFile(filepath.Join(root, "charts", "zitadel-operator", "templates", tmpl))
		if err != nil {
			t.Fatalf("read %s: %v", tmpl, err)
		}

		granted := grantedResources(string(raw))

		for _, plural := range plurals {
			want := []string{plural, plural + "/status"}
			if !readOnly[plural] {
				want = append(want, plural+"/finalizers")
			}

			for _, res := range want {
				if !granted[res] {
					t.Errorf("%s: CRD %q shipped but %q is not granted — the operator will crashloop on a forbidden watch", tmpl, plural, res)
				}
			}
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up")
		}

		dir = parent
	}
}

// crdPlurals derives the resource plurals from the CRD chart's file
// names (zitadel.truvity.io_<plural>.yaml) — the same set the operator
// registers informers for.
func crdPlurals(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read CRD templates: %v", err)
	}

	var plurals []string

	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "zitadel.truvity.io_") || !strings.HasSuffix(name, ".yaml") {
			continue
		}

		plurals = append(plurals, strings.TrimSuffix(strings.TrimPrefix(name, "zitadel.truvity.io_"), ".yaml"))
	}

	sort.Strings(plurals)

	return plurals
}

// grantedResources collects every `- <resource>` list entry from the
// template's rules. Coarse on purpose: it reads resource lines wherever
// they appear, because a resource granted under the WRONG verbs is a
// different failure than one absent entirely, and absence is the one
// that crashloops.
func grantedResources(raw string) map[string]bool {
	granted := map[string]bool{}

	re := regexp.MustCompile(`(?m)^\s+-\s+([a-z]+(?:/(?:status|finalizers))?)\s*$`)
	for _, m := range re.FindAllStringSubmatch(raw, -1) {
		granted[m[1]] = true
	}

	return granted
}
