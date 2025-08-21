#!/bin/bash

# 场地监控启动脚本
# 监控19:00-22:00时段的场地变更

echo "🎾 Starting Venue Monitor System"
echo "================================"
echo "Monitored Sources:"
echo "  1. drip_ground_board - Drip Ground Board"  
echo "  2. venue_massage_site - Venue Massage Site"
echo "  3. pospal_venue_5662377 - Pospal Store 5662377"
echo "  4. pospal_venue_classroom - Pospal Classroom"
echo ""
echo "Monitored Time Slots: 19:00-22:00"
echo "Notification: DingTalk Webhook"
echo "================================"

# 设置钉钉 webhook 环境变量
export DINGTALK_WEBHOOK_URL="https://oapi.dingtalk.com/robot/send?access_token=348d61c66dc7ba986ebd59696a2850f1697ed1b397995737a2213c6cc025217f"

# 检查服务依赖
echo "Checking dependencies..."

# 检查 Redis
redis-cli ping > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "❌ Redis is not running. Please start Redis first."
    echo "   Run: redis-server"
    exit 1
fi
echo "✅ Redis is running"

# 检查 MongoDB
mongosh --eval "db.version()" > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "❌ MongoDB is not running. Please start MongoDB first."
    echo "   Run: mongod"
    exit 1
fi
echo "✅ MongoDB is running"

# 启动 Hub
echo ""
echo "Starting Crawler Hub with venue monitoring..."
go run distributed/hub/main.go config config/hub.yaml