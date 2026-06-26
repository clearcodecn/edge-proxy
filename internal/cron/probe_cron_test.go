package cron

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"edge-proxy/internal/probe"
	"edge-proxy/internal/store"
)

func newProbeRepo(t *testing.T) *store.DomainRepo {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "edge.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return store.NewDomainRepo(db)
}

func makeProbeFn(result probe.ProbeResult, calls *atomic.Int32, hostHit *[]string) ProbeFn {
	return func(_ context.Context, host, _ string) probe.ProbeResult {
		calls.Add(1)
		if hostHit != nil {
			*hostHit = append(*hostHit, host)
		}
		return result
	}
}

func TestProbeTick_OnlineDegradesAfterThreshold(t *testing.T) {
	repo := newProbeRepo(t)
	d, _ := repo.Create("on.com")
	_ = repo.UpdateStatus(d.ID, store.StatusOnline)

	notif := &fakeNotifier{}
	var calls atomic.Int32
	c := NewProbeCron(repo, makeProbeFn(probe.ProbeResult{OK: false, StatusCode: 502}, &calls, nil), notif, "/")

	// Two failures should NOT degrade yet.
	c.Tick(context.Background())
	c.Tick(context.Background())
	got, _ := repo.GetByID(d.ID)
	if got.Status != store.StatusOnline {
		t.Errorf("after 2 failures status = %q, want online", got.Status)
	}
	if got.FailCount != 2 {
		t.Errorf("FailCount = %d, want 2", got.FailCount)
	}
	if len(notif.calls) != 0 {
		t.Errorf("should not alert yet, got %d alerts", len(notif.calls))
	}

	// Third failure crosses threshold.
	c.Tick(context.Background())
	got, _ = repo.GetByID(d.ID)
	if got.Status != store.StatusDegraded {
		t.Errorf("after 3 failures status = %q, want degraded", got.Status)
	}
	if got.FailCount != 0 {
		t.Errorf("FailCount should reset to 0 on transition, got %d", got.FailCount)
	}
	if len(notif.calls) != 1 || notif.calls[0].key != "edge_probe:on.com" {
		t.Errorf("alerts = %+v", notif.calls)
	}
}

func TestProbeTick_DegradedRecoversAfterThreshold(t *testing.T) {
	repo := newProbeRepo(t)
	d, _ := repo.Create("rec.com")
	_ = repo.UpdateStatus(d.ID, store.StatusDegraded)

	notif := &fakeNotifier{}
	var calls atomic.Int32
	c := NewProbeCron(repo, makeProbeFn(probe.ProbeResult{OK: true, StatusCode: 200}, &calls, nil), notif, "/")

	// One success: counts toward recovery but does not recover yet.
	c.Tick(context.Background())
	got, _ := repo.GetByID(d.ID)
	if got.Status != store.StatusDegraded {
		t.Errorf("after 1 success status = %q, want degraded", got.Status)
	}
	if got.FailCount != 1 {
		t.Errorf("recovery streak = %d, want 1", got.FailCount)
	}

	// Second success crosses recovery threshold.
	c.Tick(context.Background())
	got, _ = repo.GetByID(d.ID)
	if got.Status != store.StatusOnline {
		t.Errorf("after 2 successes status = %q, want online", got.Status)
	}
	if got.FailCount != 0 {
		t.Errorf("FailCount should reset on recovery, got %d", got.FailCount)
	}
	if len(notif.calls) != 0 {
		t.Errorf("recovery should not alert, got %d alerts", len(notif.calls))
	}
}

func TestProbeTick_DegradedSingleFailResetsRecoveryStreak(t *testing.T) {
	repo := newProbeRepo(t)
	d, _ := repo.Create("flap.com")
	_ = repo.UpdateStatus(d.ID, store.StatusDegraded)

	notif := &fakeNotifier{}

	// First a success to bump recovery streak.
	var ok atomic.Int32
	c := NewProbeCron(repo, makeProbeFn(probe.ProbeResult{OK: true, StatusCode: 200}, &ok, nil), notif, "/")
	c.Tick(context.Background())
	got, _ := repo.GetByID(d.ID)
	if got.FailCount != 1 {
		t.Fatalf("setup: FailCount = %d", got.FailCount)
	}

	// Then a failure: still degraded, recovery streak reset.
	var fail atomic.Int32
	c.Probe = makeProbeFn(probe.ProbeResult{OK: false, StatusCode: 500}, &fail, nil)
	c.Tick(context.Background())
	got, _ = repo.GetByID(d.ID)
	if got.Status != store.StatusDegraded {
		t.Errorf("status = %q", got.Status)
	}
	if got.FailCount != 0 {
		t.Errorf("FailCount should reset to 0 on flap-back, got %d", got.FailCount)
	}
	if len(notif.calls) != 0 {
		t.Errorf("flap-back should not re-alert (already degraded)")
	}
}

func TestProbeTick_OnlineSuccessKeepsFailCountZero(t *testing.T) {
	repo := newProbeRepo(t)
	d, _ := repo.Create("happy.com")
	_ = repo.UpdateStatus(d.ID, store.StatusOnline)

	notif := &fakeNotifier{}
	var calls atomic.Int32
	c := NewProbeCron(repo, makeProbeFn(probe.ProbeResult{OK: true, StatusCode: 200}, &calls, nil), notif, "/")
	c.Tick(context.Background())
	got, _ := repo.GetByID(d.ID)
	if got.Status != store.StatusOnline {
		t.Errorf("status = %q", got.Status)
	}
	if !got.LastProbeOK {
		t.Error("LastProbeOK should be true")
	}
	if got.LastProbeAt == nil {
		t.Error("LastProbeAt nil")
	}
}

func TestProbeTick_OnlineFailRecoveryStreakInterrupted(t *testing.T) {
	repo := newProbeRepo(t)
	d, _ := repo.Create("blip.com")
	_ = repo.UpdateStatus(d.ID, store.StatusOnline)

	notif := &fakeNotifier{}

	// 2 consecutive failures
	var failCalls atomic.Int32
	c := NewProbeCron(repo, makeProbeFn(probe.ProbeResult{OK: false, StatusCode: 500}, &failCalls, nil), notif, "/")
	c.Tick(context.Background())
	c.Tick(context.Background())

	// One success interrupts the streak
	var okCalls atomic.Int32
	c.Probe = makeProbeFn(probe.ProbeResult{OK: true, StatusCode: 200}, &okCalls, nil)
	c.Tick(context.Background())

	got, _ := repo.GetByID(d.ID)
	if got.Status != store.StatusOnline {
		t.Errorf("status = %q", got.Status)
	}
	if got.FailCount != 0 {
		t.Errorf("FailCount should reset on success while online, got %d", got.FailCount)
	}

	// Now do 3 fails - should still degrade (counter was reset).
	c.Probe = makeProbeFn(probe.ProbeResult{OK: false, StatusCode: 500}, &failCalls, nil)
	c.Tick(context.Background())
	c.Tick(context.Background())
	c.Tick(context.Background())
	got, _ = repo.GetByID(d.ID)
	if got.Status != store.StatusDegraded {
		t.Errorf("after 3 fresh failures status = %q, want degraded", got.Status)
	}
	if len(notif.calls) != 1 {
		t.Errorf("alerts = %d, want 1", len(notif.calls))
	}
}

func TestProbeTick_IgnoresDeprecatedAndPending(t *testing.T) {
	repo := newProbeRepo(t)
	online, _ := repo.Create("on.com")
	_ = repo.UpdateStatus(online.ID, store.StatusOnline)

	deprecated, _ := repo.Create("dep.com")
	_ = repo.UpdateStatus(deprecated.ID, store.StatusDeprecated)

	pending, _ := repo.Create("pen.com")
	_ = pending
	// pending stays pending

	notif := &fakeNotifier{}
	var hitHosts []string
	var calls atomic.Int32
	c := NewProbeCron(repo, makeProbeFn(probe.ProbeResult{OK: true, StatusCode: 200}, &calls, &hitHosts), notif, "/")
	c.Tick(context.Background())

	if len(hitHosts) != 1 || hitHosts[0] != "on.com" {
		t.Errorf("probed hosts = %v, want only [on.com]", hitHosts)
	}
}

func TestProbeTick_AlertFormatCarriesError(t *testing.T) {
	repo := newProbeRepo(t)
	d, _ := repo.Create("err.com")
	_ = repo.UpdateStatus(d.ID, store.StatusOnline)

	notif := &fakeNotifier{}
	var calls atomic.Int32
	c := NewProbeCron(repo, makeProbeFn(probe.ProbeResult{OK: false, Err: errors.New("tls: handshake failure")}, &calls, nil), notif, "/")
	for i := 0; i < 3; i++ {
		c.Tick(context.Background())
	}
	if len(notif.calls) != 1 {
		t.Fatalf("alerts = %d", len(notif.calls))
	}
	msg := notif.calls[0].msg
	if !contains(msg, "TLS 校验失败") {
		t.Errorf("alert should include TLS classification, got:\n%s", msg)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
