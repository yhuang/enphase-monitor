#!/usr/bin/env bash

# Script to ensure cached results exist and run all test cases
# Checks cache for each date, generates missing cache with proper rate limiting,
# then runs validation tests for all dates

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Auto-discover test dates from enphase_api_*.json files in test-data/
# This automatically includes any new test cases without manual maintenance.
# To disable a test temporarily, rename the file (e.g., add .disabled extension)
DATES=()
while IFS= read -r file; do
    if [[ -n "$file" ]]; then
        # Extract date from filename: enphase_api_2026-01-15.json -> 2026-01-15
        date=$(basename "$file" | sed 's/^enphase_api_//' | sed 's/\.json$//')
        DATES+=("$date")
    fi
done < <(find test-data -name "enphase_api_*.json" -type f 2>/dev/null | sort)

# Verify we found at least one test case
if [ ${#DATES[@]} -eq 0 ]; then
    echo -e "${RED}ERROR: No test cases found in test-data/enphase_api_*.json${NC}"
    echo "Please create at least one expected values file to run tests."
    exit 1
fi

# Program executable (assumes we are in the project root)
PROGRAM="./enphase-monitor"

echo "=========================================="
echo "Enphase Monitor Test Suite"
echo "=========================================="
echo ""

# Track which dates need cache generation
NEEDS_CACHE=()
HAS_CACHE=()

# First pass: Check which dates have cache
echo "Step 1: Checking existing cache..."
for DATE in "${DATES[@]}"; do
    echo -n "  Checking ${DATE}... "

    # Try to run in test mode - capture output
    OUTPUT=$($PROGRAM --test --once --date "$DATE" 2>&1) || true

    # Check if we got metrics output (cache exists) or cache error
    if echo "$OUTPUT" | grep -q "INDIVIDUAL SYSTEMS"; then
        echo -e "${GREEN}✓ Cache exists${NC}"
        HAS_CACHE+=("$DATE")
    elif echo "$OUTPUT" | grep -q "no cached response available"; then
        echo -e "${YELLOW}✗ Cache missing${NC}"
        NEEDS_CACHE+=("$DATE")
    elif echo "$OUTPUT" | grep -q "TEST MODE"; then
        # Test mode ran - check if we got any output (cache exists, even if validation failed)
        if echo "$OUTPUT" | grep -q "ENPHASE DUAL SYSTEM MONITOR"; then
            echo -e "${GREEN}✓ Cache exists${NC}"
            HAS_CACHE+=("$DATE")
        else
            echo -e "${YELLOW}✗ Cache missing${NC}"
            NEEDS_CACHE+=("$DATE")
        fi
    else
        # Unknown state - assume cache missing to be safe
        echo -e "${YELLOW}✗ Cache missing${NC}"
        NEEDS_CACHE+=("$DATE")
    fi
done

echo ""
echo "Summary:"
echo "  Dates with cache: ${#HAS_CACHE[@]}"
echo "  Dates needing cache: ${#NEEDS_CACHE[@]}"
echo ""

# If cache is missing, generate it
if [ ${#NEEDS_CACHE[@]} -gt 0 ]; then
       echo "Step 2: Generating cache for missing dates..."
       echo "  (Rate limit: 10 API calls per minute)"
       echo "  (Each date requires 10 API calls per invocation)"
    echo ""

    LAST_CACHE_TIME=$(date +%s)
    for i in "${!NEEDS_CACHE[@]}"; do
        DATE="${NEEDS_CACHE[$i]}"

        # Wait 60 seconds (1 minute) between dates to respect the 10 calls per minute limit
        CURRENT_TIME=$(date +%s)
        if [ $i -gt 0 ]; then
            TIME_SINCE_LAST=$((CURRENT_TIME - LAST_CACHE_TIME))
            # Wait 1 minute (60 seconds) between dates
            if [ $TIME_SINCE_LAST -lt 60 ]; then
                WAIT_TIME=$((60 - TIME_SINCE_LAST))
                echo "  Waiting ${WAIT_TIME} seconds before next date (rate limit: 10 calls per minute)..."
                sleep $WAIT_TIME
            fi
        fi

        echo "  Generating cache for ${DATE} (10 API calls)..."
        START_TIME=$(date +%s)
        if $PROGRAM --once --date "$DATE" >/dev/null 2>&1; then
            END_TIME=$(date +%s)
            ELAPSED=$((END_TIME - START_TIME))
            echo -e "    ${GREEN}✓ Cache generated successfully (took ${ELAPSED} seconds)${NC}"
            LAST_CACHE_TIME=$(date +%s)
        else
            echo -e "    ${RED}✗ Failed to generate cache${NC}"
            echo "    Error output:"
            $PROGRAM --once --date "$DATE" 2>&1 | head -5 | sed 's/^/      /'
            exit 1
        fi

        echo ""
    done
    echo -e "${GREEN}✓ Cache generation complete!${NC}"
    echo ""
else
    echo -e "${GREEN}✓ All dates have cache.${NC}"
    echo ""
fi

# Step 3: Run all validation tests
echo "Step 3: Running validation tests..."
echo "=========================================="
echo ""

# Track test results
PASSED=()
FAILED=()
TOTAL_PASSED=0
TOTAL_FAILED=0

for DATE in "${DATES[@]}"; do
    echo -e "${BLUE}Testing ${DATE}...${NC}"
    echo "----------------------------------------"

    # Run test and capture output
    OUTPUT=$($PROGRAM --test --once --date "$DATE" 2>&1) || TEST_EXIT_CODE=$?

    # Extract validation results
    if echo "$OUTPUT" | grep -q "ALL VALIDATIONS PASSED"; then
        echo -e "${GREEN}✓ PASSED${NC}"
        PASSED+=("$DATE")
        TOTAL_PASSED=$((TOTAL_PASSED + 1))
    elif echo "$OUTPUT" | grep -q "SOME VALIDATIONS FAILED"; then
        echo -e "${RED}✗ FAILED${NC}"
        FAILED+=("$DATE")
        TOTAL_FAILED=$((TOTAL_FAILED + 1))

        # Show validation details
        echo "$OUTPUT" | grep -A 100 "=== VALIDATION RESULTS ===" | head -30
    elif echo "$OUTPUT" | grep -q "Validation failed"; then
        echo -e "${RED}✗ FAILED${NC}"
        FAILED+=("$DATE")
        TOTAL_FAILED=$((TOTAL_FAILED + 1))

        # Show error message
        echo "$OUTPUT" | grep "Validation failed" | head -1
    else
        echo -e "${YELLOW}⚠ UNKNOWN${NC}"
        echo "$OUTPUT" | tail -5
    fi

    echo ""
done

# Final summary
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo ""
echo "Total tests: ${#DATES[@]}"
echo -e "  ${GREEN}Passed: ${TOTAL_PASSED}${NC}"
echo -e "  ${RED}Failed: ${TOTAL_FAILED}${NC}"
echo ""

if [ ${#FAILED[@]} -gt 0 ]; then
    echo -e "${RED}Failed dates:${NC}"
    for DATE in "${FAILED[@]}"; do
        echo "  - $DATE"
    done
    echo ""
    echo "To view detailed results for a failed date, run:"
    echo "  $PROGRAM --test --once --date <date>"
    echo ""
    exit 1
else
    echo -e "${GREEN}✓ All tests passed!${NC}"
    echo ""
    exit 0
fi
