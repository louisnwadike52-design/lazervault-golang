package models

import (
	"time"

	"gorm.io/gorm"
)

// DocumentType represents the type of identity document
type DocumentType string

const (
	DocumentTypePassport        DocumentType = "PASSPORT"
	DocumentTypeDriversLicense  DocumentType = "DRIVERS_LICENSE"
	DocumentTypeNationalID      DocumentType = "NATIONAL_ID"
	DocumentTypeResidencePermit DocumentType = "RESIDENCE_PERMIT"
)

// VerificationStatus represents the verification status of a document
type VerificationStatus string

const (
	VerificationStatusPending    VerificationStatus = "PENDING"
	VerificationStatusProcessing VerificationStatus = "PROCESSING"
	VerificationStatusVerified   VerificationStatus = "VERIFIED"
	VerificationStatusRejected   VerificationStatus = "REJECTED"
	VerificationStatusExpired    VerificationStatus = "EXPIRED"
)

// IDDocument represents an identity document uploaded by a user
type IDDocument struct {
	ID                 string             `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID             uint               `gorm:"not null;index" json:"user_id"`
	User               User               `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	DocumentType       DocumentType       `gorm:"type:varchar(50);not null" json:"document_type"`
	DocumentNumber     string             `gorm:"type:varchar(100);index" json:"document_number"`
	FullName           string             `gorm:"type:varchar(255)" json:"full_name"`
	DateOfBirth        string             `gorm:"type:varchar(20)" json:"date_of_birth"`
	IssueDate          string             `gorm:"type:varchar(20)" json:"issue_date"`
	ExpiryDate         string             `gorm:"type:varchar(20)" json:"expiry_date"`
	IssuingCountry     string             `gorm:"type:varchar(100)" json:"issuing_country"`
	DocumentFrontURL   string             `gorm:"type:text" json:"document_front_url"`
	DocumentBackURL    string             `gorm:"type:text" json:"document_back_url"`
	VerificationStatus VerificationStatus `gorm:"type:varchar(50);default:'PENDING'" json:"verification_status"`
	RejectionReason    string             `gorm:"type:text" json:"rejection_reason"`
	CreatedAt          time.Time          `json:"created_at"`
	VerifiedAt         *time.Time         `json:"verified_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	DeletedAt          gorm.DeletedAt     `gorm:"index" json:"-"`
}

// TableName specifies the table name for IDDocument
func (IDDocument) TableName() string {
	return "id_documents"
}

// FacialData represents facial biometric data for a user
type FacialData struct {
	ID             string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         uint           `gorm:"not null;uniqueIndex" json:"user_id"`
	User           User           `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	FaceID         string         `gorm:"type:varchar(255);index" json:"face_id"` // ID from AI service
	FaceEncoding   string         `gorm:"type:text" json:"-"`                     // Base64 encoded facial features - sensitive, not exposed in JSON
	ImageURL       string         `gorm:"type:text" json:"image_url"`
	IsVerified     bool           `gorm:"default:false" json:"is_verified"`
	CreatedAt      time.Time      `json:"created_at"`
	LastVerifiedAt *time.Time     `json:"last_verified_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName specifies the table name for FacialData
func (FacialData) TableName() string {
	return "facial_data"
}

// PermissionType represents a type of device permission
type PermissionType string

const (
	PermissionTypeCamera     PermissionType = "CAMERA"
	PermissionTypeLocation   PermissionType = "LOCATION"
	PermissionTypeMicrophone PermissionType = "MICROPHONE"
	PermissionTypeStorage    PermissionType = "STORAGE"
	PermissionTypeContacts   PermissionType = "CONTACTS"
	PermissionTypeBiometric  PermissionType = "BIOMETRIC"
)

// DevicePermission represents a device permission granted by the user
type DevicePermission struct {
	ID             uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         uint           `gorm:"not null;index:idx_user_permission" json:"user_id"`
	User           User           `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	PermissionType PermissionType `gorm:"type:varchar(50);not null;index:idx_user_permission" json:"permission_type"`
	IsGranted      bool           `gorm:"default:false" json:"is_granted"`
	GrantedAt      *time.Time     `json:"granted_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName specifies the table name for DevicePermission
func (DevicePermission) TableName() string {
	return "device_permissions"
}
