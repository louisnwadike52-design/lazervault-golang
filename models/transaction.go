package models

import (
	"time"
)

// Transaction represents a unified transaction record in the database
type Transaction struct {
	ID                            uint      `gorm:"primaryKey"`
	UserID                        uint64    `gorm:"index;not null"`
	Timestamp                     time.Time `gorm:"index;not null"`
	Type                          string    `gorm:"index;not null"` // DEPOSIT, WITHDRAWAL, TRANSFER_OUT, TRANSFER_IN, EXCHANGE_OUT, EXCHANGE_IN
	Amount                        float64   `gorm:"not null"`
	Currency                      string    `gorm:"not null"`
	Status                        string    `gorm:"index;not null"`
	Description                   string    `gorm:"not null"`
	Reference                     string    `gorm:"index;not null"`
	FailureReason                 *string
	CompletedAt                   *time.Time
	ProcessingAt                  *time.Time
	FailedAt                      *time.Time
	ExternalTransactionID         *string
	FromAccountID                 *uint
	ToAccountID                   *uint
	DepositSourceBankName         *string
	WithdrawalTargetBankName      *string
	WithdrawalTargetAccountNumber *string
	WithdrawalTargetSortCode      *string
	TransferFee                   *float64
	TransferTotalAmount           *float64
	TransferCategory              *string
	TransferScheduledAt           *string
	SenderInfo                    string
	RecipientInfo                 string
	RecipientID                   *uint
	ExchangeFromCurrency          *string
	ExchangeToCurrency            *string
	ExchangeAmountFrom            *float64
	ExchangeAmountTo              *float64
	ExchangeRate                  *float64
	ExchangeFees                  *float64
	ExchangeReceiverDetails       *string
	RelatedParty                  *string
	CreatedAt                     time.Time
	UpdatedAt                     time.Time
}

// TableName specifies the table name for the Transaction model
func (Transaction) TableName() string {
	return "transactions"
}

// TransactionFilters represents the filters for querying transactions
type TransactionFilters struct {
	UserID    uint64
	Type      *string
	Status    *string
	StartDate *time.Time
	EndDate   *time.Time
	Limit     int
	Offset    int
}

// TransactionResponse represents the response for transaction queries
type TransactionResponse struct {
	Transactions []Transaction
	Total        int64
	Limit        int
	Offset       int
}
