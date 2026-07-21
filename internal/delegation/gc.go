package delegation

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/truvity/zitadel-operator/internal/scopemap"
)

// DefaultSweepInterval is the periodic orphan-GC cadence.
const DefaultSweepInterval = 10 * time.Minute

// GC computes the live scope set from current scope-map resolution across all
// namespaces and sweeps the delegation Manager: delegates whose scope no
// longer resolves anywhere are revoked (eager revoke on unmatch happens via
// SweepOnce being invoked from the scope-map controller; the periodic Run
// loop catches orphans).
type GC struct {
	K8s      client.Reader
	Resolver *scopemap.Resolver
	Manager  *Manager
	// Interval overrides DefaultSweepInterval for the periodic loop.
	Interval time.Duration
}

// SweepOnce resolves every namespace to its scope, then revokes delegates
// outside the live set and finishes pending key rotations.
// Aborts (without revoking anything) while scope-map informers are unsynced.
func (g *GC) SweepOnce(ctx context.Context) error {
	var namespaces corev1.NamespaceList
	if err := g.K8s.List(ctx, &namespaces); err != nil {
		return fmt.Errorf("listing namespaces for delegation sweep: %w", err)
	}

	live := map[string]bool{}
	for i := range namespaces.Items {
		ns := &namespaces.Items[i]
		s, err := g.Resolver.Resolve(ctx, ns.Name)
		if err != nil {
			if errors.Is(err, scopemap.ErrMapsNotSynced) {
				return err // never revoke on an unsynced view
			}
			// NoMatch / Conflict / InstanceMismatch / NotReady: the namespace
			// contributes no live scope.
			continue
		}
		if s != nil {
			live[s.Hash()] = true
		}
	}
	return g.Manager.Sweep(ctx, live)
}

// Run periodically sweeps until the context is canceled. Intended to be
// registered as a leadership-gated manager Runnable.
func (g *GC) Run(ctx context.Context) error {
	interval := g.Interval
	if interval <= 0 {
		interval = DefaultSweepInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger := log.FromContext(ctx).WithName("delegation-gc")
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := g.SweepOnce(ctx); err != nil {
				logger.Error(err, "delegation sweep failed")
			}
		}
	}
}
