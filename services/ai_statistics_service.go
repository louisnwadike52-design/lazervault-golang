package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"lazervaultGo/configs"
	"lazervaultGo/pb"
	"net/http"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type AIStatisticsService struct {
	db             *gorm.DB
	config         *configs.Config
	statsService   *StatisticsService
	aiChatService  *AIChatService
}

func NewAIStatisticsService(db *gorm.DB, cfg *configs.Config, statsService *StatisticsService, aiChatService *AIChatService) *AIStatisticsService {
	return &AIStatisticsService{
		db:            db,
		config:        cfg,
		statsService:  statsService,
		aiChatService: aiChatService,
	}
}

// ========================================
// AI SPENDING INSIGHTS
// ========================================

func (s *AIStatisticsService) GetAISpendingInsights(ctx context.Context, userID uint, accessToken string, req *pb.GetAISpendingInsightsRequest) (*pb.GetAISpendingInsightsResponse, error) {
	// Get spending analytics from statistics service
	startDate := time.Now().AddDate(0, -1, 0)
	if req.StartDate != nil {
		startDate = req.StartDate.AsTime()
	}

	endDate := time.Now()
	if req.EndDate != nil {
		endDate = req.EndDate.AsTime()
	}

	// Fetch user's financial data
	spendingReq := &pb.GetSpendingAnalyticsRequest{
		Period:    "monthly",
		StartDate: timestamppb.New(startDate),
		EndDate:   timestamppb.New(endDate),
	}

	analyticsResp, err := s.statsService.GetSpendingAnalytics(ctx, userID, spendingReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get spending analytics: %w", err)
	}

	// Get expenses for detailed analysis
	expensesReq := &pb.GetExpensesRequest{
		Page:      1,
		PerPage:   100,
		StartDate: timestamppb.New(startDate),
		EndDate:   timestamppb.New(endDate),
	}

	expenses, _, _, _, err := s.statsService.GetExpenses(ctx, userID, expensesReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get expenses: %w", err)
	}

	// Get budgets
	budgetsReq := &pb.GetBudgetsRequest{
		Page:    1,
		PerPage: 50,
	}

	budgets, _, _, _, err := s.statsService.GetBudgets(ctx, userID, budgetsReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get budgets: %w", err)
	}

	// Prepare context for AI
	contextData := map[string]interface{}{
		"time_period": map[string]string{
			"start": startDate.Format("2006-01-02"),
			"end":   endDate.Format("2006-01-02"),
		},
		"summary": map[string]interface{}{
			"total_spent":         analyticsResp.TotalSpent,
			"total_budget":        analyticsResp.TotalBudget,
			"remaining_budget":    analyticsResp.RemainingBudget,
			"transaction_count":   analyticsResp.TransactionCount,
			"average_transaction": analyticsResp.AverageTransaction,
			"savings_rate":        analyticsResp.SavingsRate,
		},
		"top_spending_categories": s.formatCategoryBreakdown(analyticsResp.CategoryBreakdown),
		"expense_count":           len(expenses),
		"budget_count":            len(budgets),
		"focus_area":              req.FocusArea,
	}

	// Call AI service for insights
	aiQuery := s.buildInsightsQuery(contextData, req.FocusArea)
	aiResponse, err := s.callAIService(ctx, userID, accessToken, aiQuery, contextData)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI insights: %w", err)
	}

	// Parse AI response and structure it
	insights, recommendations, anomalies := s.parseAIInsightsResponse(aiResponse, expenses, budgets)

	return &pb.GetAISpendingInsightsResponse{
		Success:         true,
		Summary:         aiResponse,
		Insights:        insights,
		Recommendations: recommendations,
		Anomalies:       anomalies,
	}, nil
}

// ========================================
// AI BUDGETING RECOMMENDATIONS
// ========================================

func (s *AIStatisticsService) GetAIBudgetingRecommendations(ctx context.Context, userID uint, accessToken string, req *pb.GetAIBudgetingRecommendationsRequest) (*pb.GetAIBudgetingRecommendationsResponse, error) {
	// Get current budgets and spending patterns
	budgetsReq := &pb.GetBudgetsRequest{
		Page:    1,
		PerPage: 50,
	}

	budgets, _, totalBudget, totalSpent, err := s.statsService.GetBudgets(ctx, userID, budgetsReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get budgets: %w", err)
	}

	// Get category breakdown for the last 3 months
	startDate := time.Now().AddDate(0, -3, 0)
	categoryReq := &pb.GetCategoryBreakdownRequest{
		StartDate: timestamppb.New(startDate),
		EndDate:   timestamppb.New(time.Now()),
	}

	categories, totalCategorySpent, err := s.statsService.GetCategoryBreakdown(ctx, userID, categoryReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get category breakdown: %w", err)
	}

	// Prepare context for AI
	contextData := map[string]interface{}{
		"monthly_income":      req.MonthlyIncome,
		"financial_goals":     req.FinancialGoals,
		"risk_tolerance":      req.RiskTolerance,
		"current_budgets":     s.formatBudgets(budgets),
		"spending_by_category": s.formatCategoryBreakdown(categories),
		"total_budget":        totalBudget,
		"total_spent":         totalSpent,
		"avg_monthly_spent":   totalCategorySpent / 3, // Average over 3 months
	}

	// Call AI service for recommendations
	aiQuery := s.buildBudgetingQuery(contextData)
	aiResponse, err := s.callAIService(ctx, userID, accessToken, aiQuery, contextData)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI budgeting recommendations: %w", err)
	}

	// Parse AI response and structure recommendations
	budgetRecommendations, savingsRate, rationale := s.parseAIBudgetingResponse(aiResponse, categories, req.MonthlyIncome)

	return &pb.GetAIBudgetingRecommendationsResponse{
		Success:                  true,
		Summary:                  aiResponse,
		BudgetRecommendations:    budgetRecommendations,
		RecommendedSavingsRate:   savingsRate,
		Rationale:                rationale,
	}, nil
}

// ========================================
// AUTO CATEGORIZE EXPENSE
// ========================================

func (s *AIStatisticsService) AutoCategorizeExpense(ctx context.Context, userID uint, accessToken string, req *pb.AutoCategorizeExpenseRequest) (*pb.AutoCategorizeExpenseResponse, error) {
	// Get user's historical expenses for context
	expensesReq := &pb.GetExpensesRequest{
		Page:    1,
		PerPage: 50,
	}

	expenses, _, _, _, err := s.statsService.GetExpenses(ctx, userID, expensesReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get historical expenses: %w", err)
	}

	// Prepare context for AI
	contextData := map[string]interface{}{
		"expense_to_categorize": map[string]interface{}{
			"description": req.Description,
			"merchant":    req.Merchant,
			"amount":      req.Amount,
			"notes":       req.Notes,
		},
		"historical_patterns": s.formatHistoricalExpenses(expenses),
		"available_categories": s.getAllCategories(),
	}

	// Call AI service for categorization
	aiQuery := fmt.Sprintf(`Categorize this expense: '%s' at '%s' for $%.2f.

IMPORTANT: Respond with ONLY a valid JSON object (no markdown, no backticks, no extra text) in this exact format:
{
  "suggested_category": "EXPENSE_CATEGORY_NAME",
  "confidence": 0.85,
  "alternatives": [
    {
      "category": "EXPENSE_CATEGORY_NAME",
      "confidence": 0.65
    }
  ],
  "reasoning": "Brief explanation of why this category fits"
}

Use these exact category names: EXPENSE_CATEGORY_FOOD_DINING, EXPENSE_CATEGORY_GROCERIES, EXPENSE_CATEGORY_TRANSPORTATION, EXPENSE_CATEGORY_SHOPPING, EXPENSE_CATEGORY_ENTERTAINMENT, EXPENSE_CATEGORY_BILLS_UTILITIES, EXPENSE_CATEGORY_HEALTHCARE, EXPENSE_CATEGORY_EDUCATION, EXPENSE_CATEGORY_TRAVEL, EXPENSE_CATEGORY_RENT_MORTGAGE, EXPENSE_CATEGORY_INSURANCE, EXPENSE_CATEGORY_INVESTMENTS, EXPENSE_CATEGORY_GIFTS_DONATIONS, EXPENSE_CATEGORY_PERSONAL_CARE, EXPENSE_CATEGORY_SUBSCRIPTIONS.`,
		req.Description, req.Merchant, req.Amount)

	aiResponse, err := s.callAIService(ctx, userID, accessToken, aiQuery, contextData)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI categorization: %w", err)
	}

	// Parse AI response and extract category
	suggestedCategory, confidence, alternatives, reasoning := s.parseAICategorization(aiResponse)

	return &pb.AutoCategorizeExpenseResponse{
		Success:               true,
		SuggestedCategory:     suggestedCategory,
		CategoryName:          s.getCategoryName(suggestedCategory),
		ConfidenceScore:       confidence,
		AlternativeCategories: alternatives,
		Reasoning:             reasoning,
	}, nil
}

// ========================================
// AI FINANCIAL ADVICE
// ========================================

func (s *AIStatisticsService) GetAIFinancialAdvice(ctx context.Context, userID uint, accessToken string, req *pb.GetAIFinancialAdviceRequest) (*pb.GetAIFinancialAdviceResponse, error) {
	// Gather relevant financial data based on context areas
	contextData := make(map[string]interface{})

	for _, area := range req.ContextAreas {
		switch area {
		case "spending":
			analyticsReq := &pb.GetSpendingAnalyticsRequest{
				Period: "monthly",
			}
			analytics, err := s.statsService.GetSpendingAnalytics(ctx, userID, analyticsReq)
			if err == nil {
				contextData["spending_analytics"] = s.formatSpendingAnalytics(analytics)
			}

		case "budgeting":
			budgetsReq := &pb.GetBudgetsRequest{
				Page:    1,
				PerPage: 20,
			}
			budgets, _, _, _, err := s.statsService.GetBudgets(ctx, userID, budgetsReq)
			if err == nil {
				contextData["budgets"] = s.formatBudgets(budgets)
			}

		case "savings":
			// Calculate savings rate
			analyticsReq := &pb.GetSpendingAnalyticsRequest{
				Period: "monthly",
			}
			analytics, err := s.statsService.GetSpendingAnalytics(ctx, userID, analyticsReq)
			if err == nil {
				contextData["savings_info"] = map[string]interface{}{
					"savings_rate":     analytics.SavingsRate,
					"remaining_budget": analytics.RemainingBudget,
				}
			}
		}
	}

	// Add user query to context
	contextData["user_question"] = req.Query

	// Call AI service for advice
	aiResponse, err := s.callAIService(ctx, userID, accessToken, req.Query, contextData)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI financial advice: %w", err)
	}

	// Parse AI response and extract actionable steps
	actionSteps := s.parseAIAdviceSteps(aiResponse)

	disclaimer := "This advice is generated by AI and should not be considered as professional financial advice. Please consult with a certified financial advisor for personalized guidance."

	return &pb.GetAIFinancialAdviceResponse{
		Success:           true,
		Query:             req.Query,
		Advice:            aiResponse,
		ActionSteps:       actionSteps,
		RelevantResources: []string{}, // Can be populated with helpful links
		Disclaimer:        disclaimer,
	}, nil
}

// ========================================
// HELPER FUNCTIONS
// ========================================

func (s *AIStatisticsService) callAIService(ctx context.Context, userID uint, accessToken string, query string, contextData map[string]interface{}) (string, error) {
	// Prepare request payload
	payload := map[string]interface{}{
		"query":         query,
		"context":       contextData,
		"user_id":       userID,
		"model":         "gpt-4",
		"max_tokens":    1500,
		"temperature":   0.7,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create HTTP request to AI microservice
	aiServiceURL := s.config.AiServiceURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", aiServiceURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

	// Make HTTP request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to call AI service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI service returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var aiResp struct {
		Response string `json:"response"`
		Success  bool   `json:"success"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		return "", fmt.Errorf("failed to decode AI response: %w", err)
	}

	if !aiResp.Success {
		return "", fmt.Errorf("AI service returned unsuccessful response")
	}

	return aiResp.Response, nil
}

func (s *AIStatisticsService) buildInsightsQuery(contextData map[string]interface{}, focusArea string) string {
	baseQuery := `Analyze the spending patterns and provide actionable insights.

IMPORTANT: Respond with ONLY a valid JSON object (no markdown, no backticks, no extra text) in this exact format:
{
  "summary": "Brief overview of spending patterns in 1-2 sentences",
  "insights": [
    {
      "title": "Insight title",
      "description": "Detailed description",
      "type": "positive|warning|informational",
      "impact_amount": 0.00
    }
  ],
  "recommendations": [
    {
      "title": "Recommendation title",
      "description": "What to do",
      "action": "Specific action step",
      "potential_savings": 0.00,
      "priority": "high|medium|low"
    }
  ],
  "anomalies": [
    {
      "description": "Unusual pattern detected",
      "amount": 0.00,
      "severity": "high|medium|low"
    }
  ]
}`

	if focusArea != "" {
		switch focusArea {
		case "savings":
			baseQuery += "\nFocus on identifying opportunities to save money and reduce unnecessary expenses."
		case "categories":
			baseQuery += "\nFocus on analyzing spending across different categories and identifying trends."
		case "trends":
			baseQuery += "\nFocus on identifying spending trends over time and predicting future patterns."
		case "anomalies":
			baseQuery += "\nFocus on detecting unusual spending patterns, duplicate transactions, or potential fraud."
		}
	}

	return baseQuery
}

func (s *AIStatisticsService) buildBudgetingQuery(contextData map[string]interface{}) string {
	return fmt.Sprintf(`Based on my monthly income of $%.2f, financial goals (%v), and current spending patterns, provide personalized budget recommendations.

IMPORTANT: Respond with ONLY a valid JSON object (no markdown, no backticks, no extra text) in this exact format:
{
  "summary": "Brief overview of budget strategy in 1-2 sentences",
  "recommendations": [
    {
      "category": "EXPENSE_CATEGORY_NAME",
      "current_amount": 0.00,
      "recommended_amount": 0.00,
      "difference": 0.00,
      "reasoning": "Why this budget amount"
    }
  ],
  "savings_rate": 20.0,
  "rationale": "Overall budgeting philosophy and approach"
}

Use these exact category names: EXPENSE_CATEGORY_FOOD_DINING, EXPENSE_CATEGORY_GROCERIES, EXPENSE_CATEGORY_TRANSPORTATION, EXPENSE_CATEGORY_SHOPPING, EXPENSE_CATEGORY_ENTERTAINMENT, EXPENSE_CATEGORY_BILLS_UTILITIES, EXPENSE_CATEGORY_HEALTHCARE, EXPENSE_CATEGORY_EDUCATION, EXPENSE_CATEGORY_TRAVEL, EXPENSE_CATEGORY_RENT_MORTGAGE, EXPENSE_CATEGORY_INSURANCE, EXPENSE_CATEGORY_INVESTMENTS, EXPENSE_CATEGORY_GIFTS_DONATIONS, EXPENSE_CATEGORY_PERSONAL_CARE, EXPENSE_CATEGORY_SUBSCRIPTIONS.
Consider my risk tolerance: %s`,
		contextData["monthly_income"],
		contextData["financial_goals"],
		contextData["risk_tolerance"])
}

func (s *AIStatisticsService) formatCategoryBreakdown(categories []*pb.CategorySpending) []map[string]interface{} {
	result := make([]map[string]interface{}, len(categories))
	for i, cat := range categories {
		result[i] = map[string]interface{}{
			"category":    cat.CategoryName,
			"amount":      cat.Amount,
			"percentage":  cat.Percentage,
			"count":       cat.TransactionCount,
		}
	}
	return result
}

func (s *AIStatisticsService) formatBudgets(budgets []*pb.BudgetMessage) []map[string]interface{} {
	result := make([]map[string]interface{}, len(budgets))
	for i, budget := range budgets {
		result[i] = map[string]interface{}{
			"name":             budget.Name,
			"amount":           budget.Amount,
			"spent":            budget.SpentAmount,
			"remaining":        budget.RemainingAmount,
			"percentage_used":  budget.PercentageUsed,
		}
	}
	return result
}

func (s *AIStatisticsService) formatHistoricalExpenses(expenses []*pb.ExpenseMessage) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	for _, expense := range expenses {
		if len(result) >= 20 { // Limit to 20 examples
			break
		}
		result = append(result, map[string]interface{}{
			"description": expense.Description,
			"merchant":    expense.Merchant,
			"amount":      expense.Amount,
			"category":    expense.Category.String(),
		})
	}
	return result
}

func (s *AIStatisticsService) formatSpendingAnalytics(analytics *pb.SpendingAnalytics) map[string]interface{} {
	return map[string]interface{}{
		"total_spent":       analytics.TotalSpent,
		"total_budget":      analytics.TotalBudget,
		"transaction_count": analytics.TransactionCount,
		"savings_rate":      analytics.SavingsRate,
		"top_category":      analytics.TopCategory,
	}
}

func (s *AIStatisticsService) getAllCategories() []string {
	return []string{
		"EXPENSE_CATEGORY_FOOD_DINING",
		"EXPENSE_CATEGORY_GROCERIES",
		"EXPENSE_CATEGORY_TRANSPORTATION",
		"EXPENSE_CATEGORY_SHOPPING",
		"EXPENSE_CATEGORY_ENTERTAINMENT",
		"EXPENSE_CATEGORY_BILLS_UTILITIES",
		"EXPENSE_CATEGORY_HEALTHCARE",
		"EXPENSE_CATEGORY_EDUCATION",
		"EXPENSE_CATEGORY_TRAVEL",
		"EXPENSE_CATEGORY_RENT_MORTGAGE",
		"EXPENSE_CATEGORY_INSURANCE",
		"EXPENSE_CATEGORY_INVESTMENTS",
		"EXPENSE_CATEGORY_GIFTS_DONATIONS",
		"EXPENSE_CATEGORY_PERSONAL_CARE",
		"EXPENSE_CATEGORY_SUBSCRIPTIONS",
	}
}

func (s *AIStatisticsService) getCategoryName(category pb.ExpenseCategory) string {
	switch category {
	case pb.ExpenseCategory_EXPENSE_CATEGORY_FOOD_DINING:
		return "Food & Dining"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_GROCERIES:
		return "Groceries"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_TRANSPORTATION:
		return "Transportation"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_SHOPPING:
		return "Shopping"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_ENTERTAINMENT:
		return "Entertainment"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_BILLS_UTILITIES:
		return "Bills & Utilities"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_HEALTHCARE:
		return "Healthcare"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_EDUCATION:
		return "Education"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_TRAVEL:
		return "Travel"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_RENT_MORTGAGE:
		return "Rent/Mortgage"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_INSURANCE:
		return "Insurance"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_INVESTMENTS:
		return "Investments"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_GIFTS_DONATIONS:
		return "Gifts & Donations"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_PERSONAL_CARE:
		return "Personal Care"
	case pb.ExpenseCategory_EXPENSE_CATEGORY_SUBSCRIPTIONS:
		return "Subscriptions"
	default:
		return "Other"
	}
}

// Parse AI responses - parse JSON from AI service

func (s *AIStatisticsService) parseAIInsightsResponse(aiResponse string, expenses []*pb.ExpenseMessage, budgets []*pb.BudgetMessage) ([]*pb.AIInsight, []*pb.AIRecommendation, []*pb.AnomalyDetection) {
	// Clean potential markdown formatting
	cleaned := strings.TrimSpace(aiResponse)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Parse JSON response
	var aiData struct {
		Summary        string `json:"summary"`
		Insights       []struct {
			Title        string  `json:"title"`
			Description  string  `json:"description"`
			Type         string  `json:"type"`
			ImpactAmount float64 `json:"impact_amount"`
		} `json:"insights"`
		Recommendations []struct {
			Title            string  `json:"title"`
			Description      string  `json:"description"`
			Action           string  `json:"action"`
			PotentialSavings float64 `json:"potential_savings"`
			Priority         string  `json:"priority"`
		} `json:"recommendations"`
		Anomalies []struct {
			Description string  `json:"description"`
			Amount      float64 `json:"amount"`
			Severity    string  `json:"severity"`
		} `json:"anomalies"`
	}

	if err := json.Unmarshal([]byte(cleaned), &aiData); err != nil {
		// If parsing fails, return empty arrays
		return []*pb.AIInsight{}, []*pb.AIRecommendation{}, []*pb.AnomalyDetection{}
	}

	// Convert insights
	insights := make([]*pb.AIInsight, 0, len(aiData.Insights))
	for _, insight := range aiData.Insights {
		insights = append(insights, &pb.AIInsight{
			Title:          insight.Title,
			Description:    insight.Description,
			InsightType:    insight.Type,
			ImpactAmount:   insight.ImpactAmount,
			SupportingData: []string{},
		})
	}

	// Convert recommendations
	recommendations := make([]*pb.AIRecommendation, 0, len(aiData.Recommendations))
	for _, rec := range aiData.Recommendations {
		recommendations = append(recommendations, &pb.AIRecommendation{
			Title:            rec.Title,
			Description:      rec.Description,
			Action:           rec.Action,
			PotentialSavings: rec.PotentialSavings,
			Priority:         rec.Priority,
		})
	}

	// Convert anomalies
	anomalies := make([]*pb.AnomalyDetection, 0, len(aiData.Anomalies))
	for _, anomaly := range aiData.Anomalies {
		anomalies = append(anomalies, &pb.AnomalyDetection{
			Description:  anomaly.Description,
			Amount:       anomaly.Amount,
			Severity:     anomaly.Severity,
			DetectedDate: timestamppb.Now(),
			AnomalyType:  "unusual_spending",
		})
	}

	return insights, recommendations, anomalies
}

func (s *AIStatisticsService) parseAIBudgetingResponse(aiResponse string, categories []*pb.CategorySpending, monthlyIncome float64) ([]*pb.BudgetRecommendation, float64, string) {
	// Clean potential markdown formatting
	cleaned := strings.TrimSpace(aiResponse)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Parse JSON response
	var aiData struct {
		Summary         string `json:"summary"`
		Recommendations []struct {
			Category          string  `json:"category"`
			CurrentAmount     float64 `json:"current_amount"`
			RecommendedAmount float64 `json:"recommended_amount"`
			Difference        float64 `json:"difference"`
			Reasoning         string  `json:"reasoning"`
		} `json:"recommendations"`
		SavingsRate float64 `json:"savings_rate"`
		Rationale   string  `json:"rationale"`
	}

	if err := json.Unmarshal([]byte(cleaned), &aiData); err != nil {
		// If parsing fails, return defaults
		return []*pb.BudgetRecommendation{}, 20.0, aiResponse
	}

	// Convert recommendations
	recommendations := make([]*pb.BudgetRecommendation, 0, len(aiData.Recommendations))
	for _, rec := range aiData.Recommendations {
		// Parse category name to enum
		category := s.parseCategoryString(rec.Category)

		recommendations = append(recommendations, &pb.BudgetRecommendation{
			Category:          category,
			CurrentAmount:     rec.CurrentAmount,
			RecommendedAmount: rec.RecommendedAmount,
			Difference:        rec.Difference,
			Reasoning:         rec.Reasoning,
		})
	}

	savingsRate := aiData.SavingsRate
	if savingsRate == 0 {
		savingsRate = 20.0 // Default
	}

	rationale := aiData.Rationale
	if rationale == "" {
		rationale = aiData.Summary
	}

	return recommendations, savingsRate, rationale
}

func (s *AIStatisticsService) parseAICategorization(aiResponse string) (pb.ExpenseCategory, float64, []*pb.CategorySuggestion, string) {
	// Clean potential markdown formatting
	cleaned := strings.TrimSpace(aiResponse)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Parse JSON response
	var aiData struct {
		SuggestedCategory string `json:"suggested_category"`
		Confidence        float64 `json:"confidence"`
		Alternatives      []struct {
			Category   string  `json:"category"`
			Confidence float64 `json:"confidence"`
		} `json:"alternatives"`
		Reasoning string `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(cleaned), &aiData); err != nil {
		// If parsing fails, return default
		return pb.ExpenseCategory_EXPENSE_CATEGORY_SHOPPING, 0.5, []*pb.CategorySuggestion{}, aiResponse
	}

	// Parse suggested category
	suggestedCategory := s.parseCategoryString(aiData.SuggestedCategory)
	confidence := aiData.Confidence
	if confidence == 0 {
		confidence = 0.5
	}

	// Parse alternatives
	alternatives := make([]*pb.CategorySuggestion, 0, len(aiData.Alternatives))
	for _, alt := range aiData.Alternatives {
		altCategory := s.parseCategoryString(alt.Category)
		alternatives = append(alternatives, &pb.CategorySuggestion{
			Category:        altCategory,
			CategoryName:    s.getCategoryName(altCategory),
			ConfidenceScore: alt.Confidence,
		})
	}

	return suggestedCategory, confidence, alternatives, aiData.Reasoning
}

func (s *AIStatisticsService) parseAIAdviceSteps(aiResponse string) []*pb.ActionStep {
	// Clean potential markdown formatting
	cleaned := strings.TrimSpace(aiResponse)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// Try to parse JSON response with action steps
	var aiData struct {
		Advice      string `json:"advice"`
		ActionSteps []struct {
			StepNumber      int32  `json:"step_number"`
			Title           string `json:"title"`
			Description     string `json:"description"`
			EstimatedImpact string `json:"estimated_impact"`
		} `json:"action_steps"`
	}

	// Try parsing as structured JSON first
	if err := json.Unmarshal([]byte(cleaned), &aiData); err == nil && len(aiData.ActionSteps) > 0 {
		steps := make([]*pb.ActionStep, 0, len(aiData.ActionSteps))
		for _, step := range aiData.ActionSteps {
			steps = append(steps, &pb.ActionStep{
				StepNumber:      step.StepNumber,
				Title:           step.Title,
				Description:     step.Description,
				EstimatedImpact: step.EstimatedImpact,
				IsCompleted:     false,
			})
		}
		return steps
	}

	// If JSON parsing fails, try to extract action steps from text
	// Split by numbered list patterns (1. 2. 3. etc)
	lines := strings.Split(aiResponse, "\n")
	steps := make([]*pb.ActionStep, 0)
	var stepNum int32 = 1

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Look for patterns like "1." or "1)" at start of line
		if len(trimmed) > 2 && (trimmed[1] == '.' || trimmed[1] == ')') {
			if trimmed[0] >= '1' && trimmed[0] <= '9' {
				content := strings.TrimSpace(trimmed[2:])
				if content != "" {
					steps = append(steps, &pb.ActionStep{
						StepNumber:      stepNum,
						Title:           content,
						Description:     content,
						EstimatedImpact: "Medium",
						IsCompleted:     false,
					})
					stepNum++
				}
			}
		}
	}

	// If no steps extracted, return a single generic step
	if len(steps) == 0 {
		steps = []*pb.ActionStep{
			{
				StepNumber:      1,
				Title:           "Review AI Advice",
				Description:     aiResponse,
				EstimatedImpact: "Medium",
				IsCompleted:     false,
			},
		}
	}

	return steps
}

// Helper function to parse category string to protobuf enum
func (s *AIStatisticsService) parseCategoryString(categoryStr string) pb.ExpenseCategory {
	// Normalize the string
	categoryStr = strings.ToUpper(strings.TrimSpace(categoryStr))

	// Map string to enum
	switch categoryStr {
	case "EXPENSE_CATEGORY_FOOD_DINING", "FOOD_DINING", "FOOD", "DINING", "RESTAURANT":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_FOOD_DINING
	case "EXPENSE_CATEGORY_GROCERIES", "GROCERIES", "GROCERY":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_GROCERIES
	case "EXPENSE_CATEGORY_TRANSPORTATION", "TRANSPORTATION", "TRANSPORT", "GAS", "FUEL":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_TRANSPORTATION
	case "EXPENSE_CATEGORY_SHOPPING", "SHOPPING", "RETAIL":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_SHOPPING
	case "EXPENSE_CATEGORY_ENTERTAINMENT", "ENTERTAINMENT", "MOVIES", "GAMES":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_ENTERTAINMENT
	case "EXPENSE_CATEGORY_BILLS_UTILITIES", "BILLS_UTILITIES", "BILLS", "UTILITIES", "ELECTRIC", "WATER":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_BILLS_UTILITIES
	case "EXPENSE_CATEGORY_HEALTHCARE", "HEALTHCARE", "HEALTH", "MEDICAL", "DOCTOR":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_HEALTHCARE
	case "EXPENSE_CATEGORY_EDUCATION", "EDUCATION", "SCHOOL", "TUITION":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_EDUCATION
	case "EXPENSE_CATEGORY_TRAVEL", "TRAVEL", "VACATION", "HOTEL":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_TRAVEL
	case "EXPENSE_CATEGORY_RENT_MORTGAGE", "RENT_MORTGAGE", "RENT", "MORTGAGE", "HOUSING":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_RENT_MORTGAGE
	case "EXPENSE_CATEGORY_INSURANCE", "INSURANCE":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_INSURANCE
	case "EXPENSE_CATEGORY_INVESTMENTS", "INVESTMENTS", "INVESTING", "STOCKS", "BONDS":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_INVESTMENTS
	case "EXPENSE_CATEGORY_GIFTS_DONATIONS", "GIFTS_DONATIONS", "GIFTS", "DONATIONS", "CHARITY":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_GIFTS_DONATIONS
	case "EXPENSE_CATEGORY_PERSONAL_CARE", "PERSONAL_CARE", "PERSONAL", "BEAUTY", "HAIRCUT":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_PERSONAL_CARE
	case "EXPENSE_CATEGORY_SUBSCRIPTIONS", "SUBSCRIPTIONS", "SUBSCRIPTION", "STREAMING", "NETFLIX":
		return pb.ExpenseCategory_EXPENSE_CATEGORY_SUBSCRIPTIONS
	default:
		return pb.ExpenseCategory_EXPENSE_CATEGORY_SHOPPING // Default fallback
	}
}
