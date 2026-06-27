package store

import (
	"sort"
	"testing"
)

func seedUpstreams(t *testing.T, repo *UpstreamRepo, addrs ...string) []*Upstream {
	t.Helper()
	out := make([]*Upstream, 0, len(addrs))
	for _, a := range addrs {
		u, err := repo.Create(UpstreamInput{Addr: a})
		if err != nil {
			t.Fatalf("Create %s: %v", a, err)
		}
		out = append(out, u)
	}
	return out
}

func boolPtr(b bool) *bool { return &b }

func TestUpstreamRepo_Search_EmptyAddrsPages(t *testing.T) {
	repo := openUpstreamRepo(t)
	seedUpstreams(t, repo, "1:80", "2:80", "3:80", "4:80", "5:80")

	page1, total, err := repo.Search(nil, nil, 1, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 5 || len(page1) != 2 {
		t.Errorf("page1 total/len = %d/%d, want 5/2", total, len(page1))
	}
	page2, _, _ := repo.Search(nil, nil, 2, 2)
	if len(page2) != 2 || page2[0].ID == page1[0].ID {
		t.Errorf("page2 = %+v, overlaps page1", page2)
	}
}

func TestUpstreamRepo_Search_AddrsExactNoPagination(t *testing.T) {
	repo := openUpstreamRepo(t)
	seedUpstreams(t, repo, "1:80", "2:80", "3:80")
	items, total, _ := repo.Search([]string{"1:80", "3:80", "nope:80"}, nil, 1, 1)
	if total != 2 || len(items) != 2 {
		t.Errorf("total/len = %d/%d, want 2/2 (no pagination)", total, len(items))
	}
	addrs := []string{items[0].Addr, items[1].Addr}
	sort.Strings(addrs)
	if addrs[0] != "1:80" || addrs[1] != "3:80" {
		t.Errorf("addrs = %v", addrs)
	}
}

func TestUpstreamRepo_Search_EnabledFilter(t *testing.T) {
	repo := openUpstreamRepo(t)
	us := seedUpstreams(t, repo, "1:80", "2:80", "3:80")
	_ = repo.Toggle(us[0].ID) // disable us[0]

	enabled, totalE, _ := repo.Search(nil, boolPtr(true), 1, 50)
	if totalE != 2 || len(enabled) != 2 {
		t.Errorf("enabled total/len = %d/%d", totalE, len(enabled))
	}

	disabled, totalD, _ := repo.Search(nil, boolPtr(false), 1, 50)
	if totalD != 1 || len(disabled) != 1 || disabled[0].Addr != "1:80" {
		t.Errorf("disabled = %+v", disabled)
	}
}

func TestUpstreamRepo_BatchSetEnabled_Partial(t *testing.T) {
	repo := openUpstreamRepo(t)
	us := seedUpstreams(t, repo, "1:80", "2:80", "3:80")
	_ = repo.Toggle(us[0].ID) // us[0] disabled, others enabled

	// Try to enable all three + a missing id; only us[0] should succeed.
	succ, failed := repo.BatchSetEnabled([]int64{us[0].ID, us[1].ID, us[2].ID, 9999}, true)
	if len(succ) != 1 || succ[0] != us[0].ID {
		t.Errorf("succeeded = %v, want [%d]", succ, us[0].ID)
	}
	if len(failed) != 3 {
		t.Errorf("failed = %v, want 3 (two already-enabled + one missing)", failed)
	}

	u0, _ := repo.GetByID(us[0].ID)
	if !u0.Enabled {
		t.Error("us[0] should be enabled now")
	}
}

func TestUpstreamRepo_BatchSetEnabled_Disable(t *testing.T) {
	repo := openUpstreamRepo(t)
	us := seedUpstreams(t, repo, "1:80", "2:80")
	succ, failed := repo.BatchSetEnabled([]int64{us[0].ID, us[1].ID}, false)
	if len(succ) != 2 {
		t.Errorf("succeeded = %v", succ)
	}
	if len(failed) != 0 {
		t.Errorf("failed = %v", failed)
	}
	for _, u := range us {
		got, _ := repo.GetByID(u.ID)
		if got.Enabled {
			t.Errorf("%s still enabled", got.Addr)
		}
	}
}

func TestUpstreamRepo_BatchDelete_ReturnsDeleted(t *testing.T) {
	repo := openUpstreamRepo(t)
	us := seedUpstreams(t, repo, "a:80", "b:80", "c:80")
	deleted, failed := repo.BatchDelete([]int64{us[0].ID, us[1].ID, 4242})
	if len(deleted) != 2 {
		t.Errorf("deleted = %v, want 2", deleted)
	}
	if len(failed) != 1 || failed[0].ID != 4242 {
		t.Errorf("failed = %v, want one 4242 missing", failed)
	}
	addrs := []string{deleted[0].Addr, deleted[1].Addr}
	sort.Strings(addrs)
	if addrs[0] != "a:80" || addrs[1] != "b:80" {
		t.Errorf("deleted addrs = %v", addrs)
	}
	if _, err := repo.GetByID(us[2].ID); err != nil {
		t.Errorf("c:80 should remain, got %v", err)
	}
}
