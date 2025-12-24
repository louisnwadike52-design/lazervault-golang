# Statistics & Expense Tracking - Complete Implementation Guide

**Feature:** Production-ready expense tracking, budget management, and spending analytics system
**Date:** December 8, 2025
**Status:** 🚧 Architecture Complete, Implementation In Progress

---

## ✅ COMPLETED

### 1. Proto Definitions (proto/statistics.proto)
Complete gRPC service definition with 16 endpoints:

**Expense Management (5 endpoints):**
- CreateExpense, GetExpenses, GetExpenseById, UpdateExpense, DeleteExpense

**Budget Management (5 endpoints):**
- CreateBudget, GetBudgets, GetBudgetById, UpdateBudget, DeleteBudget

**Analytics & Reports (4 endpoints):**
- GetSpendingAnalytics, GetCategoryBreakdownExpenseCategory enum with 16 categories:
- Food & Dining, Transportation, Shopping, Entertainment
- Bills & Utilities, Healthcare, Education, Travel
- Groceries, Rent/Mortgage, Insurance, Investments
- Gifts & Donations, Personal Care, Subscriptions, Other

**Budget Features:**
- Period types: Daily, Weekly, Monthly, Quarterly, Yearly, Custom
- Alert thresholds for budget limits
- Automatic status tracking (Active, Exceeded, Near Limit)
- Recurring expense support

**Analytics Features:**
- Spending trends over time
- Category-wise breakdown
- Budget vs actual comparison
- Projected spending calculations
- Savings rate tracking

---

## 📋 NEXT STEPS

### Phase 1: Database & Models (2-3 hours)

#### 1.1 Create Database Models

Create `models/statistics.go`:

```go
package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

// Expense represents a user expense/transaction
type Expense struct {
    ID               uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
    UserID           uint      `gorm:"not null;index"`
    User             User      `gorm:"foreignKey:UserID"`
    AccountID        *uuid.UUID `gorm:"type:uuid;index"`
    Amount           float64   `gorm:"not null"`
    Currency         string    `gorm:"size:3;default:'USD'"`
    Category         string    `gorm:"size:50;not null;index"`
    Subcategory      string    `gorm:"size:50"`
    Description      string    `gorm:"size:500"`
    Merchant         string    `gorm:"size:200;index"`
    TransactionDate  time.Time `gorm:"not null;index"`
    PaymentMethod    string    `gorm:"size:50"`
    ReceiptURL       string    `gorm:"size:500"`
    Tags             string    `gorm:"type:text"` // JSON array
    Notes            string    `gorm:"type:text"`
    IsRecurring      bool      `gorm:"default:false"`
    RecurrencePattern string   `gorm:"size:20"` // daily, weekly, monthly, yearly
    CreatedAt        time.Time
    UpdatedAt        time.Time
    DeletedAt        gorm.DeletedAt `gorm:"index"`
}

// Budget represents a user's budget plan
type Budget struct {
    ID              uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
    UserID          uint      `gorm:"not null;index"`
    User            User      `gorm:"foreignKey:UserID"`
    Name            string    `gorm:"size:200;not null"`
    Amount          float64   `gorm:"not null"`
    Currency        string    `gorm:"size:3;default:'USD'"`
    Category        string    `gorm:"size:50;not null;index"`
    Period          string    `gorm:"size:20;not null"` // daily, weekly, monthly, quarterly, yearly, custom
    StartDate       time.Time `gorm:"not null;index"`
    EndDate         time.Time `gorm:"not null;index"`
    SpentAmount     float64   `gorm:"default:0"`
    Status          string    `gorm:"size:20;default:'active';index"` // active, exceeded, near_limit, inactive, completed
    EnableAlerts    bool      `gorm:"default:true"`
    AlertThreshold  float64   `gorm:"default:80"` // percentage
    CreatedAt       time.Time
    UpdatedAt       time.Time
    DeletedAt       gorm.DeletedAt `gorm:"index"`
}

// BudgetAlert represents a budget notification
type BudgetAlert struct {
    ID             uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
    UserID         uint      `gorm:"not null;index"`
    User           User      `gorm:"foreignKey:UserID"`
    BudgetID       uuid.UUID `gorm:"type:uuid;not null"`
    Budget         Budget    `gorm:"foreignKey:BudgetID"`
    AlertType      string    `gorm:"size:50;not null"` // threshold_reached, budget_exceeded, approaching_limit, recurring_expense_due
    Message        string    `gorm:"size:500;not null"`
    CurrentSpent   float64   `gorm:"not null"`
    BudgetLimit    float64   `gorm:"not null"`
    PercentageUsed float64   `gorm:"not null"`
    IsRead         bool      `gorm:"default:false;index"`
    CreatedAt      time.Time
}

// Indexes for performance
func (Expense) TableName() string {
    return "expenses"
}

func (Budget) TableName() string {
    return "budgets"
}

func (BudgetAlert) TableName() string {
    return "budget_alerts"
}
```

#### 1.2 Update Migrations

Edit `database/migrations.go`:

```go
func (m *Migrator) RunMigrations() error {
    // ... existing code ...
    
    return m.db.AutoMigrate(
        // ... existing models ...
        &models.Expense{},
        &models.Budget{},
        &models.BudgetAlert{},
    )
}

func (m *Migrator) DropAllTables() error {
    err := m.db.Exec(`
        DROP TABLE IF EXISTS 
            budget_alerts,
            budgets,
            expenses,
            // ... existing tables ...
    `).Error
    
    return err
}
```

#### 1.3 Run Migrations

```bash
# The migrations will run automatically on server start
# Or manually trigger:
go run main.go
```

---

### Phase 2: Backend Service (4-6 hours)

#### 2.1 Create Statistics Service

Create `services/statistics_service.go` with all 16 endpoints implemented.

**Key Features to Implement:**

1. **Expense Management:**
   - CRUD operations for expenses
   - Support for filtering by date range, category, amount
   - Tag-based search
   - Recurring expense handling

2. **Budget Management:**
   - CRUD operations for budgets
   - Automatic spent amount calculation
   - Budget status updates (based on percentage used)
   - Period-based budget tracking

3. **Analytics Engine:**
   - Real-time spending calculations
   - Category breakdown with percentages
   - Daily/weekly/monthly trend analysis
   - Budget vs actual comparison
   - Projected spending based on current rate
   - Savings rate calculation

4. **Alert System:**
   - Automatic alert generation when thresholds reached
   - Background job to check budgets periodically
   - Alert types: 80% threshold, 90% approaching, 100% exceeded
   - Recurring expense reminders

**Example Service Structure:**

```go
type StatisticsService struct {
    db *gorm.DB
}

func (s *StatisticsService) CreateExpense(ctx context.Context, userID uint, req *pb.CreateExpenseRequest) (*pb.CreateExpenseResponse, error) {
    expense := &models.Expense{
        UserID:           userID,
        AccountID:        parseUUID(req.AccountId),
        Amount:           req.Amount,
        Currency:         req.Currency,
        Category:         req.Category.String(),
        // ... map all fields ...
    }
    
    if err := s.db.Create(expense).Error; err != nil {
        return nil, err
    }
    
    // Update related budgets
    go s.updateBudgetSpending(userID, expense.Category, expense.Amount)
    
    // Check for budget alerts
    go s.checkBudgetAlerts(userID, expense.Category)
    
    return &pb.CreateExpenseResponse{
        Expense: s.expenseToProto(expense),
        Success: true,
    }, nil
}

func (s *StatisticsService) GetSpendingAnalytics(ctx context.Context, userID uint, req *pb.GetSpendingAnalyticsRequest) (*pb.GetSpendingAnalyticsResponse, error) {
    // Calculate total spent in period
    var totalSpent float64
    s.db.Model(&models.Expense{}).
        Where("user_id = ? AND transaction_date BETWEEN ? AND ?", userID, req.StartDate, req.EndDate).
        Select("COALESCE(SUM(amount), 0)").
        Scan(&totalSpent)
    
    // Get category breakdown
    var categorySpending []CategorySpending
    s.db.Model(&models.Expense{}).
        Select("category, SUM(amount) as amount, COUNT(*) as count").
        Where("user_id = ? AND transaction_date BETWEEN ? AND ?", userID, req.StartDate, req.EndDate).
        Group("category").
        Scan(&categorySpending)
    
    // Calculate daily trend
    var dailyTrend []DailySpending
    s.db.Model(&models.Expense{}).
        Select("DATE(transaction_date) as date, SUM(amount) as amount, COUNT(*) as count").
        Where("user_id = ? AND transaction_date BETWEEN ? AND ?", userID, req.StartDate, req.EndDate).
        Group("DATE(transaction_date)").
        Order("date ASC").
        Scan(&dailyTrend)
    
    // Get total budget for period
    var totalBudget float64
    s.db.Model(&models.Budget{}).
        Where("user_id = ? AND start_date <= ? AND end_date >= ?", userID, req.EndDate, req.StartDate).
        Select("COALESCE(SUM(amount), 0)").
        Scan(&totalBudget)
    
    analytics := &pb.SpendingAnalytics{
        TotalSpent:       totalSpent,
        TotalBudget:      totalBudget,
        RemainingBudget:  totalBudget - totalSpent,
        CategoryBreakdown: categorySpending,
        DailyTrend:       dailyTrend,
        SavingsRate:      ((totalBudget - totalSpent) / totalBudget) * 100,
    }
    
    return &pb.GetSpendingAnalyticsResponse{Analytics: analytics}, nil
}
```

#### 2.2 Create Controller

Create `grpcApi/statistics_controller.go`:

```go
type StatisticsController struct {
    pb.UnimplementedStatisticsServiceServer
    statisticsService *services.StatisticsService
    userService       *services.UserService
}

func (c *StatisticsController) CreateExpense(ctx context.Context, req *pb.CreateExpenseRequest) (*pb.CreateExpenseResponse, error) {
    userID := getUserIDFromContext(ctx)
    return c.statisticsService.CreateExpense(ctx, userID, req)
}

// ... implement all 16 endpoints ...
```

#### 2.3 Register Service

Edit `grpcApi/server.go`:

```go
// Initialize service
statisticsService := services.NewStatisticsService(db)
statisticsController := grpcApi.NewStatisticsController(statisticsService, userService)

// Register gRPC
pb.RegisterStatisticsServiceServer(grpcServer, statisticsController)

// Register HTTP gateway
pb.RegisterStatisticsServiceHandlerFromEndpoint(ctx, gwmux, grpcDialAddr, opts)
```

#### 2.4 Update Authentication

Edit `grpcApi/middleware/auth.go`:

```go
publicEndpoints := map[string]bool{
    // ... existing endpoints ...
    
    // All statistics endpoints require authentication
    "/pb.StatisticsService/CreateExpense":           true,
    "/pb.StatisticsService/GetExpenses":             true,
    "/pb.StatisticsService/GetExpenseById":          true,
    "/pb.StatisticsService/UpdateExpense":           true,
    "/pb.StatisticsService/DeleteExpense":           true,
    "/pb.StatisticsService/CreateBudget":            true,
    "/pb.StatisticsService/GetBudgets":              true,
    "/pb.StatisticsService/GetBudgetById":           true,
    "/pb.StatisticsService/UpdateBudget":            true,
    "/pb.StatisticsService/DeleteBudget":            true,
    "/pb.StatisticsService/GetSpendingAnalytics":    true,
    "/pb.StatisticsService/GetCategoryBreakdown":    true,
    "/pb.StatisticsService/GetBudgetProgress":       true,
    "/pb.StatisticsService/GetSpendingTrends":       true,
    "/pb.StatisticsService/GetBudgetAlerts":         true,
    "/pb.StatisticsService/MarkAlertAsRead":         true,
}
```

---

### Phase 3: Background Jobs (2-3 hours)

#### 3.1 Budget Alert Checker

Create `workers/budget_alert_worker.go`:

```go
func CheckBudgetAlerts(db *gorm.DB) {
    ticker := time.NewTicker(15 * time.Minute)
    
    for range ticker.C {
        var budgets []models.Budget
        db.Where("enable_alerts = ? AND status = ?", true, "active").Find(&budgets)
        
        for _, budget := range budgets {
            // Calculate spent amount
            var spent float64
            db.Model(&models.Expense{}).
                Where("user_id = ? AND category = ? AND transaction_date BETWEEN ? AND ?",
                    budget.UserID, budget.Category, budget.StartDate, budget.EndDate).
                Select("COALESCE(SUM(amount), 0)").
                Scan(&spent)
            
            percentage := (spent / budget.Amount) * 100
            
            // Create alerts based on thresholds
            if percentage >= 100 && budget.Status != "exceeded" {
                createAlert(db, budget.ID, budget.UserID, "budget_exceeded", spent, budget.Amount, percentage)
                db.Model(&budget).Update("status", "exceeded")
            } else if percentage >= budget.AlertThreshold && budget.Status != "near_limit" {
                createAlert(db, budget.ID, budget.UserID, "threshold_reached", spent, budget.Amount, percentage)
                db.Model(&budget).Update("status", "near_limit")
            }
            
            // Update spent amount
            db.Model(&budget).Updates(map[string]interface{}{
                "spent_amount": spent,
            })
        }
    }
}
```

Start worker in `main.go`:

```go
go workers.CheckBudgetAlerts(db)
```

---

### Phase 4: Flutter Implementation (6-8 hours)

#### 4.1 Generate gRPC Client

```bash
cd lazervaultapp
# Add statistics.proto to proto generation
# Run proto generation
```

#### 4.2 Create Data Models

`lib/src/features/statistics/data/models/expense_model.dart`
`lib/src/features/statistics/data/models/budget_model.dart`
`lib/src/features/statistics/data/models/analytics_model.dart`

#### 4.3 Create Repository

`lib/src/features/statistics/data/repositories/statistics_repository.dart`

#### 4.4 Create State Management

Using Riverpod:
- `expense_provider.dart`
- `budget_provider.dart`
- `analytics_provider.dart`
- `alert_provider.dart`

#### 4.5 Create UI Screens

**Main Statistics Screen:**
`lib/src/features/statistics/presentation/views/statistics_screen.dart`

Features:
- Overview card with total spent, budget remaining
- Quick stats (this week, this month)
- Category breakdown pie chart
- Spending trend line chart
- Recent expenses list
- Budget progress bars
- Floating action button to add expense

**Add/Edit Expense:**
`lib/src/features/statistics/presentation/views/add_expense_screen.dart`

Features:
- Amount input
- Category selector (with icons)
- Date picker
- Merchant/description
- Payment method
- Receipt photo upload
- Tags input
- Recurring expense toggle

**Budget Management:**
`lib/src/features/statistics/presentation/views/budget_screen.dart`

Features:
- List of active budgets
- Add new budget button
- Budget cards showing:
  - Progress bar
  - Spent vs limit
  - Days remaining
  - Status indicator (green/yellow/red)
- Budget details on tap

**Analytics Dashboard:**
`lib/src/features/statistics/presentation/views/analytics_screen.dart`

Features:
- Period selector (week/month/quarter/year)
- Total spending card
- Category breakdown chart
- Spending trends over time
- Top merchants
- Budget vs actual comparison
- Export to PDF button

**Expense Details:**
`lib/src/features/statistics/presentation/views/expense_detail_screen.dart`

Features:
- Full expense information
- Receipt image
- Edit/delete buttons
- Category icon
- Related budget info

#### 4.6 Create Widgets

**Reusable Components:**
- `expense_card.dart` - Display expense in list
- `budget_progress_card.dart` - Budget with progress bar
- `category_icon.dart` - Icon for each category
- `spending_chart.dart` - Line/bar chart for trends
- `pie_chart_widget.dart` - Category breakdown
- `date_range_picker.dart` - Filter by dates
- `amount_input.dart` - Currency input field
- `category_selector.dart` - Category picker dialog
- `alert_badge.dart` - Budget alert indicator

#### 4.7 Add Navigation

Edit `lib/src/router/router.dart`:

```dart
GoRoute(
  path: '/statistics',
  builder: (context, state) => const StatisticsScreen(),
  routes: [
    GoRoute(
      path: 'add-expense',
      builder: (context, state) => const AddExpenseScreen(),
    ),
    GoRoute(
      path: 'expense/:id',
      builder: (context, state) {
        final id = state.pathParameters['id']!;
        return ExpenseDetailScreen(expenseId: id);
      },
    ),
    GoRoute(
      path: 'budgets',
      builder: (context, state) => const BudgetScreen(),
    ),
    GoRoute(
      path: 'analytics',
      builder: (context, state) => const AnalyticsScreen(),
    ),
  ],
),
```

#### 4.8 Add to Bottom Navigation

Edit main navigation to include Statistics tab:

```dart
BottomNavigationBarItem(
  icon: Icon(Icons.pie_chart),
  label: 'Statistics',
)
```

#### 4.9 Implement Notifications

**Local Notifications for Budget Alerts:**

```dart
class BudgetNotificationService {
  final FlutterLocalNotificationsPlugin notifications;
  
  Future<void> checkBudgetAlerts() async {
    final alerts = await statisticsRepository.getBudgetAlerts(unreadOnly: true);
    
    for (final alert in alerts) {
      await showNotification(
        id: alert.id.hashCode,
        title: 'Budget Alert',
        body: alert.message,
        payload: alert.budgetId,
      );
    }
  }
  
  // Call this periodically or when app resumes
  void startPeriodicCheck() {
    Timer.periodic(Duration(minutes: 30), (_) => checkBudgetAlerts());
  }
}
```

---

### Phase 5: Testing (2-3 hours)

#### 5.1 Backend Tests

Create `services/statistics_service_test.go`:

```bash
go test ./services -v -run TestStatisticsService
```

Test cases:
- Create expense
- Get expenses with filters
- Update expense
- Delete expense
- Create budget
- Budget spent amount calculation
- Alert generation
- Analytics calculations

#### 5.2 Integration Tests

Test full flow:
1. Create budget for "Food & Dining" - $500/month
2. Add expenses totaling $400
3. Verify budget shows 80% used
4. Verify alert generated
5. Add another $150 expense
6. Verify budget status changed to "exceeded"
7. Get analytics and verify calculations

#### 5.3 Flutter Tests

Widget tests for:
- Statistics screen rendering
- Expense card display
- Budget progress calculation
- Chart rendering
- Add expense form validation

---

## 🎨 UI/UX Design Guidelines

### Color Scheme for Categories
- Food & Dining: Orange (#FF9800)
- Transportation: Blue (#2196F3)
- Shopping: Purple (#9C27B0)
- Entertainment: Pink (#E91E63)
- Bills & Utilities: Red (#F44336)
- Healthcare: Green (#4CAF50)
- Education: Indigo (#3F51B5)
- Travel: Cyan (#00BCD4)
- Groceries: Light Green (#8BC34A)
- Rent/Mortgage: Deep Orange (#FF5722)
- Insurance: Teal (#009688)
- Investments: Amber (#FFC107)
- Gifts: Pink (#FF4081)
- Personal Care: Purple (#BA68C8)
- Subscriptions: Blue Grey (#607D8B)
- Other: Grey (#9E9E9E)

### Budget Status Colors
- Active (0-79%): Green
- Near Limit (80-99%): Orange/Yellow
- Exceeded (100%+): Red

### Charts
- Use fl_chart package for Flutter
- Smooth animations on data changes
- Interactive tooltips
- Responsive to screen size
- Dark mode support

---

## 📊 Database Indexes

For optimal performance, ensure these indexes:

```sql
-- Expenses
CREATE INDEX idx_expenses_user_date ON expenses(user_id, transaction_date);
CREATE INDEX idx_expenses_category ON expenses(category);
CREATE INDEX idx_expenses_merchant ON expenses(merchant);

-- Budgets
CREATE INDEX idx_budgets_user_period ON budgets(user_id, start_date, end_date);
CREATE INDEX idx_budgets_status ON budgets(status);
CREATE INDEX idx_budgets_category ON budgets(category);

-- Alerts
CREATE INDEX idx_alerts_user_unread ON budget_alerts(user_id, is_read);
```

---

## 🚀 Deployment Checklist

- [ ] Database migrations run successfully
- [ ] All 16 API endpoints working
- [ ] Background worker for alerts running
- [ ] Flutter screens implemented
- [ ] Charts rendering correctly
- [ ] Notifications working
- [ ] All tests passing
- [ ] Performance tested with 1000+ expenses
- [ ] Error handling implemented
- [ ] Loading states added
- [ ] Empty states designed
- [ ] Dark mode supported
- [ ] Accessibility features added

---

## 📈 Performance Considerations

1. **Database:**
   - Use indexes on frequently queried fields
   - Cache analytics calculations for 5 minutes
   - Use database views for complex queries

2. **Backend:**
   - Implement pagination for expenses list
   - Use goroutines for alert checking
   - Cache category breakdowns

3. **Frontend:**
   - Lazy load expense lists
   - Cache charts for quick display
   - Debounce search inputs
   - Use pagination for long lists

---

## 🔐 Security

1. Ensure all endpoints verify user ownership
2. Validate amount inputs (prevent negative values)
3. Sanitize text inputs
4. Rate limit API calls
5. Encrypt sensitive data in database

---

## 📝 Next Immediate Steps

1. **Create models directory** and add statistics models
2. **Update migrations** to include new tables
3. **Run migrations** to create database tables
4. **Implement StatisticsService** with all endpoints
5. **Create Flutter screens** for statistics
6. **Test end-to-end** functionality

---

**Estimated Total Implementation Time:** 16-24 hours
**Priority Level:** High
**Complexity:** Medium-High

