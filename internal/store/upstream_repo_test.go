package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func openUpstreamRepo(t *testing.T) *UpstreamRepo {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "edge.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewUpstreamRepo(db)
}

func TestUpstreamRepo_CreateAndDefaults(t *testing.T) {
	repo := openUpstreamRepo(t)
	u, err := repo.Create(UpstreamInput{Addr: "10.0.0.5:80"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Weight != 1 {
		t.Errorf("default weight = %d", u.Weight)
	}
	if !u.Enabled {
		t.Error("default enabled = false")
	}
	if u.IsBackup {
		t.Error("default backup = true")
	}
}

func TestUpstreamRepo_DuplicateAddr(t *testing.T) {
	repo := openUpstreamRepo(t)
	if _, err := repo.Create(UpstreamInput{Addr: "10.0.0.5:80"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := repo.Create(UpstreamInput{Addr: "10.0.0.5:80"})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("expected ErrDuplicate, got %v", err)
	}
}

func TestUpstreamRepo_ListEnabled(t *testing.T) {
	repo := openUpstreamRepo(t)
	a, _ := repo.Create(UpstreamInput{Addr: "1.1.1.1:80"})
	_, _ = repo.Create(UpstreamInput{Addr: "2.2.2.2:80"})
	_ = repo.Toggle(a.ID)
	enabled, err := repo.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(enabled) != 1 || enabled[0].Addr != "2.2.2.2:80" {
		t.Errorf("enabled = %+v", enabled)
	}
}

func TestUpstreamRepo_Toggle(t *testing.T) {
	repo := openUpstreamRepo(t)
	u, _ := repo.Create(UpstreamInput{Addr: "x:80"})
	if !u.Enabled {
		t.Fatal("Should be enabled by default")
	}
	if err := repo.Toggle(u.ID); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	got, _ := repo.GetByID(u.ID)
	if got.Enabled {
		t.Error("expected disabled after first toggle")
	}
	if err := repo.Toggle(u.ID); err != nil {
		t.Fatalf("Toggle back: %v", err)
	}
	got, _ = repo.GetByID(u.ID)
	if !got.Enabled {
		t.Error("expected enabled after second toggle")
	}
}

func TestUpstreamRepo_Delete(t *testing.T) {
	repo := openUpstreamRepo(t)
	u, _ := repo.Create(UpstreamInput{Addr: "del:80"})
	if err := repo.Delete(u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(u.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}
