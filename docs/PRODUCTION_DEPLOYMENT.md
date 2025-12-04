# APK Analysis Platform - 生产环境部署指南

> **目标**: 首次全量部署 Go 版本 APK 动态分析平台到生产环境
> **适用场景**: 全新部署,无历史流量,无现有系统
> **部署方式**: Docker Compose 全量部署

---

## 📋 目录

- [系统要求](#系统要求)
- [部署前准备](#部署前准备)
- [快速部署](#快速部署)
- [详细部署步骤](#详细部署步骤)
- [部署验证](#部署验证)
- [监控配置](#监控配置)
- [安全配置](#安全配置)
- [故障排查](#故障排查)
- [维护操作](#维护操作)

---

## 系统要求

### 硬件要求

| 组件 | 最低配置 | 推荐配置 | 说明 |
|------|---------|---------|------|
| CPU | 4 核心 | 8 核心 | 多任务并发需要更多核心 |
| 内存 | 8 GB | 16 GB | MySQL + Redis + 应用内存占用 |
| 磁盘 | 100 GB | 500 GB SSD | 存储 APK、截图、流量数据 |
| 网络 | 100 Mbps | 1 Gbps | 下载依赖、数据传输 |

### 软件要求

| 软件 | 版本要求 | 说明 |
|------|---------|------|
| 操作系统 | Ubuntu 20.04+ / CentOS 8+ | 推荐 Ubuntu 22.04 LTS |
| Docker | 20.10+ | 容器运行环境 |
| Docker Compose | 2.0+ | 服务编排工具 |
| Git | 2.0+ | 代码拉取 (可选) |

### 端口要求

确保以下端口未被占用:

```
8080  - API 服务
9090  - Prometheus Metrics
3306  - MySQL
5672  - RabbitMQ AMQP
15672 - RabbitMQ 管理界面
6379  - Redis
9091  - Prometheus 服务
3000  - Grafana
80    - Nginx HTTP (可选)
443   - Nginx HTTPS (可选)
```

---

## 部署前准备

### 1. 安装 Docker 和 Docker Compose

**Ubuntu/Debian:**

```bash
# 更新包索引
sudo apt-get update

# 安装依赖
sudo apt-get install -y \
    ca-certificates \
    curl \
    gnupg \
    lsb-release

# 添加 Docker 官方 GPG key
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg

# 添加 Docker 仓库
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# 安装 Docker Engine
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# 验证安装
docker --version
docker compose version
```

**CentOS/RHEL:**

```bash
# 安装 Docker
sudo yum install -y yum-utils
sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
sudo yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# 启动 Docker
sudo systemctl start docker
sudo systemctl enable docker

# 验证安装
docker --version
docker compose version
```

### 2. 添加用户到 Docker 组 (可选)

```bash
# 添加当前用户到 docker 组
sudo usermod -aG docker $USER

# 重新登录生效
newgrp docker

# 验证
docker ps
```

### 3. 获取项目代码

**方式 1: Git Clone**

```bash
git clone https://github.com/your-org/apk-analysis-go.git
cd apk-analysis-go
```

**方式 2: 直接下载**

```bash
# 下载并解压
wget https://github.com/your-org/apk-analysis-go/archive/main.zip
unzip main.zip
cd apk-analysis-go-main
```

---

## 快速部署

**最快 5 分钟部署:**

```bash
# 1. 复制环境变量模板
cp .env.example .env

# 2. 修改必要配置 (数据库密码等)
nano .env

# 3. 运行自动部署脚本
./deployments/production/deploy.sh

# 4. 验证部署
./deployments/production/verify.sh
```

---

## 详细部署步骤

### Step 1: 配置环境变量

```bash
# 复制环境变量模板
cp .env.example .env

# 编辑配置文件
nano .env
```

**必须修改的配置项:**

```bash
# MySQL 密码
MYSQL_ROOT_PASSWORD=your_secure_root_password_here
MYSQL_USER=apk_analysis_user
MYSQL_PASS=your_secure_mysql_password_here

# RabbitMQ 密码
RABBITMQ_USER=apk_analysis_user
RABBITMQ_PASS=your_secure_rabbitmq_password_here

# Grafana 管理员密码
GRAFANA_ADMIN_PASSWORD=your_secure_grafana_password_here
```

**可选配置 (根据需要修改):**

```bash
# AI 功能 (需要智谱 AI API Key)
AI_UI_ANALYSIS_ENABLED=true
ZAI_API_KEY=your_zhipu_ai_api_key_here

# Frida SSL Unpinning
FRIDA_ENABLED=true

# IP 归属地查询 (需要 IP138 Token)
IP138_TOKEN=your_ip138_token_here

# 域名备案查询
BEIAN_CHECK_ENABLED=true
```

### Step 2: 创建必要目录

```bash
# 创建数据目录
mkdir -p results logs inbound_apks configs backups

# 设置权限
chmod 755 results logs inbound_apks configs
chmod 700 backups

# 验证
ls -la
```

### Step 3: 构建 Docker 镜像

```bash
# 构建主应用镜像
docker build -t apk-analysis-go:latest .

# 验证镜像
docker images | grep apk-analysis-go
```

**预期输出:**

```
apk-analysis-go   latest   abc123def456   2 minutes ago   500MB
```

### Step 4: 启动服务

```bash
# 启动所有服务 (后台运行)
docker compose -f docker-compose.prod.yml up -d

# 查看启动日志
docker compose -f docker-compose.prod.yml logs -f
```

**预期看到的日志:**

```
apk-analysis-server  | [GIN-debug] Listening and serving HTTP on :8080
apk-analysis-mysql   | mysqld: ready for connections
apk-analysis-rabbitmq| Server startup complete
apk-analysis-redis   | Ready to accept connections
```

### Step 5: 等待服务启动

```bash
# 查看容器状态
docker compose -f docker-compose.prod.yml ps

# 等待健康检查通过 (约 30-60 秒)
watch -n 2 "docker compose -f docker-compose.prod.yml ps"
```

**健康状态标识:**

```
NAME                      STATUS
apk-analysis-server       Up 1 minute (healthy)
apk-analysis-mysql        Up 1 minute (healthy)
apk-analysis-rabbitmq     Up 1 minute (healthy)
apk-analysis-redis        Up 1 minute
```

### Step 6: 验证部署

```bash
# 运行自动验证脚本
./deployments/production/verify.sh
```

**预期输出:**

```
[✓] 容器 apk-analysis-server 运行正常
[✓] 容器 apk-analysis-mysql 运行正常
[✓] 容器 apk-analysis-rabbitmq 运行正常
[✓] 容器 apk-analysis-redis 运行正常
[✓] 端口 8080 (API 服务) 监听正常
[✓] API 健康检查通过
[✓] MySQL 数据库连接正常
[✓] RabbitMQ 连接正常
[✓] Redis 连接正常
========================================
  所有验证通过! 系统运行正常
========================================
```

---

## 部署验证

### 手动验证步骤

#### 1. API 服务验证

```bash
# 健康检查
curl http://localhost:8080/api/health

# 预期响应
{"status":"ok","timestamp":"2025-11-05T10:00:00Z"}

# 获取任务列表
curl http://localhost:8080/api/tasks

# 预期响应
[]  # 首次部署为空数组

# 系统统计
curl http://localhost:8080/api/stats

# 预期响应
{
  "total_tasks": 0,
  "completed_tasks": 0,
  "failed_tasks": 0,
  "running_tasks": 0
}
```

#### 2. Prometheus 验证

```bash
# 访问 Prometheus Web UI
open http://localhost:9091

# 查询指标
curl http://localhost:9091/api/v1/query?query=up

# 验证目标
curl http://localhost:9091/api/v1/targets
```

#### 3. Grafana 验证

```bash
# 访问 Grafana Web UI
open http://localhost:3000

# 默认登录:
# 用户名: admin
# 密码: .env 中配置的 GRAFANA_ADMIN_PASSWORD
```

#### 4. RabbitMQ 验证

```bash
# 访问 RabbitMQ 管理界面
open http://localhost:15672

# 默认登录:
# 用户名: .env 中配置的 RABBITMQ_USER
# 密码: .env 中配置的 RABBITMQ_PASS

# 命令行检查队列
docker exec apk-analysis-rabbitmq rabbitmqctl list_queues
```

#### 5. 数据库验证

```bash
# 连接 MySQL
docker exec -it apk-analysis-mysql mysql -uroot -p

# 输入密码后执行
USE apk_analysis;
SHOW TABLES;

# 预期输出 (7 张表)
+-------------------------+
| Tables_in_apk_analysis  |
+-------------------------+
| apk_tasks               |
| task_activities         |
| task_mobsf_reports      |
| task_domain_analysis    |
| task_app_domains        |
| task_ai_logs            |
| third_party_sdk_rules   |
+-------------------------+
```

### 功能测试

#### 测试 APK 上传和分析

```bash
# 1. 准备测试 APK
cp /path/to/test.apk inbound_apks/

# 2. 观察日志
docker logs -f apk-analysis-server

# 3. 查看任务状态
curl http://localhost:8080/api/tasks | jq

# 4. 等待任务完成
watch -n 5 "curl -s http://localhost:8080/api/tasks | jq '.[0].status'"

# 5. 查看结果
ls -lh results/$(curl -s http://localhost:8080/api/tasks | jq -r '.[0].id')/
```

---

## 监控配置

### Prometheus 配置

**数据源配置已自动完成**, 验证方法:

```bash
# 检查 Prometheus 配置
docker exec apk-analysis-prometheus cat /etc/prometheus/prometheus.yml

# 检查抓取目标
curl http://localhost:9091/api/v1/targets | jq
```

### Grafana 配置

#### 1. 添加 Prometheus 数据源

访问 `http://localhost:3000/datasources/new`

- **Type**: Prometheus
- **URL**: `http://prometheus:9090` (内部网络)
- **Access**: Server (default)
- **点击 "Save & Test"**

#### 2. 导入监控面板

访问 `http://localhost:3000/dashboard/import`

**推荐面板 ID:**

- **Node Exporter Full**: 1860
- **Docker Monitoring**: 893
- **MySQL Overview**: 7362
- **RabbitMQ Overview**: 4279
- **Redis Dashboard**: 11835

**或导入项目自定义面板:**

```bash
# 面板位置
./deployments/grafana/dashboards/apk-analysis-dashboard.json
```

### 告警配置 (可选)

#### 1. 配置 Alertmanager

```yaml
# deployments/prometheus/alertmanager.yml
global:
  smtp_smarthost: 'smtp.example.com:587'
  smtp_from: 'alerts@example.com'
  smtp_auth_username: 'alerts@example.com'
  smtp_auth_password: 'your_email_password'

route:
  receiver: 'email-alerts'
  group_by: ['alertname', 'severity']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 12h

receivers:
  - name: 'email-alerts'
    email_configs:
      - to: 'admin@example.com'
        send_resolved: true
```

#### 2. 重启 Prometheus

```bash
docker compose -f docker-compose.prod.yml restart prometheus
```

---

## 安全配置

### 1. 防火墙配置

**仅允许必要端口:**

```bash
# Ubuntu (UFW)
sudo ufw allow 80/tcp    # HTTP (可选)
sudo ufw allow 443/tcp   # HTTPS (可选)
sudo ufw allow 8080/tcp  # API (限制来源 IP)
sudo ufw enable

# CentOS (firewalld)
sudo firewall-cmd --permanent --add-port=80/tcp
sudo firewall-cmd --permanent --add-port=443/tcp
sudo firewall-cmd --reload
```

**限制管理端口访问:**

```bash
# 仅允许本地访问
sudo ufw allow from 127.0.0.1 to any port 3000  # Grafana
sudo ufw allow from 127.0.0.1 to any port 15672 # RabbitMQ
sudo ufw allow from 127.0.0.1 to any port 9091  # Prometheus
```

### 2. Nginx 反向代理 (推荐生产环境)

**配置 Nginx SSL:**

```bash
# 安装 Certbot (Let's Encrypt)
sudo apt-get install -y certbot python3-certbot-nginx

# 获取 SSL 证书
sudo certbot --nginx -d apk-analysis.example.com

# 自动续期
sudo certbot renew --dry-run
```

**Nginx 配置示例:**

```nginx
# /etc/nginx/sites-available/apk-analysis

server {
    listen 443 ssl http2;
    server_name apk-analysis.example.com;

    ssl_certificate /etc/letsencrypt/live/apk-analysis.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/apk-analysis.example.com/privkey.pem;

    # 安全头
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;

    # API 代理
    location /api/ {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 超时配置
        proxy_connect_timeout 60s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }

    # Grafana 代理
    location /grafana/ {
        proxy_pass http://localhost:3000/;
        proxy_set_header Host $host;
    }
}

# HTTP 重定向到 HTTPS
server {
    listen 80;
    server_name apk-analysis.example.com;
    return 301 https://$server_name$request_uri;
}
```

### 3. 定期更新密码

```bash
# 更新 MySQL root 密码
docker exec -it apk-analysis-mysql mysql -uroot -p
ALTER USER 'root'@'%' IDENTIFIED BY 'new_password';
FLUSH PRIVILEGES;

# 更新 .env 文件
nano .env
# 修改 MYSQL_ROOT_PASSWORD

# 重启服务
docker compose -f docker-compose.prod.yml restart
```

---

## 故障排查

### 常见问题

#### 1. 容器启动失败

**症状:**

```bash
docker compose ps
# 显示 Exit 1 或 Restarting
```

**解决方法:**

```bash
# 查看日志
docker logs apk-analysis-server

# 常见错误:
# - 端口被占用: 修改端口或停止占用进程
# - 环境变量错误: 检查 .env 文件
# - 数据库连接失败: 检查 MySQL 容器状态
```

#### 2. API 健康检查失败

**症状:**

```bash
curl http://localhost:8080/api/health
# curl: (7) Failed to connect
```

**解决方法:**

```bash
# 检查容器状态
docker ps | grep apk-analysis-server

# 查看应用日志
docker logs apk-analysis-server | tail -50

# 检查端口占用
netstat -tuln | grep 8080

# 重启服务
docker compose -f docker-compose.prod.yml restart apk-analysis-server
```

#### 3. 数据库连接失败

**症状:**

```
Error 1045 (28000): Access denied for user 'apk_analysis_user'@'%'
```

**解决方法:**

```bash
# 检查 MySQL 日志
docker logs apk-analysis-mysql

# 重置用户权限
docker exec -it apk-analysis-mysql mysql -uroot -p${MYSQL_ROOT_PASSWORD}

# 执行 SQL
CREATE USER 'apk_analysis_user'@'%' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON apk_analysis.* TO 'apk_analysis_user'@'%';
FLUSH PRIVILEGES;
```

#### 4. RabbitMQ 连接失败

**症状:**

```
[error] failed to connect to RabbitMQ: dial tcp: connection refused
```

**解决方法:**

```bash
# 检查 RabbitMQ 状态
docker exec apk-analysis-rabbitmq rabbitmqctl status

# 检查用户权限
docker exec apk-analysis-rabbitmq rabbitmqctl list_users

# 添加用户 (如果不存在)
docker exec apk-analysis-rabbitmq rabbitmqctl add_user apk_analysis_user password
docker exec apk-analysis-rabbitmq rabbitmqctl set_permissions -p / apk_analysis_user ".*" ".*" ".*"
```

#### 5. 磁盘空间不足

**症状:**

```
no space left on device
```

**解决方法:**

```bash
# 检查磁盘使用
df -h

# 清理 Docker 数据
docker system prune -a --volumes

# 清理旧任务结果
find results/ -type d -mtime +30 -exec rm -rf {} +

# 清理旧日志
find logs/ -name "*.log" -mtime +7 -delete
```

### 日志查看

```bash
# 查看所有服务日志
docker compose -f docker-compose.prod.yml logs

# 查看特定服务日志
docker compose -f docker-compose.prod.yml logs apk-analysis-server

# 实时跟踪日志
docker compose -f docker-compose.prod.yml logs -f --tail=100

# 查看应用内部日志
tail -f logs/app.log

# 查看错误日志
grep ERROR logs/app.log
```

---

## 维护操作

### 日常维护

#### 1. 数据库备份

**自动备份 (推荐):**

```bash
# 配置 cron 定时任务
crontab -e

# 每天凌晨 2 点备份
0 2 * * * /home/user/apk-analysis-go/deployments/production/backup.sh
```

**手动备份:**

```bash
# 备份 MySQL 数据库
docker exec apk-analysis-mysql sh -c \
  "mysqldump -u${MYSQL_USER} -p${MYSQL_PASS} ${MYSQL_DB} | gzip" \
  > backups/mysql_backup_$(date +%Y%m%d_%H%M%S).sql.gz

# 验证备份
ls -lh backups/

# 恢复备份
gunzip < backups/mysql_backup_20251105_020000.sql.gz | \
  docker exec -i apk-analysis-mysql mysql -u${MYSQL_USER} -p${MYSQL_PASS} ${MYSQL_DB}
```

#### 2. 日志轮转

**配置 logrotate:**

```bash
# 创建配置文件
sudo nano /etc/logrotate.d/apk-analysis

# 内容
/home/user/apk-analysis-go/logs/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0644 user user
}
```

#### 3. 清理旧数据

```bash
# 清理 30 天前的任务结果
find results/ -type d -mtime +30 -exec rm -rf {} +

# 清理 7 天前的日志
find logs/ -name "*.log" -mtime +7 -delete

# 清理 Docker 未使用资源
docker system prune -f
```

### 升级部署

#### 1. 拉取新代码

```bash
cd /home/user/apk-analysis-go

# 备份当前版本
git stash

# 拉取最新代码
git pull origin main

# 恢复本地修改 (如果需要)
git stash pop
```

#### 2. 构建新镜像

```bash
# 构建新镜像
docker build -t apk-analysis-go:v1.1.0 .

# 标记为 latest
docker tag apk-analysis-go:v1.1.0 apk-analysis-go:latest
```

#### 3. 滚动更新

```bash
# 停止旧服务
docker compose -f docker-compose.prod.yml down apk-analysis-server

# 启动新服务
docker compose -f docker-compose.prod.yml up -d apk-analysis-server

# 验证
./deployments/production/verify.sh
```

### 性能监控

#### 1. 资源使用监控

```bash
# 查看容器资源使用
docker stats

# 查看磁盘使用
df -h

# 查看内存使用
free -h

# 查看 CPU 使用
top
```

#### 2. 应用性能监控

```bash
# Prometheus 查询 API
curl http://localhost:9091/api/v1/query?query=rate(http_requests_total[5m])

# 查看任务统计
curl http://localhost:8080/api/stats | jq

# 查看数据库连接数
docker exec apk-analysis-mysql mysql -uroot -p${MYSQL_ROOT_PASSWORD} -e "SHOW STATUS LIKE 'Threads_connected';"
```

---

## 附录

### 常用命令速查

```bash
# 启动服务
docker compose -f docker-compose.prod.yml up -d

# 停止服务
docker compose -f docker-compose.prod.yml down

# 重启服务
docker compose -f docker-compose.prod.yml restart

# 查看日志
docker compose -f docker-compose.prod.yml logs -f

# 查看容器状态
docker compose -f docker-compose.prod.yml ps

# 进入容器
docker exec -it apk-analysis-server sh

# 查看资源使用
docker stats

# 清理资源
docker system prune -a
```

### 服务访问地址

```
API 服务:        http://localhost:8080
API 文档:        http://localhost:8080/swagger/index.html
Prometheus:      http://localhost:9091
Grafana:         http://localhost:3000
RabbitMQ 管理:   http://localhost:15672
```

### 文件路径

```
配置文件:        .env
Docker Compose:  docker-compose.prod.yml
部署脚本:        deployments/production/deploy.sh
验证脚本:        deployments/production/verify.sh
日志目录:        logs/
结果目录:        results/
备份目录:        backups/
```

---

## 支持和反馈

如遇到问题,请:

1. 查看 [故障排查](#故障排查) 章节
2. 查看应用日志: `docker logs apk-analysis-server`
3. 提交 Issue: https://github.com/your-org/apk-analysis-go/issues

---

**部署愉快! 🚀**
