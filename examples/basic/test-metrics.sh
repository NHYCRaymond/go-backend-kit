#!/bin/bash

echo "测试 Go Backend Kit 指标端点"
echo "============================"

# 检查应用是否运行
echo -n "检查应用状态... "
if curl -s http://localhost:8080/health > /dev/null; then
    echo "✓ 运行中"
else
    echo "✗ 未运行"
    echo "请先运行: go run main.go"
    exit 1
fi

# 检查指标端点
echo -n "检查指标端点... "
if curl -s http://localhost:9090/metrics > /dev/null; then
    echo "✓ 可访问"
else
    echo "✗ 无法访问"
    exit 1
fi

echo ""
echo "主要指标:"
echo "----------"

# 检查关键指标
echo "1. HTTP 请求指标:"
curl -s http://localhost:9090/metrics | grep -E "^gin_requests_total" | head -5

echo ""
echo "2. 响应时间指标:"
curl -s http://localhost:9090/metrics | grep -E "^gin_request_duration_seconds" | head -5

echo ""
echo "3. 活跃请求:"
curl -s http://localhost:9090/metrics | grep -E "^gin_active_requests"

echo ""
echo "4. 数据库连接:"
curl -s http://localhost:9090/metrics | grep -E "^db_connections"

echo ""
echo "5. 应用信息:"
curl -s http://localhost:9090/metrics | grep -E "^app_info"

echo ""
echo "----------"
echo "指标总数:"
curl -s http://localhost:9090/metrics | grep -v "^#" | wc -l

echo ""
echo "提示: 使用 'curl http://localhost:9090/metrics' 查看所有指标"