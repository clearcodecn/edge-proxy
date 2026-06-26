package cron

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRenewer struct {
	renewed []string
	failed  []string
	err     error
	calls   int
}

func (f *fakeRenewer) Renew() ([]string, []string, error) {
	f.calls++
	return f.renewed, f.failed, f.err
}

type fakeReloader struct {
	err   error
	calls int
}

func (f *fakeReloader) Reload() error {
	f.calls++
	return f.err
}

type fakeRenewRepo struct {
	updates map[string]time.Time
	err     error
}

func newFakeRenewRepo() *fakeRenewRepo {
	return &fakeRenewRepo{updates: make(map[string]time.Time)}
}

func (f *fakeRenewRepo) UpdateCertAt(host string, t time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.updates[host] = t
	return nil
}

func TestRenewTick_SuccessUpdatesCertAtAndReloads(t *testing.T) {
	repo := newFakeRenewRepo()
	cb := &fakeRenewer{renewed: []string{"a.com", "b.com"}}
	nx := &fakeReloader{}
	notif := &fakeNotifier{}
	c := NewRenewCron(repo, cb, nx, notif)
	now := time.Date(2026, 6, 26, 3, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return now }

	c.Tick(context.Background())

	if cb.calls != 1 {
		t.Errorf("renew called %d times, want 1", cb.calls)
	}
	if nx.calls != 1 {
		t.Errorf("reload called %d times, want 1", nx.calls)
	}
	if len(repo.updates) != 2 {
		t.Errorf("updates = %v", repo.updates)
	}
	if !repo.updates["a.com"].Equal(now) {
		t.Errorf("a.com cert_at = %v, want %v", repo.updates["a.com"], now)
	}
	if len(notif.calls) != 0 {
		t.Errorf("should not alert on full success, got %d", len(notif.calls))
	}
}

func TestRenewTick_FailedHostAlertsButStillReloads(t *testing.T) {
	repo := newFakeRenewRepo()
	cb := &fakeRenewer{
		renewed: []string{"good.com"},
		failed:  []string{"bad.com"},
		err:     errors.New("certbot renew: 1 host(s) failed: bad.com"),
	}
	nx := &fakeReloader{}
	notif := &fakeNotifier{}
	c := NewRenewCron(repo, cb, nx, notif)

	c.Tick(context.Background())

	if nx.calls != 1 {
		t.Errorf("reload should still run, called %d times", nx.calls)
	}
	if _, ok := repo.updates["good.com"]; !ok {
		t.Error("good.com should still get UpdateCertAt despite peer failure")
	}
	if len(notif.calls) != 1 {
		t.Fatalf("alerts = %d, want 1", len(notif.calls))
	}
	if notif.calls[0].key != "edge_renew:bad.com" {
		t.Errorf("alert key = %q", notif.calls[0].key)
	}
}

func TestRenewTick_MultipleFailedHostsEachAlerted(t *testing.T) {
	repo := newFakeRenewRepo()
	cb := &fakeRenewer{
		failed: []string{"x.com", "y.com", "z.com"},
		err:    errors.New("3 failed"),
	}
	nx := &fakeReloader{}
	notif := &fakeNotifier{}
	c := NewRenewCron(repo, cb, nx, notif)
	c.Tick(context.Background())

	if len(notif.calls) != 3 {
		t.Errorf("alerts = %d, want 3", len(notif.calls))
	}
	keys := map[string]bool{}
	for _, c := range notif.calls {
		keys[c.key] = true
	}
	for _, host := range []string{"x.com", "y.com", "z.com"} {
		if !keys["edge_renew:"+host] {
			t.Errorf("missing alert for %s", host)
		}
	}
}

func TestRenewTick_NothingToRenewStillReloads(t *testing.T) {
	repo := newFakeRenewRepo()
	cb := &fakeRenewer{}
	nx := &fakeReloader{}
	notif := &fakeNotifier{}
	c := NewRenewCron(repo, cb, nx, notif)
	c.Tick(context.Background())

	if nx.calls != 1 {
		t.Errorf("reload called %d times, want 1 (idempotent)", nx.calls)
	}
	if len(notif.calls) != 0 {
		t.Error("no alerts expected")
	}
}

func TestRenewTick_ReloadFailureAlertsAndContinues(t *testing.T) {
	repo := newFakeRenewRepo()
	cb := &fakeRenewer{renewed: []string{"r.com"}}
	nx := &fakeReloader{err: errors.New("reload denied")}
	notif := &fakeNotifier{}
	c := NewRenewCron(repo, cb, nx, notif)
	c.Tick(context.Background())

	if _, ok := repo.updates["r.com"]; !ok {
		t.Error("UpdateCertAt should still happen even if reload fails")
	}
	if len(notif.calls) != 1 || notif.calls[0].key != "edge_nginx:renew" {
		t.Errorf("expected edge_nginx:renew alert, got %+v", notif.calls)
	}
}

func TestRenewTick_RepoUpdateErrorTolerated(t *testing.T) {
	repo := newFakeRenewRepo()
	repo.err = errors.New("db locked")
	cb := &fakeRenewer{renewed: []string{"e.com"}}
	nx := &fakeReloader{}
	notif := &fakeNotifier{}
	c := NewRenewCron(repo, cb, nx, notif)
	c.Tick(context.Background())

	if nx.calls != 1 {
		t.Errorf("reload should still run even if UpdateCertAt errors")
	}
}
