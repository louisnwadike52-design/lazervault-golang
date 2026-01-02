# Electricity Bill Payment - Test Plan

## Test Environment Setup

### Prerequisites
1. Backend server running
2. Database migrated with all electricity bill tables
3. Redis running for async tasks
4. Valid Flutterwave/Paystack API keys configured

### Environment Variables Required
```bash
export FLUTTERWAVE_SECRET_KEY="test_your_secret_key"
export FLUTTERWAVE_PUBLIC_KEY="test_your_public_key"
export FLUTTERWAVE_BASE_URL="https://api.flutterwave.com"
export FLUTTERWAVE_ENABLED="true"

export PAYSTACK_SECRET_KEY="test_your_secret_key"
export PAYSTACK_PUBLIC_KEY="test_your_public_key"
export PAYSTACK_BASE_URL="https://api.paystack.co"
export PAYSTACK_ENABLED="true"
```

## Test Scenarios

### 1. Provider Sync Test
**Endpoint:** `SyncProviders`
**Method:** POST
**Path:** `/v1/electricity/providers/sync`

**Request:**
```json
{
  "payment_gateway": "flutterwave"
}
```

**Expected Response:**
```json
{
  "synced_count": 15,
  "message": "Successfully synced 15 providers"
}
```

**Validation:**
- Providers saved to database
- Each provider has valid min/max amounts
- Logo URLs are accessible

---

### 2. Get Providers Test
**Endpoint:** `GetProviders`
**Method:** GET
**Path:** `/v1/electricity/providers?country=NG`

**Expected Response:**
```json
{
  "providers": [
    {
      "id": "uuid",
      "provider_code": "EKEDC",
      "provider_name": "Eko Electricity Distribution Company",
      "country": "NG",
      "is_active": true,
      "payment_gateway": "flutterwave",
      "min_amount": 1000,
      "max_amount": 100000,
      "service_fee": 100,
      "fee_type": "fixed"
    }
  ]
}
```

**Validation:**
- Returns at least 1 provider
- All required fields present
- Provider codes are valid

---

### 3. Meter Validation Test
**Endpoint:** `ValidateMeterNumber`
**Method:** POST
**Path:** `/v1/electricity/validate-meter`

**Request:**
```json
{
  "provider_code": "EKEDC",
  "meter_number": "1234567890",
  "meter_type": "prepaid"
}
```

**Expected Response:**
```json
{
  "is_valid": true,
  "customer_name": "John Doe",
  "customer_address": "123 Lagos Street, Lagos",
  "meter_type": "prepaid",
  "outstanding_balance": 0,
  "message": "Meter validated successfully",
  "meter_number": "1234567890"
}
```

**Test Cases:**
- ✅ Valid prepaid meter
- ✅ Valid postpaid meter
- ❌ Invalid meter number (should return is_valid: false)
- ❌ Non-existent provider code

---

### 4. Payment Initiation Test
**Endpoint:** `InitiatePayment`
**Method:** POST
**Path:** `/v1/electricity/pay`
**Auth:** Required (Bearer token)

**Request:**
```json
{
  "provider_code": "EKEDC",
  "meter_number": "1234567890",
  "amount": 5000,
  "currency": "NGN",
  "meter_type": "prepaid",
  "payment_gateway": "flutterwave",
  "source_account_id": "user_account_uuid"
}
```

**Expected Response:**
```json
{
  "payment_id": "payment_uuid",
  "reference_number": "ELEC-EKEDC-1234567890",
  "status": "pending",
  "total_amount": 5100,
  "service_fee": 100,
  "message": "Payment initiated successfully"
}
```

**Validation:**
- Payment record created in database
- Status is "pending"
- Async task enqueued
- Total amount includes service fee

**Test Cases:**
- ✅ Valid payment within limits
- ❌ Amount below min_amount
- ❌ Amount above max_amount
- ❌ Invalid provider code
- ❌ Unauthenticated request

---

### 5. Payment Verification Test
**Endpoint:** `VerifyPayment`
**Method:** GET
**Path:** `/v1/electricity/payments/{payment_id}/verify`

**Expected Response (Pending):**
```json
{
  "payment": {
    "id": "payment_uuid",
    "user_id": "user_uuid",
    "provider_code": "EKEDC",
    "provider_name": "Eko Electricity",
    "meter_number": "1234567890",
    "customer_name": "John Doe",
    "amount": 5000,
    "service_fee": 100,
    "total_amount": 5100,
    "currency": "NGN",
    "status": "processing",
    "payment_gateway": "flutterwave",
    "reference_number": "ELEC-EKEDC-1234567890",
    "meter_type": "prepaid",
    "created_at": "2025-01-01T10:00:00Z"
  },
  "message": "Payment verification successful"
}
```

**Expected Response (Completed):**
```json
{
  "payment": {
    "status": "completed",
    "token": "1234-5678-9012-3456",
    "units": 25.5,
    "gateway_reference": "FLW-REF-123456",
    "completed_at": "2025-01-01T10:05:00Z"
  }
}
```

**Test Cases:**
- ✅ Check status while processing
- ✅ Check status after completion
- ✅ Check status after failure
- ❌ Invalid payment_id

---

### 6. Payment History Test
**Endpoint:** `GetPaymentHistory`
**Method:** GET
**Path:** `/v1/electricity/payments?limit=10&offset=0`
**Auth:** Required

**Expected Response:**
```json
{
  "payments": [
    {
      "id": "payment_uuid",
      "status": "completed",
      "amount": 5000,
      "created_at": "2025-01-01T10:00:00Z"
    }
  ],
  "total": 1
}
```

**Test Cases:**
- ✅ Get all payments
- ✅ Filter by provider_code
- ✅ Filter by status
- ✅ Pagination works correctly

---

### 7. Payment Receipt Test
**Endpoint:** `GetPaymentReceipt`
**Method:** GET
**Path:** `/v1/electricity/payments/{payment_id}/receipt`

**Expected Response:**
```json
{
  "payment": { /* full payment object */ },
  "receipt_data": {
    "receipt_number": "ELEC-EKEDC-1234567890",
    "customer_name": "John Doe",
    "meter_number": "1234567890",
    "provider_name": "Eko Electricity",
    "amount_paid": 5000,
    "service_fee": 100,
    "total_amount": 5100,
    "token": "1234-5678-9012-3456",
    "units": 25.5,
    "payment_date": "2025-01-01 10:05:00",
    "reference_number": "ELEC-EKEDC-1234567890"
  }
}
```

---

### 8. Save Beneficiary Test
**Endpoint:** `SaveBeneficiary`
**Method:** POST
**Path:** `/v1/electricity/beneficiaries`
**Auth:** Required

**Request:**
```json
{
  "provider_code": "EKEDC",
  "meter_number": "1234567890",
  "customer_name": "John Doe",
  "nickname": "Home Meter",
  "meter_type": "prepaid",
  "is_default": true,
  "provider_id": "provider_uuid",
  "provider_name": "Eko Electricity",
  "customer_address": "123 Lagos Street"
}
```

**Expected Response:**
```json
{
  "beneficiary": {
    "id": "beneficiary_uuid",
    "nickname": "Home Meter",
    "is_default": true,
    "created_at": "2025-01-01T10:00:00Z"
  },
  "message": "Beneficiary saved successfully"
}
```

**Test Cases:**
- ✅ Create new beneficiary
- ✅ Set as default (unsets other defaults)
- ✅ Create multiple beneficiaries

---

### 9. Get Beneficiaries Test
**Endpoint:** `GetBeneficiaries`
**Method:** GET
**Path:** `/v1/electricity/beneficiaries`
**Auth:** Required

**Expected Response:**
```json
{
  "beneficiaries": [
    {
      "id": "beneficiary_uuid",
      "provider_code": "EKEDC",
      "meter_number": "1234567890",
      "nickname": "Home Meter",
      "is_default": true
    }
  ]
}
```

---

### 10. Update Beneficiary Test
**Endpoint:** `UpdateBeneficiary`
**Method:** PUT
**Path:** `/v1/electricity/beneficiaries/{beneficiary_id}`
**Auth:** Required

**Request:**
```json
{
  "nickname": "Updated Home Meter",
  "is_default": false
}
```

---

### 11. Delete Beneficiary Test
**Endpoint:** `DeleteBeneficiary`
**Method:** DELETE
**Path:** `/v1/electricity/beneficiaries/{beneficiary_id}`
**Auth:** Required

**Expected Response:**
```json
{
  "message": "Beneficiary deleted successfully"
}
```

---

### 12. Create Auto-Recharge Test
**Endpoint:** `CreateAutoRecharge`
**Method:** POST
**Path:** `/v1/electricity/auto-recharge`
**Auth:** Required

**Request:**
```json
{
  "beneficiary_id": "beneficiary_uuid",
  "amount": 5000,
  "currency": "NGN",
  "frequency": "monthly",
  "day_of_month": 1,
  "max_retries": 3
}
```

**Expected Response:**
```json
{
  "auto_recharge": {
    "id": "auto_recharge_uuid",
    "frequency": "monthly",
    "next_run_date": "2025-02-01T00:00:00Z",
    "status": "active",
    "max_retries": 3
  },
  "message": "Auto-recharge created successfully"
}
```

**Test Cases:**
- ✅ Daily auto-recharge
- ✅ Weekly auto-recharge (specific day)
- ✅ Monthly auto-recharge (specific day)
- ❌ Invalid beneficiary_id

---

### 13. Auto-Recharge Execution Test
**Scheduled Task:** Runs hourly

**Test:**
1. Create auto-recharge with next_run_date in the past
2. Wait for scheduled processor to run
3. Verify payment created
4. Check next_run_date updated
5. Verify failure_count reset on success

**Expected:**
- Payment created automatically
- Status updated to "processing" → "completed"
- next_run_date calculated correctly
- On failure: failure_count incremented
- After max_retries: status = "expired"

---

### 14. Create Reminder Test
**Endpoint:** `CreateReminder`
**Method:** POST
**Path:** `/v1/electricity/reminders`
**Auth:** Required

**Request:**
```json
{
  "beneficiary_id": "beneficiary_uuid",
  "title": "Monthly Electricity Bill",
  "description": "Pay home electricity",
  "reminder_date": "2025-02-01T09:00:00Z",
  "amount": 5000,
  "is_recurring": true,
  "recurrence_type": "monthly",
  "currency": "NGN"
}
```

**Expected Response:**
```json
{
  "reminder": {
    "id": "reminder_uuid",
    "title": "Monthly Electricity Bill",
    "is_recurring": true,
    "status": "active"
  },
  "message": "Reminder created successfully"
}
```

---

### 15. Worker Task Tests

#### Bill Payment Processing Task
**Test:**
1. Create payment via InitiatePayment
2. Monitor task queue
3. Verify task executes
4. Check payment status updated
5. Verify token and units stored

**Mock Gateway Response:**
```json
{
  "success": true,
  "reference": "FLW-REF-123",
  "token": "1234-5678-9012-3456",
  "units": 25.5,
  "message": "Payment successful"
}
```

---

## Integration Test Checklist

### Database Tests
- [ ] All tables created successfully
- [ ] Foreign key constraints work
- [ ] Indexes created for performance
- [ ] UUID generation works

### Service Layer Tests
- [ ] GetProviders returns data
- [ ] SyncProviders saves to DB
- [ ] ValidateMeter calls gateway
- [ ] InitiatePayment creates record
- [ ] VerifyPayment returns status
- [ ] Beneficiary CRUD works
- [ ] Auto-recharge CRUD works
- [ ] Reminder CRUD works

### Worker Tests
- [ ] Bill payment task processes successfully
- [ ] Auto-recharge scheduler runs
- [ ] Reminder scheduler runs
- [ ] Failed payments marked correctly
- [ ] Retry logic works

### Error Handling Tests
- [ ] Invalid meter returns proper error
- [ ] Amount outside limits rejected
- [ ] Unauthenticated requests blocked
- [ ] Invalid provider code handled
- [ ] Gateway timeout handled
- [ ] Database errors handled

### Security Tests
- [ ] Authentication required for all user endpoints
- [ ] User can only access own data
- [ ] Resource ownership verified
- [ ] No SQL injection vulnerabilities
- [ ] API keys not exposed in logs

---

## Performance Tests

### Load Tests
- [ ] 100 concurrent payment requests
- [ ] 1000 provider lookups/second
- [ ] Large payment history pagination
- [ ] Multiple auto-recharge executions

### Response Time Targets
- GetProviders: < 100ms
- ValidateMeter: < 2s (gateway dependent)
- InitiatePayment: < 200ms
- VerifyPayment: < 100ms

---

## End-to-End Test Script

```bash
#!/bin/bash

# 1. Sync Providers
curl -X POST http://localhost:8080/v1/electricity/providers/sync \
  -H "Content-Type: application/json" \
  -d '{"payment_gateway": "flutterwave"}'

# 2. Get Providers
curl http://localhost:8080/v1/electricity/providers?country=NG

# 3. Validate Meter
curl -X POST http://localhost:8080/v1/electricity/validate-meter \
  -H "Content-Type: application/json" \
  -d '{
    "provider_code": "EKEDC",
    "meter_number": "1234567890",
    "meter_type": "prepaid"
  }'

# 4. Initiate Payment (requires auth token)
curl -X POST http://localhost:8080/v1/electricity/pay \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider_code": "EKEDC",
    "meter_number": "1234567890",
    "amount": 5000,
    "currency": "NGN",
    "meter_type": "prepaid",
    "payment_gateway": "flutterwave",
    "source_account_id": "$ACCOUNT_ID"
  }'

# 5. Verify Payment
PAYMENT_ID="<payment_id_from_step_4>"
curl http://localhost:8080/v1/electricity/payments/$PAYMENT_ID/verify \
  -H "Authorization: Bearer $TOKEN"

# 6. Get Receipt
curl http://localhost:8080/v1/electricity/payments/$PAYMENT_ID/receipt \
  -H "Authorization: Bearer $TOKEN"
```

---

## Test Results Template

### Test Run: [Date]
**Environment:** [Development/Staging/Production]
**Tester:** [Name]

| Test Case | Status | Notes |
|-----------|--------|-------|
| Sync Providers | ✅ | Synced 15 providers |
| Get Providers | ✅ | Returns all active |
| Validate Meter | ✅ | Valid response |
| Initiate Payment | ✅ | Payment created |
| Verify Payment | ✅ | Status updated |
| Payment Receipt | ✅ | Receipt generated |
| Save Beneficiary | ✅ | Saved successfully |
| Create Auto-Recharge | ✅ | Scheduled correctly |
| Create Reminder | ✅ | Reminder created |
| Worker Task | ✅ | Payment processed |

**Overall Status:** PASS/FAIL
**Comments:** [Any issues or observations]
