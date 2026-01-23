package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type StatisticsService struct {
	db        *gorm.DB
	txTracker *TransactionTracker // Tracks income/expenditure for statistics
}

func NewStatisticsService(db *gorm.DB) *StatisticsService {
	return &StatisticsService{
		db:        db,
		txTracker: NewTransactionTracker(db), // Initialize transaction tracker
	}
}

// ========================================
// EXPENSE MANAGEMENT
// ========================================

func (s *StatisticsService) CreateExpense(ctx context.Context, userID uint, req *pb.CreateExpenseRequest) (*pb.ExpenseMessage, error) {
	// Parse transaction date
	transactionDate := time.Now()
	if req.TransactionDate != nil {
		transactionDate = req.TransactionDate.AsTime()
	}

	// Convert tags to JSON string
	tagsJSON, err := json.Marshal(req.Tags)
	return nil, fmt.Errorf("failed to marshal tags: %w", err)

	// Create expense
	expense := &models.Expense{
		UserID:            userID,
		Amount:            req.Amount,
		Currency:          req.Currency,
		Category:          req.Category.String(),
		Subcategory:       req.Subcategory,
		Description:       req.Description,
		Merchant:          req.Merchant,
		TransactionDate:   transactionDate,
		PaymentMethod:     req.PaymentMethod,
		ReceiptURL:        req.ReceiptUrl,
		Tags:              string(tagsJSON),
		Notes:             req.Notes,
		IsRecurring:       req.IsRecurring,
		RecurrencePattern: req.RecurrencePattern,
	}

	// Parse account ID if provided
	if req.AccountId != "" {
		accountID, err := uuid.Parse(req.AccountId)
		return nil, fmt.Errorf("invalid account_id: %w", err)
		expense.AccountID = &accountID
	}

	// Save to database
	if err := s.db.Create(expense).Error; err != nil {
		return nil, fmt.Errorf("failed to create expense: %w", err)
	}

	// Update related budgets
	if err := s.updateBudgetsForExpense(expense); err != nil {
		// Log error but don't fail the expense creation
		fmt.Printf("Warning: failed to update budgets: %v\n", err)
	}

	// Convert to proto message
	return s.expenseToProto(expense), nil
}

func (s *StatisticsService) GetExpenses(ctx context.Context, userID uint, req *pb.GetExpensesRequest) ([]*pb.ExpenseMessage, *pb.PaginationMetadata, float64, int32, error) {
	query := s.db.Model(&models.Expense{}).Where("user_id = ?", userID)

	// Apply filters
	if req.StartDate != nil {
		query = query.Where("transaction_date >= ?", req.StartDate.AsTime())
	}
	if req.EndDate != nil {
		query = query.Where("transaction_date <= ?", req.EndDate.AsTime())
	}
	if req.Category != pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED {
		query = query.Where("category = ?", req.Category.String())
	}
	if req.MinAmount > 0 {
		query = query.Where("amount >= ?", req.MinAmount)
	}
	if req.MaxAmount > 0 {
		query = query.Where("amount <= ?", req.MaxAmount)
	}
	if req.SearchQuery != "" {
		searchPattern := "%" + req.SearchQuery + "%"
		query = query.Where("description ILIKE ? OR merchant ILIKE ? OR notes ILIKE ?",
			searchPattern, searchPattern, searchPattern)
	}

	// Get total count
	var totalCount int64
	countQuery := s.db.Model(&models.Expense{}).Where("user_id = ?", userID)

	// Apply same filters for count
	if req.StartDate != nil {
		countQuery = countQuery.Where("transaction_date >= ?", req.StartDate.AsTime())
	}
	if req.EndDate != nil {
		countQuery = countQuery.Where("transaction_date <= ?", req.EndDate.AsTime())
	}
	if req.Category != pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED {
		countQuery = countQuery.Where("category = ?", req.Category.String())
	}
	if req.MinAmount > 0 {
		countQuery = countQuery.Where("amount >= ?", req.MinAmount)
	}
	if req.MaxAmount > 0 {
		countQuery = countQuery.Where("amount <= ?", req.MaxAmount)
	}
	if req.SearchQuery != "" {
		searchPattern := "%" + req.SearchQuery + "%"
		countQuery = countQuery.Where("description ILIKE ? OR merchant ILIKE ? OR notes ILIKE ?",
			searchPattern, searchPattern, searchPattern)
	}

	if err := countQuery.Count(&totalCount).Error; err != nil {
		return nil, nil, 0, 0, fmt.Errorf("failed to count expenses: %w", err)
	}

	// Calculate total amount
	var totalAmount float64
	var result struct {
		Total float64
	}
	sumQuery := s.db.Model(&models.Expense{}).Where("user_id = ?", userID)

	// Apply same filters for sum
	if req.StartDate != nil {
		sumQuery = sumQuery.Where("transaction_date >= ?", req.StartDate.AsTime())
	}
	if req.EndDate != nil {
		sumQuery = sumQuery.Where("transaction_date <= ?", req.EndDate.AsTime())
	}
	if req.Category != pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED {
		sumQuery = sumQuery.Where("category = ?", req.Category.String())
	}
	if req.MinAmount > 0 {
		sumQuery = sumQuery.Where("amount >= ?", req.MinAmount)
	}
	if req.MaxAmount > 0 {
		sumQuery = sumQuery.Where("amount <= ?", req.MaxAmount)
	}
	if req.SearchQuery != "" {
		searchPattern := "%" + req.SearchQuery + "%"
		sumQuery = sumQuery.Where("description ILIKE ? OR merchant ILIKE ? OR notes ILIKE ?",
			searchPattern, searchPattern, searchPattern)
	}

	if err := sumQuery.Select("COALESCE(SUM(amount), 0) as total").Scan(&result).Error; err != nil {
		return nil, nil, 0, 0, fmt.Errorf("failed to sum expenses: %w", err)
	}
	totalAmount = result.Total

	// Pagination
	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage
	totalPages := int32((totalCount + int64(perPage) - 1) / int64(perPage))

	// Fetch expenses
	var expenses []models.Expense
	if err := query.Order("transaction_date DESC").
		Limit(int(perPage)).
		Offset(int(offset)).
		Find(&expenses).Error; err != nil {
		return nil, nil, 0, 0, fmt.Errorf("failed to fetch expenses: %w", err)
	}

	// Convert to proto
	protoExpenses := make([]*pb.ExpenseMessage, len(expenses))
	for i, expense := range expenses {
		protoExpenses[i] = s.expenseToProto(&expense)
	}

	pagination := &pb.PaginationMetadata{
		CurrentPage: page,
		PerPage:     perPage,
		TotalPages:  totalPages,
		TotalItems:  int32(totalCount),
		HasNext:     page < totalPages,
		HasPrev:     page > 1,
	}

	return protoExpenses, pagination, totalAmount, int32(totalCount), nil
}

func (s *StatisticsService) GetExpenseById(ctx context.Context, userID uint, expenseID string) (*pb.ExpenseMessage, error) {
	id, err := uuid.Parse(expenseID)
	return nil, fmt.Errorf("invalid expense_id: %w", err)

	var expense models.Expense
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&expense).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("expense not found")
		}
		return nil, fmt.Errorf("failed to fetch expense: %w", err)
	}

	return s.expenseToProto(&expense), nil
}

func (s *StatisticsService) UpdateExpense(ctx context.Context, userID uint, req *pb.UpdateExpenseRequest) (*pb.ExpenseMessage, error) {
	id, err := uuid.Parse(req.ExpenseId)
	return nil, fmt.Errorf("invalid expense_id: %w", err)

	var expense models.Expense
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&expense).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("expense not found")
		}
		return nil, fmt.Errorf("failed to fetch expense: %w", err)
	}

	// Update fields
	if req.Amount > 0 {
		expense.Amount = req.Amount
	}
	if req.Category != pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED {
		expense.Category = req.Category.String()
	}
	if req.Subcategory != "" {
		expense.Subcategory = req.Subcategory
	}
	if req.Description != "" {
		expense.Description = req.Description
	}
	if req.Merchant != "" {
		expense.Merchant = req.Merchant
	}
	if req.TransactionDate != nil {
		expense.TransactionDate = req.TransactionDate.AsTime()
	}
	if req.PaymentMethod != "" {
		expense.PaymentMethod = req.PaymentMethod
	}
	if len(req.Tags) > 0 {
		tagsJSON, err := json.Marshal(req.Tags)
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
		expense.Tags = string(tagsJSON)
	}
	if req.Notes != "" {
		expense.Notes = req.Notes
	}

	// Save changes
	if err := s.db.Save(&expense).Error; err != nil {
		return nil, fmt.Errorf("failed to update expense: %w", err)
	}

	// Update related budgets
	if err := s.updateBudgetsForExpense(&expense); err != nil {
		fmt.Printf("Warning: failed to update budgets: %v\n", err)
	}

	return s.expenseToProto(&expense), nil
}

func (s *StatisticsService) DeleteExpense(ctx context.Context, userID uint, expenseID string) error {
	id, err := uuid.Parse(expenseID)
	return fmt.Errorf("invalid expense_id: %w", err)

	var expense models.Expense
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&expense).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("expense not found")
		}
		return fmt.Errorf("failed to fetch expense: %w", err)
	}

	// Soft delete
	if err := s.db.Delete(&expense).Error; err != nil {
		return fmt.Errorf("failed to delete expense: %w", err)
	}

	// Update related budgets
	if err := s.updateBudgetsForExpense(&expense); err != nil {
		fmt.Printf("Warning: failed to update budgets: %v\n", err)
	}

	return nil
}

// ========================================
// BUDGET MANAGEMENT
// ========================================

func (s *StatisticsService) CreateBudget(ctx context.Context, userID uint, req *pb.CreateBudgetRequest) (*pb.BudgetMessage, error) {
	// Parse dates
	startDate := time.Now()
	if req.StartDate != nil {
		startDate = req.StartDate.AsTime()
	}

	endDate := s.calculateEndDate(startDate, req.Period)
	if req.EndDate != nil {
		endDate = req.EndDate.AsTime()
	}

	// Create budget
	budget := &models.Budget{
		UserID:         userID,
		Name:           req.Name,
		Amount:         req.Amount,
		Currency:       req.Currency,
		Category:       req.Category.String(),
		Period:         req.Period.String(),
		StartDate:      startDate,
		EndDate:        endDate,
		EnableAlerts:   req.EnableAlerts,
		AlertThreshold: req.AlertThreshold,
	}

	// Calculate initial spending
	if err := budget.UpdateBudgetCalculations(s.db); err != nil {
		return nil, fmt.Errorf("failed to calculate budget: %w", err)
	}

	// Save to database
	if err := s.db.Create(budget).Error; err != nil {
		return nil, fmt.Errorf("failed to create budget: %w", err)
	}

	// Check if alert should be generated
	if budget.EnableAlerts {
		s.checkAndCreateAlert(budget)
	}

	return s.budgetToProto(budget), nil
}

func (s *StatisticsService) GetBudgets(ctx context.Context, userID uint, req *pb.GetBudgetsRequest) ([]*pb.BudgetMessage, *pb.PaginationMetadata, float64, float64, error) {
	query := s.db.Model(&models.Budget{}).Where("user_id = ?", userID)

	// Apply filters
	if req.Status != pb.BudgetStatus_BUDGET_STATUS_UNSPECIFIED {
		query = query.Where("status = ?", req.Status.String())
	}
	if req.Category != pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED {
		query = query.Where("category = ?", req.Category.String())
	}

	// Get total count
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, nil, 0, 0, fmt.Errorf("failed to count budgets: %w", err)
	}

	// Calculate totals
	var totalBudgetAmount, totalSpentAmount float64
	var result struct {
		TotalBudget float64
		TotalSpent  float64
	}
	// Create a new query for the sum to avoid query pollution
	sumQuery := s.db.Model(&models.Budget{}).Where("user_id = ?", userID)
	if req.Status != pb.BudgetStatus_BUDGET_STATUS_UNSPECIFIED {
		sumQuery = sumQuery.Where("status = ?", req.Status.String())
	}
	if req.Category != pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED {
		sumQuery = sumQuery.Where("category = ?", req.Category.String())
	}
	if err := sumQuery.Select("COALESCE(SUM(amount), 0) as total_budget, COALESCE(SUM(spent_amount), 0) as total_spent").
		Scan(&result).Error; err != nil {
		return nil, nil, 0, 0, fmt.Errorf("failed to sum budgets: %w", err)
	}
	totalBudgetAmount = result.TotalBudget
	totalSpentAmount = result.TotalSpent

	// Pagination
	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage
	totalPages := int32((totalCount + int64(perPage) - 1) / int64(perPage))

	// Fetch budgets - create fresh query to avoid SELECT clause pollution from sum query
	fetchQuery := s.db.Model(&models.Budget{}).Where("user_id = ?", userID)
	if req.Status != pb.BudgetStatus_BUDGET_STATUS_UNSPECIFIED {
		fetchQuery = fetchQuery.Where("status = ?", req.Status.String())
	}
	if req.Category != pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED {
		fetchQuery = fetchQuery.Where("category = ?", req.Category.String())
	}
	var budgets []models.Budget
	if err := fetchQuery.Order("start_date DESC").
		Limit(int(perPage)).
		Offset(int(offset)).
		Find(&budgets).Error; err != nil {
		return nil, nil, 0, 0, fmt.Errorf("failed to fetch budgets: %w", err)
	}

	// Update budget calculations
	for i := range budgets {
		budgets[i].UpdateBudgetCalculations(s.db)
		s.db.Save(&budgets[i])
	}

	// Convert to proto
	protoBudgets := make([]*pb.BudgetMessage, len(budgets))
	for i, budget := range budgets {
		protoBudgets[i] = s.budgetToProto(&budget)
	}

	pagination := &pb.PaginationMetadata{
		CurrentPage: page,
		PerPage:     perPage,
		TotalPages:  totalPages,
		TotalItems:  int32(totalCount),
		HasNext:     page < totalPages,
		HasPrev:     page > 1,
	}

	return protoBudgets, pagination, totalBudgetAmount, totalSpentAmount, nil
}

func (s *StatisticsService) GetBudgetById(ctx context.Context, userID uint, budgetID string) (*pb.BudgetMessage, []*pb.ExpenseMessage, error) {
	id, err := uuid.Parse(budgetID)
	return nil, nil, fmt.Errorf("invalid budget_id: %w", err)

	var budget models.Budget
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&budget).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("budget not found")
		}
		return nil, nil, fmt.Errorf("failed to fetch budget: %w", err)
	}

	// Update budget calculations
	budget.UpdateBudgetCalculations(s.db)
	s.db.Save(&budget)

	// Fetch recent expenses for this category
	var expenses []models.Expense
	s.db.Where("user_id = ? AND category = ? AND transaction_date >= ? AND transaction_date <= ?",
		userID, budget.Category, budget.StartDate, budget.EndDate).
		Order("transaction_date DESC").
		Limit(10).
		Find(&expenses)

	protoExpenses := make([]*pb.ExpenseMessage, len(expenses))
	for i, expense := range expenses {
		protoExpenses[i] = s.expenseToProto(&expense)
	}

	return s.budgetToProto(&budget), protoExpenses, nil
}

func (s *StatisticsService) UpdateBudget(ctx context.Context, userID uint, req *pb.UpdateBudgetRequest) (*pb.BudgetMessage, error) {
	id, err := uuid.Parse(req.BudgetId)
	return nil, fmt.Errorf("invalid budget_id: %w", err)

	var budget models.Budget
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&budget).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("budget not found")
		}
		return nil, fmt.Errorf("failed to fetch budget: %w", err)
	}

	// Update fields
	if req.Name != "" {
		budget.Name = req.Name
	}
	if req.Amount > 0 {
		budget.Amount = req.Amount
	}
	if req.Period != pb.BudgetPeriod_BUDGET_PERIOD_UNSPECIFIED {
		budget.Period = req.Period.String()
	}
	if req.StartDate != nil {
		budget.StartDate = req.StartDate.AsTime()
	}
	if req.EndDate != nil {
		budget.EndDate = req.EndDate.AsTime()
	}
	budget.EnableAlerts = req.EnableAlerts
	if req.AlertThreshold > 0 {
		budget.AlertThreshold = req.AlertThreshold
	}

	// Recalculate
	if err := budget.UpdateBudgetCalculations(s.db); err != nil {
		return nil, fmt.Errorf("failed to calculate budget: %w", err)
	}

	// Save changes
	if err := s.db.Save(&budget).Error; err != nil {
		return nil, fmt.Errorf("failed to update budget: %w", err)
	}

	// Check if alert should be generated
	if budget.EnableAlerts {
		s.checkAndCreateAlert(&budget)
	}

	return s.budgetToProto(&budget), nil
}

func (s *StatisticsService) DeleteBudget(ctx context.Context, userID uint, budgetID string) error {
	id, err := uuid.Parse(budgetID)
	return fmt.Errorf("invalid budget_id: %w", err)

	var budget models.Budget
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&budget).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("budget not found")
		}
		return fmt.Errorf("failed to fetch budget: %w", err)
	}

	// Soft delete
	if err := s.db.Delete(&budget).Error; err != nil {
		return fmt.Errorf("failed to delete budget: %w", err)
	}

	return nil
}

// ========================================
// ANALYTICS & REPORTS
// ========================================

func (s *StatisticsService) GetSpendingAnalytics(ctx context.Context, userID uint, req *pb.GetSpendingAnalyticsRequest) (*pb.SpendingAnalytics, error) {
	startDate, endDate := s.getDateRangeForPeriod(req.Period, req.StartDate, req.EndDate)

	query := s.db.Model(&models.Expense{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
			userID, startDate, endDate)

	if req.Category != pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED {
		query = query.Where("category = ?", req.Category.String())
	}

	// Calculate total spent and transaction count
	var totalSpent float64
	var transactionCount int64
	var result struct {
		Total float64
		Count int64
	}
	if err := query.Select("COALESCE(SUM(amount), 0) as total, COUNT(*) as count").
		Scan(&result).Error; err != nil {
		return nil, fmt.Errorf("failed to calculate spending: %w", err)
	}
	totalSpent = result.Total
	transactionCount = result.Count

	// Calculate average transaction
	averageTransaction := 0.0
	if transactionCount > 0 {
		averageTransaction = totalSpent / float64(transactionCount)
	}

	// Get category breakdown
	categoryBreakdown, topCategory, topCategoryAmount := s.getCategoryBreakdown(userID, startDate, endDate)

	// Get daily trend
	dailyTrend := s.getDailyTrend(userID, startDate, endDate)

	// Get total budget for period
	var totalBudget float64
	s.db.Model(&models.Budget{}).
		Where("user_id = ? AND start_date <= ? AND end_date >= ?",
			userID, endDate, startDate).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalBudget)

	remainingBudget := totalBudget - totalSpent
	savingsRate := 0.0
	if totalBudget > 0 {
		savingsRate = (remainingBudget / totalBudget) * 100
	}

	analytics := &pb.SpendingAnalytics{
		Period:             req.Period,
		TotalSpent:         totalSpent,
		TotalBudget:        totalBudget,
		RemainingBudget:    remainingBudget,
		TransactionCount:   int32(transactionCount),
		AverageTransaction: averageTransaction,
		CategoryBreakdown:  categoryBreakdown,
		DailyTrend:         dailyTrend,
		TopCategory:        topCategory,
		TopCategoryAmount:  topCategoryAmount,
		SavingsRate:        savingsRate,
	}

	return analytics, nil
}

func (s *StatisticsService) GetCategoryBreakdown(ctx context.Context, userID uint, req *pb.GetCategoryBreakdownRequest) ([]*pb.CategorySpending, float64, error) {
	startDate := time.Now().AddDate(0, -1, 0)
	if req.StartDate != nil {
		startDate = req.StartDate.AsTime()
	}

	endDate := time.Now()
	if req.EndDate != nil {
		endDate = req.EndDate.AsTime()
	}

	categories, _, _ := s.getCategoryBreakdown(userID, startDate, endDate)

	var totalSpent float64
	for _, cat := range categories {
		totalSpent += cat.Amount
	}

	return categories, totalSpent, nil
}

func (s *StatisticsService) GetBudgetProgress(ctx context.Context, userID uint, req *pb.GetBudgetProgressRequest) ([]*pb.BudgetProgressItem, float64, float64, float64, error) {
	query := s.db.Model(&models.Budget{}).Where("user_id = ?", userID)

	if req.Period != pb.BudgetPeriod_BUDGET_PERIOD_UNSPECIFIED {
		query = query.Where("period = ?", req.Period.String())
	}

	// Fetch active budgets
	var budgets []models.Budget
	if err := query.Where("status IN ?", []string{"active", "near_limit", "exceeded"}).
		Find(&budgets).Error; err != nil {
		return nil, 0, 0, 0, fmt.Errorf("failed to fetch budgets: %w", err)
	}

	// Update and convert to progress items
	progressItems := make([]*pb.BudgetProgressItem, 0, len(budgets))
	var totalBudget, totalSpent float64

	for _, budget := range budgets {
		budget.UpdateBudgetCalculations(s.db)
		s.db.Save(&budget)

		// Calculate days remaining
		daysRemaining := int32(time.Until(budget.EndDate).Hours() / 24)
		if daysRemaining < 0 {
			daysRemaining = 0
		}

		// Calculate daily average spend
		daysPassed := int32(time.Since(budget.StartDate).Hours() / 24)
		if daysPassed < 1 {
			daysPassed = 1
		}
		dailyAverageSpend := budget.SpentAmount / float64(daysPassed)

		// Project total spend
		totalDays := int32(budget.EndDate.Sub(budget.StartDate).Hours() / 24)
		projectedSpend := dailyAverageSpend * float64(totalDays)

		willExceed := projectedSpend > budget.Amount

		// Parse category enum
		categoryEnum := pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED
		if catVal, ok := pb.ExpenseCategory_value[budget.Category]; ok {
			categoryEnum = pb.ExpenseCategory(catVal)
		}

		// Parse status enum
		statusEnum := pb.BudgetStatus_BUDGET_STATUS_UNSPECIFIED
		if statVal, ok := pb.BudgetStatus_value[budget.Status]; ok {
			statusEnum = pb.BudgetStatus(statVal)
		}

		progressItems = append(progressItems, &pb.BudgetProgressItem{
			BudgetId:          budget.ID.String(),
			BudgetName:        budget.Name,
			Category:          categoryEnum,
			BudgetAmount:      budget.Amount,
			SpentAmount:       budget.SpentAmount,
			RemainingAmount:   budget.RemainingAmount,
			PercentageUsed:    budget.PercentageUsed,
			Status:            statusEnum,
			DaysRemaining:     daysRemaining,
			DailyAverageSpend: dailyAverageSpend,
			ProjectedSpend:    projectedSpend,
			WillExceed:        willExceed,
		})

		totalBudget += budget.Amount
		totalSpent += budget.SpentAmount
	}

	overallPercentage := 0.0
	if totalBudget > 0 {
		overallPercentage = (totalSpent / totalBudget) * 100
	}

	return progressItems, totalBudget, totalSpent, overallPercentage, nil
}

func (s *StatisticsService) GetSpendingTrends(ctx context.Context, userID uint, req *pb.GetSpendingTrendsRequest) ([]*pb.SpendingTrend, error) {
	periodsCount := req.PeriodsCount
	if periodsCount < 1 || periodsCount > 24 {
		periodsCount = 12
	}

	endDate := time.Now()
	if req.EndDate != nil {
		endDate = req.EndDate.AsTime()
	}

	trends := make([]*pb.SpendingTrend, 0, periodsCount)

	for i := int32(0); i < periodsCount; i++ {
		var periodStart, periodEnd time.Time
		var periodLabel string

		switch req.PeriodType {
		case "daily":
			periodEnd = endDate.AddDate(0, 0, -int(i))
			periodStart = periodEnd.AddDate(0, 0, -1)
			periodLabel = periodEnd.Format("Jan 2")
		case "weekly":
			periodEnd = endDate.AddDate(0, 0, -int(i)*7)
			periodStart = periodEnd.AddDate(0, 0, -7)
			periodLabel = fmt.Sprintf("Week %d", periodsCount-i)
		default: // monthly
			periodEnd = endDate.AddDate(0, -int(i), 0)
			periodStart = time.Date(periodEnd.Year(), periodEnd.Month(), 1, 0, 0, 0, 0, periodEnd.Location())
			periodEnd = periodStart.AddDate(0, 1, 0).Add(-time.Second)
			periodLabel = periodStart.Format("Jan 2006")
		}

		// Calculate spending for period
		var totalSpent float64
		s.db.Model(&models.Expense{}).
			Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
				userID, periodStart, periodEnd).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&totalSpent)

		// Get budget for period
		var budgetAmount float64
		s.db.Model(&models.Budget{}).
			Where("user_id = ? AND start_date <= ? AND end_date >= ?",
				userID, periodEnd, periodStart).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&budgetAmount)

		variance := budgetAmount - totalSpent
		variancePercentage := 0.0
		if budgetAmount > 0 {
			variancePercentage = (variance / budgetAmount) * 100
		}

		// Get category breakdown for period
		categories, _, _ := s.getCategoryBreakdown(userID, periodStart, periodEnd)

		trends = append([]*pb.SpendingTrend{{
			PeriodLabel:        periodLabel,
			PeriodStart:        timestamppb.New(periodStart),
			PeriodEnd:          timestamppb.New(periodEnd),
			TotalSpent:         totalSpent,
			BudgetAmount:       budgetAmount,
			Variance:           variance,
			VariancePercentage: variancePercentage,
			Categories:         categories,
		}}, trends...) // Prepend to reverse order
	}

	return trends, nil
}

// ========================================
// NOTIFICATIONS / ALERTS
// ========================================

func (s *StatisticsService) GetBudgetAlerts(ctx context.Context, userID uint, req *pb.GetBudgetAlertsRequest) ([]*pb.BudgetAlertMessage, int32, error) {
	query := s.db.Model(&models.BudgetAlert{}).Where("user_id = ?", userID)

	if req.UnreadOnly {
		query = query.Where("is_read = ?", false)
	}

	limit := req.Limit
	if limit < 1 || limit > 100 {
		limit = 50
	}

	var alerts []models.BudgetAlert
	if err := query.Order("created_at DESC").Limit(int(limit)).Find(&alerts).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch alerts: %w", err)
	}

	// Get unread count
	var unreadCount int64
	s.db.Model(&models.BudgetAlert{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&unreadCount)

	protoAlerts := make([]*pb.BudgetAlertMessage, len(alerts))
	for i, alert := range alerts {
		protoAlerts[i] = s.alertToProto(&alert)
	}

	return protoAlerts, int32(unreadCount), nil
}

func (s *StatisticsService) MarkAlertAsRead(ctx context.Context, userID uint, alertID string) error {
	id, err := uuid.Parse(alertID)
	return fmt.Errorf("invalid alert_id: %w", err)

	var alert models.BudgetAlert
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&alert).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("alert not found")
		}
		return fmt.Errorf("failed to fetch alert: %w", err)
	}

	alert.IsRead = true
	if err := s.db.Save(&alert).Error; err != nil {
		return fmt.Errorf("failed to update alert: %w", err)
	}

	return nil
}

// ========================================
// HELPER FUNCTIONS
// ========================================

func (s *StatisticsService) expenseToProto(expense *models.Expense) *pb.ExpenseMessage {
	var tags []string
	if expense.Tags != "" {
		json.Unmarshal([]byte(expense.Tags), &tags)
	}

	accountID := ""
	if expense.AccountID != nil {
		accountID = expense.AccountID.String()
	}

	// Parse category enum
	categoryEnum := pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED
	if catVal, ok := pb.ExpenseCategory_value[expense.Category]; ok {
		categoryEnum = pb.ExpenseCategory(catVal)
	}

	return &pb.ExpenseMessage{
		Id:                expense.ID.String(),
		UserId:            uint64(expense.UserID),
		AccountId:         accountID,
		Amount:            expense.Amount,
		Currency:          expense.Currency,
		Category:          categoryEnum,
		Subcategory:       expense.Subcategory,
		Description:       expense.Description,
		Merchant:          expense.Merchant,
		TransactionDate:   timestamppb.New(expense.TransactionDate),
		PaymentMethod:     expense.PaymentMethod,
		ReceiptUrl:        expense.ReceiptURL,
		Tags:              tags,
		Notes:             expense.Notes,
		IsRecurring:       expense.IsRecurring,
		RecurrencePattern: expense.RecurrencePattern,
		CreatedAt:         timestamppb.New(expense.CreatedAt),
		UpdatedAt:         timestamppb.New(expense.UpdatedAt),
	}
}

func (s *StatisticsService) budgetToProto(budget *models.Budget) *pb.BudgetMessage {
	// Parse category enum
	categoryEnum := pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED
	if catVal, ok := pb.ExpenseCategory_value[budget.Category]; ok {
		categoryEnum = pb.ExpenseCategory(catVal)
	}

	// Parse period enum
	periodEnum := pb.BudgetPeriod_BUDGET_PERIOD_UNSPECIFIED
	if perVal, ok := pb.BudgetPeriod_value[budget.Period]; ok {
		periodEnum = pb.BudgetPeriod(perVal)
	}

	// Parse status enum
	statusEnum := pb.BudgetStatus_BUDGET_STATUS_UNSPECIFIED
	if statVal, ok := pb.BudgetStatus_value[budget.Status]; ok {
		statusEnum = pb.BudgetStatus(statVal)
	}

	return &pb.BudgetMessage{
		Id:              budget.ID.String(),
		UserId:          uint64(budget.UserID),
		Name:            budget.Name,
		Amount:          budget.Amount,
		Currency:        budget.Currency,
		Category:        categoryEnum,
		Period:          periodEnum,
		StartDate:       timestamppb.New(budget.StartDate),
		EndDate:         timestamppb.New(budget.EndDate),
		SpentAmount:     budget.SpentAmount,
		RemainingAmount: budget.RemainingAmount,
		PercentageUsed:  budget.PercentageUsed,
		Status:          statusEnum,
		EnableAlerts:    budget.EnableAlerts,
		AlertThreshold:  budget.AlertThreshold,
		CreatedAt:       timestamppb.New(budget.CreatedAt),
		UpdatedAt:       timestamppb.New(budget.UpdatedAt),
	}
}

func (s *StatisticsService) alertToProto(alert *models.BudgetAlert) *pb.BudgetAlertMessage {
	// Parse alert type enum
	alertTypeEnum := pb.AlertType_ALERT_TYPE_UNSPECIFIED
	if typeVal, ok := pb.AlertType_value[alert.AlertType]; ok {
		alertTypeEnum = pb.AlertType(typeVal)
	}

	return &pb.BudgetAlertMessage{
		Id:             alert.ID.String(),
		UserId:         uint64(alert.UserID),
		BudgetId:       alert.BudgetID.String(),
		BudgetName:     alert.BudgetName,
		AlertType:      alertTypeEnum,
		Message:        alert.Message,
		CurrentSpent:   alert.CurrentSpent,
		BudgetLimit:    alert.BudgetLimit,
		PercentageUsed: alert.PercentageUsed,
		IsRead:         alert.IsRead,
		CreatedAt:      timestamppb.New(alert.CreatedAt),
	}
}

func (s *StatisticsService) updateBudgetsForExpense(expense *models.Expense) error {
	var budgets []models.Budget
	if err := s.db.Where("user_id = ? AND category = ? AND start_date <= ? AND end_date >= ?",
		expense.UserID, expense.Category, expense.TransactionDate, expense.TransactionDate).
		Find(&budgets).Error; err != nil {
		return err
	}

	for i := range budgets {
		if err := budgets[i].UpdateBudgetCalculations(s.db); err != nil {
			return err
		}
		if err := s.db.Save(&budgets[i]).Error; err != nil {
			return err
		}

		// Check if alert should be generated
		if budgets[i].EnableAlerts {
			s.checkAndCreateAlert(&budgets[i])
		}
	}

	return nil
}

func (s *StatisticsService) checkAndCreateAlert(budget *models.Budget) {
	if budget.PercentageUsed < budget.AlertThreshold {
		return
	}

	// Check if similar alert already exists in the last 24 hours
	var existingCount int64
	s.db.Model(&models.BudgetAlert{}).
		Where("budget_id = ? AND created_at > ?", budget.ID, time.Now().Add(-24*time.Hour)).
		Count(&existingCount)

	if existingCount > 0 {
		return // Don't create duplicate alerts
	}

	alertType := "ALERT_TYPE_THRESHOLD_REACHED"
	message := fmt.Sprintf("You've used %.1f%% of your %s budget", budget.PercentageUsed, budget.Name)

	if budget.PercentageUsed >= 100 {
		alertType = "ALERT_TYPE_BUDGET_EXCEEDED"
		message = fmt.Sprintf("You've exceeded your %s budget by $%.2f", budget.Name, budget.SpentAmount-budget.Amount)
	} else if budget.PercentageUsed >= 90 {
		alertType = "ALERT_TYPE_APPROACHING_LIMIT"
		message = fmt.Sprintf("You're approaching your %s budget limit (%.1f%% used)", budget.Name, budget.PercentageUsed)
	}

	alert := &models.BudgetAlert{
		UserID:         budget.UserID,
		BudgetID:       budget.ID,
		BudgetName:     budget.Name,
		AlertType:      alertType,
		Message:        message,
		CurrentSpent:   budget.SpentAmount,
		BudgetLimit:    budget.Amount,
		PercentageUsed: budget.PercentageUsed,
		IsRead:         false,
	}

	s.db.Create(alert)
}

func (s *StatisticsService) calculateEndDate(startDate time.Time, period pb.BudgetPeriod) time.Time {
	switch period {
	case pb.BudgetPeriod_BUDGET_PERIOD_DAILY:
		return startDate.AddDate(0, 0, 1)
	case pb.BudgetPeriod_BUDGET_PERIOD_WEEKLY:
		return startDate.AddDate(0, 0, 7)
	case pb.BudgetPeriod_BUDGET_PERIOD_MONTHLY:
		return startDate.AddDate(0, 1, 0)
	case pb.BudgetPeriod_BUDGET_PERIOD_QUARTERLY:
		return startDate.AddDate(0, 3, 0)
	case pb.BudgetPeriod_BUDGET_PERIOD_YEARLY:
		return startDate.AddDate(1, 0, 0)
	default:
		return startDate.AddDate(0, 1, 0) // Default to monthly
	}
}

func (s *StatisticsService) getDateRangeForPeriod(period string, startDateProto, endDateProto *timestamppb.Timestamp) (time.Time, time.Time) {
	now := time.Now()

	if startDateProto != nil && endDateProto != nil {
		return startDateProto.AsTime(), endDateProto.AsTime()
	}

	switch period {
	case "daily":
		return now.AddDate(0, 0, -1), now
	case "weekly":
		return now.AddDate(0, 0, -7), now
	case "monthly":
		return now.AddDate(0, -1, 0), now
	case "yearly":
		return now.AddDate(-1, 0, 0), now
	default:
		return now.AddDate(0, -1, 0), now
	}
}

func (s *StatisticsService) getCategoryBreakdown(userID uint, startDate, endDate time.Time) ([]*pb.CategorySpending, string, float64) {
	type CategoryResult struct {
		Category         string
		Amount           float64
		TransactionCount int32
	}

	var results []CategoryResult
	s.db.Model(&models.Expense{}).
		Select("category, COALESCE(SUM(amount), 0) as amount, COUNT(*) as transaction_count").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
			userID, startDate, endDate).
		Group("category").
		Order("amount DESC").
		Scan(&results)

	var totalAmount float64
	for _, r := range results {
		totalAmount += r.Amount
	}

	categories := make([]*pb.CategorySpending, len(results))
	var topCategory string
	var topCategoryAmount float64

	for i, r := range results {
		percentage := 0.0
		if totalAmount > 0 {
			percentage = (r.Amount / totalAmount) * 100
		}

		// Get budget for category
		var budgetAllocated, budgetRemaining float64
		var budget models.Budget
		if err := s.db.Where("user_id = ? AND category = ? AND start_date <= ? AND end_date >= ?",
			userID, r.Category, endDate, startDate).First(&budget).Error; err == nil {
			budgetAllocated = budget.Amount
			budgetRemaining = budget.RemainingAmount
		}

		// Parse category enum
		categoryEnum := pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED
		if catVal, ok := pb.ExpenseCategory_value[r.Category]; ok {
			categoryEnum = pb.ExpenseCategory(catVal)
		}

		// Get category name
		categoryName := models.CategoryNames[r.Category]
		if categoryName == "" {
			categoryName = r.Category
		}

		categories[i] = &pb.CategorySpending{
			Category:         categoryEnum,
			CategoryName:     categoryName,
			Amount:           r.Amount,
			Percentage:       percentage,
			TransactionCount: r.TransactionCount,
			BudgetAllocated:  budgetAllocated,
			BudgetRemaining:  budgetRemaining,
		}

		if i == 0 {
			topCategory = categoryName
			topCategoryAmount = r.Amount
		}
	}

	return categories, topCategory, topCategoryAmount
}

func (s *StatisticsService) getDailyTrend(userID uint, startDate, endDate time.Time) []*pb.DailySpending {
	type DailyResult struct {
		Date             time.Time
		Amount           float64
		TransactionCount int32
	}

	var results []DailyResult
	s.db.Model(&models.Expense{}).
		Select("DATE(transaction_date) as date, COALESCE(SUM(amount), 0) as amount, COUNT(*) as transaction_count").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
			userID, startDate, endDate).
		Group("DATE(transaction_date)").
		Order("date ASC").
		Scan(&results)

	dailySpending := make([]*pb.DailySpending, len(results))
	for i, r := range results {
		dailySpending[i] = &pb.DailySpending{
			Date:             timestamppb.New(r.Date),
			Amount:           r.Amount,
			TransactionCount: r.TransactionCount,
		}
	}

	return dailySpending
}

// ========================================
// INCOME MANAGEMENT METHODS
// ========================================

// GetIncomeSources retrieves all income sources for a user
func (s *StatisticsService) GetIncomeSources(ctx context.Context, userID uint, req *pb.GetIncomeSourcesRequest) (*pb.GetIncomeSourcesResponse, error) {

	// Query income sources
	var incomeSources []models.IncomeSource
	query := s.db.Where("user_id = ?", userID)

	if req.ActiveOnly {
		query = query.Where("is_active = ?", true)
	}

	if err := query.Find(&incomeSources).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch income sources")
	}

	// Convert to proto messages
	pbIncomeSources := make([]*pb.IncomeSource, len(incomeSources))
	var totalMonthlyIncome float64

	for i, source := range incomeSources {
		pbIncomeSources[i] = convertIncomeSourceToProto(&source)

		// Calculate monthly income
		if source.IsRecurring && source.RecurrencePattern == "monthly" {
			totalMonthlyIncome += source.Amount
		}
	}

	return &pb.GetIncomeSourcesResponse{
		IncomeSources:      pbIncomeSources,
		TotalMonthlyIncome: totalMonthlyIncome,
	}, nil
}

// GetIncomeBreakdown retrieves income breakdown by category
func (s *StatisticsService) GetIncomeBreakdown(ctx context.Context, userID uint, req *pb.GetIncomeBreakdownRequest) (*pb.GetIncomeBreakdownResponse, error) {

	startDate := req.StartDate.AsTime()
	endDate := req.EndDate.AsTime()

	// Aggregate income by category
	type CategoryResult struct {
		Category    string
		TotalAmount float64
		SourceCount int64
	}

	var results []CategoryResult
	err := s.db.Model(&models.IncomeSource{}).
		Select("category, SUM(amount) as total_amount, COUNT(*) as source_count").
		Where("user_id = ? AND is_active = ? AND created_at >= ? AND created_at <= ?",
			userID, true, startDate, endDate).
		Group("category").
		Scan(&results).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch income breakdown")
	}

	// Calculate total
	var totalIncome float64
	for _, r := range results {
		totalIncome += r.TotalAmount
	}

	// Convert to proto
	categories := make([]*pb.IncomeCategoryData, len(results))
	for i, r := range results {
		percentage := 0.0
		if totalIncome > 0 {
			percentage = (r.TotalAmount / totalIncome) * 100
		}

		categories[i] = &pb.IncomeCategoryData{
			Category:     convertStringToIncomeCategory(r.Category),
			CategoryName: formatIncomeCategoryName(r.Category),
			Amount:       r.TotalAmount,
			Percentage:   percentage,
			SourceCount:  int32(r.SourceCount),
		}
	}

	return &pb.GetIncomeBreakdownResponse{
		Breakdown: &pb.IncomeBreakdown{
			Categories:  categories,
			TotalIncome: totalIncome,
			Period:      "custom",
		},
	}, nil
}

// CreateIncomeSource creates a new income source
func (s *StatisticsService) CreateIncomeSource(ctx context.Context, userID uint, req *pb.CreateIncomeSourceRequest) (*pb.CreateIncomeSourceResponse, error) {

	// Create income source
	incomeSource := models.IncomeSource{
		UserID:            uint(userID),
		Name:              req.Name,
		Amount:            req.Amount,
		Currency:          req.Currency,
		Category:          req.Category.String(),
		IsRecurring:       req.IsRecurring,
		RecurrencePattern: req.RecurrencePattern,
		IsActive:          true,
	}

	if req.LastReceived != nil {
		lastReceived := req.LastReceived.AsTime()
		incomeSource.LastReceived = &lastReceived
	}

	if req.NextExpected != nil {
		nextExpected := req.NextExpected.AsTime()
		incomeSource.NextExpected = &nextExpected
	}

	if err := s.db.Create(&incomeSource).Error; err != nil {
		return nil, fmt.Errorf("failed to create income source")
	}

	return &pb.CreateIncomeSourceResponse{
		IncomeSource: convertIncomeSourceToProto(&incomeSource),
		Success:      true,
		Message:      "Income source created successfully",
	}, nil
}

// ========================================
// INVESTMENT MANAGEMENT METHODS
// ========================================

// GetInvestmentPortfolio retrieves user's investment portfolio
func (s *StatisticsService) GetInvestmentPortfolio(ctx context.Context, userID uint, req *pb.GetInvestmentPortfolioRequest) (*pb.GetInvestmentPortfolioResponse, error) {

	// Query investments
	var investments []models.Investment
	if err := s.db.Where("user_id = ?", userID).Find(&investments).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch investments")
	}

	// Aggregate by type
	typeMap := make(map[string]*pb.InvestmentTypeData)
	var totalValue, totalInvested, totalGainLoss float64

	for _, inv := range investments {
		totalValue += inv.CurrentValue
		totalInvested += inv.InitialInvestment
		totalGainLoss += inv.GainLoss

		if typeMap[inv.InvestmentType] == nil {
			typeMap[inv.InvestmentType] = &pb.InvestmentTypeData{
				InvestmentType:     convertStringToInvestmentType(inv.InvestmentType),
				TypeName:           formatInvestmentTypeName(inv.InvestmentType),
				CurrentValue:       0,
				GainLoss:           0,
				GainLossPercentage: 0,
				AssetCount:         0,
			}
		}

		typeMap[inv.InvestmentType].CurrentValue += inv.CurrentValue
		typeMap[inv.InvestmentType].GainLoss += inv.GainLoss
		typeMap[inv.InvestmentType].AssetCount++
	}

	// Calculate percentages
	for _, typeData := range typeMap {
		if totalInvested > 0 {
			typeData.GainLossPercentage = (typeData.GainLoss / totalInvested) * 100
		}
	}

	// Convert map to slice
	investmentTypes := make([]*pb.InvestmentTypeData, 0, len(typeMap))
	for _, typeData := range typeMap {
		investmentTypes = append(investmentTypes, typeData)
	}

	// Convert individual investments
	pbInvestments := make([]*pb.Investment, len(investments))
	for i, inv := range investments {
		pbInvestments[i] = convertInvestmentToProto(&inv)
	}

	totalGainLossPercentage := 0.0
	if totalInvested > 0 {
		totalGainLossPercentage = (totalGainLoss / totalInvested) * 100
	}

	return &pb.GetInvestmentPortfolioResponse{
		Portfolio: &pb.InvestmentPortfolio{
			Investments:             investmentTypes,
			TotalValue:              totalValue,
			TotalInvested:           totalInvested,
			TotalGainLoss:           totalGainLoss,
			TotalGainLossPercentage: totalGainLossPercentage,
		},
		IndividualInvestments: pbInvestments,
	}, nil
}

// CreateInvestment creates a new investment
func (s *StatisticsService) CreateInvestment(ctx context.Context, userID uint, req *pb.CreateInvestmentRequest) (*pb.CreateInvestmentResponse, error) {

	// Create investment
	investment := models.Investment{
		UserID:            uint(userID),
		Name:              req.Name,
		InvestmentType:    req.InvestmentType.String(),
		CurrentValue:      req.InitialInvestment, // Initially same as investment
		InitialInvestment: req.InitialInvestment,
		Currency:          req.Currency,
		PurchaseDate:      req.PurchaseDate.AsTime(),
		LastUpdated:       time.Now(),
		TickerSymbol:      req.TickerSymbol,
		Quantity:          int(req.Quantity),
	}

	if err := s.db.Create(&investment).Error; err != nil {
		return nil, fmt.Errorf("failed to create investment")
	}

	return &pb.CreateInvestmentResponse{
		Investment: convertInvestmentToProto(&investment),
		Success:    true,
		Message:    "Investment created successfully",
	}, nil
}

// ========================================
// FINANCIAL GOALS MANAGEMENT METHODS
// ========================================

// GetFinancialGoals retrieves user's financial goals
func (s *StatisticsService) GetFinancialGoals(ctx context.Context, userID uint, req *pb.GetFinancialGoalsRequest) (*pb.GetFinancialGoalsResponse, error) {

	// Query goals
	query := s.db.Where("user_id = ?", userID)

	if req.Status != pb.GoalStatus_GOAL_STATUS_UNSPECIFIED {
		query = query.Where("status = ?", req.Status.String())
	}

	var goals []models.FinancialGoal
	if err := query.Find(&goals).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch financial goals")
	}

	// Convert to proto
	pbGoals := make([]*pb.FinancialGoal, len(goals))
	var totalTarget, totalSaved float64
	activeCount := 0

	for i, goal := range goals {
		pbGoals[i] = convertFinancialGoalToProto(&goal)
		totalTarget += goal.TargetAmount
		totalSaved += goal.CurrentAmount

		if goal.Status == "in_progress" {
			activeCount++
		}
	}

	return &pb.GetFinancialGoalsResponse{
		GoalsList: &pb.FinancialGoalsList{
			Goals:            pbGoals,
			TotalTarget:      totalTarget,
			TotalSaved:       totalSaved,
			ActiveGoalsCount: int32(activeCount),
		},
	}, nil
}

// CreateFinancialGoal creates a new financial goal
func (s *StatisticsService) CreateFinancialGoal(ctx context.Context, userID uint, req *pb.CreateFinancialGoalRequest) (*pb.CreateFinancialGoalResponse, error) {

	// Create goal
	goal := models.FinancialGoal{
		UserID:              uint(userID),
		Name:                req.Name,
		GoalType:            req.GoalType.String(),
		TargetAmount:        req.TargetAmount,
		CurrentAmount:       req.CurrentAmount,
		MonthlyContribution: req.MonthlyContribution,
		Currency:            req.Currency,
		TargetDate:          req.TargetDate.AsTime(),
		Icon:                req.Icon,
		Color:               req.Color,
		Status:              "in_progress",
	}

	if err := s.db.Create(&goal).Error; err != nil {
		return nil, fmt.Errorf("failed to create financial goal")
	}

	return &pb.CreateFinancialGoalResponse{
		Goal:    convertFinancialGoalToProto(&goal),
		Success: true,
		Message: "Financial goal created successfully",
	}, nil
}

// UpdateFinancialGoalProgress updates progress on a financial goal
func (s *StatisticsService) UpdateFinancialGoalProgress(ctx context.Context, userID uint, req *pb.UpdateFinancialGoalProgressRequest) (*pb.UpdateFinancialGoalProgressResponse, error) {

	// Find goal
	var goal models.FinancialGoal
	if err := s.db.Where("id = ? AND user_id = ?", req.GoalId, userID).First(&goal).Error; err != nil {
		return nil, fmt.Errorf("financial goal not found")
	}

	// Update amount
	goal.CurrentAmount += req.AmountToAdd

	if err := s.db.Save(&goal).Error; err != nil {
		return nil, fmt.Errorf("failed to update financial goal")
	}

	return &pb.UpdateFinancialGoalProgressResponse{
		Goal:    convertFinancialGoalToProto(&goal),
		Success: true,
		Message: "Financial goal updated successfully",
	}, nil
}

// ========================================
// SAVINGS GOAL MANAGEMENT METHODS
// ========================================

// GetSavingsGoal retrieves user's primary savings goal
func (s *StatisticsService) GetSavingsGoal(ctx context.Context, userID uint, req *pb.GetSavingsGoalRequest) (*pb.GetSavingsGoalResponse, error) {

	// Query savings goal
	var savingsGoal models.SavingsGoal
	err := s.db.Where("user_id = ?", userID).First(&savingsGoal).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.GetSavingsGoalResponse{
				HasGoal: false,
			}, nil
		}
		return nil, fmt.Errorf("failed to fetch savings goal")
	}

	return &pb.GetSavingsGoalResponse{
		SavingsGoal: convertSavingsGoalToProto(&savingsGoal),
		HasGoal:     true,
	}, nil
}

// CreateOrUpdateSavingsGoal creates or updates user's savings goal
func (s *StatisticsService) CreateOrUpdateSavingsGoal(ctx context.Context, userID uint, req *pb.CreateOrUpdateSavingsGoalRequest) (*pb.CreateOrUpdateSavingsGoalResponse, error) {

	// Check if goal exists
	var savingsGoal models.SavingsGoal
	err := s.db.Where("user_id = ?", userID).First(&savingsGoal).Error

	if err == gorm.ErrRecordNotFound {
		// Create new goal
		savingsGoal = models.SavingsGoal{
			UserID:        uint(userID),
			Name:          req.Name,
			TargetAmount:  req.TargetAmount,
			CurrentAmount: req.CurrentAmount,
			TargetDate:    req.TargetDate.AsTime(),
		}

		if err := s.db.Create(&savingsGoal).Error; err != nil {
			return nil, fmt.Errorf("failed to create savings goal")
		}
		// Update existing goal
		savingsGoal.Name = req.Name
		savingsGoal.TargetAmount = req.TargetAmount
		savingsGoal.CurrentAmount = req.CurrentAmount
		savingsGoal.TargetDate = req.TargetDate.AsTime()

		if err := s.db.Save(&savingsGoal).Error; err != nil {
			return nil, fmt.Errorf("failed to update savings goal")
		}
	}

	return &pb.CreateOrUpdateSavingsGoalResponse{
		SavingsGoal: convertSavingsGoalToProto(&savingsGoal),
		Success:     true,
		Message:     "Savings goal saved successfully",
	}, nil
}

// ========================================
// RECURRING BILLS MANAGEMENT METHODS
// ========================================

// GetUpcomingBills retrieves user's upcoming bills
func (s *StatisticsService) GetUpcomingBills(ctx context.Context, userID uint, req *pb.GetUpcomingBillsRequest) (*pb.GetUpcomingBillsResponse, error) {

	// Calculate date range
	endDate := time.Now().AddDate(0, 0, int(req.DaysAhead))

	// Query bills
	var bills []models.RecurringBill
	err := s.db.Where("user_id = ? AND next_due_date <= ? AND status IN (?)",
		userID, endDate, []string{"upcoming", "overdue"}).
		Order("next_due_date ASC").
		Find(&bills).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch upcoming bills: %w", err)
	}

	// Convert to proto
	pbBills := make([]*pb.RecurringBill, len(bills))
	var totalUpcoming float64

	for i, bill := range bills {
		pbBills[i] = convertRecurringBillToProto(&bill)
		if bill.Status == "upcoming" {
			totalUpcoming += bill.Amount
		}
	}

	return &pb.GetUpcomingBillsResponse{
		BillsList: &pb.UpcomingBillsList{
			Bills:         pbBills,
			TotalUpcoming: totalUpcoming,
			BillsCount:    int32(len(bills)),
		},
	}, nil
}

// CreateRecurringBill creates a new recurring bill
func (s *StatisticsService) CreateRecurringBill(ctx context.Context, userID uint, req *pb.CreateRecurringBillRequest) (*pb.CreateRecurringBillResponse, error) {

	// Create bill
	bill := models.RecurringBill{
		UserID:            uint(userID),
		Name:              req.Name,
		Amount:            req.Amount,
		Currency:          req.Currency,
		Category:          req.Category.String(),
		RecurrencePattern: req.RecurrencePattern,
		NextDueDate:       req.NextDueDate.AsTime(),
		Merchant:          req.Merchant,
		Icon:              req.Icon,
		AutoPayEnabled:    req.AutoPayEnabled,
		Status:            "upcoming",
	}

	if err := s.db.Create(&bill).Error; err != nil {
		return nil, fmt.Errorf("failed to create recurring bill")
	}

	return &pb.CreateRecurringBillResponse{
		Bill:    convertRecurringBillToProto(&bill),
		Success: true,
		Message: "Recurring bill created successfully",
	}, nil
}

// ========================================
// TRACKED TRANSACTION METHODS
// ========================================

// GetTrackedIncome retrieves total tracked income for a period
func (s *StatisticsService) GetTrackedIncome(ctx context.Context, userID uint, startDate, endDate time.Time) (float64, error) {
	return s.txTracker.GetTotalIncome(ctx, userID, startDate, endDate)
}

// GetTrackedExpenditure retrieves total tracked expenditure for a period
func (s *StatisticsService) GetTrackedExpenditure(ctx context.Context, userID uint, startDate, endDate time.Time) (float64, error) {
	return s.txTracker.GetTotalExpenditure(ctx, userID, startDate, endDate)
}

// GetTrackedIncomeBreakdown retrieves income breakdown by source type
func (s *StatisticsService) GetTrackedIncomeBreakdown(ctx context.Context, userID uint, startDate, endDate time.Time) (map[string]float64, error) {
	return s.txTracker.GetIncomeBreakdownBySource(ctx, userID, startDate, endDate)
}

// GetTrackedExpenditureBreakdown retrieves expenditure breakdown by expense type
func (s *StatisticsService) GetTrackedExpenditureBreakdown(ctx context.Context, userID uint, startDate, endDate time.Time) (map[string]float64, error) {
	return s.txTracker.GetExpenditureBreakdownByType(ctx, userID, startDate, endDate)
}

// GetTrackedIncomeTransactions retrieves tracked income transactions for a period
func (s *StatisticsService) GetTrackedIncomeTransactions(ctx context.Context, userID uint, startDate, endDate time.Time, limit int) ([]models.IncomeTransaction, error) {
	return s.txTracker.GetIncomeTransactions(ctx, userID, startDate, endDate, limit)
}

// GetTrackedExpenditureTransactions retrieves tracked expenditure transactions for a period
func (s *StatisticsService) GetTrackedExpenditureTransactions(ctx context.Context, userID uint, startDate, endDate time.Time, limit int) ([]models.ExpenditureTransaction, error) {
	return s.txTracker.GetExpenditureTransactions(ctx, userID, startDate, endDate, limit)
}

// GetComprehensiveFinancialSummary combines manual and tracked data for comprehensive view
func (s *StatisticsService) GetComprehensiveFinancialSummary(ctx context.Context, userID uint, startDate, endDate time.Time) (*ComprehensiveFinancialSummary, error) {
	// Get manual expenses total
	var manualExpenses float64
	var expenseResult struct {
		Total float64
	}
	if err := s.db.Model(&models.Expense{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Select("COALESCE(SUM(amount), 0) as total").
		Scan(&expenseResult).Error; err != nil {
		return nil, fmt.Errorf("failed to calculate manual expenses: %w", err)
	}
	manualExpenses = expenseResult.Total

	// Get manual income total
	var manualIncome float64
	var incomeResult struct {
		Total float64
	}
	if err := s.db.Model(&models.IncomeSource{}).
		Where("user_id = ? AND is_active = ? AND created_at >= ? AND created_at <= ?", userID, true, startDate, endDate).
		Select("COALESCE(SUM(amount), 0) as total").
		Scan(&incomeResult).Error; err != nil {
		return nil, fmt.Errorf("failed to calculate manual income: %w", err)
	}
	manualIncome = incomeResult.Total

	// Get tracked income and expenditure
	trackedIncome, err := s.txTracker.GetTotalIncome(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get tracked income: %w", err)
	}

	trackedExpenditure, err := s.txTracker.GetTotalExpenditure(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get tracked expenditure: %w", err)
	}

	// Get breakdowns
	incomeBreakdown, err := s.txTracker.GetIncomeBreakdownBySource(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get income breakdown: %w", err)
	}

	expenditureBreakdown, err := s.txTracker.GetExpenditureBreakdownByType(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get expenditure breakdown: %w", err)
	}

	// Calculate totals
	totalIncome := manualIncome + trackedIncome
	totalExpenditure := manualExpenses + trackedExpenditure
	netIncome := totalIncome - totalExpenditure

	summary := &ComprehensiveFinancialSummary{
		Period: ComprehensivePeriod{
			StartDate: startDate,
			EndDate:   endDate,
		},
		Income: ComprehensiveIncomeData{
			ManualIncome:    manualIncome,
			TrackedIncome:   trackedIncome,
			TotalIncome:     totalIncome,
			IncomeBreakdown: incomeBreakdown,
		},
		Expenditure: ComprehensiveExpenditureData{
			ManualExpenses:       manualExpenses,
			TrackedExpenditure:   trackedExpenditure,
			TotalExpenditure:     totalExpenditure,
			ExpenditureBreakdown: expenditureBreakdown,
		},
		NetIncome:   netIncome,
		SavingsRate: 0,
	}

	// Calculate savings rate
	if totalIncome > 0 {
		summary.SavingsRate = (netIncome / totalIncome) * 100
	}

	return summary, nil
}

// ComprehensiveFinancialSummary combines manual and tracked financial data
type ComprehensiveFinancialSummary struct {
	Period      ComprehensivePeriod
	Income      ComprehensiveIncomeData
	Expenditure ComprehensiveExpenditureData
	NetIncome   float64
	SavingsRate float64
}

type ComprehensivePeriod struct {
	StartDate time.Time
	EndDate   time.Time
}

type ComprehensiveIncomeData struct {
	ManualIncome    float64
	TrackedIncome   float64
	TotalIncome     float64
	IncomeBreakdown map[string]float64 // breakdown by source type
}

type ComprehensiveExpenditureData struct {
	ManualExpenses       float64
	TrackedExpenditure   float64
	TotalExpenditure     float64
	ExpenditureBreakdown map[string]float64 // breakdown by expense type
}

// ========================================
// HELPER CONVERSION FUNCTIONS
// ========================================

func convertIncomeSourceToProto(source *models.IncomeSource) *pb.IncomeSource {
	pbSource := &pb.IncomeSource{
		Id:                source.ID.String(),
		UserId:            uint64(source.UserID),
		Name:              source.Name,
		Amount:            source.Amount,
		Currency:          source.Currency,
		Category:          convertStringToIncomeCategory(source.Category),
		IsRecurring:       source.IsRecurring,
		RecurrencePattern: source.RecurrencePattern,
		IsActive:          source.IsActive,
		CreatedAt:         timestamppb.New(source.CreatedAt),
		UpdatedAt:         timestamppb.New(source.UpdatedAt),
	}

	if source.LastReceived != nil {
		pbSource.LastReceived = timestamppb.New(*source.LastReceived)
	}

	if source.NextExpected != nil {
		pbSource.NextExpected = timestamppb.New(*source.NextExpected)
	}

	return pbSource
}

func convertInvestmentToProto(inv *models.Investment) *pb.Investment {
	return &pb.Investment{
		Id:                 inv.ID.String(),
		UserId:             uint64(inv.UserID),
		Name:               inv.Name,
		InvestmentType:     convertStringToInvestmentType(inv.InvestmentType),
		CurrentValue:       inv.CurrentValue,
		InitialInvestment:  inv.InitialInvestment,
		GainLoss:           inv.GainLoss,
		GainLossPercentage: inv.GainLossPercentage,
		Currency:           inv.Currency,
		PurchaseDate:       timestamppb.New(inv.PurchaseDate),
		LastUpdated:        timestamppb.New(inv.LastUpdated),
		TickerSymbol:       inv.TickerSymbol,
		Quantity:           int32(inv.Quantity),
		CurrentPrice:       inv.CurrentPrice,
	}
}

func convertFinancialGoalToProto(goal *models.FinancialGoal) *pb.FinancialGoal {
	return &pb.FinancialGoal{
		Id:                  goal.ID.String(),
		UserId:              uint64(goal.UserID),
		Name:                goal.Name,
		GoalType:            convertStringToGoalType(goal.GoalType),
		TargetAmount:        goal.TargetAmount,
		CurrentAmount:       goal.CurrentAmount,
		MonthlyContribution: goal.MonthlyContribution,
		Currency:            goal.Currency,
		TargetDate:          timestamppb.New(goal.TargetDate),
		Status:              convertStringToGoalStatus(goal.Status),
		PercentageComplete:  goal.PercentageComplete,
		MonthsRemaining:     int32(goal.MonthsRemaining),
		Icon:                goal.Icon,
		Color:               goal.Color,
		CreatedAt:           timestamppb.New(goal.CreatedAt),
		UpdatedAt:           timestamppb.New(goal.UpdatedAt),
	}
}

func convertSavingsGoalToProto(goal *models.SavingsGoal) *pb.SavingsGoal {
	return &pb.SavingsGoal{
		Id:                 goal.ID.String(),
		UserId:             uint64(goal.UserID),
		Name:               goal.Name,
		TargetAmount:       goal.TargetAmount,
		CurrentAmount:      goal.CurrentAmount,
		PercentageComplete: goal.PercentageComplete,
		TargetDate:         timestamppb.New(goal.TargetDate),
		CreatedAt:          timestamppb.New(goal.CreatedAt),
		UpdatedAt:          timestamppb.New(goal.UpdatedAt),
	}
}

func convertRecurringBillToProto(bill *models.RecurringBill) *pb.RecurringBill {
	pbBill := &pb.RecurringBill{
		Id:                bill.ID.String(),
		UserId:            uint64(bill.UserID),
		Name:              bill.Name,
		Amount:            bill.Amount,
		Currency:          bill.Currency,
		Category:          pb.ExpenseCategory(pb.ExpenseCategory_value[bill.Category]),
		RecurrencePattern: bill.RecurrencePattern,
		NextDueDate:       timestamppb.New(bill.NextDueDate),
		Status:            convertStringToBillStatus(bill.Status),
		DaysUntilDue:      int32(bill.DaysUntilDue),
		Merchant:          bill.Merchant,
		Icon:              bill.Icon,
		AutoPayEnabled:    bill.AutoPayEnabled,
		CreatedAt:         timestamppb.New(bill.CreatedAt),
		UpdatedAt:         timestamppb.New(bill.UpdatedAt),
	}

	if bill.LastPaidDate != nil {
		pbBill.LastPaidDate = timestamppb.New(*bill.LastPaidDate)
	}

	return pbBill
}

// Enum conversion helpers
func convertStringToIncomeCategory(s string) pb.IncomeCategory {
	if val, ok := pb.IncomeCategory_value[s]; ok {
		return pb.IncomeCategory(val)
	}
	return pb.IncomeCategory_INCOME_CATEGORY_UNSPECIFIED
}

func convertStringToInvestmentType(s string) pb.InvestmentType {
	if val, ok := pb.InvestmentType_value[s]; ok {
		return pb.InvestmentType(val)
	}
	return pb.InvestmentType_INVESTMENT_TYPE_UNSPECIFIED
}

func convertStringToGoalType(s string) pb.GoalType {
	if val, ok := pb.GoalType_value[s]; ok {
		return pb.GoalType(val)
	}
	return pb.GoalType_GOAL_TYPE_UNSPECIFIED
}

func convertStringToGoalStatus(s string) pb.GoalStatus {
	statusMap := map[string]pb.GoalStatus{
		"in_progress": pb.GoalStatus_GOAL_STATUS_IN_PROGRESS,
		"completed":   pb.GoalStatus_GOAL_STATUS_COMPLETED,
		"cancelled":   pb.GoalStatus_GOAL_STATUS_CANCELLED,
		"paused":      pb.GoalStatus_GOAL_STATUS_PAUSED,
	}
	if val, ok := statusMap[s]; ok {
		return val
	}
	return pb.GoalStatus_GOAL_STATUS_UNSPECIFIED
}

func convertStringToBillStatus(s string) pb.BillStatus {
	statusMap := map[string]pb.BillStatus{
		"upcoming":  pb.BillStatus_BILL_STATUS_UPCOMING,
		"paid":      pb.BillStatus_BILL_STATUS_PAID,
		"overdue":   pb.BillStatus_BILL_STATUS_OVERDUE,
		"cancelled": pb.BillStatus_BILL_STATUS_CANCELLED,
	}
	if val, ok := statusMap[s]; ok {
		return val
	}
	return pb.BillStatus_BILL_STATUS_UNSPECIFIED
}

func formatIncomeCategoryName(category string) string {
	nameMap := map[string]string{
		"INCOME_CATEGORY_SALARY":      "Salary",
		"INCOME_CATEGORY_FREELANCE":   "Freelance",
		"INCOME_CATEGORY_INVESTMENTS": "Investments",
		"INCOME_CATEGORY_RENTAL":      "Rental Income",
		"INCOME_CATEGORY_BUSINESS":    "Business",
		"INCOME_CATEGORY_DIVIDEND":    "Dividends",
		"INCOME_CATEGORY_INTEREST":    "Interest",
		"INCOME_CATEGORY_GIFT":        "Gifts",
		"INCOME_CATEGORY_OTHER":       "Other",
	}
	if name, ok := nameMap[category]; ok {
		return name
	}
	return "Other"
}

func formatInvestmentTypeName(invType string) string {
	nameMap := map[string]string{
		"INVESTMENT_TYPE_STOCKS":       "Stocks",
		"INVESTMENT_TYPE_CRYPTO":       "Cryptocurrency",
		"INVESTMENT_TYPE_MUTUAL_FUNDS": "Mutual Funds",
		"INVESTMENT_TYPE_BONDS":        "Bonds",
		"INVESTMENT_TYPE_ETF":          "ETF",
		"INVESTMENT_TYPE_REAL_ESTATE":  "Real Estate",
		"INVESTMENT_TYPE_COMMODITIES":  "Commodities",
		"INVESTMENT_TYPE_OTHER":        "Other",
	}
	if name, ok := nameMap[invType]; ok {
		return name
	}
	return "Other"
}
