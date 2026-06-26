package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *DomainRepo {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "edge.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewDomainRepo(db)
}

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "edge.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !db.Migrator().HasTable(&Domain{}) {
		t.Error("Domain table missing")
	}
	if !db.Migrator().HasTable(&Upstream{}) {
		t.Error("Upstream table missing")
	}
	if !db.Migrator().HasTable(&AlertDedup{}) {
		t.Error("AlertDedup table missing")
	}
}

func TestMigrateIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "edge.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestDomainUniqueHost(t *testing.T) {
	repo := openTest(t)
	if _, err := repo.Create("a.com"); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	_, err := repo.Create("a.com")
	if err == nil {
		t.Fatal("expected UNIQUE violation, got nil")
	}
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("expected ErrDuplicate, got %v", err)
	}
}
