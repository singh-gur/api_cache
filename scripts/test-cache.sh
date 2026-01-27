#!/bin/bash

# Simple script to test the API Cache Proxy functionality

set -e

API_URL="${API_URL:-http://localhost:8085}"
ENDPOINT="${ENDPOINT:-/get}"

echo "Testing API Cache Proxy at $API_URL"
echo "=========================================="
echo ""

# Health check
echo "1. Health Check:"
curl -s "$API_URL/health" | jq '.' || echo "Health check failed"
echo ""
echo ""

# First request (cache miss)
echo "2. First Request (Cache MISS expected):"
echo "Request: GET $API_URL$ENDPOINT"
RESPONSE1=$(curl -s -w "\nStatus: %{http_code}\nTime: %{time_total}s\n" "$API_URL$ENDPOINT")
echo "$RESPONSE1"
echo ""
echo ""

# Second request (cache hit)
echo "3. Second Request (Cache HIT expected):"
echo "Request: GET $API_URL$ENDPOINT"
sleep 1
RESPONSE2=$(curl -s -w "\nStatus: %{http_code}\nTime: %{time_total}s\n" -v "$API_URL$ENDPOINT" 2>&1 | grep -E "(X-Cache|Status|Time)")
echo "$RESPONSE2"
echo ""
echo ""

# Request with query parameters
echo "4. Request with Query Parameters:"
echo "Request: GET $API_URL$ENDPOINT?page=1&limit=10"
curl -s -w "\nStatus: %{http_code}\n" "$API_URL$ENDPOINT?page=1&limit=10" | tail -n 1
echo ""
echo ""

# Rate limit test
echo "5. Rate Limit Test (sending 10 rapid requests):"
for i in {1..10}; do
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL$ENDPOINT?test=$i")
  echo "Request $i: HTTP $STATUS"
done
echo ""
echo ""

echo "=========================================="
echo "Test completed!"
echo ""
echo "To view logs:"
echo "  docker-compose logs -f api-cache"
echo ""
echo "To view cache contents:"
echo "  docker exec -it api-cache-valkey valkey-cli KEYS 'cache:*'"
