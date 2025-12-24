#!/bin/bash

# AutoSave Integration Test Script
# Tests the complete flow from frontend to backend

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
GRPC_SERVER="localhost:9090"
AUTH_TOKEN=""
USER_ID=""
SOURCE_ACCOUNT_ID=""
DEST_ACCOUNT_ID=""

echo -e "${YELLOW}=== Auto-Save Integration Test ===${NC}\n"

# Function to print test step
test_step() {
    echo -e "${YELLOW}>>> $1${NC}"
}

# Function to print success
test_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

# Function to print error
test_error() {
    echo -e "${RED}✗ $1${NC}"
    exit 1
}

# Check if grpcurl is installed
if ! command -v grpcurl &> /dev/null; then
    test_error "grpcurl is not installed. Install with: brew install grpcurl"
fi

# Check if server is running
test_step "Checking if gRPC server is running on $GRPC_SERVER"
if ! grpcurl -plaintext $GRPC_SERVER list > /dev/null 2>&1; then
    test_error "gRPC server is not running on $GRPC_SERVER"
fi
test_success "Server is running"

# Step 1: Login to get auth token
test_step "Step 1: Authenticating user"
LOGIN_RESPONSE=$(grpcurl -plaintext \
    -d '{
        "email": "test@example.com",
        "password": "testpassword123"
    }' \
    $GRPC_SERVER lazervault.AuthService/Login 2>&1)

if echo "$LOGIN_RESPONSE" | grep -q "access_token"; then
    AUTH_TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.accessToken')
    test_success "Authenticated successfully"
else
    echo "$LOGIN_RESPONSE"
    test_error "Failed to authenticate. Make sure you have a test user created."
fi

# Step 2: Get user accounts
test_step "Step 2: Fetching user accounts"
ACCOUNTS_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    $GRPC_SERVER lazervault.AccountService/GetAccountsV3 2>&1)

if echo "$ACCOUNTS_RESPONSE" | grep -q "accounts"; then
    SOURCE_ACCOUNT_ID=$(echo "$ACCOUNTS_RESPONSE" | jq -r '.accounts[0].id')
    DEST_ACCOUNT_ID=$(echo "$ACCOUNTS_RESPONSE" | jq -r '.accounts[1].id // .accounts[0].id')
    test_success "Retrieved accounts: Source=$SOURCE_ACCOUNT_ID, Dest=$DEST_ACCOUNT_ID"
else
    echo "$ACCOUNTS_RESPONSE"
    test_error "Failed to get accounts. Make sure you have at least one account."
fi

# Step 3: Create an auto-save rule
test_step "Step 3: Creating auto-save rule (10% of deposits)"
CREATE_RULE_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{
        \"name\": \"Test Auto-Save Rule\",
        \"description\": \"Save 10% of every deposit\",
        \"trigger_type\": \"TRIGGER_ON_DEPOSIT\",
        \"amount_type\": \"AMOUNT_PERCENTAGE\",
        \"amount_value\": 10.0,
        \"source_account_id\": \"$SOURCE_ACCOUNT_ID\",
        \"destination_account_id\": \"$DEST_ACCOUNT_ID\"
    }" \
    $GRPC_SERVER lazervault.AutoSaveService/CreateAutoSaveRule 2>&1)

if echo "$CREATE_RULE_RESPONSE" | grep -q "success.*true"; then
    RULE_ID=$(echo "$CREATE_RULE_RESPONSE" | jq -r '.rule.id')
    test_success "Auto-save rule created with ID: $RULE_ID"
else
    echo "$CREATE_RULE_RESPONSE"
    test_error "Failed to create auto-save rule"
fi

# Step 4: Verify rule was created
test_step "Step 4: Verifying rule was created"
GET_RULES_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{}" \
    $GRPC_SERVER lazervault.AutoSaveService/GetAutoSaveRules 2>&1)

if echo "$GET_RULES_RESPONSE" | grep -q "$RULE_ID"; then
    test_success "Rule retrieved successfully"
else
    echo "$GET_RULES_RESPONSE"
    test_error "Failed to retrieve created rule"
fi

# Step 5: Simulate a deposit to trigger auto-save
test_step "Step 5: Simulating deposit to trigger auto-save"
DEPOSIT_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{
        \"target_account_id\": \"$SOURCE_ACCOUNT_ID\",
        \"amount\": 100.0,
        \"currency\": \"USD\",
        \"source_bank_name\": \"Test Bank\"
    }" \
    $GRPC_SERVER lazervault.DepositService/InitiateDeposit 2>&1)

if echo "$DEPOSIT_RESPONSE" | grep -q "success.*true"; then
    DEPOSIT_ID=$(echo "$DEPOSIT_RESPONSE" | jq -r '.deposit.id')
    test_success "Deposit initiated with ID: $DEPOSIT_ID"
else
    echo "$DEPOSIT_RESPONSE"
    test_error "Failed to initiate deposit"
fi

# Step 6: Wait for deposit to process (worker task)
test_step "Step 6: Waiting for deposit to process (10 seconds)"
sleep 10

# Step 7: Check auto-save transactions
test_step "Step 7: Checking auto-save transactions"
TRANSACTIONS_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{}" \
    $GRPC_SERVER lazervault.AutoSaveService/GetAutoSaveTransactions 2>&1)

if echo "$TRANSACTIONS_RESPONSE" | grep -q "transactions"; then
    TRANSACTION_COUNT=$(echo "$TRANSACTIONS_RESPONSE" | jq '.transactions | length')
    test_success "Found $TRANSACTION_COUNT auto-save transaction(s)"
else
    echo "$TRANSACTIONS_RESPONSE"
    test_error "Failed to retrieve auto-save transactions"
fi

# Step 8: Check auto-save statistics
test_step "Step 8: Checking auto-save statistics"
STATS_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{}" \
    $GRPC_SERVER lazervault.AutoSaveService/GetAutoSaveStatistics 2>&1)

if echo "$STATS_RESPONSE" | grep -q "statistics"; then
    TOTAL_SAVED=$(echo "$STATS_RESPONSE" | jq -r '.statistics.totalSavedAllTime')
    ACTIVE_RULES=$(echo "$STATS_RESPONSE" | jq -r '.statistics.activeRulesCount')
    test_success "Statistics: Total Saved=\$$TOTAL_SAVED, Active Rules=$ACTIVE_RULES"
else
    echo "$STATS_RESPONSE"
    test_error "Failed to retrieve auto-save statistics"
fi

# Step 9: Test manual trigger
test_step "Step 9: Testing manual trigger"
TRIGGER_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{
        \"rule_id\": \"$RULE_ID\",
        \"custom_amount\": 5.0
    }" \
    $GRPC_SERVER lazervault.AutoSaveService/TriggerAutoSave 2>&1)

if echo "$TRIGGER_RESPONSE" | grep -q "success.*true"; then
    test_success "Manual trigger executed successfully"
else
    echo "$TRIGGER_RESPONSE"
    test_error "Failed to manually trigger auto-save"
fi

# Step 10: Pause the rule
test_step "Step 10: Pausing auto-save rule"
PAUSE_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{
        \"rule_id\": \"$RULE_ID\",
        \"action\": \"pause\"
    }" \
    $GRPC_SERVER lazervault.AutoSaveService/ToggleAutoSaveRule 2>&1)

if echo "$PAUSE_RESPONSE" | grep -q "success.*true"; then
    test_success "Rule paused successfully"
else
    echo "$PAUSE_RESPONSE"
    test_error "Failed to pause rule"
fi

# Step 11: Resume the rule
test_step "Step 11: Resuming auto-save rule"
RESUME_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{
        \"rule_id\": \"$RULE_ID\",
        \"action\": \"resume\"
    }" \
    $GRPC_SERVER lazervault.AutoSaveService/ToggleAutoSaveRule 2>&1)

if echo "$RESUME_RESPONSE" | grep -q "success.*true"; then
    test_success "Rule resumed successfully"
else
    echo "$RESUME_RESPONSE"
    test_error "Failed to resume rule"
fi

# Step 12: Update the rule
test_step "Step 12: Updating auto-save rule"
UPDATE_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{
        \"rule_id\": \"$RULE_ID\",
        \"name\": \"Updated Auto-Save Rule\",
        \"amount_value\": 15.0
    }" \
    $GRPC_SERVER lazervault.AutoSaveService/UpdateAutoSaveRule 2>&1)

if echo "$UPDATE_RESPONSE" | grep -q "success.*true"; then
    test_success "Rule updated successfully"
else
    echo "$UPDATE_RESPONSE"
    test_error "Failed to update rule"
fi

# Step 13: Delete the rule (cleanup)
test_step "Step 13: Deleting auto-save rule (cleanup)"
DELETE_RESPONSE=$(grpcurl -plaintext \
    -H "authorization: Bearer $AUTH_TOKEN" \
    -d "{
        \"rule_id\": \"$RULE_ID\"
    }" \
    $GRPC_SERVER lazervault.AutoSaveService/DeleteAutoSaveRule 2>&1)

if echo "$DELETE_RESPONSE" | grep -q "success.*true"; then
    test_success "Rule deleted successfully"
else
    echo "$DELETE_RESPONSE"
    test_error "Failed to delete rule"
fi

echo -e "\n${GREEN}=== All tests passed! ===${NC}"
echo -e "${GREEN}Auto-Save integration is working end-to-end.${NC}\n"

# Summary
echo -e "${YELLOW}Test Summary:${NC}"
echo "✓ Authentication"
echo "✓ Rule Creation"
echo "✓ Rule Retrieval"
echo "✓ Deposit Triggering"
echo "✓ Transaction History"
echo "✓ Statistics"
echo "✓ Manual Triggering"
echo "✓ Rule Toggle (Pause/Resume)"
echo "✓ Rule Update"
echo "✓ Rule Deletion"

exit 0
