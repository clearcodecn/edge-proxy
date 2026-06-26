package store

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var (
	ErrDuplicate = errors.New("duplicate")
	ErrNotFound  = errors.New("not found")
)

type DomainRepo struct {
	db *gorm.DB
}

func NewDomainRepo(db *gorm.DB) *DomainRepo {
	return &DomainRepo{db: db}
}

func (r *DomainRepo) Create(host string) (*Domain, error) {
	d := &Domain{Host: host, Status: StatusPending}
	if err := r.db.Create(d).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return d, nil
}

func (r *DomainRepo) GetByID(id int64) (*Domain, error) {
	var d Domain
	if err := r.db.First(&d, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (r *DomainRepo) GetByHost(host string) (*Domain, error) {
	var d Domain
	if err := r.db.Where("host = ?", host).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (r *DomainRepo) List() ([]Domain, error) {
	var list []Domain
	err := r.db.Order("id DESC").Find(&list).Error
	return list, err
}

func (r *DomainRepo) ListByStatus(statuses ...string) ([]Domain, error) {
	var list []Domain
	err := r.db.Where("status IN ?", statuses).Order("id DESC").Find(&list).Error
	return list, err
}

// PickUnready returns the oldest domain awaiting ACME apply / retry,
// honoring the next_retry_at backoff. Returns ErrNotFound when no candidate.
func (r *DomainRepo) PickUnready(now time.Time) (*Domain, error) {
	var d Domain
	err := r.db.
		Where("status IN ?", []string{StatusPending, StatusCertFailed}).
		Where("next_retry_at IS NULL OR next_retry_at <= ?", now).
		Order("created_at ASC").
		First(&d).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (r *DomainRepo) UpdateStatus(id int64, status string) error {
	return r.db.Model(&Domain{}).Where("id = ?", id).Update("status", status).Error
}

// MarkCertApplied moves a domain to online after successful certbot apply.
func (r *DomainRepo) MarkCertApplied(id int64) error {
	now := time.Now()
	return r.db.Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"status":        StatusOnline,
		"cert_at":       &now,
		"fail_count":    0,
		"last_error":    "",
		"next_retry_at": nil,
	}).Error
}

// MarkCertFailed records a failure and bumps fail_count.
func (r *DomainRepo) MarkCertFailed(id int64, errMsg string) error {
	return r.db.Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"status":     StatusCertFailed,
		"last_error": errMsg,
		"fail_count": gorm.Expr("fail_count + 1"),
	}).Error
}

func (r *DomainRepo) SetNextRetryAt(id int64, t time.Time) error {
	return r.db.Model(&Domain{}).Where("id = ?", id).Update("next_retry_at", t).Error
}

// ResetRetry clears fail_count, last_error and next_retry_at; used by UI retry button.
func (r *DomainRepo) ResetRetry(id int64) error {
	return r.db.Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"status":        StatusPending,
		"fail_count":    0,
		"last_error":    "",
		"next_retry_at": nil,
	}).Error
}

func (r *DomainRepo) UpdateProbeResult(id int64, ok bool, failCount int, status string) error {
	now := time.Now()
	return r.db.Model(&Domain{}).Where("id = ?", id).Updates(map[string]any{
		"last_probe_at": &now,
		"last_probe_ok": ok,
		"fail_count":    failCount,
		"status":        status,
	}).Error
}

func (r *DomainRepo) UpdateCertAt(host string, t time.Time) error {
	return r.db.Model(&Domain{}).Where("host = ?", host).Update("cert_at", t).Error
}

func (r *DomainRepo) Delete(id int64) error {
	return r.db.Unscoped().Delete(&Domain{}, id).Error
}
