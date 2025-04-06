package database

import (
	"lazervaultGo/models"

	"gorm.io/gorm"
)

func AutoMigrateDB(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
	)
}

func DropAllTables(db *gorm.DB) error {
	return db.Migrator().DropTable(
		&models.User{},
	)
}
