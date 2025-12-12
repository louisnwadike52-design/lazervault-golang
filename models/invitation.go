package models

import (
	"time"

	"gorm.io/gorm"
)

type InvitationType string

const (
	InvitationTypeEmail InvitationType = "email"
	InvitationTypeSMS   InvitationType = "sms"
)

type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusSent     InvitationStatus = "sent"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusExpired  InvitationStatus = "expired"
)

// Invitation represents an invitation sent to a user
type Invitation struct {
	gorm.Model
	InviterID    uint             `json:"inviter_id" gorm:"not null;index"`
	InviteeID    *uint            `json:"invitee_id,omitempty" gorm:"index"` // Partial user ID
	Type         InvitationType   `json:"type" gorm:"type:varchar(50);not null"`
	Status       InvitationStatus `json:"status" gorm:"type:varchar(50);not null;default:'pending';index"`
	Email        *string          `json:"email,omitempty" gorm:"size:255;index"`
	PhoneNumber  *string          `json:"phone_number,omitempty" gorm:"size:255;index"`
	GroupID      *uint            `json:"group_id,omitempty" gorm:"index"` // Which group they're invited to
	Message      *string          `json:"message,omitempty" gorm:"type:text"`
	SentAt       *time.Time       `json:"sent_at"`
	AcceptedAt   *time.Time       `json:"accepted_at"`
	ExpiresAt    time.Time        `json:"expires_at" gorm:"not null;index"`

	// Relationships
	Inviter User  `json:"inviter" gorm:"foreignKey:InviterID"`
	Invitee *User `json:"invitee,omitempty" gorm:"foreignKey:InviteeID"`
	Group   *GroupAccount `json:"group,omitempty" gorm:"foreignKey:GroupID"`
}

func (Invitation) TableName() string {
	return "invitations"
}
