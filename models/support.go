package models

import (
	"time"

	"gorm.io/gorm"
)

// TicketCategory represents the category of a support ticket
type TicketCategory string

const (
	TicketCategoryGeneralInquiry     TicketCategory = "GENERAL_INQUIRY"
	TicketCategoryTransactionIssue   TicketCategory = "TRANSACTION_ISSUE"
	TicketCategoryAccountProblem     TicketCategory = "ACCOUNT_PROBLEM"
	TicketCategoryTechnicalSupport   TicketCategory = "TECHNICAL_SUPPORT"
	TicketCategorySecurityConcern    TicketCategory = "SECURITY_CONCERN"
	TicketCategoryOther              TicketCategory = "OTHER"
)

// TicketStatus represents the status of a support ticket
type TicketStatus string

const (
	TicketStatusOpen                TicketStatus = "OPEN"
	TicketStatusInProgress          TicketStatus = "IN_PROGRESS"
	TicketStatusWaitingForCustomer  TicketStatus = "WAITING_FOR_CUSTOMER"
	TicketStatusResolved            TicketStatus = "RESOLVED"
	TicketStatusClosed              TicketStatus = "CLOSED"
)

// TicketPriority represents the priority level of a support ticket
type TicketPriority string

const (
	TicketPriorityLow    TicketPriority = "LOW"
	TicketPriorityMedium TicketPriority = "MEDIUM"
	TicketPriorityHigh   TicketPriority = "HIGH"
	TicketPriorityUrgent TicketPriority = "URGENT"
)

// SupportTicket represents a customer support ticket
type SupportTicket struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	UserID       int            `gorm:"not null;index" json:"user_id"`
	TicketNumber string         `gorm:"uniqueIndex;not null" json:"ticket_number"`
	Category     TicketCategory `gorm:"type:varchar(50);not null" json:"category"`
	Subject      string         `gorm:"type:varchar(255);not null" json:"subject"`
	Description  string         `gorm:"type:text;not null" json:"description"`
	Status       TicketStatus   `gorm:"type:varchar(50);not null;default:'OPEN'" json:"status"`
	Priority     TicketPriority `gorm:"type:varchar(20);not null;default:'MEDIUM'" json:"priority"`
	ResolvedAt   *time.Time     `json:"resolved_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	User    User          `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Replies []TicketReply `gorm:"foreignKey:TicketID" json:"replies,omitempty"`
}

// TicketReply represents a reply to a support ticket
type TicketReply struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	TicketID  uint      `gorm:"not null;index" json:"ticket_id"`
	UserID    int       `gorm:"not null" json:"user_id"`
	Message   string    `gorm:"type:text;not null" json:"message"`
	IsStaff   bool      `gorm:"default:false" json:"is_staff"`
	CreatedAt time.Time `json:"created_at"`

	// Relationships
	Ticket SupportTicket `gorm:"foreignKey:TicketID" json:"ticket,omitempty"`
	User   User          `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// ContactMessage represents a message from the contact form
type ContactMessage struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	Name      string     `gorm:"type:varchar(255);not null" json:"name"`
	Email     string     `gorm:"type:varchar(255);not null" json:"email"`
	Topic     string     `gorm:"type:varchar(100);not null" json:"topic"`
	Subject   string     `gorm:"type:varchar(255);not null" json:"subject"`
	Message   string     `gorm:"type:text;not null" json:"message"`
	UserID    *int       `gorm:"index" json:"user_id"` // Optional - if user is logged in
	IsRead    bool       `gorm:"default:false" json:"is_read"`
	CreatedAt time.Time  `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName specifies the table name for SupportTicket
func (SupportTicket) TableName() string {
	return "support_tickets"
}

// TableName specifies the table name for TicketReply
func (TicketReply) TableName() string {
	return "ticket_replies"
}

// TableName specifies the table name for ContactMessage
func (ContactMessage) TableName() string {
	return "contact_messages"
}
