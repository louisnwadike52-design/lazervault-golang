# LazerVault Insurance System Implementation

## Overview

This document describes the complete implementation of the insurance management system for LazerVault, following the existing project architecture and patterns. The system provides comprehensive insurance policy management, payment processing, claims handling, and statistical reporting.

## Architecture

The insurance system follows the same layered architecture as the existing LazerVault project:

```
┌─────────────────────────────────────────────────────────────┐
│                    gRPC Controllers                         │
│                 (grpcApi/insurance_controller.go)           │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Service Layer                            │
│                (services/insurance_service.go)              │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Data Models                              │
│                  (models/insurance.go)                      │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Database Layer                           │
│                    (GORM + PostgreSQL)                      │
└─────────────────────────────────────────────────────────────┘
```

## Features Implemented

### 1. Insurance Policy Management
- ✅ Create insurance policies
- ✅ Retrieve user insurance policies with pagination
- ✅ Get insurance by ID
- ✅ Update insurance policies
- ✅ Delete insurance policies
- ✅ Search insurance policies
- ✅ Support for multiple insurance types (health, auto, home, life, travel, business)

### 2. Payment Management
- ✅ Create insurance payments
- ✅ Process payments with multiple payment methods
- ✅ Retrieve payment history
- ✅ Get overdue payments
- ✅ Payment status tracking (pending, processing, completed, failed, cancelled, refunded)
- ✅ Receipt generation

### 3. Claims Management
- ✅ Create insurance claims
- ✅ Update claims
- ✅ Retrieve claim history
- ✅ Claim status tracking (submitted, under_review, approved, rejected, settled)
- ✅ Support for attachments and documents

### 4. Statistics and Reporting
- ✅ Insurance statistics (total policies, active policies, coverage amounts)
- ✅ Payment statistics (total payments, completed payments, amounts by method)
- ✅ Policies by type breakdown
- ✅ Payment method breakdown

## Database Schema

### Insurance Policies Table
```sql
CREATE TABLE insurances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_number VARCHAR(50) UNIQUE NOT NULL,
    policy_holder_name VARCHAR(255) NOT NULL,
    policy_holder_email VARCHAR(255) NOT NULL,
    policy_holder_phone VARCHAR(20) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('health', 'auto', 'home', 'life', 'travel', 'business')),
    provider VARCHAR(255) NOT NULL,
    provider_logo TEXT,
    premium_amount DECIMAL(15,2) NOT NULL,
    coverage_amount DECIMAL(15,2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    start_date TIMESTAMP NOT NULL,
    end_date TIMESTAMP NOT NULL,
    next_payment_date TIMESTAMP NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('active', 'pending', 'expired', 'cancelled', 'suspended')),
    beneficiaries JSONB NOT NULL DEFAULT '[]',
    coverage_details JSONB NOT NULL DEFAULT '{}',
    description TEXT,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### Insurance Payments Table
```sql
CREATE TABLE insurance_payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    insurance_id UUID NOT NULL REFERENCES insurances(id) ON DELETE CASCADE,
    policy_number VARCHAR(50) NOT NULL,
    amount DECIMAL(15,2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    payment_method VARCHAR(20) NOT NULL CHECK (payment_method IN ('bank_transfer', 'card', 'mobile_money', 'crypto', 'wallet')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled', 'refunded')),
    transaction_id VARCHAR(100),
    reference_number VARCHAR(100),
    payment_date TIMESTAMP NOT NULL DEFAULT NOW(),
    due_date TIMESTAMP NOT NULL,
    processed_at TIMESTAMP,
    payment_details JSONB,
    failure_reason TEXT,
    receipt_url TEXT,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### Insurance Claims Table
```sql
CREATE TABLE insurance_claims (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    claim_number VARCHAR(50) UNIQUE NOT NULL,
    insurance_id UUID NOT NULL REFERENCES insurances(id) ON DELETE CASCADE,
    policy_number VARCHAR(50) NOT NULL,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted', 'under_review', 'approved', 'rejected', 'settled')),
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    claim_amount DECIMAL(15,2) NOT NULL,
    approved_amount DECIMAL(15,2),
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    incident_date TIMESTAMP NOT NULL,
    incident_location TEXT NOT NULL,
    attachments JSONB NOT NULL DEFAULT '[]',
    documents JSONB NOT NULL DEFAULT '[]',
    additional_info JSONB NOT NULL DEFAULT '{}',
    rejection_reason TEXT,
    settlement_date TIMESTAMP,
    settlement_details TEXT,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

## API Endpoints

### Insurance Policy Management

#### Get User Insurances
```
GET /v1/insurance?page=1&limit=10
Authorization: Bearer <token>
```

#### Get Insurance by ID
```
GET /v1/insurance/{id}
Authorization: Bearer <token>
```

#### Create Insurance
```
POST /v1/insurance
Authorization: Bearer <token>
Content-Type: application/json

{
  "policy_holder_name": "John Doe",
  "policy_holder_email": "john@example.com",
  "policy_holder_phone": "+1234567890",
  "type": "health",
  "provider": "Test Insurance Co",
  "premium_amount": 299.99,
  "coverage_amount": 50000.00,
  "start_date": "2024-01-01",
  "end_date": "2025-01-01"
}
```

#### Update Insurance
```
PUT /v1/insurance
Authorization: Bearer <token>
Content-Type: application/json

{
  "id": "uuid",
  "policy_holder_name": "John Doe Updated",
  "premium_amount": 350.00
}
```

#### Delete Insurance
```
DELETE /v1/insurance/{id}
Authorization: Bearer <token>
```

#### Search Insurances
```
GET /v1/insurance/search?query=health&page=1&limit=10
Authorization: Bearer <token>
```

### Payment Management

#### Get Insurance Payments
```
GET /v1/insurance/{insurance_id}/payments?page=1&limit=10
Authorization: Bearer <token>
```

#### Get User Payments
```
GET /v1/insurance/payments?page=1&limit=10
Authorization: Bearer <token>
```

#### Create Payment
```
POST /v1/insurance/payments
Authorization: Bearer <token>
Content-Type: application/json

{
  "insurance_id": "uuid",
  "amount": 299.99,
  "payment_method": "card",
  "due_date": "2024-02-01T00:00:00Z"
}
```

#### Process Payment
```
POST /v1/insurance/payments/{payment_id}/process
Authorization: Bearer <token>
Content-Type: application/json

{
  "payment_method": "card",
  "payment_details": {
    "card_number": "4111111111111111",
    "expiry": "12/25"
  }
}
```

#### Get Overdue Payments
```
GET /v1/insurance/payments/overdue
Authorization: Bearer <token>
```

### Claims Management

#### Get Insurance Claims
```
GET /v1/insurance/{insurance_id}/claims?page=1&limit=10
Authorization: Bearer <token>
```

#### Get User Claims
```
GET /v1/insurance/claims?page=1&limit=10
Authorization: Bearer <token>
```

#### Create Claim
```
POST /v1/insurance/claims
Authorization: Bearer <token>
Content-Type: application/json

{
  "insurance_id": "uuid",
  "type": "medical",
  "title": "Medical Emergency",
  "description": "Emergency room visit",
  "claim_amount": 5000.00,
  "incident_date": "2024-01-15",
  "incident_location": "City General Hospital"
}
```

#### Update Claim
```
PUT /v1/insurance/claims
Authorization: Bearer <token>
Content-Type: application/json

{
  "id": "uuid",
  "title": "Updated Title",
  "description": "Updated description"
}
```

### Statistics

#### Get Insurance Statistics
```
GET /v1/insurance/statistics
Authorization: Bearer <token>
```

#### Get Payment Statistics
```
GET /v1/insurance/payments/statistics?start_date=2024-01-01&end_date=2024-12-31
Authorization: Bearer <token>
```

## Data Models

### Insurance Model
```go
type Insurance struct {
    gorm.Model
    ID                string    `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    PolicyNumber      string    `json:"policy_number" gorm:"uniqueIndex;size:50;not null"`
    PolicyHolderName  string    `json:"policy_holder_name" gorm:"size:255;not null"`
    PolicyHolderEmail string    `json:"policy_holder_email" gorm:"size:255;not null"`
    PolicyHolderPhone string    `json:"policy_holder_phone" gorm:"size:20;not null"`
    Type              string    `json:"type" gorm:"size:20;not null;check:type IN ('health', 'auto', 'home', 'life', 'travel', 'business')"`
    Provider          string    `json:"provider" gorm:"size:255;not null"`
    ProviderLogo      string    `json:"provider_logo" gorm:"type:text"`
    PremiumAmount     float64   `json:"premium_amount" gorm:"type:decimal(15,2);not null"`
    CoverageAmount    float64   `json:"coverage_amount" gorm:"type:decimal(15,2);not null"`
    Currency          string    `json:"currency" gorm:"size:3;not null;default:'USD'"`
    StartDate         time.Time `json:"start_date" gorm:"not null"`
    EndDate           time.Time `json:"end_date" gorm:"not null"`
    NextPaymentDate   time.Time `json:"next_payment_date" gorm:"not null"`
    Status            string    `json:"status" gorm:"size:20;not null;default:'pending'"`
    Beneficiaries     JSON      `json:"beneficiaries" gorm:"type:jsonb;default:'[]'"`
    CoverageDetails   JSON      `json:"coverage_details" gorm:"type:jsonb;default:'{}'"`
    Description       *string   `json:"description" gorm:"type:text"`
    UserID            uint      `json:"user_id" gorm:"not null"`
    User              User      `json:"user" gorm:"foreignKey:UserID"`
}
```

### InsurancePayment Model
```go
type InsurancePayment struct {
    gorm.Model
    ID            string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    InsuranceID   string     `json:"insurance_id" gorm:"type:uuid;not null"`
    PolicyNumber  string     `json:"policy_number" gorm:"size:50;not null"`
    Amount        float64    `json:"amount" gorm:"type:decimal(15,2);not null"`
    Currency      string     `json:"currency" gorm:"size:3;not null;default:'USD'"`
    PaymentMethod string     `json:"payment_method" gorm:"size:20;not null"`
    Status        string     `json:"status" gorm:"size:20;not null;default:'pending'"`
    TransactionID *string    `json:"transaction_id" gorm:"size:100"`
    ReferenceNumber *string  `json:"reference_number" gorm:"size:100"`
    PaymentDate   time.Time  `json:"payment_date" gorm:"not null;default:now()"`
    DueDate       time.Time  `json:"due_date" gorm:"not null"`
    ProcessedAt   *time.Time `json:"processed_at"`
    PaymentDetails JSON      `json:"payment_details" gorm:"type:jsonb"`
    FailureReason *string    `json:"failure_reason" gorm:"type:text"`
    ReceiptURL    *string    `json:"receipt_url" gorm:"type:text"`
    UserID        uint       `json:"user_id" gorm:"not null"`
    User          User       `json:"user" gorm:"foreignKey:UserID"`
    Insurance     Insurance  `json:"insurance,omitempty" gorm:"foreignKey:InsuranceID"`
}
```

### InsuranceClaim Model
```go
type InsuranceClaim struct {
    gorm.Model
    ID               string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    ClaimNumber      string     `json:"claim_number" gorm:"uniqueIndex;size:50;not null"`
    InsuranceID      string     `json:"insurance_id" gorm:"type:uuid;not null"`
    PolicyNumber     string     `json:"policy_number" gorm:"size:50;not null"`
    Type             string     `json:"type" gorm:"size:50;not null"`
    Status           string     `json:"status" gorm:"size:20;not null;default:'submitted'"`
    Title            string     `json:"title" gorm:"size:255;not null"`
    Description      string     `json:"description" gorm:"type:text;not null"`
    ClaimAmount      float64    `json:"claim_amount" gorm:"type:decimal(15,2);not null"`
    ApprovedAmount   *float64   `json:"approved_amount" gorm:"type:decimal(15,2)"`
    Currency         string     `json:"currency" gorm:"size:3;not null;default:'USD'"`
    IncidentDate     time.Time  `json:"incident_date" gorm:"not null"`
    IncidentLocation string     `json:"incident_location" gorm:"type:text;not null"`
    Attachments      JSON       `json:"attachments" gorm:"type:jsonb;default:'[]'"`
    Documents        JSON       `json:"documents" gorm:"type:jsonb;default:'[]'"`
    AdditionalInfo   JSON       `json:"additional_info" gorm:"type:jsonb;default:'{}'"`
    RejectionReason  *string    `json:"rejection_reason" gorm:"type:text"`
    SettlementDate   *time.Time `json:"settlement_date"`
    SettlementDetails *string   `json:"settlement_details" gorm:"type:text"`
    UserID           uint       `json:"user_id" gorm:"not null"`
    User             User       `json:"user" gorm:"foreignKey:UserID"`
    Insurance        Insurance  `json:"insurance,omitempty" gorm:"foreignKey:InsuranceID"`
}
```

## Service Layer

The insurance service (`services/insurance_service.go`) provides the business logic for:

- Insurance policy CRUD operations
- Payment processing and management
- Claims handling
- Statistics calculation
- Data validation and business rules

### Key Service Methods

```go
type IInsuranceService interface {
    // Insurance Policy Management
    GetUserInsurances(ctx context.Context, userID uint, page, limit int) ([]models.Insurance, *models.PaginationInfo, error)
    GetInsuranceById(ctx context.Context, id string, userID uint) (*models.Insurance, error)
    CreateInsurance(ctx context.Context, insurance *models.Insurance) error
    UpdateInsurance(ctx context.Context, insurance *models.Insurance, userID uint) error
    DeleteInsurance(ctx context.Context, id string, userID uint) error
    SearchInsurances(ctx context.Context, userID uint, query string, page, limit int) ([]models.Insurance, *models.PaginationInfo, error)
    
    // Payment Management
    GetInsurancePayments(ctx context.Context, insuranceID string, userID uint, page, limit int) ([]models.InsurancePayment, *models.PaginationInfo, error)
    GetUserPayments(ctx context.Context, userID uint, page, limit int) ([]models.InsurancePayment, *models.PaginationInfo, error)
    CreatePayment(ctx context.Context, payment *models.InsurancePayment) error
    ProcessPayment(ctx context.Context, paymentID string, paymentMethod string, paymentDetails map[string]string, userID uint) (*models.InsurancePayment, error)
    GetPaymentById(ctx context.Context, id string, userID uint) (*models.InsurancePayment, error)
    GetOverduePayments(ctx context.Context, userID uint) ([]models.InsurancePayment, error)
    
    // Claims Management
    GetInsuranceClaims(ctx context.Context, insuranceID string, userID uint, page, limit int) ([]models.InsuranceClaim, *models.PaginationInfo, error)
    GetUserClaims(ctx context.Context, userID uint, page, limit int) ([]models.InsuranceClaim, *models.PaginationInfo, error)
    CreateClaim(ctx context.Context, claim *models.InsuranceClaim) error
    UpdateClaim(ctx context.Context, claim *models.InsuranceClaim, userID uint) error
    GetClaimById(ctx context.Context, id string, userID uint) (*models.InsuranceClaim, error)
    
    // Statistics
    GetInsuranceStatistics(ctx context.Context, userID uint) (*models.InsuranceStatistics, error)
    GetPaymentStatistics(ctx context.Context, userID uint, startDate, endDate *time.Time) (*models.PaymentStatistics, error)
}
```

## Controller Layer

The insurance controller (`grpcApi/insurance_controller.go`) handles:

- gRPC request/response conversion
- Authentication and authorization
- Error handling and status codes
- Data validation
- Protocol buffer serialization

## Testing

Comprehensive tests are included in `test/insurance_test.go` covering:

- Insurance policy creation and management
- Payment processing
- Claims handling
- Statistics calculation
- Search functionality
- Error scenarios

### Running Tests

```bash
# Run insurance tests
go test ./test -v -run TestInsuranceFlow

# Run all tests
go test ./test -v
```

## Setup and Installation

### 1. Generate Protobuf Files

```bash
# Make the script executable
chmod +x scripts/generate-insurance-proto.sh

# Generate insurance protobuf files
./scripts/generate-insurance-proto.sh
```

### 2. Run Database Migrations

The insurance tables will be automatically created when you run the application, as they're included in the migrations.

### 3. Start the Server

```bash
go run main.go
```

## Integration with Existing System

The insurance system is fully integrated with the existing LazerVault architecture:

- ✅ Uses existing authentication middleware
- ✅ Follows existing service patterns
- ✅ Uses existing database connection and GORM setup
- ✅ Integrates with existing task distribution system
- ✅ Follows existing error handling patterns
- ✅ Uses existing logging and monitoring

## Security Features

- ✅ User authentication required for all endpoints
- ✅ User authorization (users can only access their own data)
- ✅ Input validation and sanitization
- ✅ SQL injection protection through GORM
- ✅ UUID-based IDs for security
- ✅ Audit trails with created_at/updated_at timestamps

## Performance Considerations

- ✅ Database indexing on frequently queried fields
- ✅ Pagination for large result sets
- ✅ Efficient database queries with proper joins
- ✅ JSONB fields for flexible data storage
- ✅ Connection pooling through GORM

## Future Enhancements

Potential future improvements:

1. **Payment Gateway Integration**: Integrate with real payment processors (Stripe, PayPal, etc.)
2. **Document Management**: Add file upload for claim attachments and documents
3. **Notification System**: Email/SMS notifications for payment reminders and claim updates
4. **Advanced Analytics**: More detailed reporting and analytics
5. **Multi-currency Support**: Enhanced currency handling
6. **Insurance Provider API Integration**: Real-time policy validation
7. **Mobile App Support**: Push notifications and offline capabilities

## API Documentation

The API documentation is automatically generated from the protobuf definitions and available at:

- Swagger UI: `http://localhost:8080/swagger/`
- gRPC Reflection: Available on the gRPC port

## Monitoring and Logging

The insurance system integrates with the existing logging and monitoring infrastructure:

- Structured logging with zerolog
- Error tracking and reporting
- Performance metrics
- Database query monitoring

## Conclusion

The insurance system provides a complete, production-ready solution for insurance management within the LazerVault platform. It follows all existing patterns and conventions, ensuring seamless integration with the current codebase while providing comprehensive functionality for insurance policy management, payments, and claims processing. 