package alert

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"edge-proxy/internal/store"
)

type fakeChannel struct {
	name  string
	err   error
	count atomic.Int32
}

func (f *fakeChannel) Name() string { return f.name }
func (f *fakeChannel) Send(_ context.Context, _ string) error {
	f.count.Add(1)
	return f.err
}

func openDedup(t *testing.T) *store.AlertDedupRepo {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "edge.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return store.NewAlertDedupRepo(db)
}

func TestNotifier_DedupWithinWindow(t *testing.T) {
	dedup := openDedup(t)
	ding := &fakeChannel{name: "ding"}
	tg := &fakeChannel{name: "tg"}
	n := NewNotifier(dedup, time.Hour, ding, tg)

	n.Alert("edge_probe:a.com", "msg")
	n.Alert("edge_probe:a.com", "msg")

	if ding.count.Load() != 1 {
		t.Errorf("ding called %d times, want 1", ding.count.Load())
	}
	if tg.count.Load() != 1 {
		t.Errorf("tg called %d times, want 1", tg.count.Load())
	}
}

func TestNotifier_OneChannelFailsOtherSucceeds_StillMarksFired(t *testing.T) {
	dedup := openDedup(t)
	ding := &fakeChannel{name: "ding", err: errors.New("http 502")}
	tg := &fakeChannel{name: "tg"} // success
	n := NewNotifier(dedup, time.Hour, ding, tg)

	n.Alert("edge_acme:b.com", "msg")
	// Second call in window should be deduped because tg succeeded.
	n.Alert("edge_acme:b.com", "msg")

	if ding.count.Load() != 1 {
		t.Errorf("ding count = %d", ding.count.Load())
	}
	if tg.count.Load() != 1 {
		t.Errorf("tg count = %d", tg.count.Load())
	}
}

func TestNotifier_AllChannelsFail_DoesNotMarkFired(t *testing.T) {
	dedup := openDedup(t)
	ding := &fakeChannel{name: "ding", err: errors.New("http 502")}
	tg := &fakeChannel{name: "tg", err: errors.New("http 502")}
	n := NewNotifier(dedup, time.Hour, ding, tg)

	n.Alert("edge_renew:c.com", "msg")
	n.Alert("edge_renew:c.com", "msg")

	// Both attempts should have triggered both channels (no dedup mark on full failure).
	if ding.count.Load() != 2 {
		t.Errorf("ding count = %d, want 2 (no dedup after total failure)", ding.count.Load())
	}
	if tg.count.Load() != 2 {
		t.Errorf("tg count = %d, want 2", tg.count.Load())
	}
}

func TestNotifier_DifferentKeysIndependent(t *testing.T) {
	dedup := openDedup(t)
	ding := &fakeChannel{name: "ding"}
	n := NewNotifier(dedup, time.Hour, ding)

	n.Alert("edge_probe:x.com", "msg")
	n.Alert("edge_acme:x.com", "msg") // different key

	if ding.count.Load() != 2 {
		t.Errorf("expected 2 different-key sends, got %d", ding.count.Load())
	}
}

func TestNotifier_ZeroChannelsNoCrash(t *testing.T) {
	dedup := openDedup(t)
	n := NewNotifier(dedup, time.Hour)
	n.Alert("k", "no channels configured")
	// Just exercising the path; should not panic, should not write dedup.
	cooled, _ := dedup.IsCooled("k", time.Hour)
	if cooled {
		t.Error("dedup should not be marked when no channel succeeded")
	}
}
