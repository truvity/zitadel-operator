// Package scopemap resolves tenant namespaces to Zitadel org/project scopes
// via ScopeMap CRs living in the operator's namespace.
//
// v0.18 (INF-423). Semantics:
//   - First-match top-down within a map; evaluated across ALL maps in the
//     operator namespace (maps ordered by name for determinism).
//   - A namespace matching rules in two different maps is a conflict:
//     Events are emitted on both maps and the namespace is fail-closed.
//   - A namespace matching no rule is fail-closed (NoMatchingRule).
//   - A map whose spec.instance does not match the operator binding is
//     fail-closed (InstanceMismatch) for any namespace it would serve.
//   - Zero maps in the operator namespace = passthrough (feature off).
//     This gate is a prototype finding: without it, enabling the CRD is a
//     flag-day for every namespace the operator serves.
package scopemap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

// ErrMapsNotSynced signals that the scope-map informers have not synced yet.
// Consumers must requeue rather than treat this as a steady-state rejection.
var ErrMapsNotSynced = errors.New("scope map informers not yet synced")

// NoMatchError is the steady-state rejection: maps exist but none matched.
type NoMatchError struct {
	Namespace string
}

func (e *NoMatchError) Error() string {
	return fmt.Sprintf("namespace %q matches no rule in any ScopeMap (fail-closed)", e.Namespace)
}

// ConflictError signals a namespace matched rules in two or more maps.
type ConflictError struct {
	Namespace string
	Maps      []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("namespace %q matches rules in multiple ScopeMaps %v (fail-closed)", e.Namespace, e.Maps)
}

// InstanceMismatchError signals the only matching map binds a different instance.
type InstanceMismatchError struct {
	MapName  string
	Expected string
	Actual   string
}

func (e *InstanceMismatchError) Error() string {
	return fmt.Sprintf("ScopeMap %q instance %q does not match operator binding %q (fail-closed)",
		e.MapName, e.Actual, e.Expected)
}

// MapNotReadyError signals the matching map has not resolved its organization yet.
type MapNotReadyError struct {
	MapName string
}

func (e *MapNotReadyError) Error() string {
	return fmt.Sprintf("ScopeMap %q matched but has no resolved organization id yet", e.MapName)
}

// Scope is the resolved Zitadel scope for a namespace.
type Scope struct {
	// Instance is the Zitadel domain (matches the operator binding).
	Instance string
	// OrganizationID is the resolved org ID (authoritative).
	OrganizationID string
	// OrganizationName is the org name from the map spec (informational).
	OrganizationName string
	// ProjectName is the project name; empty means org-scope.
	ProjectName string
	// ProjectID pins the project when the rule set projectId. Empty otherwise
	// (the delegation layer resolves/creates the project by name).
	ProjectID string
	// MapName / RuleName identify the winning rule.
	MapName  string
	RuleName string
}

// IsOrgScope reports whether the scope is org-wide (no project).
func (s *Scope) IsOrgScope() bool { return s.ProjectName == "" && s.ProjectID == "" }

// scopeKey is the canonical JSON identity of a scope used for hashing.
type scopeKey struct {
	Instance       string `json:"instance"`
	OrganizationID string `json:"organizationId"`
	Project        string `json:"project,omitempty"`
	ProjectID      string `json:"projectId,omitempty"`
}

// KeyJSON returns the canonical JSON identity of the scope.
func (s *Scope) KeyJSON() []byte {
	b, _ := json.Marshal(scopeKey{
		Instance:       s.Instance,
		OrganizationID: s.OrganizationID,
		Project:        s.ProjectName,
		ProjectID:      s.ProjectID,
	})
	return b
}

// Hash returns a short stable hash of the scope identity, used in Secret names
// (zitadel-delegation-<hash>) and delegate usernames.
func (s *Scope) Hash() string {
	sum := sha256.Sum256(s.KeyJSON())
	return hex.EncodeToString(sum[:])[:10]
}

// Resolver resolves namespaces to scopes from cached ScopeMap objects.
type Resolver struct {
	// Reader is a cache-backed client (the manager's client).
	Reader client.Reader
	// Namespace is the operator namespace holding the maps.
	Namespace string
	// Instance is the operator's binding domain.
	Instance string
	// Synced reports whether the relevant informers have synced.
	Synced func() bool
	// Recorder emits Events on ScopeMap objects (conflicts).
	Recorder record.EventRecorder
}

// match is one map's first-matching rule for a namespace.
type match struct {
	m    *zitadelv1alpha2.ScopeMap
	rule *zitadelv1alpha2.ScopeMapRule
}

// Resolve maps a namespace to a Scope.
//
// Returns (nil, nil) when no maps exist (passthrough / feature off).
// Error taxonomy: ErrMapsNotSynced (requeue), *NoMatchError, *ConflictError,
// *InstanceMismatchError, *MapNotReadyError (all fail-closed except the first).
func (r *Resolver) Resolve(ctx context.Context, namespace string) (*Scope, error) {
	if r.Synced != nil && !r.Synced() {
		return nil, ErrMapsNotSynced
	}

	var maps zitadelv1alpha2.ScopeMapList
	if err := r.Reader.List(ctx, &maps, client.InNamespace(r.Namespace)); err != nil {
		return nil, fmt.Errorf("listing ScopeMaps in %q: %w", r.Namespace, err)
	}
	if len(maps.Items) == 0 {
		return nil, nil // passthrough: feature not enabled
	}

	var ns corev1.Namespace
	if err := r.Reader.Get(ctx, types.NamespacedName{Name: namespace}, &ns); err != nil {
		return nil, fmt.Errorf("get namespace %q: %w", namespace, err)
	}

	// Deterministic order across maps.
	sort.Slice(maps.Items, func(i, j int) bool { return maps.Items[i].Name < maps.Items[j].Name })

	matches, err := collectMatches(maps.Items, &ns)
	if err != nil {
		return nil, err
	}

	switch len(matches) {
	case 0:
		return nil, &NoMatchError{Namespace: namespace}
	case 1:
		// fallthrough below
	default:
		return nil, r.reportConflict(namespace, matches)
	}

	mt := matches[0]
	if mt.m.Spec.Instance != r.Instance {
		return nil, &InstanceMismatchError{MapName: mt.m.Name, Expected: r.Instance, Actual: mt.m.Spec.Instance}
	}

	// Organization ID: spec is authoritative when present, else the map
	// controller's name-lookup result from status.
	orgID := mt.m.Spec.OrganizationId
	if orgID == "" {
		orgID = mt.m.Status.ResolvedOrganizationId
	}
	if orgID == "" {
		return nil, &MapNotReadyError{MapName: mt.m.Name}
	}

	return &Scope{
		Instance:         mt.m.Spec.Instance,
		OrganizationID:   orgID,
		OrganizationName: mt.m.Spec.Organization,
		ProjectName:      mt.rule.Project,
		ProjectID:        mt.rule.ProjectId,
		MapName:          mt.m.Name,
		RuleName:         mt.rule.Name,
	}, nil
}

// collectMatches gathers each map's first-matching rule (top-down) for the namespace.
func collectMatches(items []zitadelv1alpha2.ScopeMap, ns *corev1.Namespace) ([]match, error) {
	var matches []match
	for i := range items {
		m := &items[i]
		for j := range m.Spec.Rules {
			rule := &m.Spec.Rules[j]
			ok, err := ruleMatches(rule, ns)
			if err != nil {
				return nil, fmt.Errorf("evaluating rule %q in map %q: %w", rule.Name, m.Name, err)
			}
			if ok {
				matches = append(matches, match{m: m, rule: rule})
				break // first match top-down within this map
			}
		}
	}
	return matches, nil
}

// reportConflict emits Events on all conflicting maps and returns the error.
func (r *Resolver) reportConflict(namespace string, matches []match) error {
	names := make([]string, 0, len(matches))
	for _, mt := range matches {
		names = append(names, mt.m.Name)
	}
	if r.Recorder != nil {
		for _, mt := range matches {
			r.Recorder.Eventf(mt.m, corev1.EventTypeWarning, "ScopeConflict",
				"namespace %q also matches rule in maps %v; namespace is fail-closed", namespace, names)
		}
	}
	return &ConflictError{Namespace: namespace, Maps: names}
}

// ruleMatches evaluates a single rule against a namespace object.
func ruleMatches(rule *zitadelv1alpha2.ScopeMapRule, ns *corev1.Namespace) (bool, error) {
	if err := ValidateRule(rule); err != nil {
		// Invalid rules never match (the map controller reports them).
		return false, nil
	}
	if len(rule.Namespaces) > 0 {
		for _, n := range rule.Namespaces {
			if n == ns.Name {
				return true, nil
			}
		}
		return false, nil
	}
	sel, err := metav1.LabelSelectorAsSelector(rule.NamespaceSelector)
	if err != nil {
		return false, err
	}
	return sel.Matches(labels.Set(ns.Labels)), nil
}

// ValidateRule enforces rule invariants:
//   - exactly one of namespaceSelector / namespaces
//
// project/projectId are both optional in any combination: an ID is
// authoritative when set, a bare name is resolved/created on first use.
func ValidateRule(rule *zitadelv1alpha2.ScopeMapRule) error {
	hasSelector := rule.NamespaceSelector != nil
	hasLiteral := len(rule.Namespaces) > 0
	if hasSelector == hasLiteral {
		return fmt.Errorf("rule %q: exactly one of namespaceSelector or namespaces must be set", rule.Name)
	}
	return nil
}
