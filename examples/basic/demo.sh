#!/bin/bash

# Go Backend Kit Demo Script
# This script demonstrates various features of the go-backend-kit

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
API_BASE="http://localhost:8080"
METRICS_URL="http://localhost:9091/metrics"

echo -e "${GREEN}Go Backend Kit Demo${NC}"
echo "===================="
echo ""

# Function to wait for server
wait_for_server() {
    echo -n "Waiting for server to start..."
    for i in {1..30}; do
        if curl -s "${API_BASE}/health" > /dev/null 2>&1; then
            echo -e " ${GREEN}Ready!${NC}"
            return 0
        fi
        echo -n "."
        sleep 1
    done
    echo -e " ${RED}Timeout!${NC}"
    return 1
}

# Function to make API call
api_call() {
    local method=$1
    local endpoint=$2
    local data=$3
    local token=$4
    
    echo -e "\n${YELLOW}${method} ${endpoint}${NC}"
    
    local args=("-X" "$method")
    if [ -n "$data" ]; then
        args+=("-H" "Content-Type: application/json" "-d" "$data")
    fi
    if [ -n "$token" ]; then
        args+=("-H" "Authorization: Bearer $token")
    fi
    
    response=$(curl -s "${args[@]}" "${API_BASE}${endpoint}")
    echo "$response" | jq . 2>/dev/null || echo "$response"
}

# Function to check metrics
check_metrics() {
    echo -e "\n${YELLOW}Checking Prometheus Metrics${NC}"
    echo "=============================="
    
    # Check if metrics endpoint is available
    if curl -s "$METRICS_URL" > /dev/null 2>&1; then
        echo -e "${GREEN}Metrics endpoint is available${NC}"
        
        # Show some key metrics
        echo -e "\nHTTP Request Metrics:"
        curl -s "$METRICS_URL" | grep "gin_requests_total" | head -5
        
        echo -e "\nDatabase Metrics:"
        curl -s "$METRICS_URL" | grep "db_" | head -5
        
        echo -e "\nBusiness Metrics:"
        curl -s "$METRICS_URL" | grep "user_" | head -5
    else
        echo -e "${RED}Metrics endpoint not available${NC}"
    fi
}

# Main demo flow
main() {
    # Check if server is running
    if ! wait_for_server; then
        echo -e "${RED}Error: Server is not running${NC}"
        echo "Please start the server with: go run main.go"
        exit 1
    fi
    
    echo -e "\n${GREEN}1. Testing Health Endpoints${NC}"
    echo "============================"
    api_call "GET" "/health"
    api_call "GET" "/ready"
    
    echo -e "\n${GREEN}2. Testing Public Endpoints${NC}"
    echo "============================="
    api_call "GET" "/api/v1/public/info"
    
    echo -e "\n${GREEN}3. Testing Authentication${NC}"
    echo "=========================="
    # Login
    login_response=$(api_call "POST" "/api/v1/public/login" '{"username":"test@example.com","password":"password123"}')
    token=$(echo "$login_response" | jq -r '.data.token' 2>/dev/null)
    
    if [ -n "$token" ] && [ "$token" != "null" ]; then
        echo -e "${GREEN}Login successful!${NC}"
        
        echo -e "\n${GREEN}4. Testing Protected Endpoints${NC}"
        echo "=============================="
        api_call "GET" "/api/v1/users" "" "$token"
        api_call "GET" "/api/v1/users/1" "" "$token"
        
        echo -e "\n${GREEN}5. Testing Database Operations${NC}"
        echo "==============================="
        # Create
        api_call "POST" "/api/v1/products" '{"name":"Test Product","price":99.99}' "$token"
        # List
        api_call "GET" "/api/v1/products" "" "$token"
        
        echo -e "\n${GREEN}6. Testing Error Handling${NC}"
        echo "=========================="
        api_call "GET" "/api/v1/public/error"
        api_call "GET" "/api/v1/nonexistent" "" "$token"
    else
        echo -e "${YELLOW}Login failed or not implemented${NC}"
    fi
    
    echo -e "\n${GREEN}7. Testing Rate Limiting${NC}"
    echo "========================"
    echo "Making multiple requests to test rate limiting..."
    for i in {1..10}; do
        echo -n "Request $i: "
        status=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/v1/public/info")
        if [ "$status" = "429" ]; then
            echo -e "${RED}Rate limited!${NC}"
        else
            echo -e "${GREEN}OK ($status)${NC}"
        fi
    done
    
    # Check metrics
    check_metrics
    
    echo -e "\n${GREEN}Demo completed!${NC}"
    echo "================"
    echo ""
    echo "Next steps:"
    echo "1. Check Grafana dashboards at http://localhost:3000"
    echo "2. Check Prometheus at http://localhost:9090"
    echo "3. Review logs for detailed information"
}

# Run main function
main