package cron

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"edge-proxy/internal/store"
)

// ── test doubles ──────────────────────────────────────────────────────────

type fakeCertbot struct {
	err   error
	calls atomic.Int32
}

func (f *fakeCertbot) Apply(host string) error {
	f.calls.Add(1)
	return f.err
}

type recordedNginxCall struct {
	filename string
	content  []byte
}

type fakeNginx struct {
	err   error
	calls []recordedNginxCall
}

func (f *fakeNginx) WriteAndApply(filename string, content []byte) error {
	f.calls = append(f.calls, recordedNginxCall{filename, append([]byte(nil), content...)})
	return f.err
}

type fakeNotifier struct {
	calls []struct {
		key string
		msg string
	}
}

func (f *fakeNotifier) Alert(key, msg string) {
	f.calls = append(f.calls, struct {
		key string
		msg string
	}{key, msg})
}

func openRealRepo(t *testing.T) *store.DomainRepo {
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

// ── tests ─────────────────────────────────────────────────────────────────

func TestACMETick_SuccessPath(t *testing.T) {
	repo := openRealRepo(t)
	d, _ := repo.Create("a.example.com")

	cb := &fakeCertbot{}
	nx := &fakeNginx{}
	notif := &fakeNotifier{}
	c := NewACMECron(repo, cb, nx, notif)

	c.Tick(context.Background())

	got, _ := repo.GetByID(d.ID)
	if got.Status != store.StatusOnline {
		t.Errorf("status = %q, want online", got.Status)
	}
	if got.CertAt == nil {
		t.Error("CertAt should be set")
	}
	if cb.calls.Load() != 1 {
		t.Errorf("certbot called %d times, want 1", cb.calls.Load())
	}
	if len(nx.calls) != 1 {
		t.Fatalf("nginx applied %d times, want 1", len(nx.calls))
	}
	if nx.calls[0].filename != "edge-a.example.com.conf" {
		t.Errorf("filename = %q", nx.calls[0].filename)
	}
	if len(notif.calls) != 0 {
		t.Errorf("no alerts expected on success, got %d", len(notif.calls))
	}
}

func TestACMETick_CertbotFailureMarksAndAlerts(t *testing.T) {
	repo := openRealRepo(t)
	d, _ := repo.Create("b.example.com")

	cb := &fakeCertbot{err: errors.New("dns problem")}
	nx := &fakeNginx{}
	notif := &fakeNotifier{}
	c := NewACMECron(repo, cb, nx, notif)

	c.Tick(context.Background())

	got, _ := repo.GetByID(d.ID)
	if got.Status != store.StatusCertFailed {
		t.Errorf("status = %q, want cert_failed", got.Status)
	}
	if got.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", got.FailCount)
	}
	if got.LastError == "" {
		t.Error("LastError should be set")
	}
	if len(nx.calls) != 0 {
		t.Error("nginx should not be called on certbot failure")
	}
	if len(notif.calls) != 1 || notif.calls[0].key != "edge_acme:b.example.com" {
		t.Errorf("alerts = %+v", notif.calls)
	}
}

func TestACMETick_FifthFailureSetsBackoff(t *testing.T) {
	repo := openRealRepo(t)
	d, _ := repo.Create("c.example.com")

	cb := &fakeCertbot{err: errors.New("transient")}
	nx := &fakeNginx{}
	notif := &fakeNotifier{}
	now := time.Date(2026, 6, 26, 14, 0, 0, 0, time.UTC)
	c := NewACMECron(repo, cb, nx, notif)
	c.Now = func() time.Time { return now }

	for i := 1; i <= 5; i++ {
		// Manually reset to pending so PickUnready picks the same domain again.
		_ = repo.UpdateStatus(d.ID, store.StatusPending)
		c.Tick(context.Background())
	}

	got, _ := repo.GetByID(d.ID)
	if got.FailCount != 5 {
		t.Fatalf("FailCount = %d, want 5", got.FailCount)
	}
	if got.NextRetryAt == nil {
		t.Fatal("NextRetryAt should be set after 5th failure")
	}
	expectedRetry := now.Add(FailureBackoff)
	if !got.NextRetryAt.Equal(expectedRetry) {
		t.Errorf("NextRetryAt = %v, want %v", got.NextRetryAt, expectedRetry)
	}
}

func TestACMETick_NginxApplyFailureAlertsNginxKey(t *testing.T) {
	repo := openRealRepo(t)
	d, _ := repo.Create("e.example.com")

	cb := &fakeCertbot{} // success
	nx := &fakeNginx{err: errors.New("nginx -t failed")}
	notif := &fakeNotifier{}
	c := NewACMECron(repo, cb, nx, notif)

	c.Tick(context.Background())

	got, _ := repo.GetByID(d.ID)
	if got.Status != store.StatusCertFailed {
		t.Errorf("status = %q, want cert_failed", got.Status)
	}
	if len(notif.calls) != 1 || notif.calls[0].key != "edge_nginx:e.example.com" {
		t.Errorf("expected edge_nginx alert, got %+v", notif.calls)
	}
}

func TestACMETick_SingleDomainPerTick(t *testing.T) {
	repo := openRealRepo(t)
	d1, _ := repo.Create("first.com")
	time.Sleep(2 * time.Millisecond)
	_, _ = repo.Create("second.com")

	cb := &fakeCertbot{}
	nx := &fakeNginx{}
	notif := &fakeNotifier{}
	c := NewACMECron(repo, cb, nx, notif)

	c.Tick(context.Background())

	if cb.calls.Load() != 1 {
		t.Errorf("certbot called %d times, want 1 per tick", cb.calls.Load())
	}
	got, _ := repo.GetByID(d1.ID)
	if got.Status != store.StatusOnline {
		t.Errorf("first domain status = %q, want online (oldest first)", got.Status)
	}
}

func TestACMETick_NoUnready_NoOp(t *testing.T) {
	repo := openRealRepo(t)
	cb := &fakeCertbot{}
	nx := &fakeNginx{}
	notif := &fakeNotifier{}
	c := NewACMECron(repo, cb, nx, notif)

	c.Tick(context.Background())

	if cb.calls.Load() != 0 {
		t.Errorf("certbot should not be called when no domains, got %d", cb.calls.Load())
	}
}

func TestACMETick_StatusFlipsToCertApplyingBeforeWork(t *testing.T) {
	repo := openRealRepo(t)
	d, _ := repo.Create("flip.com")

	// Inject a certbot that asserts the status is cert_applying when it's called.
	cb := &certbotChecker{repo: repo, domainID: d.ID, t: t}
	nx := &fakeNginx{}
	notif := &fakeNotifier{}
	c := NewACMECron(repo, cb, nx, notif)
	c.Tick(context.Background())
}

type certbotChecker struct {
	repo     *store.DomainRepo
	domainID int64
	t        *testing.T
}

func (c *certbotChecker) Apply(string) error {
	got, _ := c.repo.GetByID(c.domainID)
	if got == nil {
		c.t.Fatal("domain disappeared")
	}
	if got.Status != store.StatusCertApplying {
		c.t.Errorf("during certbot call status = %q, want cert_applying", got.Status)
	}
	return nil
}
