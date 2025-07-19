#!/bin/bash

# Generate traffic for monitoring demonstration

echo "Starting traffic generation for Go Backend Kit monitoring demo..."

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Base URL
BASE_URL="http://localhost:8080"

# Function to make requests with error handling
make_request() {
    local method=$1
    local url=$2
    local data=$3
    
    if [ -z "$data" ]; then
        curl -s -X $method "$url" > /dev/null 2>&1
    else
        curl -s -X $method "$url" -H "Content-Type: application/json" -d "$data" > /dev/null 2>&1
    fi
    
    if [ $? -eq 0 ]; then
        echo -n "."
    else
        echo -n "x"
    fi
}

echo -e "\n${GREEN}1. Testing authentication endpoints...${NC}"
echo "   Generating login attempts (90% success rate simulation):"
for i in {1..50}; do
    make_request POST "$BASE_URL/api/v1/auth/login" '{"username":"test","password":"test"}'
    sleep 0.1
done
echo -e " ${GREEN}Done${NC}"

echo -e "\n${GREEN}2. Testing user registration...${NC}"
for i in {1..10}; do
    make_request POST "$BASE_URL/api/v1/auth/register" '{"username":"user'$i'","password":"pass"}'
    sleep 0.5
done
echo -e " ${GREEN}Done${NC}"

echo -e "\n${GREEN}3. Testing cache operations...${NC}"
echo "   Testing cache hit/miss (80% hit rate simulation):"
for i in {1..30}; do
    key=$((i % 5))
    make_request GET "$BASE_URL/api/v1/demo/cache/key$key"
    sleep 0.2
done
echo -e " ${GREEN}Done${NC}"

echo -e "\n${GREEN}4. Testing public endpoints...${NC}"
for i in {1..20}; do
    make_request GET "$BASE_URL/api/v1/ping"
    sleep 0.1
done
echo -e " ${GREEN}Done${NC}"

echo -e "\n${YELLOW}5. Generating some errors...${NC}"
error_types=("database_error" "validation_error" "timeout_error" "permission_error")
for i in {1..20}; do
    error_type=${error_types[$((i % 4))]}
    make_request GET "$BASE_URL/api/v1/demo/error?type=$error_type"
    sleep 0.5
done
echo -e " ${YELLOW}Done${NC}"

echo -e "\n${GREEN}6. Testing message queue operations...${NC}"
queues=("orders" "notifications" "analytics" "reports")
for i in {1..25}; do
    queue=${queues[$((i % 4))]}
    make_request POST "$BASE_URL/api/v1/demo/message?queue=$queue"
    sleep 0.3
done
echo -e " ${GREEN}Done${NC}"

echo -e "\n${GREEN}7. Simulating slow endpoints (may trigger SLA violations)...${NC}"
echo "   Note: These requests may take 0.5-2 seconds each"
for i in {1..10}; do
    # This will need a valid JWT token in real scenario
    make_request GET "$BASE_URL/api/v1/data/analytics"
    sleep 1
done
echo -e " ${GREEN}Done${NC}"

echo -e "\n${GREEN}8. Testing health check endpoint...${NC}"
for i in {1..5}; do
    make_request GET "$BASE_URL/health"
    sleep 1
done
echo -e " ${GREEN}Done${NC}"

echo -e "\n${RED}9. Generating high error rate (for alert testing)...${NC}"
echo "   This should trigger error rate alerts in Prometheus"
for i in {1..30}; do
    make_request GET "$BASE_URL/api/v1/demo/error?type=critical_error"
    sleep 0.1
done
echo -e " ${RED}Done${NC}"

echo -e "\n${GREEN}10. Mixed realistic traffic...${NC}"
echo "   Simulating normal application usage patterns"
for i in {1..100}; do
    case $((i % 10)) in
        0|1|2|3)
            make_request GET "$BASE_URL/api/v1/ping"
            ;;
        4|5)
            make_request POST "$BASE_URL/api/v1/auth/login" '{"username":"test","password":"test"}'
            ;;
        6|7)
            key=$((i % 10))
            make_request GET "$BASE_URL/api/v1/demo/cache/key$key"
            ;;
        8)
            queue=${queues[$((i % 4))]}
            make_request POST "$BASE_URL/api/v1/demo/message?queue=$queue"
            ;;
        9)
            if [ $((i % 20)) -eq 0 ]; then
                make_request GET "$BASE_URL/api/v1/demo/error?type=random_error"
            else
                make_request GET "$BASE_URL/health"
            fi
            ;;
    esac
    sleep 0.2
done
echo -e " ${GREEN}Done${NC}"

echo -e "\n${GREEN}Traffic generation complete!${NC}"
echo "Check your Grafana dashboards at http://localhost:3000 to see the metrics."
echo "Default credentials: admin/admin"