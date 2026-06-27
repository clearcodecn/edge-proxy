package store

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

type UpstreamRepo struct {
	db *gorm.DB
}

func NewUpstreamRepo(db *gorm.DB) *UpstreamRepo {
	return &UpstreamRepo{db: db}
}

type UpstreamInput struct {
	Addr     string
	Weight   int
	IsBackup bool
	Remark   string
}

func (r *UpstreamRepo) Create(in UpstreamInput) (*Upstream, error) {
	if in.Weight <= 0 {
		in.Weight = 1
	}
	u := &Upstream{
		Addr:     in.Addr,
		Weight:   in.Weight,
		IsBackup: in.IsBackup,
		Enabled:  true,
		Remark:   in.Remark,
	}
	if err := r.db.Create(u).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return u, nil
}

func (r *UpstreamRepo) GetByID(id int64) (*Upstream, error) {
	var u Upstream
	if err := r.db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *UpstreamRepo) List() ([]Upstream, error) {
	var list []Upstream
	err := r.db.Order("id ASC").Find(&list).Error
	return list, err
}

func (r *UpstreamRepo) ListEnabled() ([]Upstream, error) {
	var list []Upstream
	err := r.db.Where("enabled = ?", true).Order("id ASC").Find(&list).Error
	return list, err
}

func (r *UpstreamRepo) Toggle(id int64) error {
	u, err := r.GetByID(id)
	if err != nil {
		return err
	}
	return r.db.Model(&Upstream{}).Where("id = ?", id).Update("enabled", !u.Enabled).Error
}

func (r *UpstreamRepo) Delete(id int64) error {
	return r.db.Unscoped().Delete(&Upstream{}, id).Error
}

// Search returns upstreams filtered by exact-match addrs and / or enabled
// state. When addrs is empty, results are paged (page is 1-based; pageSize <= 0
// falls back to 50). When addrs is non-empty, pagination is disabled — caller
// MUST bound the addrs slice.
//
// enabled: pass nil for "all", &true for enabled only, &false for disabled only.
func (r *UpstreamRepo) Search(addrs []string, enabled *bool, page, pageSize int) ([]Upstream, int, error) {
	q := r.db.Model(&Upstream{})
	if len(addrs) > 0 {
		q = q.Where("addr IN ?", addrs)
	}
	if enabled != nil {
		q = q.Where("enabled = ?", *enabled)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q = q.Order("id ASC")
	if len(addrs) == 0 {
		if pageSize <= 0 {
			pageSize = 50
		}
		if page < 1 {
			page = 1
		}
		q = q.Limit(pageSize).Offset((page - 1) * pageSize)
	}

	var items []Upstream
	if err := q.Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, int(total), nil
}

// BatchSetEnabled flips enabled to target on all matching IDs, only when their
// current state differs. Already-in-target rows are reported as failed with a
// reason. Missing IDs are also failed.
func (r *UpstreamRepo) BatchSetEnabled(ids []int64, target bool) ([]int64, []FailedItem) {
	if len(ids) == 0 {
		return nil, nil
	}

	var current []Upstream
	if err := r.db.Where("id IN ?", ids).Find(&current).Error; err != nil {
		return nil, allFailed(ids, "查询失败: "+err.Error())
	}
	byID := make(map[int64]bool, len(current))
	for _, u := range current {
		byID[u.ID] = u.Enabled
	}

	var (
		updateIDs []int64
		failed    []FailedItem
	)
	rejectReason := "已启用"
	if !target {
		rejectReason = "已禁用"
	}
	for _, id := range ids {
		st, ok := byID[id]
		if !ok {
			failed = append(failed, FailedItem{ID: id, Reason: "未找到"})
			continue
		}
		if st == target {
			failed = append(failed, FailedItem{ID: id, Reason: rejectReason})
			continue
		}
		updateIDs = append(updateIDs, id)
	}

	if len(updateIDs) == 0 {
		return nil, failed
	}
	if err := r.db.Model(&Upstream{}).Where("id IN ?", updateIDs).Update("enabled", target).Error; err != nil {
		for _, id := range updateIDs {
			failed = append(failed, FailedItem{ID: id, Reason: "更新失败: " + err.Error()})
		}
		return nil, failed
	}
	return updateIDs, failed
}

// BatchDelete removes upstreams by id. No status precondition (the UI relies on
// a confirm modal). Returns the deleted rows so the caller can act on addrs.
func (r *UpstreamRepo) BatchDelete(ids []int64) ([]Upstream, []FailedItem) {
	if len(ids) == 0 {
		return nil, nil
	}

	var current []Upstream
	if err := r.db.Where("id IN ?", ids).Find(&current).Error; err != nil {
		return nil, allFailed(ids, "查询失败: "+err.Error())
	}
	byID := make(map[int64]Upstream, len(current))
	for _, u := range current {
		byID[u.ID] = u
	}

	var (
		toDelete []Upstream
		delIDs   []int64
		failed   []FailedItem
	)
	for _, id := range ids {
		u, ok := byID[id]
		if !ok {
			failed = append(failed, FailedItem{ID: id, Reason: "未找到"})
			continue
		}
		toDelete = append(toDelete, u)
		delIDs = append(delIDs, id)
	}

	if len(delIDs) == 0 {
		return nil, failed
	}
	if err := r.db.Unscoped().Where("id IN ?", delIDs).Delete(&Upstream{}).Error; err != nil {
		for _, id := range delIDs {
			failed = append(failed, FailedItem{ID: id, Reason: "删除失败: " + err.Error()})
		}
		return nil, failed
	}
	return toDelete, failed
}
