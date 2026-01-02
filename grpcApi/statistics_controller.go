package grpcApi

import (
	"context"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type StatisticsController struct {
	pb.UnimplementedStatisticsServiceServer
	service      *services.StatisticsService
	aiService    *services.AIStatisticsService
	userService  services.IUserService
}

func NewStatisticsController(db *gorm.DB, userService services.IUserService, aiService *services.AIStatisticsService) *StatisticsController {
	return &StatisticsController{
		service:     services.NewStatisticsService(db),
		aiService:   aiService,
		userService: userService,
	}
}

// ========================================
// EXPENSE MANAGEMENT
// ========================================

func (c *StatisticsController) CreateExpense(ctx context.Context, req *pb.CreateExpenseRequest) (*pb.CreateExpenseResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	expense, err := c.service.CreateExpense(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.CreateExpenseResponse{
		Expense: expense,
		Success: true,
		Message: "Expense created successfully",
	}, nil
}

func (c *StatisticsController) GetExpenses(ctx context.Context, req *pb.GetExpensesRequest) (*pb.GetExpensesResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	expenses, pagination, totalAmount, totalCount, err := c.service.GetExpenses(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetExpensesResponse{
		Expenses:    expenses,
		Pagination:  pagination,
		TotalAmount: totalAmount,
		TotalCount:  totalCount,
	}, nil
}

func (c *StatisticsController) GetExpenseById(ctx context.Context, req *pb.GetExpenseByIdRequest) (*pb.GetExpenseByIdResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	expense, err := c.service.GetExpenseById(ctx, user.ID, req.ExpenseId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &pb.GetExpenseByIdResponse{
		Expense: expense,
	}, nil
}

func (c *StatisticsController) UpdateExpense(ctx context.Context, req *pb.UpdateExpenseRequest) (*pb.UpdateExpenseResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	expense, err := c.service.UpdateExpense(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.UpdateExpenseResponse{
		Expense: expense,
		Success: true,
		Message: "Expense updated successfully",
	}, nil
}

func (c *StatisticsController) DeleteExpense(ctx context.Context, req *pb.DeleteExpenseRequest) (*pb.DeleteExpenseResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	err = c.service.DeleteExpense(ctx, user.ID, req.ExpenseId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.DeleteExpenseResponse{
		Success: true,
		Message: "Expense deleted successfully",
	}, nil
}

// ========================================
// BUDGET MANAGEMENT
// ========================================

func (c *StatisticsController) CreateBudget(ctx context.Context, req *pb.CreateBudgetRequest) (*pb.CreateBudgetResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	budget, err := c.service.CreateBudget(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.CreateBudgetResponse{
		Budget:  budget,
		Success: true,
		Message: "Budget created successfully",
	}, nil
}

func (c *StatisticsController) GetBudgets(ctx context.Context, req *pb.GetBudgetsRequest) (*pb.GetBudgetsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	budgets, pagination, totalBudgetAmount, totalSpentAmount, err := c.service.GetBudgets(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetBudgetsResponse{
		Budgets:           budgets,
		Pagination:        pagination,
		TotalBudgetAmount: totalBudgetAmount,
		TotalSpentAmount:  totalSpentAmount,
	}, nil
}

func (c *StatisticsController) GetBudgetById(ctx context.Context, req *pb.GetBudgetByIdRequest) (*pb.GetBudgetByIdResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	budget, recentExpenses, err := c.service.GetBudgetById(ctx, user.ID, req.BudgetId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &pb.GetBudgetByIdResponse{
		Budget:         budget,
		RecentExpenses: recentExpenses,
	}, nil
}

func (c *StatisticsController) UpdateBudget(ctx context.Context, req *pb.UpdateBudgetRequest) (*pb.UpdateBudgetResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	budget, err := c.service.UpdateBudget(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.UpdateBudgetResponse{
		Budget:  budget,
		Success: true,
		Message: "Budget updated successfully",
	}, nil
}

func (c *StatisticsController) DeleteBudget(ctx context.Context, req *pb.DeleteBudgetRequest) (*pb.DeleteBudgetResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	err = c.service.DeleteBudget(ctx, user.ID, req.BudgetId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.DeleteBudgetResponse{
		Success: true,
		Message: "Budget deleted successfully",
	}, nil
}

// ========================================
// ANALYTICS & REPORTS
// ========================================

func (c *StatisticsController) GetSpendingAnalytics(ctx context.Context, req *pb.GetSpendingAnalyticsRequest) (*pb.GetSpendingAnalyticsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	analytics, err := c.service.GetSpendingAnalytics(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetSpendingAnalyticsResponse{
		Analytics: analytics,
	}, nil
}

func (c *StatisticsController) GetCategoryBreakdown(ctx context.Context, req *pb.GetCategoryBreakdownRequest) (*pb.GetCategoryBreakdownResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	categories, totalSpent, err := c.service.GetCategoryBreakdown(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetCategoryBreakdownResponse{
		Categories: categories,
		TotalSpent: totalSpent,
	}, nil
}

func (c *StatisticsController) GetBudgetProgress(ctx context.Context, req *pb.GetBudgetProgressRequest) (*pb.GetBudgetProgressResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	budgets, totalBudget, totalSpent, overallPercentage, err := c.service.GetBudgetProgress(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetBudgetProgressResponse{
		Budgets:           budgets,
		TotalBudget:       totalBudget,
		TotalSpent:        totalSpent,
		OverallPercentage: overallPercentage,
	}, nil
}

func (c *StatisticsController) GetSpendingTrends(ctx context.Context, req *pb.GetSpendingTrendsRequest) (*pb.GetSpendingTrendsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	trends, err := c.service.GetSpendingTrends(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetSpendingTrendsResponse{
		Trends: trends,
	}, nil
}

// ========================================
// NOTIFICATIONS / ALERTS
// ========================================

func (c *StatisticsController) GetBudgetAlerts(ctx context.Context, req *pb.GetBudgetAlertsRequest) (*pb.GetBudgetAlertsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	alerts, unreadCount, err := c.service.GetBudgetAlerts(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetBudgetAlertsResponse{
		Alerts:      alerts,
		UnreadCount: unreadCount,
	}, nil
}

func (c *StatisticsController) MarkAlertAsRead(ctx context.Context, req *pb.MarkAlertAsReadRequest) (*pb.MarkAlertAsReadResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	err = c.service.MarkAlertAsRead(ctx, user.ID, req.AlertId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.MarkAlertAsReadResponse{
		Success: true,
	}, nil
}

// ========================================
// AI-POWERED STATISTICS
// ========================================

func (c *StatisticsController) GetAISpendingInsights(ctx context.Context, req *pb.GetAISpendingInsightsRequest) (*pb.GetAISpendingInsightsResponse, error) {
	user, accessToken, err := getUserAndTokenFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.aiService.GetAISpendingInsights(ctx, user.ID, accessToken, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) GetAIBudgetingRecommendations(ctx context.Context, req *pb.GetAIBudgetingRecommendationsRequest) (*pb.GetAIBudgetingRecommendationsResponse, error) {
	user, accessToken, err := getUserAndTokenFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.aiService.GetAIBudgetingRecommendations(ctx, user.ID, accessToken, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) AutoCategorizeExpense(ctx context.Context, req *pb.AutoCategorizeExpenseRequest) (*pb.AutoCategorizeExpenseResponse, error) {
	user, accessToken, err := getUserAndTokenFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.aiService.AutoCategorizeExpense(ctx, user.ID, accessToken, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) GetAIFinancialAdvice(ctx context.Context, req *pb.GetAIFinancialAdviceRequest) (*pb.GetAIFinancialAdviceResponse, error) {
	user, accessToken, err := getUserAndTokenFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.aiService.GetAIFinancialAdvice(ctx, user.ID, accessToken, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

// ========================================
// INCOME MANAGEMENT
// ========================================

func (c *StatisticsController) GetIncomeSources(ctx context.Context, req *pb.GetIncomeSourcesRequest) (*pb.GetIncomeSourcesResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.GetIncomeSources(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) GetIncomeBreakdown(ctx context.Context, req *pb.GetIncomeBreakdownRequest) (*pb.GetIncomeBreakdownResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.GetIncomeBreakdown(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) CreateIncomeSource(ctx context.Context, req *pb.CreateIncomeSourceRequest) (*pb.CreateIncomeSourceResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.CreateIncomeSource(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

// ========================================
// INVESTMENT MANAGEMENT
// ========================================

func (c *StatisticsController) GetInvestmentPortfolio(ctx context.Context, req *pb.GetInvestmentPortfolioRequest) (*pb.GetInvestmentPortfolioResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.GetInvestmentPortfolio(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) CreateInvestment(ctx context.Context, req *pb.CreateInvestmentRequest) (*pb.CreateInvestmentResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.CreateInvestment(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

// ========================================
// FINANCIAL GOALS MANAGEMENT
// ========================================

func (c *StatisticsController) GetFinancialGoals(ctx context.Context, req *pb.GetFinancialGoalsRequest) (*pb.GetFinancialGoalsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.GetFinancialGoals(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) CreateFinancialGoal(ctx context.Context, req *pb.CreateFinancialGoalRequest) (*pb.CreateFinancialGoalResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.CreateFinancialGoal(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) UpdateFinancialGoalProgress(ctx context.Context, req *pb.UpdateFinancialGoalProgressRequest) (*pb.UpdateFinancialGoalProgressResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.UpdateFinancialGoalProgress(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

// ========================================
// SAVINGS GOALS MANAGEMENT
// ========================================

func (c *StatisticsController) GetSavingsGoal(ctx context.Context, req *pb.GetSavingsGoalRequest) (*pb.GetSavingsGoalResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.GetSavingsGoal(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) CreateOrUpdateSavingsGoal(ctx context.Context, req *pb.CreateOrUpdateSavingsGoalRequest) (*pb.CreateOrUpdateSavingsGoalResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.CreateOrUpdateSavingsGoal(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

// ========================================
// RECURRING BILLS MANAGEMENT
// ========================================

func (c *StatisticsController) GetUpcomingBills(ctx context.Context, req *pb.GetUpcomingBillsRequest) (*pb.GetUpcomingBillsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.GetUpcomingBills(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

func (c *StatisticsController) CreateRecurringBill(ctx context.Context, req *pb.CreateRecurringBillRequest) (*pb.CreateRecurringBillResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	response, err := c.service.CreateRecurringBill(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return response, nil
}

// ========================================
// TRACKED TRANSACTIONS (AUTOMATIC)
// ========================================

func (c *StatisticsController) GetTrackedIncome(ctx context.Context, req *pb.GetTrackedIncomeRequest) (*pb.GetTrackedIncomeResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	startDate := req.StartDate.AsTime()
	endDate := req.EndDate.AsTime()

	totalIncome, err := c.service.GetTrackedIncome(ctx, user.ID, startDate, endDate)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetTrackedIncomeResponse{
		TotalIncome: totalIncome,
		Success:     true,
	}, nil
}

func (c *StatisticsController) GetTrackedExpenditure(ctx context.Context, req *pb.GetTrackedExpenditureRequest) (*pb.GetTrackedExpenditureResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	startDate := req.StartDate.AsTime()
	endDate := req.EndDate.AsTime()

	totalExpenditure, err := c.service.GetTrackedExpenditure(ctx, user.ID, startDate, endDate)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetTrackedExpenditureResponse{
		TotalExpenditure: totalExpenditure,
		Success:          true,
	}, nil
}

func (c *StatisticsController) GetTrackedIncomeBreakdown(ctx context.Context, req *pb.GetTrackedIncomeBreakdownRequest) (*pb.GetTrackedIncomeBreakdownResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	startDate := req.StartDate.AsTime()
	endDate := req.EndDate.AsTime()

	breakdown, err := c.service.GetTrackedIncomeBreakdown(ctx, user.ID, startDate, endDate)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Calculate total
	var total float64
	for _, amount := range breakdown {
		total += amount
	}

	return &pb.GetTrackedIncomeBreakdownResponse{
		BreakdownBySource: breakdown,
		TotalIncome:       total,
		Success:           true,
	}, nil
}

func (c *StatisticsController) GetTrackedExpenditureBreakdown(ctx context.Context, req *pb.GetTrackedExpenditureBreakdownRequest) (*pb.GetTrackedExpenditureBreakdownResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	startDate := req.StartDate.AsTime()
	endDate := req.EndDate.AsTime()

	breakdown, err := c.service.GetTrackedExpenditureBreakdown(ctx, user.ID, startDate, endDate)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Calculate total
	var total float64
	for _, amount := range breakdown {
		total += amount
	}

	return &pb.GetTrackedExpenditureBreakdownResponse{
		BreakdownByType:  breakdown,
		TotalExpenditure: total,
		Success:          true,
	}, nil
}

func (c *StatisticsController) GetTrackedIncomeTransactions(ctx context.Context, req *pb.GetTrackedIncomeTransactionsRequest) (*pb.GetTrackedIncomeTransactionsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	startDate := req.StartDate.AsTime()
	endDate := req.EndDate.AsTime()
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100 // default limit
	}

	transactions, err := c.service.GetTrackedIncomeTransactions(ctx, user.ID, startDate, endDate, limit)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to proto messages
	var pbTransactions []*pb.TrackedIncomeTransaction
	for _, tx := range transactions {
		var senderID uint64
		if tx.SenderID != nil {
			senderID = uint64(*tx.SenderID)
		}
		pbTransactions = append(pbTransactions, &pb.TrackedIncomeTransaction{
			Id:              tx.ID.String(),
			UserId:          uint64(tx.UserID),
			Amount:          tx.Amount,
			Currency:        tx.Currency,
			SourceType:      tx.SourceType,
			SourceId:        tx.SourceID,
			SourceReference: tx.SourceReference,
			Category:        tx.Category,
			Description:     tx.Description,
			SenderId:        senderID,
			SenderName:      tx.SenderName,
			TransactionDate: timestamppb.New(tx.TransactionDate),
		})
	}

	return &pb.GetTrackedIncomeTransactionsResponse{
		Transactions: pbTransactions,
	}, nil
}

func (c *StatisticsController) GetTrackedExpenditureTransactions(ctx context.Context, req *pb.GetTrackedExpenditureTransactionsRequest) (*pb.GetTrackedExpenditureTransactionsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	startDate := req.StartDate.AsTime()
	endDate := req.EndDate.AsTime()
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100 // default limit
	}

	transactions, err := c.service.GetTrackedExpenditureTransactions(ctx, user.ID, startDate, endDate, limit)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to proto messages
	var pbTransactions []*pb.TrackedExpenditureTransaction
	for _, tx := range transactions {
		var recipientID uint64
		if tx.RecipientID != nil {
			recipientID = uint64(*tx.RecipientID)
		}
		pbTransactions = append(pbTransactions, &pb.TrackedExpenditureTransaction{
			Id:               tx.ID.String(),
			UserId:           uint64(tx.UserID),
			Amount:           tx.Amount,
			Currency:         tx.Currency,
			ExpenseType:      tx.ExpenseType,
			ExpenseId:        tx.ExpenseID,
			ExpenseReference: tx.ExpenseReference,
			Category:         tx.Category,
			RecipientId:      recipientID,
			RecipientName:    tx.RecipientName,
			Merchant:         tx.Merchant,
			Description:      tx.Description,
			TransactionDate:  timestamppb.New(tx.TransactionDate),
		})
	}

	return &pb.GetTrackedExpenditureTransactionsResponse{
		Transactions: pbTransactions,
	}, nil
}

func (c *StatisticsController) GetComprehensiveFinancialSummary(ctx context.Context, req *pb.GetComprehensiveFinancialSummaryRequest) (*pb.GetComprehensiveFinancialSummaryResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	startDate := req.StartDate.AsTime()
	endDate := req.EndDate.AsTime()

	summary, err := c.service.GetComprehensiveFinancialSummary(ctx, user.ID, startDate, endDate)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetComprehensiveFinancialSummaryResponse{
		Summary: &pb.ComprehensiveFinancialSummary{
			Period: &pb.ComprehensivePeriod{
				StartDate: timestamppb.New(summary.Period.StartDate),
				EndDate:   timestamppb.New(summary.Period.EndDate),
			},
			Income: &pb.ComprehensiveIncomeData{
				ManualIncome:  summary.Income.ManualIncome,
				TrackedIncome: summary.Income.TrackedIncome,
				TotalIncome:   summary.Income.TotalIncome,
			},
			Expenditure: &pb.ComprehensiveExpenditureData{
				ManualExpenses:      summary.Expenditure.ManualExpenses,
				TrackedExpenditure:  summary.Expenditure.TrackedExpenditure,
				TotalExpenditure:    summary.Expenditure.TotalExpenditure,
				ExpenditureBreakdown: summary.Expenditure.ExpenditureBreakdown,
			},
			NetIncome:   summary.NetIncome,
			SavingsRate: summary.SavingsRate,
		},
	}, nil
}
