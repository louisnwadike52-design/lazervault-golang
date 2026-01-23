package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"net/http"
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// WrappedService handles the Financial Wrapped experience (Spotify-style year/month review)
type WrappedService struct {
	db           *gorm.DB
	config       *configs.Config
	statsService *StatisticsService
}

// NewWrappedService creates a new instance of WrappedService
func NewWrappedService(db *gorm.DB, cfg *configs.Config, statsService *StatisticsService) *WrappedService {
	return &WrappedService{
		db:           db,
		config:       cfg,
		statsService: statsService,
	}
}

// GetFinancialWrapped generates the complete financial wrapped experience
func (s *WrappedService) GetFinancialWrapped(ctx context.Context, userID uint, accessToken string, req *pb.GetFinancialWrappedRequest) (*pb.GetFinancialWrappedResponse, error) {
	// Determine the time period
	startDate, endDate := s.calculatePeriodDates(req)

	// Calculate all wrapped sections in parallel (conceptually - Go handles this efficiently)
	summary, err := s.calculateSummary(ctx, userID, startDate, endDate)
	if err != nil {
		return &pb.GetFinancialWrappedResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to calculate summary: %v", err),
		}, nil
	}

	topCategories, err := s.calculateCategoryRankings(ctx, userID, startDate, endDate)
	if err != nil {
		return &pb.GetFinancialWrappedResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to calculate category rankings: %v", err),
		}, nil
	}

	topMerchants, err := s.calculateMerchantRankings(ctx, userID, startDate, endDate)
	if err != nil {
		return &pb.GetFinancialWrappedResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to calculate merchant rankings: %v", err),
		}, nil
	}

	trends, err := s.calculateTrends(ctx, userID, startDate, endDate, req.Period)
	if err != nil {
		return &pb.GetFinancialWrappedResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to calculate trends: %v", err),
		}, nil
	}

	funFacts := s.generateFunFacts(summary, topCategories, topMerchants)

	achievements := s.calculateAchievements(summary, topCategories, trends)

	// Generate AI insights
	aiInsights, err := s.generateAIInsights(ctx, userID, accessToken, summary, topCategories, topMerchants, startDate, endDate)
	if err != nil {
		// Don't fail the whole request if AI fails - use fallback
		aiInsights = s.getFallbackAIInsights(summary, topCategories)
	}

	// Build the final wrapped response
	wrapped := &pb.FinancialWrapped{
		Period:        req.Period,
		Year:          req.Year,
		Month:         req.Month,
		PeriodStart:   timestamppb.New(startDate),
		PeriodEnd:     timestamppb.New(endDate),
		Summary:       summary,
		TopCategories: topCategories,
		TopMerchants:  topMerchants,
		Trends:        trends,
		AiInsights:    aiInsights,
		FunFacts:      funFacts,
		Achievements:  achievements,
	}

	return &pb.GetFinancialWrappedResponse{
		Success: true,
		Wrapped: wrapped,
	}, nil
}

// calculatePeriodDates returns start and end dates based on the period type
func (s *WrappedService) calculatePeriodDates(req *pb.GetFinancialWrappedRequest) (time.Time, time.Time) {
	year := int(req.Year)
	if year == 0 {
		year = time.Now().Year()
	}

	switch req.Period {
	case pb.WrappedPeriod_WRAPPED_PERIOD_MONTHLY:
		month := time.Month(req.Month)
		if month == 0 {
			month = time.Now().Month()
		}
		startDate := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		endDate := startDate.AddDate(0, 1, 0).Add(-time.Second)
		return startDate, endDate

	case pb.WrappedPeriod_WRAPPED_PERIOD_YEARLY:
		fallthrough
	default:
		startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC)
		return startDate, endDate
	}
}

// calculateSummary calculates the overall summary stats for the period
func (s *WrappedService) calculateSummary(ctx context.Context, userID uint, startDate, endDate time.Time) (*pb.WrappedSummary, error) {
	// Get total expenditure
	var totalSpent float64
	var spentResult struct{ Total float64 }
	if err := s.db.Model(&models.Expense{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Select("COALESCE(SUM(amount), 0) as total").
		Scan(&spentResult).Error; err != nil {
		return nil, err
	}
	totalSpent = spentResult.Total

	// Also include tracked expenditure
	var trackedSpent float64
	var trackedResult struct{ Total float64 }
	if err := s.db.Model(&models.ExpenditureTransaction{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Select("COALESCE(SUM(amount), 0) as total").
		Scan(&trackedResult).Error; err == nil {
		trackedSpent = trackedResult.Total
	}
	totalSpent += trackedSpent

	// Get total income
	var totalEarned float64
	var earnedResult struct{ Total float64 }
	if err := s.db.Model(&models.IncomeTransaction{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Select("COALESCE(SUM(amount), 0) as total").
		Scan(&earnedResult).Error; err == nil {
		totalEarned = earnedResult.Total
	}

	// Get transaction count
	var transactionCount int64
	s.db.Model(&models.Expense{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Count(&transactionCount)

	var trackedCount int64
	s.db.Model(&models.ExpenditureTransaction{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Count(&trackedCount)
	transactionCount += trackedCount

	// Calculate savings
	totalSaved := totalEarned - totalSpent
	if totalSaved < 0 {
		totalSaved = 0
	}

	// Calculate savings rate
	savingsRate := 0.0
	if totalEarned > 0 {
		savingsRate = (totalSaved / totalEarned) * 100
	}

	// Calculate average transaction
	averageTransaction := 0.0
	if transactionCount > 0 {
		averageTransaction = totalSpent / float64(transactionCount)
	}

	// Find biggest spending day
	type DailySpend struct {
		Date   time.Time
		Amount float64
	}
	var dailySpends []DailySpend
	s.db.Model(&models.Expense{}).
		Select("DATE(transaction_date) as date, SUM(amount) as amount").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Group("DATE(transaction_date)").
		Order("amount DESC").
		Limit(1).
		Scan(&dailySpends)

	biggestSpendingDay := ""
	biggestSpendingDayAmount := 0.0
	if len(dailySpends) > 0 {
		biggestSpendingDay = dailySpends[0].Date.Format("January 2, 2006")
		biggestSpendingDayAmount = dailySpends[0].Amount
	}

	// Find most active day of week
	type DayOfWeekSpend struct {
		DayOfWeek int
		Count     int64
	}
	var dayOfWeekSpends []DayOfWeekSpend
	s.db.Model(&models.Expense{}).
		Select("EXTRACT(DOW FROM transaction_date) as day_of_week, COUNT(*) as count").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Group("EXTRACT(DOW FROM transaction_date)").
		Order("count DESC").
		Limit(1).
		Scan(&dayOfWeekSpends)

	mostActiveDayOfWeek := "Friday"
	if len(dayOfWeekSpends) > 0 {
		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		if dayOfWeekSpends[0].DayOfWeek >= 0 && dayOfWeekSpends[0].DayOfWeek < 7 {
			mostActiveDayOfWeek = days[dayOfWeekSpends[0].DayOfWeek]
		}
	}

	return &pb.WrappedSummary{
		TotalSpent:               totalSpent,
		TotalEarned:              totalEarned,
		TotalSaved:               totalSaved,
		SavingsRate:              savingsRate,
		TransactionCount:         int32(transactionCount),
		AverageTransaction:       averageTransaction,
		BiggestSpendingDay:       biggestSpendingDay,
		BiggestSpendingDayAmount: biggestSpendingDayAmount,
		MostActiveDayOfWeek:      mostActiveDayOfWeek,
	}, nil
}

// calculateCategoryRankings calculates top spending categories
func (s *WrappedService) calculateCategoryRankings(ctx context.Context, userID uint, startDate, endDate time.Time) ([]*pb.WrappedCategoryRanking, error) {
	type CategoryResult struct {
		Category         string
		Amount           float64
		TransactionCount int32
	}

	var results []CategoryResult
	if err := s.db.Model(&models.Expense{}).
		Select("category, COALESCE(SUM(amount), 0) as amount, COUNT(*) as transaction_count").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Group("category").
		Order("amount DESC").
		Limit(5).
		Scan(&results).Error; err != nil {
		return nil, err
	}

	// Calculate total for percentages
	var totalAmount float64
	for _, r := range results {
		totalAmount += r.Amount
	}

	// Get previous period data for trend comparison
	periodDuration := endDate.Sub(startDate)
	prevStartDate := startDate.Add(-periodDuration)
	prevEndDate := startDate.Add(-time.Second)

	var prevResults []CategoryResult
	s.db.Model(&models.Expense{}).
		Select("category, COALESCE(SUM(amount), 0) as amount").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, prevStartDate, prevEndDate).
		Group("category").
		Scan(&prevResults)

	prevAmountMap := make(map[string]float64)
	for _, r := range prevResults {
		prevAmountMap[r.Category] = r.Amount
	}

	// Build rankings
	rankings := make([]*pb.WrappedCategoryRanking, 0, len(results))
	for i, r := range results {
		percentage := 0.0
		if totalAmount > 0 {
			percentage = (r.Amount / totalAmount) * 100
		}

		// Calculate trend
		trend := "stable"
		trendPercentage := 0.0
		if prevAmount, ok := prevAmountMap[r.Category]; ok && prevAmount > 0 {
			trendPercentage = ((r.Amount - prevAmount) / prevAmount) * 100
			if trendPercentage > 5 {
				trend = "up"
			} else if trendPercentage < -5 {
				trend = "down"
			}
		}

		// Parse category enum
		categoryEnum := pb.ExpenseCategory_EXPENSE_CATEGORY_UNSPECIFIED
		if catVal, ok := pb.ExpenseCategory_value[r.Category]; ok {
			categoryEnum = pb.ExpenseCategory(catVal)
		}

		rankings = append(rankings, &pb.WrappedCategoryRanking{
			Rank:             int32(i + 1),
			Category:         categoryEnum,
			CategoryName:     models.CategoryNames[r.Category],
			Amount:           r.Amount,
			Percentage:       percentage,
			TransactionCount: r.TransactionCount,
			Trend:            trend,
			TrendPercentage:  trendPercentage,
			Emoji:            getCategoryEmoji(r.Category),
		})
	}

	return rankings, nil
}

// calculateMerchantRankings calculates top merchants by spending
func (s *WrappedService) calculateMerchantRankings(ctx context.Context, userID uint, startDate, endDate time.Time) ([]*pb.WrappedMerchantRanking, error) {
	type MerchantResult struct {
		Merchant   string
		Amount     float64
		VisitCount int32
		Category   string
	}

	var results []MerchantResult
	if err := s.db.Model(&models.Expense{}).
		Select("merchant, COALESCE(SUM(amount), 0) as amount, COUNT(*) as visit_count, MAX(category) as category").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ? AND merchant != ''", userID, startDate, endDate).
		Group("merchant").
		Order("amount DESC").
		Limit(5).
		Scan(&results).Error; err != nil {
		return nil, err
	}

	// Also check tracked expenditure for recipient names
	type RecipientResult struct {
		RecipientName string
		Amount        float64
		VisitCount    int32
	}
	var recipientResults []RecipientResult
	s.db.Model(&models.ExpenditureTransaction{}).
		Select("recipient_name, COALESCE(SUM(amount), 0) as amount, COUNT(*) as visit_count").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ? AND recipient_name != ''", userID, startDate, endDate).
		Group("recipient_name").
		Order("amount DESC").
		Limit(5).
		Scan(&recipientResults)

	// Merge and sort
	merchantMap := make(map[string]*MerchantResult)
	for _, r := range results {
		if r.Merchant != "" {
			merchantMap[r.Merchant] = &MerchantResult{
				Merchant:   r.Merchant,
				Amount:     r.Amount,
				VisitCount: r.VisitCount,
				Category:   r.Category,
			}
		}
	}
	for _, r := range recipientResults {
		if r.RecipientName != "" {
			if existing, ok := merchantMap[r.RecipientName]; ok {
				existing.Amount += r.Amount
				existing.VisitCount += r.VisitCount
			} else {
				merchantMap[r.RecipientName] = &MerchantResult{
					Merchant:   r.RecipientName,
					Amount:     r.Amount,
					VisitCount: r.VisitCount,
					Category:   "transfer",
				}
			}
		}
	}

	// Convert to slice and sort
	var merged []MerchantResult
	for _, v := range merchantMap {
		merged = append(merged, *v)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Amount > merged[j].Amount
	})

	// Limit to top 5
	if len(merged) > 5 {
		merged = merged[:5]
	}

	// Calculate total for percentages
	var totalAmount float64
	for _, r := range merged {
		totalAmount += r.Amount
	}

	// Build rankings
	rankings := make([]*pb.WrappedMerchantRanking, 0, len(merged))
	for i, r := range merged {
		percentage := 0.0
		if totalAmount > 0 {
			percentage = (r.Amount / totalAmount) * 100
		}

		rankings = append(rankings, &pb.WrappedMerchantRanking{
			Rank:         int32(i + 1),
			MerchantName: r.Merchant,
			Amount:       r.Amount,
			VisitCount:   r.VisitCount,
			Percentage:   percentage,
			Category:     models.CategoryNames[r.Category],
			Emoji:        getMerchantEmoji(r.Merchant, r.Category),
		})
	}

	return rankings, nil
}

// calculateTrends calculates spending trends over the period
func (s *WrappedService) calculateTrends(ctx context.Context, userID uint, startDate, endDate time.Time, period pb.WrappedPeriod) (*pb.WrappedTrends, error) {
	trends := &pb.WrappedTrends{
		MonthlyTrends:       make([]*pb.WrappedMonthlyTrend, 0),
		ImprovingCategories: make([]string, 0),
		WatchCategories:     make([]string, 0),
	}

	// Get monthly trends
	type MonthlyResult struct {
		MonthLabel       string
		Amount           float64
		TransactionCount int32
	}

	var monthlyResults []MonthlyResult
	s.db.Model(&models.Expense{}).
		Select("TO_CHAR(transaction_date, 'Mon') as month_label, COALESCE(SUM(amount), 0) as amount, COUNT(*) as transaction_count").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Group("EXTRACT(YEAR FROM transaction_date), EXTRACT(MONTH FROM transaction_date), TO_CHAR(transaction_date, 'Mon')").
		Order("EXTRACT(YEAR FROM transaction_date), EXTRACT(MONTH FROM transaction_date)").
		Scan(&monthlyResults)

	for _, r := range monthlyResults {
		trends.MonthlyTrends = append(trends.MonthlyTrends, &pb.WrappedMonthlyTrend{
			MonthLabel:       r.MonthLabel,
			Amount:           r.Amount,
			TransactionCount: r.TransactionCount,
		})
	}

	// Calculate overall spending trend
	if len(monthlyResults) >= 2 {
		firstHalf := 0.0
		secondHalf := 0.0
		midpoint := len(monthlyResults) / 2

		for i, r := range monthlyResults {
			if i < midpoint {
				firstHalf += r.Amount
			} else {
				secondHalf += r.Amount
			}
		}

		if firstHalf > 0 {
			trends.SpendingTrendPercentage = ((secondHalf - firstHalf) / firstHalf) * 100
		}

		if trends.SpendingTrendPercentage > 10 {
			trends.SpendingTrendDescription = "Your spending increased in the second half of the period"
		} else if trends.SpendingTrendPercentage < -10 {
			trends.SpendingTrendDescription = "Great job! Your spending decreased in the second half"
		} else {
			trends.SpendingTrendDescription = "Your spending stayed relatively consistent"
		}
	}

	// Find improving and watch categories
	periodDuration := endDate.Sub(startDate)
	prevStartDate := startDate.Add(-periodDuration)
	prevEndDate := startDate.Add(-time.Second)

	type CategoryTrend struct {
		Category      string
		CurrentAmount float64
		PrevAmount    float64
		Change        float64
	}

	var categoryTrends []CategoryTrend

	// Current period by category
	type CatAmount struct {
		Category string
		Amount   float64
	}
	var currentCats []CatAmount
	s.db.Model(&models.Expense{}).
		Select("category, COALESCE(SUM(amount), 0) as amount").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, startDate, endDate).
		Group("category").
		Scan(&currentCats)

	// Previous period by category
	var prevCats []CatAmount
	s.db.Model(&models.Expense{}).
		Select("category, COALESCE(SUM(amount), 0) as amount").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?", userID, prevStartDate, prevEndDate).
		Group("category").
		Scan(&prevCats)

	prevMap := make(map[string]float64)
	for _, c := range prevCats {
		prevMap[c.Category] = c.Amount
	}

	for _, c := range currentCats {
		prev := prevMap[c.Category]
		change := 0.0
		if prev > 0 {
			change = ((c.Amount - prev) / prev) * 100
		}
		categoryTrends = append(categoryTrends, CategoryTrend{
			Category:      c.Category,
			CurrentAmount: c.Amount,
			PrevAmount:    prev,
			Change:        change,
		})
	}

	// Sort by change percentage
	sort.Slice(categoryTrends, func(i, j int) bool {
		return categoryTrends[i].Change < categoryTrends[j].Change
	})

	// Top 3 improving (biggest decrease)
	for i := 0; i < len(categoryTrends) && i < 3; i++ {
		if categoryTrends[i].Change < -10 {
			name := models.CategoryNames[categoryTrends[i].Category]
			if name != "" {
				trends.ImprovingCategories = append(trends.ImprovingCategories, name)
			}
		}
	}

	// Top 3 watch (biggest increase)
	for i := len(categoryTrends) - 1; i >= 0 && len(trends.WatchCategories) < 3; i-- {
		if categoryTrends[i].Change > 10 {
			name := models.CategoryNames[categoryTrends[i].Category]
			if name != "" {
				trends.WatchCategories = append(trends.WatchCategories, name)
			}
		}
	}

	return trends, nil
}

// generateFunFacts generates fun comparisons and facts
func (s *WrappedService) generateFunFacts(summary *pb.WrappedSummary, categories []*pb.WrappedCategoryRanking, merchants []*pb.WrappedMerchantRanking) []*pb.WrappedFunFact {
	funFacts := make([]*pb.WrappedFunFact, 0)

	// Coffee comparison (assuming Food & Dining or Groceries)
	for _, cat := range categories {
		if cat.CategoryName == "Food & Dining" {
			coffeeCount := int(cat.Amount / 5.0) // Assuming $5 per coffee
			funFacts = append(funFacts, &pb.WrappedFunFact{
				Emoji:       "☕",
				Title:       "Coffee Equivalent",
				Description: fmt.Sprintf("You spent $%.2f on food & dining", cat.Amount),
				Comparison:  fmt.Sprintf("That's about %d cups of coffee!", coffeeCount),
			})
			break
		}
	}

	// Transaction frequency
	if summary.TransactionCount > 0 {
		daysInPeriod := 365.0 // Adjust based on period
		avgPerDay := float64(summary.TransactionCount) / daysInPeriod
		funFacts = append(funFacts, &pb.WrappedFunFact{
			Emoji:       "🧮",
			Title:       "Transaction Frequency",
			Description: fmt.Sprintf("You made %d transactions", summary.TransactionCount),
			Comparison:  fmt.Sprintf("That's about %.1f transactions per day!", avgPerDay),
		})
	}

	// Top merchant loyalty
	if len(merchants) > 0 {
		topMerchant := merchants[0]
		funFacts = append(funFacts, &pb.WrappedFunFact{
			Emoji:       "🏆",
			Title:       "Your Favorite Spot",
			Description: fmt.Sprintf("You visited %s %d times", topMerchant.MerchantName, topMerchant.VisitCount),
			Comparison:  fmt.Sprintf("Spending a total of $%.2f there!", topMerchant.Amount),
		})
	}

	// Savings achievement
	if summary.SavingsRate > 0 {
		funFacts = append(funFacts, &pb.WrappedFunFact{
			Emoji:       "💰",
			Title:       "Savings Superstar",
			Description: fmt.Sprintf("Your savings rate was %.1f%%", summary.SavingsRate),
			Comparison:  fmt.Sprintf("You saved $%.2f this period!", summary.TotalSaved),
		})
	}

	// Most active day
	if summary.MostActiveDayOfWeek != "" {
		funFacts = append(funFacts, &pb.WrappedFunFact{
			Emoji:       "📅",
			Title:       "Shopping Day",
			Description: fmt.Sprintf("Your most active shopping day was %s", summary.MostActiveDayOfWeek),
			Comparison:  "When you tend to make the most purchases!",
		})
	}

	return funFacts
}

// calculateAchievements determines which badges the user earned
func (s *WrappedService) calculateAchievements(summary *pb.WrappedSummary, categories []*pb.WrappedCategoryRanking, trends *pb.WrappedTrends) []*pb.WrappedAchievement {
	achievements := make([]*pb.WrappedAchievement, 0)

	// Savings Champion
	savingsUnlocked := summary.SavingsRate >= 20
	savingsTier := "bronze"
	if summary.SavingsRate >= 40 {
		savingsTier = "gold"
	} else if summary.SavingsRate >= 30 {
		savingsTier = "silver"
	}
	achievements = append(achievements, &pb.WrappedAchievement{
		Id:          "savings_champion",
		Name:        "Savings Champion",
		Description: "Save at least 20% of your income",
		Emoji:       "💰",
		Tier:        savingsTier,
		Unlocked:    savingsUnlocked,
		Progress:    min(summary.SavingsRate/20*100, 100),
	})

	// Budget Master
	budgetUnlocked := len(trends.ImprovingCategories) >= 2
	achievements = append(achievements, &pb.WrappedAchievement{
		Id:          "budget_master",
		Name:        "Budget Master",
		Description: "Reduce spending in 2+ categories",
		Emoji:       "📊",
		Tier:        "silver",
		Unlocked:    budgetUnlocked,
		Progress:    float64(len(trends.ImprovingCategories)) / 2 * 100,
	})

	// Consistent Spender
	consistentUnlocked := trends.SpendingTrendPercentage > -20 && trends.SpendingTrendPercentage < 20
	achievements = append(achievements, &pb.WrappedAchievement{
		Id:          "consistent_spender",
		Name:        "Consistent Spender",
		Description: "Keep spending variation under 20%",
		Emoji:       "⚖️",
		Tier:        "bronze",
		Unlocked:    consistentUnlocked,
		Progress:    100 - min(abs(trends.SpendingTrendPercentage)/20*100, 100),
	})

	// Active Tracker
	activeUnlocked := summary.TransactionCount >= 50
	activeTier := "bronze"
	if summary.TransactionCount >= 200 {
		activeTier = "gold"
	} else if summary.TransactionCount >= 100 {
		activeTier = "silver"
	}
	achievements = append(achievements, &pb.WrappedAchievement{
		Id:          "active_tracker",
		Name:        "Active Tracker",
		Description: "Log 50+ transactions",
		Emoji:       "📝",
		Tier:        activeTier,
		Unlocked:    activeUnlocked,
		Progress:    min(float64(summary.TransactionCount)/50*100, 100),
	})

	// Diversified Spender
	diversifiedUnlocked := len(categories) >= 4
	achievements = append(achievements, &pb.WrappedAchievement{
		Id:          "diversified_spender",
		Name:        "Diversified Spender",
		Description: "Spend across 4+ categories",
		Emoji:       "🎯",
		Tier:        "bronze",
		Unlocked:    diversifiedUnlocked,
		Progress:    min(float64(len(categories))/4*100, 100),
	})

	return achievements
}

// generateAIInsights calls the AI service to generate personalized insights
func (s *WrappedService) generateAIInsights(ctx context.Context, userID uint, accessToken string, summary *pb.WrappedSummary, categories []*pb.WrappedCategoryRanking, merchants []*pb.WrappedMerchantRanking, startDate, endDate time.Time) (*pb.WrappedAIInsights, error) {
	// Prepare context for AI
	categoryData := make([]map[string]interface{}, len(categories))
	for i, c := range categories {
		categoryData[i] = map[string]interface{}{
			"name":       c.CategoryName,
			"amount":     c.Amount,
			"percentage": c.Percentage,
		}
	}

	merchantData := make([]map[string]interface{}, len(merchants))
	for i, m := range merchants {
		merchantData[i] = map[string]interface{}{
			"name":   m.MerchantName,
			"amount": m.Amount,
			"visits": m.VisitCount,
		}
	}

	contextData := map[string]interface{}{
		"period": map[string]string{
			"start": startDate.Format("2006-01-02"),
			"end":   endDate.Format("2006-01-02"),
		},
		"summary": map[string]interface{}{
			"total_spent":       summary.TotalSpent,
			"total_earned":      summary.TotalEarned,
			"total_saved":       summary.TotalSaved,
			"savings_rate":      summary.SavingsRate,
			"transaction_count": summary.TransactionCount,
			"avg_transaction":   summary.AverageTransaction,
		},
		"top_categories": categoryData,
		"top_merchants":  merchantData,
	}

	query := `Generate a celebratory, non-judgmental financial wrapped summary for the user.

IMPORTANT: Respond with ONLY a valid JSON object (no markdown, no backticks, no extra text) in this exact format:
{
  "overall_summary": "A celebratory 1-2 sentence summary of their financial year",
  "personality_traits": ["Trait 1", "Trait 2", "Trait 3"],
  "financial_persona": "The [Creative Name] - a short catchy persona name",
  "spending_style": "A positive description of their spending habits",
  "personalized_insights": ["Insight 1 with emoji", "Insight 2 with emoji", "Insight 3 with emoji"],
  "encouragement": "An encouraging message about their financial journey"
}

Personality traits should be fun like "Coffee Connoisseur", "Travel Bug", "Foodie Explorer", etc.
Keep the tone celebratory and positive - this is their financial wrapped, like Spotify Wrapped!`

	// Call AI service
	payload := map[string]interface{}{
		"query":       query,
		"context":     contextData,
		"user_id":     fmt.Sprintf("%d", userID),
		"model":       "gpt-4",
		"max_tokens":  1000,
		"temperature": 0.8,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	aiServiceURL := s.config.AiServiceURL + "/api/wrapped/generate-insights"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", aiServiceURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI service returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var aiResp struct {
		OverallSummary       string   `json:"overall_summary"`
		PersonalityTraits    []string `json:"personality_traits"`
		FinancialPersona     string   `json:"financial_persona"`
		SpendingStyle        string   `json:"spending_style"`
		PersonalizedInsights []string `json:"personalized_insights"`
		Encouragement        string   `json:"encouragement"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		return nil, fmt.Errorf("failed to decode AI response: %w", err)
	}

	return &pb.WrappedAIInsights{
		OverallSummary:       aiResp.OverallSummary,
		PersonalityTraits:    aiResp.PersonalityTraits,
		FinancialPersona:     aiResp.FinancialPersona,
		SpendingStyle:        aiResp.SpendingStyle,
		PersonalizedInsights: aiResp.PersonalizedInsights,
		Encouragement:        aiResp.Encouragement,
	}, nil
}

// getFallbackAIInsights returns generic insights when AI fails
func (s *WrappedService) getFallbackAIInsights(summary *pb.WrappedSummary, categories []*pb.WrappedCategoryRanking) *pb.WrappedAIInsights {
	traits := make([]string, 0)
	insights := make([]string, 0)

	// Generate traits based on categories
	for _, cat := range categories {
		switch cat.CategoryName {
		case "Food & Dining":
			traits = append(traits, "Foodie Explorer 🍕")
		case "Travel":
			traits = append(traits, "Travel Bug ✈️")
		case "Shopping":
			traits = append(traits, "Retail Therapist 🛍️")
		case "Entertainment":
			traits = append(traits, "Fun Seeker 🎬")
		case "Groceries":
			traits = append(traits, "Home Chef 🥗")
		}
	}
	if len(traits) > 3 {
		traits = traits[:3]
	}

	// Generate insights
	if summary.SavingsRate >= 20 {
		insights = append(insights, "💪 You're saving like a pro! Keep it up!")
	}
	if summary.TransactionCount > 100 {
		insights = append(insights, "📊 You're on top of tracking your finances!")
	}
	if len(categories) >= 3 {
		insights = append(insights, fmt.Sprintf("🎯 %s was your top spending category", categories[0].CategoryName))
	}

	persona := "The Balanced Spender"
	if summary.SavingsRate >= 30 {
		persona = "The Super Saver"
	} else if summary.TransactionCount > 150 {
		persona = "The Active Tracker"
	}

	return &pb.WrappedAIInsights{
		OverallSummary:       fmt.Sprintf("You had an amazing financial journey! You spent $%.2f and saved $%.2f.", summary.TotalSpent, summary.TotalSaved),
		PersonalityTraits:    traits,
		FinancialPersona:     persona,
		SpendingStyle:        "You have a balanced approach to spending and saving.",
		PersonalizedInsights: insights,
		Encouragement:        "Keep up the great work! Every step counts on your financial journey. 🚀",
	}
}

// Helper functions

func getCategoryEmoji(category string) string {
	emojis := map[string]string{
		"EXPENSE_CATEGORY_FOOD_DINING":     "🍔",
		"EXPENSE_CATEGORY_GROCERIES":       "🛒",
		"EXPENSE_CATEGORY_TRANSPORTATION":  "🚗",
		"EXPENSE_CATEGORY_SHOPPING":        "🛍️",
		"EXPENSE_CATEGORY_ENTERTAINMENT":   "🎬",
		"EXPENSE_CATEGORY_BILLS_UTILITIES": "💡",
		"EXPENSE_CATEGORY_HEALTHCARE":      "🏥",
		"EXPENSE_CATEGORY_EDUCATION":       "📚",
		"EXPENSE_CATEGORY_TRAVEL":          "✈️",
		"EXPENSE_CATEGORY_RENT_MORTGAGE":   "🏠",
		"EXPENSE_CATEGORY_INSURANCE":       "🛡️",
		"EXPENSE_CATEGORY_INVESTMENTS":     "📈",
		"EXPENSE_CATEGORY_GIFTS_DONATIONS": "🎁",
		"EXPENSE_CATEGORY_PERSONAL_CARE":   "💄",
		"EXPENSE_CATEGORY_SUBSCRIPTIONS":   "📺",
	}
	if emoji, ok := emojis[category]; ok {
		return emoji
	}
	return "💵"
}

func getMerchantEmoji(merchant, category string) string {
	// Could expand this with merchant-specific emojis
	return getCategoryEmoji(category)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
