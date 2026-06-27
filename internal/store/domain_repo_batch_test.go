package store

import (
	"sort"
	"testing"
)

func seedDomains(t *testing.T, repo *DomainRepo, hosts ...string) []*Domain {
	t.Helper()
	out := make([]*Domain, 0, len(hosts))
	for _, h := range hosts {
		d, err := repo.Create(h)
		if err != nil {
			t.Fatalf("Create %s: %v", h, err)
		}
		out = append(out, d)
	}
	return out
}

func TestDomainRepo_Search_EmptyHostsPages(t *testing.T) {
	repo := openTest(t)
	// 5 domains created in order; List orders by id DESC.
	seedDomains(t, repo, "a.com", "b.com", "c.com", "d.com", "e.com")

	items, total, err := repo.Search(nil, "", 1, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(items) != 2 {
		t.Errorf("page1 len = %d, want 2", len(items))
	}

	page2, _, _ := repo.Search(nil, "", 2, 2)
	if len(page2) != 2 {
		t.Errorf("page2 len = %d, want 2", len(page2))
	}
	if page2[0].ID == items[0].ID {
		t.Errorf("page2 overlaps page1")
	}
}

func TestDomainRepo_Search_HostsExactMatchNoPagination(t *testing.T) {
	repo := openTest(t)
	seedDomains(t, repo, "a.com", "b.com", "c.com", "d.com")

	items, total, err := repo.Search([]string{"a.com", "c.com", "nope.com"}, "", 1, 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(items) != 2 {
		t.Errorf("len = %d, want 2 (pagination MUST be disabled when hosts non-empty)", len(items))
	}
	hosts := []string{items[0].Host, items[1].Host}
	sort.Strings(hosts)
	if hosts[0] != "a.com" || hosts[1] != "c.com" {
		t.Errorf("hosts = %v, want [a.com c.com]", hosts)
	}
}

func TestDomainRepo_Search_StatusFilterCombined(t *testing.T) {
	repo := openTest(t)
	ds := seedDomains(t, repo, "a.com", "b.com", "c.com")
	_ = repo.UpdateStatus(ds[0].ID, StatusOnline)
	_ = repo.UpdateStatus(ds[1].ID, StatusOnline)
	// c stays pending

	online, total, err := repo.Search(nil, StatusOnline, 1, 50)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 2 || len(online) != 2 {
		t.Errorf("online total/len = %d/%d, want 2/2", total, len(online))
	}

	// hosts filter + status filter combined
	combined, total, _ := repo.Search([]string{"a.com", "c.com"}, StatusOnline, 0, 0)
	if total != 1 || len(combined) != 1 || combined[0].Host != "a.com" {
		t.Errorf("combined = %+v, total=%d, want only a.com", combined, total)
	}
}

func TestDomainRepo_BatchUpdateStatus_PartialSuccess(t *testing.T) {
	repo := openTest(t)
	ds := seedDomains(t, repo, "a.com", "b.com", "c.com")
	_ = repo.UpdateStatus(ds[0].ID, StatusOnline)
	_ = repo.UpdateStatus(ds[1].ID, StatusOnline)
	_ = repo.UpdateStatus(ds[2].ID, StatusDeprecated) // c is already deprecated

	ids := []int64{ds[0].ID, ds[1].ID, ds[2].ID, 99999}
	succ, failed := repo.BatchUpdateStatus(ids, StatusDeprecated,
		[]string{StatusOnline, StatusDegraded, StatusPending, StatusCertFailed, StatusCertApplying})

	if len(succ) != 2 {
		t.Errorf("succeeded = %v, want 2", succ)
	}
	if len(failed) != 2 {
		t.Fatalf("failed = %v, want 2", failed)
	}
	reasons := map[int64]string{}
	for _, f := range failed {
		reasons[f.ID] = f.Reason
	}
	if _, ok := reasons[ds[2].ID]; !ok {
		t.Errorf("expected ds[2] (already deprecated) in failed, got %v", reasons)
	}
	if _, ok := reasons[99999]; !ok {
		t.Errorf("expected 99999 (not found) in failed, got %v", reasons)
	}

	// Verify side effects: both online → deprecated, ds[2] unchanged.
	a, _ := repo.GetByID(ds[0].ID)
	b, _ := repo.GetByID(ds[1].ID)
	c, _ := repo.GetByID(ds[2].ID)
	if a.Status != StatusDeprecated || b.Status != StatusDeprecated {
		t.Errorf("a/b not deprecated: %s/%s", a.Status, b.Status)
	}
	if c.Status != StatusDeprecated {
		t.Errorf("c status = %s (unchanged but already deprecated)", c.Status)
	}
}

func TestDomainRepo_BatchUpdateStatus_EmptyInput(t *testing.T) {
	repo := openTest(t)
	s, f := repo.BatchUpdateStatus(nil, StatusDeprecated, []string{StatusOnline})
	if s != nil || f != nil {
		t.Errorf("empty input should return (nil, nil), got (%v, %v)", s, f)
	}
}

func TestDomainRepo_BatchResetRetry_OnlyCertFailedAllowed(t *testing.T) {
	repo := openTest(t)
	ds := seedDomains(t, repo, "ok.com", "fail.com", "online.com")
	_ = repo.MarkCertFailed(ds[1].ID, "boom")
	_ = repo.UpdateStatus(ds[2].ID, StatusOnline)

	succ, failed := repo.BatchResetRetry([]int64{ds[0].ID, ds[1].ID, ds[2].ID})
	if len(succ) != 1 || succ[0] != ds[1].ID {
		t.Errorf("succeeded = %v, want [%d]", succ, ds[1].ID)
	}
	if len(failed) != 2 {
		t.Errorf("failed = %v, want 2", failed)
	}

	fail, _ := repo.GetByID(ds[1].ID)
	if fail.Status != StatusPending {
		t.Errorf("status after reset = %s, want pending", fail.Status)
	}
	if fail.FailCount != 0 || fail.LastError != "" {
		t.Errorf("retry state not cleared: FailCount=%d LastError=%q", fail.FailCount, fail.LastError)
	}
}

func TestDomainRepo_BatchDelete_OnlyDeprecatedReturnsDeleted(t *testing.T) {
	repo := openTest(t)
	ds := seedDomains(t, repo, "keep.com", "byebye.com", "online.com")
	_ = repo.UpdateStatus(ds[1].ID, StatusDeprecated)
	_ = repo.UpdateStatus(ds[2].ID, StatusOnline)

	deleted, failed := repo.BatchDelete(
		[]int64{ds[0].ID, ds[1].ID, ds[2].ID, 4242},
		[]string{StatusDeprecated},
	)
	if len(deleted) != 1 || deleted[0].Host != "byebye.com" {
		t.Errorf("deleted = %+v, want one byebye.com", deleted)
	}
	if len(failed) != 3 {
		t.Errorf("failed = %v, want 3 (pending, online, not-found)", failed)
	}

	// Real DB-level effects
	if _, err := repo.GetByID(ds[1].ID); err == nil {
		t.Error("expected byebye.com to be gone from DB")
	}
	if _, err := repo.GetByID(ds[0].ID); err != nil {
		t.Errorf("keep.com should still exist, got %v", err)
	}
}
