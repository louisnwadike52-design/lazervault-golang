package models

import (
	"time"

	"gorm.io/gorm"
)

// UserTransactionFile stores the path to the generated transaction CSV file in GCS for a user.
type UserTransactionFile struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null;uniqueIndex"` // Foreign key to User model, unique constraint
	FilePath  string `gorm:"not null"`             // The GCS path (e.g., gs://bucket/path/to/file.csv)
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	User User `gorm:"foreignKey:UserID"` // Optional relation
}

// TableName specifies the custom table name if needed
// func (UserTransactionFile) TableName() string {
// 	return "user_transaction_files"
// }
