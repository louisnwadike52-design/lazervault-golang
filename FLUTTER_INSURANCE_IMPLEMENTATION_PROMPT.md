# Flutter Insurance System Implementation Prompt

## Backend API Information

### Base Configuration
- **gRPC Server**: Running on port 9090
- **HTTP Gateway**: Running on port 8080
- **Authentication**: JWT Bearer token required for all endpoints
- **Base URL**: `http://localhost:8080` (for HTTP gateway) or `localhost:9090` (for gRPC)

### Authentication
All insurance endpoints require authentication. Include the JWT token in the Authorization header:
```
Authorization: Bearer <jwt_token>
```

## Insurance Models

### Insurance Policy
```dart
class Insurance {
  final String id;
  final String policyNumber;
  final String policyHolderName;
  final String policyHolderEmail;
  final String policyHolderPhone;
  final String type; // 'health', 'auto', 'home', 'life', 'travel', 'business'
  final String provider;
  final String providerLogo;
  final double premiumAmount;
  final double coverageAmount;
  final String currency;
  final DateTime startDate;
  final DateTime endDate;
  final DateTime nextPaymentDate;
  final String status; // 'active', 'pending', 'expired', 'cancelled', 'suspended'
  final List<String> beneficiaries;
  final Map<String, String> coverageDetails;
  final String description;
  final String userId;
  final DateTime createdAt;
  final DateTime updatedAt;
}
```

### Insurance Payment
```dart
class InsurancePayment {
  final String id;
  final String insuranceId;
  final String policyNumber;
  final double amount;
  final String currency;
  final String paymentMethod; // 'bank_transfer', 'card', 'mobile_money', 'crypto', 'wallet'
  final String status; // 'pending', 'processing', 'completed', 'failed', 'cancelled', 'refunded'
  final String transactionId;
  final String referenceNumber;
  final DateTime paymentDate;
  final DateTime dueDate;
  final DateTime? processedAt;
  final Map<String, String> paymentDetails;
  final String? failureReason;
  final String? receiptUrl;
  final String userId;
  final DateTime createdAt;
  final DateTime updatedAt;
}
```

### Insurance Claim
```dart
class InsuranceClaim {
  final String id;
  final String claimNumber;
  final String insuranceId;
  final String policyNumber;
  final String type;
  final String status; // 'submitted', 'under_review', 'approved', 'rejected', 'settled'
  final String title;
  final String description;
  final double claimAmount;
  final double? approvedAmount;
  final String currency;
  final DateTime incidentDate;
  final String incidentLocation;
  final List<String> attachments;
  final List<String> documents;
  final Map<String, String> additionalInfo;
  final String? rejectionReason;
  final DateTime? settlementDate;
  final String? settlementDetails;
  final String userId;
  final DateTime createdAt;
  final DateTime updatedAt;
}
```

## API Endpoints

### Insurance Policy Management

#### 1. Get User Insurances
- **Endpoint**: `GET /v1/insurance`
- **Query Parameters**: 
  - `page` (int, default: 1)
  - `limit` (int, default: 10)
- **Response**: List of insurance policies with pagination info

#### 2. Get Insurance by ID
- **Endpoint**: `GET /v1/insurance/{id}`
- **Response**: Single insurance policy details

#### 3. Create Insurance
- **Endpoint**: `POST /v1/insurance`
- **Body**: Insurance policy data
- **Response**: Created insurance policy

#### 4. Update Insurance
- **Endpoint**: `PUT /v1/insurance`
- **Body**: Updated insurance policy data
- **Response**: Updated insurance policy

#### 5. Delete Insurance
- **Endpoint**: `DELETE /v1/insurance/{id}`
- **Response**: Success message

#### 6. Search Insurances
- **Endpoint**: `GET /v1/insurance/search`
- **Query Parameters**:
  - `query` (string)
  - `page` (int)
  - `limit` (int)
- **Response**: Filtered insurance policies

### Payment Management

#### 1. Get Insurance Payments
- **Endpoint**: `GET /v1/insurance/{insurance_id}/payments`
- **Query Parameters**: `page`, `limit`
- **Response**: List of payments for specific insurance

#### 2. Get User Payments
- **Endpoint**: `GET /v1/insurance/payments`
- **Query Parameters**: `page`, `limit`
- **Response**: All payments for authenticated user

#### 3. Create Payment
- **Endpoint**: `POST /v1/insurance/payments`
- **Body**: Payment data
- **Response**: Created payment

#### 4. Process Payment
- **Endpoint**: `POST /v1/insurance/payments/{payment_id}/process`
- **Body**: Payment method and details
- **Response**: Processed payment with transaction info

#### 5. Get Payment by ID
- **Endpoint**: `GET /v1/insurance/payments/{id}`
- **Response**: Payment details

#### 6. Get Overdue Payments
- **Endpoint**: `GET /v1/insurance/payments/overdue`
- **Response**: List of overdue payments

### Claims Management

#### 1. Get Insurance Claims
- **Endpoint**: `GET /v1/insurance/{insurance_id}/claims`
- **Query Parameters**: `page`, `limit`
- **Response**: Claims for specific insurance

#### 2. Get User Claims
- **Endpoint**: `GET /v1/insurance/claims`
- **Query Parameters**: `page`, `limit`
- **Response**: All claims for authenticated user

#### 3. Create Claim
- **Endpoint**: `POST /v1/insurance/claims`
- **Body**: Claim data
- **Response**: Created claim

#### 4. Update Claim
- **Endpoint**: `PUT /v1/insurance/claims`
- **Body**: Updated claim data
- **Response**: Updated claim

#### 5. Get Claim by ID
- **Endpoint**: `GET /v1/insurance/claims/{id}`
- **Response**: Claim details

### Receipt Management

#### 1. Generate Payment Receipt
- **Endpoint**: `POST /v1/insurance/payments/{payment_id}/receipt`
- **Response**: Receipt URL

#### 2. Get User Receipts
- **Endpoint**: `GET /v1/insurance/receipts`
- **Query Parameters**: `page`, `limit`
- **Response**: List of receipt URLs

### Statistics

#### 1. Get Insurance Statistics
- **Endpoint**: `GET /v1/insurance/statistics`
- **Response**: Insurance statistics (total policies, active policies, etc.)

#### 2. Get Payment Statistics
- **Endpoint**: `GET /v1/insurance/payments/statistics`
- **Query Parameters**: `start_date`, `end_date`
- **Response**: Payment statistics for date range

## UI/UX Requirements

### 1. Insurance Dashboard
- **Overview Cards**: Total policies, active policies, total coverage, monthly premium
- **Quick Actions**: Add new policy, make payment, file claim
- **Recent Activity**: Latest payments, claims, policy updates
- **Charts**: Premium trends, coverage breakdown by type

### 2. Insurance Policies List
- **Search & Filter**: By type, provider, status
- **Sort Options**: By premium, coverage, expiry date
- **Card View**: Policy cards with key information
- **List View**: Detailed table view
- **Actions**: View details, edit, delete, make payment

### 3. Insurance Policy Details
- **Policy Information**: All policy details
- **Payment History**: List of payments with status
- **Claims History**: List of claims with status
- **Documents**: Policy documents, receipts
- **Actions**: Edit policy, make payment, file claim

### 4. Payment Management
- **Payment List**: All payments with filtering
- **Payment Details**: Transaction details, receipt
- **Make Payment**: Payment form with method selection
- **Payment History**: Chronological list with status

### 5. Claims Management
- **Claims List**: All claims with filtering by status
- **Claim Details**: Full claim information
- **File Claim**: Multi-step claim form
- **Claim Status**: Visual status tracker

### 6. Statistics & Analytics
- **Dashboard Charts**: Premium trends, coverage breakdown
- **Payment Analytics**: Payment methods, success rates
- **Claims Analytics**: Claim types, approval rates
- **Export Options**: PDF reports, data export

## Technical Guidelines

### 1. State Management
- Use **Provider** or **Bloc** for state management
- Implement proper loading states
- Handle error states gracefully
- Cache data for offline access

### 2. API Service Layer
```dart
class InsuranceApiService {
  final Dio _dio;
  final String _baseUrl = 'http://localhost:8080';
  
  InsuranceApiService(this._dio);
  
  // Insurance endpoints
  Future<List<Insurance>> getUserInsurances({int page = 1, int limit = 10});
  Future<Insurance> getInsuranceById(String id);
  Future<Insurance> createInsurance(Insurance insurance);
  Future<Insurance> updateInsurance(Insurance insurance);
  Future<void> deleteInsurance(String id);
  Future<List<Insurance>> searchInsurances(String query, {int page = 1, int limit = 10});
  
  // Payment endpoints
  Future<List<InsurancePayment>> getInsurancePayments(String insuranceId, {int page = 1, int limit = 10});
  Future<List<InsurancePayment>> getUserPayments({int page = 1, int limit = 10});
  Future<InsurancePayment> createPayment(InsurancePayment payment);
  Future<InsurancePayment> processPayment(String paymentId, String paymentMethod, Map<String, String> details);
  Future<InsurancePayment> getPaymentById(String id);
  Future<List<InsurancePayment>> getOverduePayments();
  
  // Claims endpoints
  Future<List<InsuranceClaim>> getInsuranceClaims(String insuranceId, {int page = 1, int limit = 10});
  Future<List<InsuranceClaim>> getUserClaims({int page = 1, int limit = 10});
  Future<InsuranceClaim> createClaim(InsuranceClaim claim);
  Future<InsuranceClaim> updateClaim(InsuranceClaim claim);
  Future<InsuranceClaim> getClaimById(String id);
  
  // Receipt endpoints
  Future<String> generatePaymentReceipt(String paymentId);
  Future<List<String>> getUserReceipts({int page = 1, int limit = 10});
  
  // Statistics endpoints
  Future<InsuranceStatistics> getInsuranceStatistics();
  Future<PaymentStatistics> getPaymentStatistics({DateTime? startDate, DateTime? endDate});
}
```

### 3. Error Handling
- Implement proper error handling for network requests
- Show user-friendly error messages
- Retry failed requests with exponential backoff
- Handle authentication errors gracefully

### 4. Form Validation
- Validate all form inputs
- Show real-time validation feedback
- Prevent form submission with invalid data
- Implement proper date pickers for date fields

### 5. File Upload
- Support image/document upload for claims
- Show upload progress
- Validate file types and sizes
- Handle upload errors

## Project Structure

```
lib/
├── models/
│   ├── insurance.dart
│   ├── insurance_payment.dart
│   ├── insurance_claim.dart
│   └── insurance_statistics.dart
├── services/
│   ├── insurance_api_service.dart
│   └── insurance_repository.dart
├── providers/
│   ├── insurance_provider.dart
│   ├── payment_provider.dart
│   └── claim_provider.dart
├── screens/
│   ├── insurance/
│   │   ├── insurance_dashboard.dart
│   │   ├── insurance_list.dart
│   │   ├── insurance_details.dart
│   │   ├── add_insurance.dart
│   │   └── edit_insurance.dart
│   ├── payments/
│   │   ├── payment_list.dart
│   │   ├── payment_details.dart
│   │   ├── make_payment.dart
│   │   └── payment_history.dart
│   ├── claims/
│   │   ├── claim_list.dart
│   │   ├── claim_details.dart
│   │   ├── file_claim.dart
│   │   └── claim_status.dart
│   └── statistics/
│       ├── insurance_statistics.dart
│       └── payment_analytics.dart
├── widgets/
│   ├── insurance/
│   │   ├── insurance_card.dart
│   │   ├── policy_summary.dart
│   │   └── coverage_details.dart
│   ├── payments/
│   │   ├── payment_card.dart
│   │   ├── payment_method_selector.dart
│   │   └── receipt_viewer.dart
│   ├── claims/
│   │   ├── claim_card.dart
│   │   ├── claim_status_indicator.dart
│   │   └── file_upload_widget.dart
│   └── common/
│       ├── loading_widget.dart
│       ├── error_widget.dart
│       └── empty_state_widget.dart
└── utils/
    ├── constants.dart
    ├── validators.dart
    ├── formatters.dart
    └── helpers.dart
```

## API Service Example

```dart
class InsuranceApiService {
  final Dio _dio;
  
  InsuranceApiService(this._dio) {
    _dio.options.baseUrl = 'http://localhost:8080';
    _dio.options.connectTimeout = Duration(seconds: 30);
    _dio.options.receiveTimeout = Duration(seconds: 30);
  }
  
  // Add authentication interceptor
  void setAuthToken(String token) {
    _dio.options.headers['Authorization'] = 'Bearer $token';
  }
  
  Future<List<Insurance>> getUserInsurances({int page = 1, int limit = 10}) async {
    try {
      final response = await _dio.get('/v1/insurance', queryParameters: {
        'page': page,
        'limit': limit,
      });
      
      if (response.data['success'] == true) {
        final List<dynamic> insurancesData = response.data['insurances'];
        return insurancesData.map((json) => Insurance.fromJson(json)).toList();
      } else {
        throw Exception(response.data['msg'] ?? 'Failed to fetch insurances');
      }
    } catch (e) {
      throw Exception('Failed to fetch insurances: $e');
    }
  }
  
  Future<Insurance> createInsurance(Insurance insurance) async {
    try {
      final response = await _dio.post('/v1/insurance', data: insurance.toJson());
      
      if (response.data['success'] == true) {
        return Insurance.fromJson(response.data['insurance']);
      } else {
        throw Exception(response.data['msg'] ?? 'Failed to create insurance');
      }
    } catch (e) {
      throw Exception('Failed to create insurance: $e');
    }
  }
  
  // Add similar methods for all other endpoints...
}
```

## State Management Example

```dart
class InsuranceProvider with ChangeNotifier {
  final InsuranceApiService _apiService;
  
  List<Insurance> _insurances = [];
  bool _isLoading = false;
  String? _error;
  
  InsuranceProvider(this._apiService);
  
  List<Insurance> get insurances => _insurances;
  bool get isLoading => _isLoading;
  String? get error => _error;
  
  Future<void> fetchInsurances({int page = 1, int limit = 10}) async {
    try {
      _isLoading = true;
      _error = null;
      notifyListeners();
      
      _insurances = await _apiService.getUserInsurances(page: page, limit: limit);
      
      _isLoading = false;
      notifyListeners();
    } catch (e) {
      _isLoading = false;
      _error = e.toString();
      notifyListeners();
    }
  }
  
  Future<void> createInsurance(Insurance insurance) async {
    try {
      _isLoading = true;
      _error = null;
      notifyListeners();
      
      final newInsurance = await _apiService.createInsurance(insurance);
      _insurances.insert(0, newInsurance);
      
      _isLoading = false;
      notifyListeners();
    } catch (e) {
      _isLoading = false;
      _error = e.toString();
      notifyListeners();
    }
  }
}
```

## Error Handling

```dart
class ApiException implements Exception {
  final String message;
  final int? statusCode;
  
  ApiException(this.message, {this.statusCode});
  
  @override
  String toString() => 'ApiException: $message (Status: $statusCode)';
}

// In your service
Future<T> handleApiCall<T>(Future<T> Function() apiCall) async {
  try {
    return await apiCall();
  } on DioException catch (e) {
    if (e.response?.statusCode == 401) {
      throw ApiException('Authentication required', statusCode: 401);
    } else if (e.response?.statusCode == 403) {
      throw ApiException('Access denied', statusCode: 403);
    } else if (e.response?.statusCode == 404) {
      throw ApiException('Resource not found', statusCode: 404);
    } else {
      throw ApiException('Network error: ${e.message}', statusCode: e.response?.statusCode);
    }
  } catch (e) {
    throw ApiException('Unexpected error: $e');
  }
}
```

## Testing

### Unit Tests
- Test API service methods
- Test state management
- Test form validation
- Test data models

### Widget Tests
- Test UI components
- Test user interactions
- Test error states
- Test loading states

### Integration Tests
- Test complete user flows
- Test API integration
- Test error handling
- Test offline scenarios

## Performance Considerations

1. **Caching**: Implement proper caching for insurance data
2. **Pagination**: Use pagination for large lists
3. **Image Optimization**: Optimize images for mobile
4. **Lazy Loading**: Load data on demand
5. **Background Sync**: Sync data in background

## Security Considerations

1. **Token Storage**: Store JWT tokens securely
2. **Input Validation**: Validate all user inputs
3. **HTTPS**: Use HTTPS for all API calls
4. **Error Messages**: Don't expose sensitive information in errors
5. **File Upload**: Validate file types and sizes

## Additional Features

### 1. Push Notifications
- Payment reminders
- Claim status updates
- Policy expiry notifications

### 2. Offline Support
- Cache insurance data
- Queue offline actions
- Sync when online

### 3. Multi-language Support
- Support multiple languages
- Localize error messages
- Localize UI text

### 4. Dark Mode
- Implement dark theme
- Respect system preferences
- Smooth theme transitions

## Deliverables

1. **Complete Insurance Dashboard** with overview and quick actions
2. **Insurance Policy Management** (CRUD operations)
3. **Payment Management** with multiple payment methods
4. **Claims Management** with file upload support
5. **Statistics & Analytics** with charts and reports
6. **Receipt Management** with PDF generation
7. **Search & Filter** functionality
8. **Error Handling** and loading states
9. **Unit Tests** for all components
10. **Documentation** for API integration

## Success Criteria

1. ✅ All insurance endpoints are properly integrated
2. ✅ UI is responsive and follows Material Design
3. ✅ Error handling is robust and user-friendly
4. ✅ State management is efficient and scalable
5. ✅ Code is well-documented and testable
6. ✅ Performance is optimized for mobile devices
7. ✅ Security best practices are followed
8. ✅ Offline functionality works correctly
9. ✅ All user flows are smooth and intuitive
10. ✅ Code coverage is above 80%

## Getting Started

1. Set up the Flutter project with proper dependencies
2. Configure the API service with the backend URL
3. Implement authentication flow
4. Create the basic UI structure
5. Implement the API service layer
6. Add state management
7. Build the UI components
8. Add error handling and loading states
9. Implement testing
10. Optimize performance and add additional features

This implementation should provide a complete, production-ready insurance management system for the LazerVault Flutter frontend. 