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

// Search returns domains filtered by exact-match hosts and / or status. When
// hosts is empty, results are paged (page is 1-based; pageSize <= 0 falls back
// to 50). When hosts is non-empty, pagination is disabled — caller MUST bound
// the hosts slice (the spec caps it at 200 with a truncation warning).
func (r *DomainRepo) Search(hosts []string, status string, page, pageSize int) ([]Domain, int, error) {
	q := r.db.Model(&Domain{})
	if len(hosts) > 0 {
		q = q.Where("host IN ?", hosts)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q = q.Order("id DESC")
	if len(hosts) == 0 {
		if pageSize <= 0 {
			pageSize = 50
		}
		if page < 1 {
			page = 1
		}
		q = q.Limit(pageSize).Offset((page - 1) * pageSize)
	}

	var items []Domain
	if err := q.Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, int(total), nil
}

// BatchUpdateStatus moves domains whose current status is in allowedFrom to
// target. IDs not present, or whose current status is not allowed, are returned
// in failed with a reason. The whole batch is processed in two queries (one
// SELECT, one UPDATE) — single-row failures do not abort the rest.
func (r *DomainRepo) BatchUpdateStatus(ids []int64, target string, allowedFrom []string) ([]int64, []FailedItem) {
	if len(ids) == 0 {
		return nil, nil
	}

	var current []Domain
	if err := r.db.Where("id IN ?", ids).Find(&current).Error; err != nil {
		return nil, allFailed(ids, "查询失败: "+err.Error())
	}

	byID := make(map[int64]string, len(current))
	for _, d := range current {
		byID[d.ID] = d.Status
	}
	allowed := make(map[string]bool, len(allowedFrom))
	for _, s := range allowedFrom {
		allowed[s] = true
	}

	var (
		updateIDs []int64
		failed    []FailedItem
	)
	for _, id := range ids {
		st, ok := byID[id]
		if !ok {
			failed = append(failed, FailedItem{ID: id, Reason: "未找到"})
			continue
		}
		if !allowed[st] {
			failed = append(failed, FailedItem{ID: id, Reason: rejectStatusReason(target, st)})
			continue
		}
		updateIDs = append(updateIDs, id)
	}

	if len(updateIDs) == 0 {
		return nil, failed
	}
	if err := r.db.Model(&Domain{}).Where("id IN ?", updateIDs).Update("status", target).Error; err != nil {
		// Single bulk failure: roll the success bucket into failed with the underlying reason.
		for _, id := range updateIDs {
			failed = append(failed, FailedItem{ID: id, Reason: "更新失败: " + err.Error()})
		}
		return nil, failed
	}
	return updateIDs, failed
}

// BatchResetRetry resets retry state on a set of domains; only those currently
// in cert_failed are touched. Equivalent to ResetRetry but in bulk.
func (r *DomainRepo) BatchResetRetry(ids []int64) ([]int64, []FailedItem) {
	if len(ids) == 0 {
		return nil, nil
	}

	var current []Domain
	if err := r.db.Where("id IN ?", ids).Find(&current).Error; err != nil {
		return nil, allFailed(ids, "查询失败: "+err.Error())
	}
	byID := make(map[int64]string, len(current))
	for _, d := range current {
		byID[d.ID] = d.Status
	}

	var (
		updateIDs []int64
		failed    []FailedItem
	)
	for _, id := range ids {
		st, ok := byID[id]
		if !ok {
			failed = append(failed, FailedItem{ID: id, Reason: "未找到"})
			continue
		}
		if st != StatusCertFailed {
			failed = append(failed, FailedItem{ID: id, Reason: "只能重试失败的申请"})
			continue
		}
		updateIDs = append(updateIDs, id)
	}

	if len(updateIDs) == 0 {
		return nil, failed
	}
	if err := r.db.Model(&Domain{}).Where("id IN ?", updateIDs).Updates(map[string]any{
		"status":        StatusPending,
		"fail_count":    0,
		"last_error":    "",
		"next_retry_at": nil,
	}).Error; err != nil {
		for _, id := range updateIDs {
			failed = append(failed, FailedItem{ID: id, Reason: "重置失败: " + err.Error()})
		}
		return nil, failed
	}
	return updateIDs, failed
}

// BatchDelete removes domains whose current status is in allowedFrom. The
// caller receives the deleted Domain values (id + host) so it can clean up
// downstream resources like nginx conf files and LE certs that are keyed by
// host. Per the spec, single-row failures do not abort the rest, and DB
// deletion is NOT rolled back if downstream cleanup fails later.
func (r *DomainRepo) BatchDelete(ids []int64, allowedFrom []string) ([]Domain, []FailedItem) {
	if len(ids) == 0 {
		return nil, nil
	}

	var current []Domain
	if err := r.db.Where("id IN ?", ids).Find(&current).Error; err != nil {
		return nil, allFailed(ids, "查询失败: "+err.Error())
	}
	byID := make(map[int64]Domain, len(current))
	for _, d := range current {
		byID[d.ID] = d
	}
	allowed := make(map[string]bool, len(allowedFrom))
	for _, s := range allowedFrom {
		allowed[s] = true
	}

	var (
		toDelete []Domain
		delIDs   []int64
		failed   []FailedItem
	)
	for _, id := range ids {
		d, ok := byID[id]
		if !ok {
			failed = append(failed, FailedItem{ID: id, Reason: "未找到"})
			continue
		}
		if !allowed[d.Status] {
			failed = append(failed, FailedItem{ID: id, Reason: "只有已废弃域名可回收"})
			continue
		}
		toDelete = append(toDelete, d)
		delIDs = append(delIDs, id)
	}

	if len(delIDs) == 0 {
		return nil, failed
	}
	if err := r.db.Unscoped().Where("id IN ?", delIDs).Delete(&Domain{}).Error; err != nil {
		for _, id := range delIDs {
			failed = append(failed, FailedItem{ID: id, Reason: "删除失败: " + err.Error()})
		}
		return nil, failed
	}
	return toDelete, failed
}

func allFailed(ids []int64, reason string) []FailedItem {
	out := make([]FailedItem, len(ids))
	for i, id := range ids {
		out[i] = FailedItem{ID: id, Reason: reason}
	}
	return out
}

func rejectStatusReason(target, current string) string {
	switch target {
	case StatusDeprecated:
		return "当前状态 " + current + " 不允许废弃"
	default:
		return "当前状态 " + current + " 不允许此操作"
	}
}
