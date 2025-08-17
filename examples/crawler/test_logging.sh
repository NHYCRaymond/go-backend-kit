#!/bin/bash

# Test script for the modular logging system
# This script starts a crawler node with enhanced logging

echo "🚀 Starting test of modular logging system..."
echo ""
echo "This will:"
echo "1. Start a Redis server (if not running)"
echo "2. Start a crawler node with log forwarding"
echo "3. Start the TUI to view logs in real-time"
echo ""

# Check if Redis is running
if ! redis-cli ping > /dev/null 2>&1; then
    echo "❌ Redis is not running. Please start Redis first:"
    echo "   brew services start redis"
    exit 1
fi

echo "✅ Redis is running"

# Clean up old logs
echo "🧹 Cleaning up old logs..."
redis-cli DEL "crawler:logs:stream" > /dev/null
redis-cli DEL "crawler:logs:pubsub" > /dev/null

# Start node in background
echo "🚀 Starting crawler node..."
cd examples/crawler/distributed/node
go run main.go -redis localhost:6379 &
NODE_PID=$!
echo "   Node PID: $NODE_PID"

# Give node time to start
sleep 2

# Start TUI
echo "📺 Starting TUI (press F4 to view logs)..."
cd ../../../..
go run examples/crawler/tui/main.go

# Cleanup
echo "🛑 Stopping node..."
kill $NODE_PID 2>/dev/null

echo "✅ Test complete!"