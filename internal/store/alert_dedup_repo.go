package store

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

type AlertDedupRepo struct {
	db *gorm.DB
}

func NewAlertDedupRepo(db *gorm.DB) *AlertDedupRepo {
	return &AlertDedupRepo{db: db}
}

// ShouldFire reports whether an alert with the given key should be sent now.
// If the key was last fired within `window`, returns (false, nil). Otherwise it
// UPSERTs last_at = now and returns (true, nil).
//
// Use IsCooled + MarkFired separately when you want to defer the mark until
// after the actual delivery (e.g. only mark when at least one channel succeeds).
func (r *AlertDedupRepo) ShouldFire(key string, window time.Duration) (bool, error) {
	cooled, err := r.IsCooled(key, window)
	if err != nil {
		return false, err
	}
	if cooled {
		return false, nil
	}
	if err := r.MarkFired(key); err != nil {
		return false, err
	}
	return true, nil
}

// IsCooled reports whether the key is within its cool-down window.
// Read-only; no DB write.
func (r *AlertDedupRepo) IsCooled(key string, window time.Duration) (bool, error) {
	var existing AlertDedup
	err := r.db.Where("key = ?", key).First(&existing).Error
	switch {
	case err == nil:
		return time.Since(existing.LastAt) < window, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return false, nil
	default:
		return false, err
	}
}

// MarkFired UPSERTs last_at = now for the key.
func (r *AlertDedupRepo) MarkFired(key string) error {
	now := time.Now()
	var existing AlertDedup
	err := r.db.Where("key = ?", key).First(&existing).Error
	switch {
	case err == nil:
		return r.db.Model(&AlertDedup{}).Where("key = ?", key).Update("last_at", now).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		return r.db.Create(&AlertDedup{Key: key, LastAt: now}).Error
	default:
		return err
	}
}
