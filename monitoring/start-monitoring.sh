#!/bin/bash

echo "启动监控服务..."

# 启动 Prometheus（使用自定义配置）
echo "启动 Prometheus..."
prometheus --config.file=/Users/raymond/Projects/go-backend-kit/monitoring/prometheus-local.yml &
PROMETHEUS_PID=$!
echo "Prometheus PID: $PROMETHEUS_PID"

# 等待 Prometheus 启动
sleep 2

# 检查 Prometheus 是否运行
if curl -s http://localhost:9090/-/ready > /dev/null; then
    echo "✅ Prometheus 已启动: http://localhost:9090"
else
    echo "❌ Prometheus 启动失败"
fi

# 检查 Grafana 是否运行
if curl -s http://localhost:3000/api/health > /dev/null; then
    echo "✅ Grafana 已启动: http://localhost:3000"
    echo "   默认用户名: admin"
    echo "   默认密码: admin"
else
    echo "❌ Grafana 启动失败"
fi

echo ""
echo "监控服务已启动！"
echo "- Prometheus: http://localhost:9090"
echo "- Grafana: http://localhost:3000"
echo ""
echo "按 Ctrl+C 停止服务"

# 等待退出信号
trap "kill $PROMETHEUS_PID; exit" INT TERM
wait $PROMETHEUS_PID