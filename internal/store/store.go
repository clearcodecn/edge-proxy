package store

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("raw db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Domain{}, &Upstream{}, &AlertDedup{})
}
