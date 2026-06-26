package store

import (
	"errors"
	"testing"
	"time"
)

func TestDomainRepo_CreateAndGet(t *testing.T) {
	repo := openTest(t)
	d, err := repo.Create("foo.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.Status != StatusPending {
		t.Errorf("status = %q, want pending", d.Status)
	}
	got, err := repo.GetByHost("foo.com")
	if err != nil {
		t.Fatalf("GetByHost: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("id mismatch")
	}

	if _, err := repo.GetByHost("missing.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDomainRepo_ListByStatus(t *testing.T) {
	repo := openTest(t)
	_, _ = repo.Create("a.com")
	b, _ := repo.Create("b.com")
	c, _ := repo.Create("c.com")
	_ = repo.UpdateStatus(b.ID, StatusOnline)
	_ = repo.UpdateStatus(c.ID, StatusDeprecated)

	pending, err := repo.ListByStatus(StatusPending)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(pending) != 1 || pending[0].Host != "a.com" {
		t.Errorf("pending = %+v", pending)
	}

	online, _ := repo.ListByStatus(StatusOnline, StatusDegraded)
	if len(online) != 1 || online[0].Host != "b.com" {
		t.Errorf("online = %+v", online)
	}
}

func TestDomainRepo_PickUnready_OrderedByCreated(t *testing.T) {
	repo := openTest(t)
	d1, _ := repo.Create("oldest.com")
	time.Sleep(2 * time.Millisecond)
	_, _ = repo.Create("newer.com")

	picked, err := repo.PickUnready(time.Now())
	if err != nil {
		t.Fatalf("PickUnready: %v", err)
	}
	if picked.ID != d1.ID {
		t.Errorf("picked %q, want oldest.com", picked.Host)
	}
}

func TestDomainRepo_PickUnready_RespectsBackoff(t *testing.T) {
	repo := openTest(t)
	d, _ := repo.Create("late.com")
	_ = repo.UpdateStatus(d.ID, StatusCertFailed)
	future := time.Now().Add(1 * time.Hour)
	_ = repo.SetNextRetryAt(d.ID, future)

	if _, err := repo.PickUnready(time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound during backoff, got %v", err)
	}

	if _, err := repo.PickUnready(future.Add(time.Minute)); err != nil {
		t.Errorf("expected pick after backoff, got %v", err)
	}
}

func TestDomainRepo_PickUnready_IgnoresOnlineAndDeprecated(t *testing.T) {
	repo := openTest(t)
	a, _ := repo.Create("online.com")
	b, _ := repo.Create("deprecated.com")
	_ = repo.UpdateStatus(a.ID, StatusOnline)
	_ = repo.UpdateStatus(b.ID, StatusDeprecated)
	if _, err := repo.PickUnready(time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected nothing pickable, got %v", err)
	}
}

func TestDomainRepo_MarkCertApplied_ResetsFailureState(t *testing.T) {
	repo := openTest(t)
	d, _ := repo.Create("ok.com")
	_ = repo.MarkCertFailed(d.ID, "first try failed")
	_ = repo.MarkCertFailed(d.ID, "second try failed")
	_ = repo.SetNextRetryAt(d.ID, time.Now().Add(1*time.Hour))

	if err := repo.MarkCertApplied(d.ID); err != nil {
		t.Fatalf("MarkCertApplied: %v", err)
	}
	got, _ := repo.GetByID(d.ID)
	if got.Status != StatusOnline {
		t.Errorf("status = %q", got.Status)
	}
	if got.CertAt == nil {
		t.Error("CertAt nil")
	}
	if got.FailCount != 0 {
		t.Errorf("FailCount = %d", got.FailCount)
	}
	if got.LastError != "" {
		t.Errorf("LastError = %q", got.LastError)
	}
	if got.NextRetryAt != nil {
		t.Errorf("NextRetryAt not cleared: %v", got.NextRetryAt)
	}
}

func TestDomainRepo_MarkCertFailed_BumpsCount(t *testing.T) {
	repo := openTest(t)
	d, _ := repo.Create("fail.com")
	for i := 1; i <= 3; i++ {
		if err := repo.MarkCertFailed(d.ID, "boom"); err != nil {
			t.Fatalf("MarkCertFailed: %v", err)
		}
	}
	got, _ := repo.GetByID(d.ID)
	if got.FailCount != 3 {
		t.Errorf("FailCount = %d, want 3", got.FailCount)
	}
	if got.Status != StatusCertFailed {
		t.Errorf("Status = %q", got.Status)
	}
	if got.LastError != "boom" {
		t.Errorf("LastError = %q", got.LastError)
	}
}

func TestDomainRepo_UpdateProbeResult(t *testing.T) {
	repo := openTest(t)
	d, _ := repo.Create("probe.com")
	_ = repo.UpdateStatus(d.ID, StatusOnline)
	if err := repo.UpdateProbeResult(d.ID, false, 2, StatusOnline); err != nil {
		t.Fatalf("UpdateProbeResult: %v", err)
	}
	got, _ := repo.GetByID(d.ID)
	if got.LastProbeOK {
		t.Error("LastProbeOK should be false")
	}
	if got.FailCount != 2 {
		t.Errorf("FailCount = %d", got.FailCount)
	}
	if got.LastProbeAt == nil {
		t.Error("LastProbeAt nil")
	}
}

func TestDomainRepo_Delete(t *testing.T) {
	repo := openTest(t)
	d, _ := repo.Create("gone.com")
	if err := repo.Delete(d.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(d.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
