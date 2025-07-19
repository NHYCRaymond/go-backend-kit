#!/bin/bash

echo "检查 Prometheus 指标收集状态"
echo "=============================="

# 1. 检查指标端点
echo -e "\n1. 检查指标端点是否正常："
curl -s http://localhost:9090/metrics | grep -c "gin_requests_total"
if [ $? -eq 0 ]; then
    echo "✓ 找到 gin_requests_total 指标"
else
    echo "✗ 未找到 gin_requests_total 指标"
fi

# 2. 显示一些关键指标
echo -e "\n2. 当前的关键指标值："
echo "gin_requests_total:"
curl -s http://localhost:9090/metrics | grep "gin_requests_total{" | head -5

echo -e "\ngin_request_duration_seconds:"
curl -s http://localhost:9090/metrics | grep "gin_request_duration_seconds_bucket" | head -5

# 3. 检查 Prometheus 是否正在抓取
echo -e "\n3. 检查 Prometheus targets:"
echo "请访问: http://localhost:9090/targets"
echo "确认 'go-backend-kit' job 的状态是 UP"

# 4. 测试查询
echo -e "\n4. 在 Prometheus 中测试查询:"
echo "请访问: http://localhost:9090/graph"
echo "尝试查询: gin_requests_total"
echo "或: sum(rate(gin_requests_total[5m]))"

echo -e "\n5. Grafana 数据源配置:"
echo "在 Grafana 中检查:"
echo "- Configuration → Data Sources"
echo "- 确认 Prometheus 数据源配置正确"
echo "- URL 应该是: http://localhost:9090"

echo -e "\n6. Grafana 面板查询调试:"
echo "在 Grafana 面板编辑模式中:"
echo "- 检查 Query Inspector"
echo "- 查看实际发送的查询和返回的数据"