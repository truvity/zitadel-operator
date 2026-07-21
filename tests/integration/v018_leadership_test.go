//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// TestLeaderElection_TwoProcessHandoff (S-270, INF-427) runs two manager
// instances ("processes") competing for one lease: exactly one leads; when
// the leader shuts down gracefully the standby takes over. This is the
// rolling-update reality that makes leader election unconditional — without
// it, old and new pods reconcile concurrently (double-minted delegates,
// racing Secret writes).
func TestLeaderElection_TwoProcessHandoff(t *testing.T) {
	newMgr := func() ctrl.Manager {
		mgr, err := ctrl.NewManager(testRestCfg, ctrl.Options{
			Scheme:                        k8sClient.Scheme(),
			Metrics:                       metricsserver.Options{BindAddress: "0"},
			HealthProbeBindAddress:        "0",
			LeaderElection:                true,
			LeaderElectionID:              "v018-leadership-handoff-test",
			LeaderElectionNamespace:       "default",
			LeaderElectionReleaseOnCancel: true,
			LeaseDuration:                 ptrDuration(5 * time.Second),
			RenewDeadline:                 ptrDuration(4 * time.Second),
			RetryPeriod:                   ptrDuration(1 * time.Second),
		})
		if err != nil {
			t.Fatalf("creating manager: %v", err)
		}
		return mgr
	}

	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	mgrA, mgrB := newMgr(), newMgr()
	go func() { _ = mgrA.Start(ctxA) }()

	select {
	case <-mgrA.Elected():
		t.Log("manager A acquired leadership")
	case <-time.After(30 * time.Second):
		t.Fatal("manager A never became leader")
	}

	go func() { _ = mgrB.Start(ctxB) }()

	// B must NOT lead while A holds the lease.
	select {
	case <-mgrB.Elected():
		t.Fatal("manager B became leader while A holds the lease")
	case <-time.After(6 * time.Second):
		t.Log("manager B correctly waiting on the lease")
	}

	// Graceful shutdown of A releases the lease; B takes over.
	cancelA()
	select {
	case <-mgrB.Elected():
		t.Log("leadership handoff: manager B took over after A shut down")
	case <-time.After(30 * time.Second):
		t.Fatal("manager B never took over leadership")
	}
	cancelB()
	time.Sleep(1 * time.Second) // let B release the lease before envtest teardown
}

func ptrDuration(d time.Duration) *time.Duration { return &d }
