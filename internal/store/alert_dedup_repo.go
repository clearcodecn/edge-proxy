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
func (r *AlertDedupRepo) ShouldFire(key string, window time.Duration) (bool, error) {
	now := time.Now()
	var existing AlertDedup
	err := r.db.Where("key = ?", key).First(&existing).Error
	switch {
	case err == nil:
		if now.Sub(existing.LastAt) < window {
			return false, nil
		}
		if err := r.db.Model(&AlertDedup{}).Where("key = ?", key).Update("last_at", now).Error; err != nil {
			return false, err
		}
		return true, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := r.db.Create(&AlertDedup{Key: key, LastAt: now}).Error; err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, err
	}
}
