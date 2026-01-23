package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Expense represents a user's expense/transaction
type Expense struct {
	ID                uuid.UUID  `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID            uint       `gorm:"not null;index:idx_expense_user"`
	User              User       `gorm:"foreignKey:UserID"`
	AccountID         *uuid.UUID `gorm:"type:uuid;index:idx_expense_account"`
	Amount            float64    `gorm:"not null"`
	Currency          string     `gorm:"size:3;default:'USD';not null"`
	Category          string     `gorm:"size:50;not null;index:idx_expense_category"`
	Subcategory       string     `gorm:"size:50"`
	Description       string     `gorm:"size:500"`
	Merchant          string     `gorm:"size:200;index:idx_expense_merchant"`
	TransactionDate   time.Time  `gorm:"not null;index:idx_expense_transaction_date"`
	PaymentMethod     string     `gorm:"size:50"`
	ReceiptURL        string     `gorm:"size:500"`
	Tags              string     `gorm:"type:text"` // JSON array stored as string
	Notes             string     `gorm:"type:text"`
	IsRecurring       bool       `gorm:"default:false"`
	RecurrencePattern string     `gorm:"size:20"` // daily, weekly, monthly, yearly
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for Expense model
func (Expense) TableName() string {
	return "expenses"
}

// Budget represents a user's budget plan
type Budget struct {
	ID              uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID          uint      `gorm:"not null;index:idx_budget_user"`
	User            User      `gorm:"foreignKey:UserID"`
	Name            string    `gorm:"size:200;not null"`
	Amount          float64   `gorm:"not null"`
	Currency        string    `gorm:"size:3;default:'USD';not null"`
	Category        string    `gorm:"size:50;not null;index:idx_budget_category"`
	Period          string    `gorm:"size:30;not null"` // daily, weekly, monthly, quarterly, yearly, custom
	StartDate       time.Time `gorm:"not null;index:idx_budget_start_date"`
	EndDate         time.Time `gorm:"not null;index:idx_budget_end_date"`
	SpentAmount     float64   `gorm:"default:0"`
	RemainingAmount float64   `gorm:"default:0"`
	PercentageUsed  float64   `gorm:"default:0"`
	Status          string    `gorm:"size:30;not null;default:'active';index:idx_budget_status"` // active, exceeded, near_limit, inactive, completed
	EnableAlerts    bool      `gorm:"default:true"`
	AlertThreshold  float64   `gorm:"default:80"` // percentage (e.g., 80 for 80%)
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for Budget model
func (Budget) TableName() string {
	return "budgets"
}

// BudgetAlert represents budget threshold alerts/notifications
type BudgetAlert struct {
	ID             uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID         uint      `gorm:"not null;index:idx_alert_user"`
	User           User      `gorm:"foreignKey:UserID"`
	BudgetID       uuid.UUID `gorm:"type:uuid;not null;index:idx_alert_budget"`
	Budget         Budget    `gorm:"foreignKey:BudgetID"`
	BudgetName     string    `gorm:"size:200;not null"`
	AlertType      string    `gorm:"size:50;not null"` // threshold_reached, budget_exceeded, approaching_limit, recurring_expense_due
	Message        string    `gorm:"size:500;not null"`
	CurrentSpent   float64   `gorm:"not null"`
	BudgetLimit    float64   `gorm:"not null"`
	PercentageUsed float64   `gorm:"not null"`
	IsRead         bool      `gorm:"default:false;index:idx_alert_read"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for BudgetAlert model
func (BudgetAlert) TableName() string {
	return "budget_alerts"
}

// BeforeCreate hook for Expense to set default values
func (e *Expense) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.Currency == "" {
		e.Currency = "USD"
	}
	return nil
}

// BeforeCreate hook for Budget to set default values and calculate remaining amount
func (b *Budget) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.Currency == "" {
		b.Currency = "USD"
	}
	if b.Status == "" {
		b.Status = "active"
	}
	if b.AlertThreshold == 0 {
		b.AlertThreshold = 80
	}
	b.RemainingAmount = b.Amount - b.SpentAmount
	b.PercentageUsed = 0
	if b.Amount > 0 {
		b.PercentageUsed = (b.SpentAmount / b.Amount) * 100
	}
	return nil
}

// BeforeCreate hook for BudgetAlert to set default values
func (ba *BudgetAlert) BeforeCreate(tx *gorm.DB) error {
	if ba.ID == uuid.Nil {
		ba.ID = uuid.New()
	}
	return nil
}

// UpdateBudgetCalculations updates spent amount, remaining amount, percentage used, and status
func (b *Budget) UpdateBudgetCalculations(tx *gorm.DB) error {
	// Calculate totals
	var totalSpent float64
	err := tx.Model(&Expense{}).
		Where("user_id = ? AND category = ? AND transaction_date >= ? AND transaction_date <= ? AND deleted_at IS NULL",
			b.UserID, b.Category, b.StartDate, b.EndDate).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalSpent).Error
	if err != nil {
		return err
	}

	// Update budget fields
	b.SpentAmount = totalSpent
	b.RemainingAmount = b.Amount - totalSpent
	b.PercentageUsed = 0
	if b.Amount > 0 {
		b.PercentageUsed = (totalSpent / b.Amount) * 100
	}

	// Update status based on percentage
	if b.PercentageUsed >= 100 {
		b.Status = "exceeded"
	} else if b.PercentageUsed >= 90 {
		b.Status = "near_limit"
	} else {
		b.Status = "active"
	}

	// Check if budget period has ended
	if time.Now().After(b.EndDate) {
		b.Status = "completed"
	}

	return nil
}

// CategoryColors maps expense categories to color codes for UI
var CategoryColors = map[string]string{
	"EXPENSE_CATEGORY_FOOD_DINING":     "#FF6B6B",
	"EXPENSE_CATEGORY_TRANSPORTATION":  "#4ECDC4",
	"EXPENSE_CATEGORY_SHOPPING":        "#FFE66D",
	"EXPENSE_CATEGORY_ENTERTAINMENT":   "#A8E6CF",
	"EXPENSE_CATEGORY_BILLS_UTILITIES": "#FF8B94",
	"EXPENSE_CATEGORY_HEALTHCARE":      "#C7CEEA",
	"EXPENSE_CATEGORY_EDUCATION":       "#B4A7D6",
	"EXPENSE_CATEGORY_TRAVEL":          "#95E1D3",
	"EXPENSE_CATEGORY_GROCERIES":       "#F38181",
	"EXPENSE_CATEGORY_RENT_MORTGAGE":   "#AA96DA",
	"EXPENSE_CATEGORY_INSURANCE":       "#FCBAD3",
	"EXPENSE_CATEGORY_INVESTMENTS":     "#A8D8EA",
	"EXPENSE_CATEGORY_GIFTS_DONATIONS": "#FFD3B6",
	"EXPENSE_CATEGORY_PERSONAL_CARE":   "#FFAAA5",
	"EXPENSE_CATEGORY_SUBSCRIPTIONS":   "#DCD6F7",
	"EXPENSE_CATEGORY_OTHER":           "#C5C6C7",
}

// CategoryNames provides human-readable names for categories
var CategoryNames = map[string]string{
	"EXPENSE_CATEGORY_FOOD_DINING":     "Food & Dining",
	"EXPENSE_CATEGORY_TRANSPORTATION":  "Transportation",
	"EXPENSE_CATEGORY_SHOPPING":        "Shopping",
	"EXPENSE_CATEGORY_ENTERTAINMENT":   "Entertainment",
	"EXPENSE_CATEGORY_BILLS_UTILITIES": "Bills & Utilities",
	"EXPENSE_CATEGORY_HEALTHCARE":      "Healthcare",
	"EXPENSE_CATEGORY_EDUCATION":       "Education",
	"EXPENSE_CATEGORY_TRAVEL":          "Travel",
	"EXPENSE_CATEGORY_GROCERIES":       "Groceries",
	"EXPENSE_CATEGORY_RENT_MORTGAGE":   "Rent/Mortgage",
	"EXPENSE_CATEGORY_INSURANCE":       "Insurance",
	"EXPENSE_CATEGORY_INVESTMENTS":     "Investments",
	"EXPENSE_CATEGORY_GIFTS_DONATIONS": "Gifts & Donations",
	"EXPENSE_CATEGORY_PERSONAL_CARE":   "Personal Care",
	"EXPENSE_CATEGORY_SUBSCRIPTIONS":   "Subscriptions",
	"EXPENSE_CATEGORY_OTHER":           "Other",
}

// ========================================
// INCOME MODELS
// ========================================

// IncomeSource represents a user's source of income
type IncomeSource struct {
	ID                uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID            uint      `gorm:"not null;index:idx_income_user"`
	User              User      `gorm:"foreignKey:UserID"`
	Name              string    `gorm:"size:200;not null"`
	Amount            float64   `gorm:"not null"`
	Currency          string    `gorm:"size:3;default:'USD';not null"`
	Category          string    `gorm:"size:50;not null;index:idx_income_category"` // salary, freelance, investments, rental, etc.
	IsRecurring       bool      `gorm:"default:false"`
	RecurrencePattern string    `gorm:"size:20"` // monthly, yearly, etc.
	LastReceived      *time.Time
	NextExpected      *time.Time
	IsActive          bool `gorm:"default:true;index:idx_income_active"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for IncomeSource model
func (IncomeSource) TableName() string {
	return "income_sources"
}

// BeforeCreate hook for IncomeSource
func (i *IncomeSource) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	if i.Currency == "" {
		i.Currency = "USD"
	}
	return nil
}

// ========================================
// INVESTMENT MODELS
// ========================================

// Investment represents a user's investment asset
type Investment struct {
	ID                 uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID             uint      `gorm:"not null;index:idx_investment_user"`
	User               User      `gorm:"foreignKey:UserID"`
	Name               string    `gorm:"size:200;not null"`
	InvestmentType     string    `gorm:"size:50;not null;index:idx_investment_type"` // stocks, crypto, mutual_funds, bonds, etc.
	CurrentValue       float64   `gorm:"not null"`
	InitialInvestment  float64   `gorm:"not null"`
	GainLoss           float64   `gorm:"default:0"`
	GainLossPercentage float64   `gorm:"default:0"`
	Currency           string    `gorm:"size:3;default:'USD';not null"`
	PurchaseDate       time.Time `gorm:"not null"`
	LastUpdated        time.Time `gorm:"not null;index:idx_investment_last_updated"`
	TickerSymbol       string    `gorm:"size:20;index:idx_investment_ticker"`
	Quantity           int       `gorm:"default:1"`
	CurrentPrice       float64   `gorm:"default:0"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for Investment model
func (Investment) TableName() string {
	return "investments"
}

// BeforeCreate hook for Investment
func (i *Investment) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	if i.Currency == "" {
		i.Currency = "USD"
	}
	// Calculate gain/loss
	i.GainLoss = i.CurrentValue - i.InitialInvestment
	if i.InitialInvestment > 0 {
		i.GainLossPercentage = (i.GainLoss / i.InitialInvestment) * 100
	}
	return nil
}

// BeforeUpdate hook for Investment to recalculate gain/loss
func (i *Investment) BeforeUpdate(tx *gorm.DB) error {
	i.GainLoss = i.CurrentValue - i.InitialInvestment
	if i.InitialInvestment > 0 {
		i.GainLossPercentage = (i.GainLoss / i.InitialInvestment) * 100
	}
	i.LastUpdated = time.Now()
	return nil
}

// ========================================
// FINANCIAL GOALS MODELS
// ========================================

// FinancialGoal represents a user's financial goal
type FinancialGoal struct {
	ID                  uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID              uint      `gorm:"not null;index:idx_goal_user"`
	User                User      `gorm:"foreignKey:UserID"`
	Name                string    `gorm:"size:200;not null"`
	GoalType            string    `gorm:"size:50;not null;index:idx_goal_type"` // emergency_fund, vacation, car, house, etc.
	TargetAmount        float64   `gorm:"not null"`
	CurrentAmount       float64   `gorm:"default:0"`
	MonthlyContribution float64   `gorm:"default:0"`
	Currency            string    `gorm:"size:3;default:'USD';not null"`
	TargetDate          time.Time `gorm:"not null;index:idx_goal_target_date"`
	Status              string    `gorm:"size:30;not null;default:'in_progress';index:idx_goal_status"` // in_progress, completed, cancelled, paused
	PercentageComplete  float64   `gorm:"default:0"`
	MonthsRemaining     int       `gorm:"default:0"`
	Icon                string    `gorm:"size:50"`
	Color               string    `gorm:"size:20"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for FinancialGoal model
func (FinancialGoal) TableName() string {
	return "financial_goals"
}

// BeforeCreate hook for FinancialGoal
func (g *FinancialGoal) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	if g.Currency == "" {
		g.Currency = "USD"
	}
	if g.Status == "" {
		g.Status = "in_progress"
	}
	g.UpdateProgress()
	return nil
}

// BeforeUpdate hook for FinancialGoal
func (g *FinancialGoal) BeforeUpdate(tx *gorm.DB) error {
	g.UpdateProgress()
	return nil
}

// UpdateProgress updates percentage and months remaining
func (g *FinancialGoal) UpdateProgress() {
	// Calculate percentage
	if g.TargetAmount > 0 {
		g.PercentageComplete = (g.CurrentAmount / g.TargetAmount) * 100
		if g.PercentageComplete > 100 {
			g.PercentageComplete = 100
		}
	}

	// Calculate months remaining
	now := time.Now()
	if g.TargetDate.After(now) {
		years := g.TargetDate.Year() - now.Year()
		months := int(g.TargetDate.Month() - now.Month())
		g.MonthsRemaining = years*12 + months
		if g.MonthsRemaining < 0 {
			g.MonthsRemaining = 0
		}
	} else {
		g.MonthsRemaining = 0
	}

	// Update status based on completion
	if g.PercentageComplete >= 100 {
		g.Status = "completed"
	}
}

// ========================================
// SAVINGS GOAL MODEL
// ========================================

// SavingsGoal represents the user's primary savings goal
type SavingsGoal struct {
	ID                 uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID             uint      `gorm:"not null;uniqueIndex:idx_savings_goal_user"` // One savings goal per user
	User               User      `gorm:"foreignKey:UserID"`
	Name               string    `gorm:"size:200;not null"`
	TargetAmount       float64   `gorm:"not null"`
	CurrentAmount      float64   `gorm:"default:0"`
	PercentageComplete float64   `gorm:"default:0"`
	TargetDate         time.Time `gorm:"not null"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for SavingsGoal model
func (SavingsGoal) TableName() string {
	return "savings_goals"
}

// BeforeCreate hook for SavingsGoal
func (s *SavingsGoal) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	s.UpdatePercentage()
	return nil
}

// BeforeUpdate hook for SavingsGoal
func (s *SavingsGoal) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatePercentage()
	return nil
}

// UpdatePercentage updates the percentage complete
func (s *SavingsGoal) UpdatePercentage() {
	if s.TargetAmount > 0 {
		s.PercentageComplete = (s.CurrentAmount / s.TargetAmount) * 100
		if s.PercentageComplete > 100 {
			s.PercentageComplete = 100
		}
	}
}

// ========================================
// RECURRING BILLS MODEL
// ========================================

// RecurringBill represents a user's recurring bill/payment
type RecurringBill struct {
	ID                uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID            uint      `gorm:"not null;index:idx_bill_user"`
	User              User      `gorm:"foreignKey:UserID"`
	Name              string    `gorm:"size:200;not null"`
	Amount            float64   `gorm:"not null"`
	Currency          string    `gorm:"size:3;default:'USD';not null"`
	Category          string    `gorm:"size:50;not null"` // bills_utilities, rent_mortgage, subscriptions, etc.
	RecurrencePattern string    `gorm:"size:20;not null"` // monthly, weekly, yearly
	NextDueDate       time.Time `gorm:"not null;index:idx_bill_due_date"`
	LastPaidDate      *time.Time
	Status            string `gorm:"size:30;not null;default:'upcoming';index:idx_bill_status"` // upcoming, paid, overdue, cancelled
	DaysUntilDue      int    `gorm:"default:0"`
	Merchant          string `gorm:"size:200"`
	Icon              string `gorm:"size:50"`
	AutoPayEnabled    bool   `gorm:"default:false"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for RecurringBill model
func (RecurringBill) TableName() string {
	return "recurring_bills"
}

// BeforeCreate hook for RecurringBill
func (r *RecurringBill) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.Currency == "" {
		r.Currency = "USD"
	}
	if r.Status == "" {
		r.Status = "upcoming"
	}
	r.UpdateDaysUntilDue()
	return nil
}

// BeforeUpdate hook for RecurringBill
func (r *RecurringBill) BeforeUpdate(tx *gorm.DB) error {
	r.UpdateDaysUntilDue()
	return nil
}

// UpdateDaysUntilDue calculates days until the bill is due
func (r *RecurringBill) UpdateDaysUntilDue() {
	now := time.Now()
	if r.NextDueDate.After(now) {
		duration := r.NextDueDate.Sub(now)
		r.DaysUntilDue = int(duration.Hours() / 24)
	} else {
		r.DaysUntilDue = 0
		if r.Status == "upcoming" {
			r.Status = "overdue"
		}
	}
}

// ========================================
// AUTOMATIC TRANSACTION TRACKING MODELS
// ========================================

// IncomeTransaction automatically tracks ALL income operations across the platform
// This is separate from user-entered income sources and tracks actual transactions
type IncomeTransaction struct {
	ID              uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID          uint      `gorm:"not null;index:idx_income_txn_user"`
	User            User      `gorm:"foreignKey:UserID"`
	Amount          float64   `gorm:"not null;index:idx_income_txn_amount"`
	Currency        string    `gorm:"size:3;default:'USD';not null"`
	SourceType      string    `gorm:"size:50;not null;index:idx_income_txn_source_type"` // deposit, transfer_received, invoice_payment_received, tagged_invoice_payment_received, etc.
	SourceID        string    `gorm:"size:255;index:idx_income_txn_source_id"`           // ID of the source transaction (transfer_id, deposit_id, etc.)
	SourceReference string    `gorm:"size:500"`                                          // Additional reference (invoice number, transfer reference, etc.)
	Category        string    `gorm:"size:50;index:idx_income_txn_category"`             // Maps to IncomeCategory enum
	Description     string    `gorm:"type:text"`
	SenderID        *uint     `gorm:"index:idx_income_txn_sender"` // User ID of sender (if applicable, e.g., for transfers)
	SenderName      string    `gorm:"size:255"`
	TransactionDate time.Time `gorm:"not null;index:idx_income_txn_date"`
	Metadata        string    `gorm:"type:jsonb"` // Additional flexible data stored as JSON
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for IncomeTransaction model
func (IncomeTransaction) TableName() string {
	return "income_transactions"
}

// BeforeCreate hook for IncomeTransaction
func (i *IncomeTransaction) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	if i.Currency == "" {
		i.Currency = "USD"
	}
	if i.TransactionDate.IsZero() {
		i.TransactionDate = time.Now()
	}
	return nil
}

// ExpenditureTransaction automatically tracks ALL expenditure operations across the platform
// This is separate from user-entered expenses and tracks actual transactions
type ExpenditureTransaction struct {
	ID               uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID           uint      `gorm:"not null;index:idx_expenditure_txn_user"`
	User             User      `gorm:"foreignKey:UserID"`
	Amount           float64   `gorm:"not null;index:idx_expenditure_txn_amount"`
	Currency         string    `gorm:"size:3;default:'USD';not null"`
	ExpenseType      string    `gorm:"size:50;not null;index:idx_expenditure_txn_type"` // withdrawal, transfer_sent, invoice_payment_made, bill_payment, exchange, etc.
	ExpenseID        string    `gorm:"size:255;index:idx_expenditure_txn_expense_id"`   // ID of the expense transaction
	ExpenseReference string    `gorm:"size:500"`                                        // Additional reference
	Category         string    `gorm:"size:50;index:idx_expenditure_txn_category"`      // Maps to ExpenseCategory enum
	RecipientID      *uint     `gorm:"index:idx_expenditure_txn_recipient"`             // User ID of recipient (if applicable)
	RecipientName    string    `gorm:"size:255"`
	Merchant         string    `gorm:"size:255;index:idx_expenditure_txn_merchant"`
	Description      string    `gorm:"type:text"`
	TransactionDate  time.Time `gorm:"not null;index:idx_expenditure_txn_date"`
	Metadata         string    `gorm:"type:jsonb"` // Additional flexible data stored as JSON
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name for ExpenditureTransaction model
func (ExpenditureTransaction) TableName() string {
	return "expenditure_transactions"
}

// BeforeCreate hook for ExpenditureTransaction
func (e *ExpenditureTransaction) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.Currency == "" {
		e.Currency = "USD"
	}
	if e.TransactionDate.IsZero() {
		e.TransactionDate = time.Now()
	}
	return nil
}
