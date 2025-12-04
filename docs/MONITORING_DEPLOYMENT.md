# 监控系统部署指南

## 📋 概述

本文档介绍如何部署 APK Analysis Platform 的完整监控栈,包括 Prometheus、Grafana 和 AlertManager。

---

## 🚀 快速开始

### 1. 前置要求

- Docker 20.10+
- Docker Compose 1.29+
- 至少 2GB 可用内存
- 端口 9090 (Prometheus)、3001 (Grafana)、9093 (AlertManager) 未被占用

### 2. 一键部署

```bash
# 1. 进入项目目录
cd /home/icyyaww/project/动态apk解析/apk-analysis-go

# 2. 创建 Docker 网络 (如果不存在)
docker network create apk-analysis-network || true

# 3. 启动监控栈
docker-compose -f docker-compose.monitoring.yml up -d

# 4. 验证服务状态
docker-compose -f docker-compose.monitoring.yml ps

# 预期输出:
# NAME                               STATUS    PORTS
# apk-analysis-prometheus            Up        0.0.0.0:9090->9090/tcp
# apk-analysis-grafana               Up        0.0.0.0:3001->3000/tcp
# apk-analysis-alertmanager          Up        0.0.0.0:9093->9093/tcp
# apk-analysis-node-exporter         Up        0.0.0.0:9100->9100/tcp
```

### 3. 访问监控界面

| 服务 | 访问地址 | 默认凭证 |
|------|----------|----------|
| Grafana | http://localhost:3001 | admin / admin123 |
| Prometheus | http://localhost:9090 | 无需登录 |
| AlertManager | http://localhost:9093 | 无需登录 |

---

## 📊 Grafana 配置

### 首次登录

1. 访问 http://localhost:3001
2. 使用凭证登录: `admin` / `admin123`
3. 首次登录会要求修改密码 (可选跳过)

### 查看 Dashboard

1. 点击左侧菜单 **Dashboards**
2. 选择 **APK Analysis Platform - Overview**
3. 调整时间范围 (右上角): 默认显示最近 6 小时

### Dashboard 功能说明

#### 核心面板

**1. 内存使用 (Memory Usage)**
- 实时监控内存占用 (MB)
- 内置告警: 超过 1.5GB 触发警告

**2. Goroutine 数量**
- 监控 Goroutine 泄漏
- 正常范围: 25-80 (取决于任务数)

**3. 任务统计 (Tasks)**
- 完成速率 (绿色)
- 失败速率 (红色)
- 进行中数量 (蓝色)

**4. API 延迟 (P50/P95/P99)**
- P50: 中位数延迟
- P95: 95% 请求延迟
- P99: 99% 请求延迟

**5. 数据库连接池**
- 总连接数 (黄色)
- 使用中 (红色)
- 空闲 (绿色)

**6. Worker Pool 状态**
- 总 Workers (蓝色)
- 活跃 Workers (绿色)
- 队列积压 (橙色)

#### 统计面板

**9. 任务进行中**
- 当前正在执行的任务数
- 实时更新

**10. 总任务数 (24h)**
- 过去 24 小时累计任务

**11. 成功率 (1h)**
- 最近 1 小时任务成功率
- 颜色阈值:
  - 绿色: ≥ 95%
  - 黄色: 80-95%
  - 红色: < 80%

**12. 平均任务耗时 (1h)**
- 最近 1 小时平均耗时

---

## 🔔 Prometheus 配置

### 查看 Targets

1. 访问 http://localhost:9090/targets
2. 验证 `apk-analysis` target 状态为 **UP**

**常见问题**:
- 状态为 **DOWN**: 检查 Orchestrator 服务是否启动
- 错误 `context deadline exceeded`: 检查网络连接

### 查询指标

1. 访问 http://localhost:9090/graph
2. 输入 PromQL 查询

**示例查询**:

```promql
# 当前内存使用 (MB)
apk_analysis_memory_usage_bytes / (1024 * 1024)

# 最近 5 分钟任务完成速率
rate(apk_analysis_tasks_total{status="completed"}[5m]) * 60

# API P95 延迟
histogram_quantile(0.95,
  rate(apk_analysis_http_request_duration_seconds_bucket[5m])
)

# 数据库连接池使用率
apk_analysis_db_connections_in_use / apk_analysis_db_connections_open
```

### 查看告警规则

1. 访问 http://localhost:9090/alerts
2. 查看所有告警规则及状态

**告警状态**:
- **Inactive**: 条件未触发
- **Pending**: 等待 `for` 时间
- **Firing**: 已触发,发送到 AlertManager

---

## 📧 AlertManager 配置

### 查看告警

1. 访问 http://localhost:9093/#/alerts
2. 查看当前活跃告警

### 告警分组

告警按以下标签分组:
- `alertname`: 告警名称
- `component`: 组件 (system/task/database/api)
- `severity`: 严重程度 (warning/critical)

### 静默告警 (Silence)

**场景**: 维护期间临时静默告警

**操作步骤**:
1. 访问 http://localhost:9093/#/silences
2. 点击 **New Silence**
3. 配置过滤条件:
   ```
   Matchers:
     alertname =~ ".*"
     severity = "warning"

   Duration: 2h
   Comment: Maintenance window
   ```
4. 点击 **Create**

### 邮件告警配置

**编辑配置文件**: `configs/alertmanager/alertmanager.yml`

```yaml
global:
  smtp_smarthost: 'smtp.gmail.com:587'
  smtp_from: 'your-email@gmail.com'
  smtp_auth_username: 'your-email@gmail.com'
  smtp_auth_password: 'your-app-password'
  smtp_require_tls: true

receivers:
  - name: 'critical-alerts'
    email_configs:
      - to: 'ops-team@example.com'
        headers:
          Subject: '[CRITICAL] APK Analysis Alert'
```

**重启 AlertManager**:
```bash
docker-compose -f docker-compose.monitoring.yml restart alertmanager
```

---

## 🔧 维护操作

### 查看日志

```bash
# 查看所有服务日志
docker-compose -f docker-compose.monitoring.yml logs -f

# 查看特定服务日志
docker-compose -f docker-compose.monitoring.yml logs -f prometheus
docker-compose -f docker-compose.monitoring.yml logs -f grafana
docker-compose -f docker-compose.monitoring.yml logs -f alertmanager
```

### 重启服务

```bash
# 重启所有监控服务
docker-compose -f docker-compose.monitoring.yml restart

# 重启特定服务
docker-compose -f docker-compose.monitoring.yml restart prometheus
```

### 停止服务

```bash
# 停止所有监控服务
docker-compose -f docker-compose.monitoring.yml stop

# 停止并删除容器 (保留数据)
docker-compose -f docker-compose.monitoring.yml down

# 停止并删除容器 + 数据卷
docker-compose -f docker-compose.monitoring.yml down -v
```

### 备份数据

```bash
# 备份 Prometheus 数据
docker run --rm \
  -v apk-analysis-go_prometheus-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/prometheus-$(date +%Y%m%d).tar.gz /data

# 备份 Grafana 数据
docker run --rm \
  -v apk-analysis-go_grafana-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/grafana-$(date +%Y%m%d).tar.gz /data
```

---

## 📈 监控最佳实践

### 1. 设置合理的告警阈值

**内存告警**:
- Warning: 1.5GB (留有缓冲)
- Critical: 2GB (目标上限)

**API 延迟**:
- Warning: 500ms (P95)
- Critical: 2s (P95)

**任务失败率**:
- Warning: 20% (5 分钟)
- Critical: 50% (5 分钟)

### 2. 定期检查 Dashboard

**日常检查** (每天):
- 内存使用趋势
- 任务成功率
- API 延迟 P95

**周期检查** (每周):
- Goroutine 数量趋势
- 数据库连接池使用率
- GC 频率

### 3. 告警疲劳预防

**避免误报**:
- 合理设置 `for` 时间 (避免瞬时抖动)
- 使用抑制规则 (严重告警抑制警告)
- 设置静默时间段 (维护窗口)

**告警降噪**:
- 按组件分组 (`component`)
- 按严重程度分组 (`severity`)
- 不同接收者处理不同级别告警

### 4. 数据保留策略

**Prometheus**:
- 默认保留 30 天
- 可通过 `--storage.tsdb.retention.time` 调整

**Grafana**:
- Dashboard 自动保存
- 定期导出 JSON 备份

---

## 🐛 故障排查

### Prometheus 无法抓取指标

**症状**: Target 状态为 DOWN

**排查步骤**:
```bash
# 1. 检查 Orchestrator 服务
curl http://localhost:8080/metrics/prometheus

# 2. 检查网络连接
docker network inspect apk-analysis-network

# 3. 查看 Prometheus 日志
docker-compose -f docker-compose.monitoring.yml logs prometheus | grep error
```

**常见原因**:
- Orchestrator 未启动
- 网络配置错误
- 端口冲突

### Grafana Dashboard 无数据

**症状**: 所有面板显示 "No data"

**排查步骤**:
1. 检查 Prometheus 数据源
   - Settings > Data Sources > Prometheus
   - 点击 **Save & Test**,应显示 "Data source is working"

2. 检查时间范围
   - 确保时间范围包含有数据的时间段
   - 尝试选择 "Last 6 hours"

3. 检查指标是否存在
   - 访问 Prometheus: http://localhost:9090/graph
   - 查询 `apk_analysis_memory_usage_bytes`
   - 如果无数据 → Orchestrator 未正确导出指标

### AlertManager 未发送邮件

**症状**: 告警触发但未收到邮件

**排查步骤**:
```bash
# 1. 检查 AlertManager 日志
docker-compose -f docker-compose.monitoring.yml logs alertmanager | grep error

# 2. 测试 SMTP 连接
docker exec apk-analysis-alertmanager \
  amtool check-config /etc/alertmanager/alertmanager.yml

# 3. 查看告警状态
curl http://localhost:9093/api/v1/alerts | jq .
```

**常见问题**:
- SMTP 凭证错误
- 邮箱未开启 SMTP 服务
- 网络防火墙阻止

---

## 📚 进阶配置

### 自定义 Grafana Dashboard

1. 复制现有 Dashboard
2. 添加新面板
3. 导出 JSON
4. 保存到 `configs/grafana/dashboards/`

### 添加新告警规则

1. 编辑 `configs/prometheus/alerts.yml`
2. 添加新规则:
   ```yaml
   - alert: CustomAlert
     expr: your_metric > threshold
     for: 5m
     labels:
       severity: warning
       component: custom
     annotations:
       summary: "Custom alert triggered"
       description: "Value: {{ $value }}"
   ```
3. 重载配置:
   ```bash
   curl -X POST http://localhost:9090/-/reload
   ```

### 集成第三方服务

**Webhook 告警**:
```yaml
receivers:
  - name: 'webhook-receiver'
    webhook_configs:
      - url: 'http://your-webhook:8080/alerts'
        send_resolved: true
```

**钉钉/企业微信告警**:
使用 prometheus-webhook-dingtalk 或类似工具

---

## ✅ 检查清单

部署完成后,验证以下项目:

- [ ] Prometheus 可访问 (http://localhost:9090)
- [ ] Grafana 可访问 (http://localhost:3001)
- [ ] AlertManager 可访问 (http://localhost:9093)
- [ ] Prometheus Target 状态为 UP
- [ ] Grafana Dashboard 显示数据
- [ ] 告警规则已加载 (15 条)
- [ ] 邮件告警配置正确 (可选)
- [ ] 数据持久化正常 (重启后数据保留)

---

## 🔗 相关文档

- [PHASE_4.2_SUMMARY.md](../PHASE_4.2_SUMMARY.md) - 完整实施文档
- [Prometheus 文档](https://prometheus.io/docs/)
- [Grafana 文档](https://grafana.com/docs/)
- [AlertManager 文档](https://prometheus.io/docs/alerting/latest/alertmanager/)
