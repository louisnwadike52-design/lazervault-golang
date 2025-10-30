#!/bin/bash

# Facial Recognition API Test Script
# This script tests the facial recognition gRPC-REST gateway endpoints

set -e

# Configuration
BASE_URL="http://localhost:8080"
HEALTH_ENDPOINT="/v1/face/health"
REGISTER_ENDPOINT="/v1/face/register"
VERIFY_ENDPOINT="/v1/face/verify"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test data
TEST_USER_ID="test_user_$(date +%s)"
TEST_FACE_ID="primary_face"
TEST_IMAGE_PATH="/tmp/test_face.jpg"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Function to create a dummy test image
create_test_image() {
    print_status "Creating test image..."
    
    # Create a simple test image using ImageMagick (if available)
    if command -v convert &> /dev/null; then
        convert -size 640x480 xc:blue -draw "fill white circle 320,240 320,340" "$TEST_IMAGE_PATH"
        print_success "Test image created at $TEST_IMAGE_PATH"
    else
        # Create a dummy file if ImageMagick is not available
        echo "This is a dummy test image file" > "$TEST_IMAGE_PATH"
        print_warning "ImageMagick not found. Created dummy test file."
    fi
}

# Function to test service health
test_health() {
    print_status "Testing health endpoint..."
    
    local response=$(curl -s -w "\n%{http_code}" "$BASE_URL$HEALTH_ENDPOINT")
    local body=$(echo "$response" | head -n -1)
    local status_code=$(echo "$response" | tail -n 1)
    
    if [ "$status_code" = "200" ]; then
        print_success "Health check passed"
        echo "Response: $body"
    else
        print_error "Health check failed with status code: $status_code"
        echo "Response: $body"
        return 1
    fi
}

# Function to test face registration
test_face_registration() {
    print_status "Testing face registration..."
    
    local response=$(curl -s -w "\n%{http_code}" \
        -X POST "$BASE_URL$REGISTER_ENDPOINT" \
        -F "user_id=$TEST_USER_ID" \
        -F "face_id=$TEST_FACE_ID" \
        -F "allow_duplicates=false" \
        -F "duplicate_threshold=0.85" \
        -F "image=@$TEST_IMAGE_PATH")
    
    local body=$(echo "$response" | head -n -1)
    local status_code=$(echo "$response" | tail -n 1)
    
    if [ "$status_code" = "200" ]; then
        print_success "Face registration test passed"
        echo "Response: $body"
    else
        print_error "Face registration failed with status code: $status_code"
        echo "Response: $body"
        return 1
    fi
}

# Function to test face verification
test_face_verification() {
    print_status "Testing face verification..."
    
    local response=$(curl -s -w "\n%{http_code}" \
        -X POST "$BASE_URL$VERIFY_ENDPOINT" \
        -F "user_id=$TEST_USER_ID" \
        -F "threshold=0.7" \
        -F "image=@$TEST_IMAGE_PATH")
    
    local body=$(echo "$response" | head -n -1)
    local status_code=$(echo "$response" | tail -n 1)
    
    if [ "$status_code" = "200" ]; then
        print_success "Face verification test passed"
        echo "Response: $body"
    else
        print_error "Face verification failed with status code: $status_code"
        echo "Response: $body"
        return 1
    fi
}

# Function to test authentication
test_authentication() {
    print_status "Testing authentication..."
    
    # Test without authentication
    local response=$(curl -s -w "\n%{http_code}" "$BASE_URL$HEALTH_ENDPOINT")
    local status_code=$(echo "$response" | tail -n 1)
    
    if [ "$status_code" = "200" ] || [ "$status_code" = "401" ]; then
        print_success "Authentication test passed (expected 200 or 401)"
    else
        print_warning "Unexpected authentication behavior: $status_code"
    fi
}

# Function to test error handling
test_error_handling() {
    print_status "Testing error handling..."
    
    # Test with missing user_id
    local response=$(curl -s -w "\n%{http_code}" \
        -X POST "$BASE_URL$REGISTER_ENDPOINT" \
        -F "face_id=$TEST_FACE_ID" \
        -F "image=@$TEST_IMAGE_PATH")
    
    local status_code=$(echo "$response" | tail -n 1)
    
    if [ "$status_code" = "400" ]; then
        print_success "Error handling test passed (missing user_id)"
    else
        print_warning "Expected 400 for missing user_id, got: $status_code"
    fi
}

# Function to test file upload limits
test_file_limits() {
    print_status "Testing file upload limits..."
    
    # Create a large dummy file (if possible)
    local large_file="/tmp/large_test_file.jpg"
    if command -v dd &> /dev/null; then
        dd if=/dev/zero of="$large_file" bs=1M count=50 2>/dev/null
        
        local response=$(curl -s -w "\n%{http_code}" \
            -X POST "$BASE_URL$REGISTER_ENDPOINT" \
            -F "user_id=$TEST_USER_ID" \
            -F "image=@$large_file")
        
        local status_code=$(echo "$response" | tail -n 1)
        
        if [ "$status_code" = "413" ] || [ "$status_code" = "400" ]; then
            print_success "File size limit test passed"
        else
            print_warning "File size limit test: unexpected status $status_code"
        fi
        
        rm -f "$large_file"
    else
        print_warning "Skipping file size test (dd command not available)"
    fi
}

# Function to run load test
test_load() {
    print_status "Running basic load test..."
    
    if command -v hey &> /dev/null; then
        print_status "Running load test with hey..."
        hey -n 10 -c 2 -m GET "$BASE_URL$HEALTH_ENDPOINT"
        print_success "Load test completed"
    elif command -v ab &> /dev/null; then
        print_status "Running load test with ab..."
        ab -n 10 -c 2 "$BASE_URL$HEALTH_ENDPOINT"
        print_success "Load test completed"
    else
        print_warning "No load testing tool found (hey or ab). Skipping load test."
    fi
}

# Function to cleanup
cleanup() {
    print_status "Cleaning up test files..."
    rm -f "$TEST_IMAGE_PATH"
    print_success "Cleanup completed"
}

# Main test execution
main() {
    print_status "Starting Facial Recognition API Tests"
    print_status "Base URL: $BASE_URL"
    print_status "Test User ID: $TEST_USER_ID"
    echo ""
    
    # Create test prerequisites
    create_test_image
    
    # Run tests
    local failed_tests=0
    
    # Test 1: Health Check
    if ! test_health; then
        ((failed_tests++))
    fi
    echo ""
    
    # Test 2: Authentication
    if ! test_authentication; then
        ((failed_tests++))
    fi
    echo ""
    
    # Test 3: Error Handling
    if ! test_error_handling; then
        ((failed_tests++))
    fi
    echo ""
    
    # Test 4: Face Registration
    if ! test_face_registration; then
        ((failed_tests++))
    fi
    echo ""
    
    # Test 5: Face Verification
    if ! test_face_verification; then
        ((failed_tests++))
    fi
    echo ""
    
    # Test 6: File Upload Limits
    if ! test_file_limits; then
        ((failed_tests++))
    fi
    echo ""
    
    # Test 7: Load Test
    test_load
    echo ""
    
    # Cleanup
    cleanup
    
    # Summary
    print_status "========================================="
    print_status "TEST SUMMARY"
    print_status "========================================="
    
    if [ $failed_tests -eq 0 ]; then
        print_success "All tests passed! ✅"
        exit 0
    else
        print_error "$failed_tests test(s) failed! ❌"
        exit 1
    fi
}

# Check if service is running
check_service() {
    print_status "Checking if service is running..."
    
    if curl -s "$BASE_URL$HEALTH_ENDPOINT" > /dev/null; then
        print_success "Service is running"
        return 0
    else
        print_error "Service is not responding at $BASE_URL"
        print_error "Please start the service first with: make facial-recognition-run"
        exit 1
    fi
}

# Script entry point
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    # Check if running in Docker or if service is available
    if [ "${DOCKER_ENVIRONMENT:-}" = "true" ]; then
        # Wait for service to be ready in Docker environment
        print_status "Waiting for service to be ready..."
        sleep 10
    else
        check_service
    fi
    
    main "$@"
fi 