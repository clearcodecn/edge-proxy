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
