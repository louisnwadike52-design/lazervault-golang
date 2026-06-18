# Auto-Save Feature - Production Ready Integration

## Overview

The Auto-Save feature allows users to automatically save money based on various triggers. This is a production-ready implementation connecting the Flutter frontend with the Go backend.

## Features

### Trigger Types
1. **On Deposit** - Automatically save a portion of every deposit
2. **Scheduled** - Save at regular intervals (daily, weekly, biweekly, monthly)
3. **Round Up** - Round up transactions and save the difference

### Amount Types
1. **Fixed** - Save a fixed amount
2. **Percentage** - Save a percentage of the trigger amount

### Rule Management
- Create auto-save rules with custom parameters
- Pause/resume rules
- Update rule settings
- Delete rules
- View transaction history
- Track statistics (total saved, average amounts, etc.)

## Architecture

### Frontend (Flutter)
```
lib/src/features/autosave/
├── data/
│   ├── models/              # Proto-to-model conversions
│   └── repositories/        # gRPC client implementation
├── domain/
│   ├── entities/            # Domain models
│   ├── repositories/        # Repository interfaces
│   └── usecases/           # Business logic use cases
└── presentation/
    ├── cubit/              # State management
    └── views/              # UI screens
```

### Backend (Go)
```
/Users/louislawrence/Music/apps/stack/lazervault-golang/
├── models/autosave.go                    # Database models
├── services/autosave_service.go          # Business logic
├── grpcApi/autosave_controller.go        # gRPC handlers
└── worker/scheduled_autosave_processor.go # Scheduled job processor
```

## Setup

### 1. Database Migration

The auto-save tables are automatically created when you run:

```bash
cd /Users/louislawrence/Music/apps/stack/lazervault-golang
go run main.go
```

Tables created:
- `autosave_rules` - Stores auto-save rule configurations
- `autosave_transactions` - Stores transaction history

### 2. Start Backend Server

```bash
cd /Users/louislawrence/Music/apps/stack/lazervault-golang
go run main.go
```

The server will:
- Start gRPC server on port 9090
- Start HTTP gateway on port 8080
- Initialize scheduled auto-save processor (runs every minute)
- Set up deposit trigger integration

### 3. Frontend Setup

The frontend is already configured in `injection_container.dart`. No additional setup needed.

## Testing

### Run Integration Tests

```bash
cd /Users/louislawrence/Music/apps/stack/lazervault-golang
./test_autosave_integration.sh
```

This script tests:
- Rule creation
- Rule retrieval
- Deposit triggering
- Transaction history
- Statistics
- Manual triggering
- Rule toggle (pause/resume)
- Rule updates
- Rule deletion

### Manual Testing with grpcurl

#### Create a rule:
```bash
grpcurl -plaintext \
    -H "authorization: Bearer YOUR_TOKEN" \
    -d '{
        "name": "Save 10% of deposits",
        "description": "Automatically save 10% of every deposit",
        "trigger_type": "TRIGGER_ON_DEPOSIT",
        "amount_type": "AMOUNT_PERCENTAGE",
        "amount_value": 10.0,
        "source_account_id": "SOURCE_ACCOUNT_ID",
        "destination_account_id": "DEST_ACCOUNT_ID"
    }' \
    localhost:9090 lazervault.AutoSaveService/CreateAutoSaveRule
```

#### Get all rules:
```bash
grpcurl -plaintext \
    -H "authorization: Bearer YOUR_TOKEN" \
    -d '{}' \
    localhost:9090 lazervault.AutoSaveService/GetAutoSaveRules
```

#### Trigger manually:
```bash
grpcurl -plaintext \
    -H "authorization: Bearer YOUR_TOKEN" \
    -d '{
        "rule_id": "RULE_ID",
        "custom_amount": 5.0
    }' \
    localhost:9090 lazervault.AutoSaveService/TriggerAutoSave
```

## Production Deployment

### Environment Variables

Ensure these are set in your `.env`:
```env
# Database
DB_HOST=your-database-host
DB_PORT=5432
DB_NAME=lazervault
DB_USER=your-db-user
DB_PASSWORD=your-db-password

# Redis (for scheduled tasks)
REDIS_SERVER_ADDR=localhost:6379

# Server Ports
GRPC_SERVER_PORT=9090
HTTP_SERVER_PORT=8080
```

### Monitoring

The auto-save service includes comprehensive logging using zerolog:

#### Key log events:
- Rule creation/update/deletion
- Auto-save triggers (deposit, scheduled, round-up)
- Transaction execution (success/failure)
- Goal completion
- Balance violations
- Scheduled processor runs

#### Example log queries (if using structured logging):
```bash
# Find all auto-save triggers for a user
cat logs/app.log | grep "user_id=123" | grep "auto-save"

# Find failed auto-save executions
cat logs/app.log | grep "Failed to execute auto-save"

# Monitor scheduled processor
cat logs/app.log | grep "scheduled auto-save rules processing"
```

### Metrics to Track

1. **Rule Metrics**
   - Total active rules
   - Rules created per day
   - Rules completed (goal reached)
   - Rules paused/cancelled

2. **Transaction Metrics**
   - Total auto-saves executed
   - Success rate
   - Average save amount
   - Total amount saved

3. **Performance Metrics**
   - Deposit trigger latency
   - Scheduled processor execution time
   - Database query performance

### Error Handling

The system handles these error scenarios:

1. **Insufficient Funds** - Returns clear error, logs warning
2. **Minimum Balance Violation** - Prevents save, logs warning
3. **Goal Reached** - Marks rule as completed, stops triggers
4. **Account Not Found** - Returns not found error
5. **Database Errors** - Rolls back transaction, logs error
6. **Concurrent Access** - Uses database transactions for safety

### Scheduled Tasks

The scheduled auto-save processor:
- Runs every 1 minute
- Checks all active scheduled rules
- Determines which rules are due based on frequency and schedule
- Executes saves for due rules
- Handles errors gracefully
- Logs all activities

Schedule logic:
- **Daily**: Triggers once per day at specified time
- **Weekly**: Triggers on specified day of week
- **Biweekly**: Triggers every 2 weeks on specified day
- **Monthly**: Triggers on specified day of month

## API Endpoints

### gRPC Endpoints

All endpoints require authentication via Bearer token.

1. `CreateAutoSaveRule` - Create a new auto-save rule
2. `GetAutoSaveRules` - Get all rules (with optional filters)
3. `UpdateAutoSaveRule` - Update an existing rule
4. `ToggleAutoSaveRule` - Pause/resume/complete/cancel a rule
5. `DeleteAutoSaveRule` - Delete a rule
6. `GetAutoSaveTransactions` - Get transaction history (paginated)
7. `GetAutoSaveStatistics` - Get user statistics
8. `TriggerAutoSave` - Manually trigger a rule

### HTTP Gateway Endpoints

All gRPC endpoints are also available via HTTP REST:

```
POST /v1/autosave/rules
GET /v1/autosave/rules
PUT /v1/autosave/rules/{rule_id}
POST /v1/autosave/rules/{rule_id}/toggle
DELETE /v1/autosave/rules/{rule_id}
GET /v1/autosave/transactions
GET /v1/autosave/statistics
POST /v1/autosave/rules/{rule_id}/trigger
```

## Database Schema

### autosave_rules
```sql
CREATE TABLE autosave_rules (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    trigger_type VARCHAR(50) NOT NULL,
    amount_type VARCHAR(50) NOT NULL,
    amount_value DOUBLE PRECISION NOT NULL,
    source_account_id INTEGER NOT NULL REFERENCES accounts(id),
    destination_account_id INTEGER NOT NULL REFERENCES accounts(id),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    frequency VARCHAR(50),
    schedule_time VARCHAR(10),
    schedule_day INTEGER,
    round_up_to INTEGER,
    target_amount DOUBLE PRECISION,
    minimum_balance DOUBLE PRECISION,
    maximum_per_save DOUBLE PRECISION,
    trigger_count INTEGER DEFAULT 0,
    total_saved DOUBLE PRECISION DEFAULT 0,
    last_triggered_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP
);
```

### autosave_transactions
```sql
CREATE TABLE autosave_transactions (
    id SERIAL PRIMARY KEY,
    rule_id INTEGER NOT NULL REFERENCES autosave_rules(id),
    user_id INTEGER NOT NULL REFERENCES users(id),
    source_account_id INTEGER NOT NULL REFERENCES accounts(id),
    destination_account_id INTEGER NOT NULL REFERENCES accounts(id),
    amount DOUBLE PRECISION NOT NULL,
    trigger_type VARCHAR(50) NOT NULL,
    trigger_reason TEXT,
    success BOOLEAN NOT NULL DEFAULT FALSE,
    error_message TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    deleted_at TIMESTAMP
);
```

## Troubleshooting

### Common Issues

1. **Auto-save not triggering on deposit**
   - Check if rule status is 'active'
   - Verify source_account_id matches deposit account
   - Check logs for errors
   - Ensure deposit processing completed successfully

2. **Scheduled saves not running**
   - Verify Redis is running
   - Check scheduled processor logs
   - Ensure server is running continuously
   - Verify schedule configuration is correct

3. **Insufficient funds error**
   - Check source account balance
   - Verify minimum_balance setting isn't too high
   - Review transaction history

4. **Rules not appearing in frontend**
   - Check authentication token
   - Verify gRPC connection
   - Check network connectivity
   - Review frontend logs

### Debug Mode

Enable debug logging:
```bash
export LOG_LEVEL=debug
go run main.go
```

### Health Checks

Check auto-save service health:
```bash
grpcurl -plaintext localhost:9090 lazervault.AutoSaveService/GetAutoSaveStatistics
```

## Best Practices

1. **Rule Configuration**
   - Set reasonable minimum balances to avoid overdrafts
   - Use maximum_per_save to cap automatic saves
   - Set target amounts for goal-based saving

2. **Production Deployment**
   - Monitor scheduled processor logs
   - Set up alerts for high failure rates
   - Regularly review auto-save statistics
   - Back up database regularly

3. **Performance**
   - Index database on user_id, account_id, status
   - Use database connection pooling
   - Monitor Redis memory usage
   - Cache frequently accessed data

4. **Security**
   - Always validate user ownership of accounts
   - Use database transactions for atomic operations
   - Log all auto-save activities for audit trail
   - Rate limit API endpoints

## Support

For issues or questions:
1. Check logs in `/logs` directory
2. Review error messages in transaction history
3. Run integration tests to verify setup
4. Check database connectivity
5. Verify Redis is running

## Roadmap

Future enhancements:
- [ ] Email notifications for auto-saves
- [ ] Savings goals with progress tracking
- [ ] Round-up to charity
- [ ] Smart savings (AI-based)
- [ ] Savings challenges
- [ ] Multi-account rules
