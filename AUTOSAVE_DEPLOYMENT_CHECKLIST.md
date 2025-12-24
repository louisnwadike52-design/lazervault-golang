# Auto-Save Production Deployment Checklist

## Pre-Deployment Verification

### Backend Checks
- [ ] All database migrations completed successfully
- [ ] `autosave_rules` table created with proper indexes
- [ ] `autosave_transactions` table created with proper indexes
- [ ] Auto-save service registered in gRPC server
- [ ] Auto-save controller registered in gRPC server
- [ ] HTTP gateway endpoints registered
- [ ] Scheduled processor initialized and running
- [ ] Deposit trigger integration added to deposit processor
- [ ] All tests passing (`./test_autosave_integration.sh`)

### Frontend Checks
- [ ] Auto-save gRPC client registered in DI container
- [ ] Repository implementation connected to backend
- [ ] All use cases registered
- [ ] Cubit/state management configured
- [ ] UI screens implemented
- [ ] Navigation routes added
- [ ] Error handling implemented
- [ ] Loading states implemented

### Infrastructure Checks
- [ ] Redis server running (for scheduled tasks)
- [ ] Database connection configured
- [ ] Environment variables set
- [ ] Logging configured
- [ ] Monitoring tools set up
- [ ] Backup strategy in place

## Deployment Steps

### 1. Database Setup
```bash
# Run migrations
go run main.go

# Verify tables created
psql -U your_user -d lazervault -c "\dt autosave*"

# Create indexes
CREATE INDEX idx_autosave_rules_user_id ON autosave_rules(user_id);
CREATE INDEX idx_autosave_rules_status ON autosave_rules(status);
CREATE INDEX idx_autosave_rules_trigger_type ON autosave_rules(trigger_type);
CREATE INDEX idx_autosave_transactions_rule_id ON autosave_transactions(rule_id);
CREATE INDEX idx_autosave_transactions_user_id ON autosave_transactions(user_id);
```

### 2. Backend Deployment
```bash
# Build the application
cd /Users/louislawrence/Music/apps/stack/lazervault-golang
go build -o lazervault-server main.go

# Run the server (or use systemd/supervisor)
./lazervault-server

# Verify services are running
grpcurl -plaintext localhost:9090 list | grep AutoSaveService
```

### 3. Frontend Deployment
```bash
# Build Flutter app
cd /Users/louislawrence/Music/apps/stack/lazervaultapp
flutter build apk --release  # For Android
# or
flutter build ios --release  # For iOS
```

### 4. Verification Tests

#### Test 1: Create a Rule
```bash
grpcurl -plaintext \
    -H "authorization: Bearer YOUR_TOKEN" \
    -d '{
        "name": "Test Rule",
        "description": "Test auto-save",
        "trigger_type": "TRIGGER_ON_DEPOSIT",
        "amount_type": "AMOUNT_PERCENTAGE",
        "amount_value": 10.0,
        "source_account_id": "1",
        "destination_account_id": "2"
    }' \
    localhost:9090 lazervault.AutoSaveService/CreateAutoSaveRule
```

#### Test 2: Trigger via Deposit
```bash
# Make a deposit
grpcurl -plaintext \
    -H "authorization: Bearer YOUR_TOKEN" \
    -d '{
        "target_account_id": "1",
        "amount": 100.0,
        "currency": "USD",
        "source_bank_name": "Test Bank"
    }' \
    localhost:9090 lazervault.DepositService/InitiateDeposit

# Wait 10 seconds for processing

# Check auto-save transactions
grpcurl -plaintext \
    -H "authorization: Bearer YOUR_TOKEN" \
    -d '{}' \
    localhost:9090 lazervault.AutoSaveService/GetAutoSaveTransactions
```

#### Test 3: Check Scheduled Processor
```bash
# Check logs for scheduled processor activity
tail -f /var/log/lazervault/app.log | grep "scheduled auto-save"

# Should see logs every minute:
# - "Starting scheduled auto-save rules processing"
# - "Completed processing scheduled auto-save rules"
```

## Post-Deployment Monitoring

### Metrics to Track

#### 1. Rule Metrics
```sql
-- Total active rules
SELECT COUNT(*) FROM autosave_rules WHERE status = 'active' AND deleted_at IS NULL;

-- Rules by trigger type
SELECT trigger_type, COUNT(*) FROM autosave_rules
WHERE status = 'active' AND deleted_at IS NULL
GROUP BY trigger_type;

-- Average save amount
SELECT AVG(amount_value) FROM autosave_rules
WHERE status = 'active' AND amount_type = 'fixed';
```

#### 2. Transaction Metrics
```sql
-- Total successful saves today
SELECT COUNT(*) FROM autosave_transactions
WHERE success = true
AND DATE(created_at) = CURRENT_DATE;

-- Total amount saved today
SELECT SUM(amount) FROM autosave_transactions
WHERE success = true
AND DATE(created_at) = CURRENT_DATE;

-- Success rate
SELECT
    COUNT(CASE WHEN success = true THEN 1 END) * 100.0 / COUNT(*) as success_rate
FROM autosave_transactions
WHERE DATE(created_at) = CURRENT_DATE;

-- Failed transactions with reasons
SELECT error_message, COUNT(*)
FROM autosave_transactions
WHERE success = false
GROUP BY error_message
ORDER BY COUNT(*) DESC
LIMIT 10;
```

#### 3. Performance Metrics
```sql
-- Average processing time (if you add timing)
-- Most active rules
SELECT r.id, r.name, r.trigger_count, r.total_saved
FROM autosave_rules r
WHERE r.status = 'active'
ORDER BY r.trigger_count DESC
LIMIT 10;
```

### Log Monitoring

Set up alerts for:
```bash
# High failure rate
grep "Failed to execute auto-save" /var/log/lazervault/app.log | wc -l

# Insufficient funds warnings
grep "insufficient funds" /var/log/lazervault/app.log

# Database errors
grep "Failed to query auto-save" /var/log/lazervault/app.log

# Scheduled processor issues
grep "Failed to process scheduled" /var/log/lazervault/app.log
```

### Health Checks

Add these to your monitoring system:

```bash
#!/bin/bash
# Auto-save health check script

# Check if scheduled processor is running
PROCESSOR_ACTIVE=$(tail -100 /var/log/lazervault/app.log | grep "scheduled auto-save rules processing" | wc -l)
if [ "$PROCESSOR_ACTIVE" -eq 0 ]; then
    echo "WARNING: Scheduled processor not active in last 100 log lines"
    exit 1
fi

# Check database connectivity
psql -U your_user -d lazervault -c "SELECT 1 FROM autosave_rules LIMIT 1" > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "ERROR: Cannot connect to auto-save database"
    exit 1
fi

# Check Redis connectivity
redis-cli ping > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "ERROR: Cannot connect to Redis"
    exit 1
fi

echo "Auto-save health check passed"
exit 0
```

## Rollback Plan

If issues are detected:

### 1. Disable Auto-Save (Quick Fix)
```sql
-- Pause all active rules temporarily
UPDATE autosave_rules
SET status = 'paused'
WHERE status = 'active';
```

### 2. Stop Scheduled Processor
```bash
# Kill the scheduled processor if needed
# The processor will stop checking for scheduled rules
pkill -f "scheduled_autosave_processor"
```

### 3. Disable Deposit Triggers
```go
// Comment out in worker/task_process_deposit.go
// Lines 120-131 (the auto-save trigger code)
```

### 4. Full Rollback
```bash
# Stop the server
systemctl stop lazervault

# Rollback database migrations (if needed)
# Restore from backup

# Deploy previous version
git checkout <previous-commit>
go build -o lazervault-server main.go
systemctl start lazervault
```

## Performance Optimization

### Database Indexes
```sql
-- Ensure these indexes exist
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_autosave_rules_user_status
ON autosave_rules(user_id, status) WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_autosave_rules_trigger
ON autosave_rules(trigger_type, status) WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_autosave_transactions_created
ON autosave_transactions(created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_autosave_transactions_user_created
ON autosave_transactions(user_id, created_at DESC);
```

### Query Optimization
```sql
-- Add EXPLAIN ANALYZE to slow queries
EXPLAIN ANALYZE
SELECT * FROM autosave_rules
WHERE user_id = 123 AND status = 'active' AND deleted_at IS NULL;
```

### Connection Pooling
```go
// Ensure proper connection pool settings in database/conn.go
sqlDB.SetMaxOpenConns(25)
sqlDB.SetMaxIdleConns(5)
sqlDB.SetConnMaxLifetime(5 * time.Minute)
```

## Security Checklist

- [ ] All auto-save endpoints require authentication
- [ ] User can only access their own rules
- [ ] Account ownership verified before rule creation
- [ ] Database transactions used for atomic operations
- [ ] SQL injection prevented (using parameterized queries)
- [ ] Rate limiting enabled on endpoints
- [ ] Audit logging enabled for all operations
- [ ] Sensitive data encrypted at rest
- [ ] TLS/SSL enabled for all connections

## Documentation

- [ ] API documentation updated
- [ ] User guide created
- [ ] Admin guide created
- [ ] Troubleshooting guide available
- [ ] Runbook for on-call engineers
- [ ] Architecture diagram updated

## Communication

- [ ] Notify users of new feature
- [ ] Provide migration guide (if needed)
- [ ] Update help center
- [ ] Train support team
- [ ] Update sales materials

## Sign-off

- [ ] QA team approval
- [ ] Security team approval
- [ ] Product manager approval
- [ ] Engineering manager approval
- [ ] Operations team ready

## Emergency Contacts

**On-Call Engineer**: [Add contact]
**Database Admin**: [Add contact]
**DevOps Lead**: [Add contact]
**Product Manager**: [Add contact]

## Post-Deployment Tasks

### Week 1
- [ ] Monitor error rates daily
- [ ] Review user feedback
- [ ] Check performance metrics
- [ ] Verify scheduled processor running smoothly
- [ ] Review database performance

### Week 2-4
- [ ] Analyze usage patterns
- [ ] Identify optimization opportunities
- [ ] Plan enhancements based on feedback
- [ ] Update documentation with learnings

## Success Criteria

### Technical Metrics
- [ ] 99.9% uptime
- [ ] < 100ms response time for rule creation
- [ ] < 50ms response time for rule retrieval
- [ ] > 99% success rate for auto-save executions
- [ ] < 5% failure rate due to insufficient funds
- [ ] Scheduled processor runs every minute without failures

### Business Metrics
- [ ] > 1000 rules created in first week
- [ ] > 10,000 auto-save transactions in first week
- [ ] > $100,000 total saved in first week
- [ ] < 5 customer support tickets
- [ ] > 4.5/5 user satisfaction score

---

**Deployment Date**: _______________
**Deployed By**: _______________
**Version**: _______________
**Status**: _______________
