package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openDedupRepo(t *testing.T) *AlertDedupRepo {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "edge.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewAlertDedupRepo(db)
}

func TestAlertDedup_FirstAlwaysFires(t *testing.T) {
	repo := openDedupRepo(t)
	ok, err := repo.ShouldFire("edge_probe:x.com", time.Hour)
	if err != nil {
		t.Fatalf("ShouldFire: %v", err)
	}
	if !ok {
		t.Error("first call should fire")
	}
}

func TestAlertDedup_WithinWindowSkips(t *testing.T) {
	repo := openDedupRepo(t)
	_, _ = repo.ShouldFire("edge_acme:y.com", time.Hour)
	ok, err := repo.ShouldFire("edge_acme:y.com", time.Hour)
	if err != nil {
		t.Fatalf("second ShouldFire: %v", err)
	}
	if ok {
		t.Error("within window should NOT fire")
	}
}

func TestAlertDedup_DifferentKeysIndependent(t *testing.T) {
	repo := openDedupRepo(t)
	_, _ = repo.ShouldFire("edge_probe:a", time.Hour)
	ok, err := repo.ShouldFire("edge_acme:a", time.Hour)
	if err != nil {
		t.Fatalf("ShouldFire other key: %v", err)
	}
	if !ok {
		t.Error("different key should fire even if other is in window")
	}
}

func TestAlertDedup_ZeroWindowAlwaysFires(t *testing.T) {
	repo := openDedupRepo(t)
	if ok, _ := repo.ShouldFire("k", 0); !ok {
		t.Error("first")
	}
	if ok, _ := repo.ShouldFire("k", 0); !ok {
		t.Error("second should fire with zero window")
	}
}
