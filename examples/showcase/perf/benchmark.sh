#!/bin/bash
set -euo pipefail

# Astra Showcase Performance Benchmark Script
# Tests API performance against defined targets:
# - Health check: 10,000+ QPS
# - Product list (cached): 5,000+ QPS
# - Order creation: 1,000+ QPS
# - gRPC stock query: 3,000+ QPS

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
API_URL="${API_URL:-http://localhost:8080}"
GRPC_URL="${GRPC_URL:-localhost:9091}"
JWT_TOKEN="${JWT_TOKEN:-}"

echo -e "${BLUE}🚀 Astra Showcase Performance Benchmark${NC}"
echo "=========================================="
echo "API URL: ${API_URL}"
echo "gRPC URL: ${GRPC_URL}"
echo "Results: ${RESULTS_DIR}"
echo ""

# Create results directory
mkdir -p "${RESULTS_DIR}"

# Function to check if service is ready
check_service() {
    local url=$1
    local max_attempts=30
    local attempt=1

    echo -n "Checking service availability at ${url}..."
    while [ $attempt -le $max_attempts ]; do
        if curl -sf "${url}/health" > /dev/null 2>&1; then
            echo -e " ${GREEN}✓${NC}"
            return 0
        fi
        echo -n "."
        sleep 2
        attempt=$((attempt + 1))
    done

    echo -e " ${RED}✗${NC}"
    echo -e "${RED}ERROR: Service not available after ${max_attempts} attempts${NC}"
    return 1
}

# Function to get JWT token
get_jwt_token() {
    if [ -z "${JWT_TOKEN}" ]; then
        echo -e "${YELLOW}⚠ No JWT_TOKEN provided, attempting to generate...${NC}"

        # Try to generate token using the gentoken tool
        if [ -f "${SCRIPT_DIR}/../tools/gentoken/main.go" ]; then
            JWT_TOKEN=$(go run "${SCRIPT_DIR}/../tools/gentoken/main.go" 2>/dev/null || echo "")
        fi

        if [ -z "${JWT_TOKEN}" ]; then
            echo -e "${YELLOW}⚠ Could not generate JWT token, authentication tests will be skipped${NC}"
            return 1
        fi
    fi
    echo -e "${GREEN}✓ JWT token available${NC}"
    return 0
}

# Check dependencies
check_dependencies() {
    local missing=0

    echo "Checking dependencies..."

    if ! command -v wrk &> /dev/null; then
        echo -e "${RED}✗ wrk not found. Install: brew install wrk (macOS) or apt-get install wrk (Ubuntu)${NC}"
        missing=1
    else
        echo -e "${GREEN}✓ wrk available${NC}"
    fi

    if ! command -v ghz &> /dev/null; then
        echo -e "${YELLOW}⚠ ghz not found. Install: go install github.com/bojand/ghz/cmd/ghz@latest${NC}"
        echo "  gRPC tests will be skipped."
    else
        echo -e "${GREEN}✓ ghz available${NC}"
    fi

    if [ $missing -eq 1 ]; then
        echo -e "${RED}ERROR: Missing required dependencies${NC}"
        exit 1
    fi

    echo ""
}

# Scenario 1: Health Check (Empty Route)
benchmark_health_check() {
    echo -e "${BLUE}📊 Scenario 1: Health Check (Empty Route)${NC}"
    echo "Target: 10,000+ QPS, P99 < 1ms"
    echo "Running: wrk -t4 -c100 -d30s --latency ${API_URL}/health"
    echo ""

    wrk -t4 -c100 -d30s --latency \
        "${API_URL}/health" \
        | tee "${RESULTS_DIR}/01_health_${TIMESTAMP}.txt"

    echo ""
}

# Scenario 2: Product List (with Redis cache)
benchmark_product_list() {
    echo -e "${BLUE}📊 Scenario 2: Product List (Cached)${NC}"
    echo "Target: 5,000+ QPS, P99 < 5ms"

    if [ -z "${JWT_TOKEN}" ]; then
        echo -e "${YELLOW}⚠ Skipped (no JWT token)${NC}"
        echo ""
        return
    fi

    echo "Warming up cache..."
    curl -sf -H "Authorization: Bearer ${JWT_TOKEN}" \
         "${API_URL}/api/v1/products?page=1&page_size=20" > /dev/null || true

    echo "Running: wrk -t8 -c200 -d30s --latency (with JWT)"
    echo ""

    wrk -t8 -c200 -d30s --latency \
        -H "Authorization: Bearer ${JWT_TOKEN}" \
        "${API_URL}/api/v1/products?page=1&page_size=20" \
        | tee "${RESULTS_DIR}/02_products_${TIMESTAMP}.txt"

    echo ""
}

# Scenario 3: Order Creation (write operation)
benchmark_order_creation() {
    echo -e "${BLUE}📊 Scenario 3: Order Creation (Write Operation)${NC}"
    echo "Target: 1,000+ QPS, P99 < 20ms"

    if [ -z "${JWT_TOKEN}" ]; then
        echo -e "${YELLOW}⚠ Skipped (no JWT token)${NC}"
        echo ""
        return
    fi

    # Create Lua script for wrk
    cat > "${RESULTS_DIR}/order_create.lua" << 'EOF'
wrk.method = "POST"
wrk.headers["Content-Type"] = "application/json"
wrk.headers["Authorization"] = "Bearer " .. os.getenv("JWT_TOKEN")

counter = 0

request = function()
    counter = counter + 1
    local body = string.format('{"items":[{"product_id":%d,"qty":1}]}', (counter % 10) + 1)
    return wrk.format(nil, nil, nil, body)
end
EOF

    echo "Running: wrk -t4 -c50 -d30s --latency -s order_create.lua"
    echo ""

    export JWT_TOKEN
    wrk -t4 -c50 -d30s --latency \
        -s "${RESULTS_DIR}/order_create.lua" \
        "${API_URL}/api/v1/orders" \
        | tee "${RESULTS_DIR}/03_orders_${TIMESTAMP}.txt"

    rm -f "${RESULTS_DIR}/order_create.lua"
    echo ""
}

# Scenario 4: gRPC Stock Query
benchmark_grpc_stock() {
    echo -e "${BLUE}📊 Scenario 4: gRPC Stock Query${NC}"
    echo "Target: 3,000+ QPS, P99 < 5ms"

    if ! command -v ghz &> /dev/null; then
        echo -e "${YELLOW}⚠ Skipped (ghz not installed)${NC}"
        echo ""
        return
    fi

    # Check if proto file exists
    if [ ! -f "${SCRIPT_DIR}/../proto/inventory.proto" ]; then
        echo -e "${YELLOW}⚠ Skipped (proto file not found)${NC}"
        echo ""
        return
    fi

    echo "Running: ghz --insecure --proto inventory.proto -n 100000 -c 50"
    echo ""

    ghz --insecure \
        --proto "${SCRIPT_DIR}/../proto/inventory.proto" \
        --call showcase.inventory.v1.InventoryService/GetStock \
        -d '{"product_id":1,"tenant_id":1}' \
        -n 100000 -c 50 \
        --format pretty \
        "${GRPC_URL}" \
        | tee "${RESULTS_DIR}/04_grpc_stock_${TIMESTAMP}.txt"

    echo ""
}

# Scenario 5: gRPC Batch Stock Query
benchmark_grpc_batch_stock() {
    echo -e "${BLUE}📊 Scenario 5: gRPC Batch Stock Query${NC}"
    echo "Target: 2,000+ QPS, P99 < 10ms"

    if ! command -v ghz &> /dev/null; then
        echo -e "${YELLOW}⚠ Skipped (ghz not installed)${NC}"
        echo ""
        return
    fi

    if [ ! -f "${SCRIPT_DIR}/../proto/inventory.proto" ]; then
        echo -e "${YELLOW}⚠ Skipped (proto file not found)${NC}"
        echo ""
        return
    fi

    echo "Running: ghz --insecure --proto inventory.proto (BatchGetStock)"
    echo ""

    ghz --insecure \
        --proto "${SCRIPT_DIR}/../proto/inventory.proto" \
        --call showcase.inventory.v1.InventoryService/BatchGetStock \
        -d '{"tenant_id":1,"product_ids":[1,2,3,4,5]}' \
        -n 60000 -c 30 \
        --format pretty \
        "${GRPC_URL}" \
        | tee "${RESULTS_DIR}/05_grpc_batch_${TIMESTAMP}.txt"

    echo ""
}

# Generate summary report
generate_summary() {
    echo -e "${BLUE}📈 Generating Summary Report${NC}"
    echo ""

    cat > "${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md" << EOF
# Astra Showcase Performance Benchmark Report

**Date**: $(date '+%Y-%m-%d %H:%M:%S')
**API URL**: ${API_URL}
**gRPC URL**: ${GRPC_URL}

## Summary

| Scenario | Target QPS | Actual QPS | Target P99 | Actual P99 | Status |
|----------|-----------|------------|-----------|------------|--------|
EOF

    # Parse wrk results and append to summary
    # Note: This is a simplified parser, real implementation would need more robust parsing

    echo "| Health Check | 10,000+ | TBD | < 1ms | TBD | ⏳ |" >> "${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md"
    echo "| Product List | 5,000+ | TBD | < 5ms | TBD | ⏳ |" >> "${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md"
    echo "| Order Creation | 1,000+ | TBD | < 20ms | TBD | ⏳ |" >> "${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md"
    echo "| gRPC Stock | 3,000+ | TBD | < 5ms | TBD | ⏳ |" >> "${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md"
    echo "| gRPC Batch | 2,000+ | TBD | < 10ms | TBD | ⏳ |" >> "${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md"

    cat >> "${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md" << EOF

## Detailed Results

### Scenario 1: Health Check
\`\`\`
$(cat "${RESULTS_DIR}/01_health_${TIMESTAMP}.txt" 2>/dev/null || echo "No data")
\`\`\`

### Scenario 2: Product List (Cached)
\`\`\`
$(cat "${RESULTS_DIR}/02_products_${TIMESTAMP}.txt" 2>/dev/null || echo "Skipped")
\`\`\`

### Scenario 3: Order Creation
\`\`\`
$(cat "${RESULTS_DIR}/03_orders_${TIMESTAMP}.txt" 2>/dev/null || echo "Skipped")
\`\`\`

### Scenario 4: gRPC Stock Query
\`\`\`
$(cat "${RESULTS_DIR}/04_grpc_stock_${TIMESTAMP}.txt" 2>/dev/null || echo "Skipped")
\`\`\`

### Scenario 5: gRPC Batch Stock Query
\`\`\`
$(cat "${RESULTS_DIR}/05_grpc_batch_${TIMESTAMP}.txt" 2>/dev/null || echo "Skipped")
\`\`\`

## System Information

- **OS**: $(uname -s) $(uname -r)
- **CPU**: $(sysctl -n machdep.cpu.brand_string 2>/dev/null || grep "model name" /proc/cpuinfo | head -1 | cut -d: -f2 | xargs)
- **Memory**: $(sysctl -n hw.memsize 2>/dev/null | awk '{print $1/1024/1024/1024 " GB"}' || free -h | grep Mem | awk '{print $2}')

## Notes

- Results marked as "TBD" require manual parsing from detailed results
- Run \`make parse-benchmark\` to automatically parse and update summary
- Ensure services are running: \`docker-compose up -d\` and \`go run ./cmd/api\`
- For JWT token: \`export JWT_TOKEN=\$(go run ./tools/gentoken/main.go)\`

EOF

    echo -e "${GREEN}✓ Summary report generated: ${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md${NC}"
    echo ""
}

# Main execution
main() {
    check_dependencies

    if ! check_service "${API_URL}"; then
        echo -e "${RED}ERROR: API service not available. Start with: docker-compose up -d && go run ./cmd/api${NC}"
        exit 1
    fi

    get_jwt_token || true

    echo -e "${GREEN}Starting benchmark...${NC}"
    echo ""

    benchmark_health_check
    benchmark_product_list
    benchmark_order_creation
    benchmark_grpc_stock
    benchmark_grpc_batch_stock

    generate_summary

    echo -e "${GREEN}✅ Benchmark completed!${NC}"
    echo ""
    echo "Results saved to: ${RESULTS_DIR}/"
    echo "Summary: ${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md"
    echo ""
    echo "To view results:"
    echo "  cat ${RESULTS_DIR}/BENCHMARK_${TIMESTAMP}.md"
}

# Run main function
main "$@"
