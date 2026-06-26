package store

import "time"

const (
	StatusPending      = "pending"
	StatusCertApplying = "cert_applying"
	StatusCertFailed   = "cert_failed"
	StatusOnline       = "online"
	StatusDegraded     = "degraded"
	StatusDeprecated   = "deprecated"
)

type Domain struct {
	ID          int64      `gorm:"primaryKey"`
	Host        string     `gorm:"uniqueIndex;size:255;not null"`
	Status      string     `gorm:"size:16;not null;default:pending;index"`
	CertAt      *time.Time `gorm:"index"`
	LastProbeAt *time.Time
	LastProbeOK bool       `gorm:"default:false"`
	FailCount   int        `gorm:"default:0"`
	LastError   string     `gorm:"type:text"`
	NextRetryAt *time.Time `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Upstream struct {
	ID        int64  `gorm:"primaryKey"`
	Addr      string `gorm:"uniqueIndex;size:255;not null"`
	Weight    int    `gorm:"default:1"`
	IsBackup  bool   `gorm:"default:false"`
	Enabled   bool   `gorm:"default:true;index"`
	Remark    string `gorm:"size:255"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type AlertDedup struct {
	Key    string `gorm:"primaryKey;size:128"`
	LastAt time.Time
}
