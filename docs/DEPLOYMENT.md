# APK Analysis Platform - 部署文档

> **版本**: 1.0.0
> **最后更新**: 2025-11-05
> **适用环境**: Linux (Ubuntu 20.04+), Docker

---

## 📋 目录

- [系统要求](#系统要求)
- [快速开始](#快速开始)
- [详细部署步骤](#详细部署步骤)
- [配置说明](#配置说明)
- [Docker 部署](#docker-部署)
- [生产环境部署](#生产环境部署)
- [监控与运维](#监控与运维)
- [常见问题](#常见问题)
- [升级指南](#升级指南)

---

## 系统要求

### 硬件要求

| 组件 | 最低配置 | 推荐配置 |
|------|---------|---------|
| **CPU** | 4 核心 | 8 核心 |
| **内存** | 8 GB | 16 GB |
| **磁盘** | 50 GB (SSD) | 100 GB (SSD) |
| **网络** | 100 Mbps | 1 Gbps |

### 软件要求

| 软件 | 版本要求 | 说明 |
|------|---------|------|
| **操作系统** | Ubuntu 20.04+ / CentOS 8+ | 推荐 Ubuntu 22.04 LTS |
| **Go** | 1.21+ | 编译时需要 |
| **Docker** | 20.10+ | 容器化部署 |
| **Docker Compose** | 2.0+ | 服务编排 |
| **MySQL** | 8.0+ | 数据库（可选 SQLite） |
| **RabbitMQ** | 3.11+ | 消息队列 |
| **Redis** | 7.0+ | 缓存服务 |

---

## 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/your-org/apk-analysis-go.git
cd apk-analysis-go
```

### 2. 配置环境变量

```bash
cp .env.example .env
vim .env
```

**关键配置**:
```env
# 数据库
DB_TYPE=mysql
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASS=your_password
MYSQL_DB=apk_analysis

# RabbitMQ
RABBITMQ_HOST=localhost
RABBITMQ_PORT=5672
RABBITMQ_USER=user
RABBITMQ_PASS=password

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
```

### 3. 构建并运行

```bash
# 使用 Docker Compose (推荐)
make deploy

# 或手动构建
make build
./bin/server --config ./configs/config.yaml
```

### 4. 验证部署

```bash
# 检查服务健康状态
curl http://localhost:8080/api/health

# 查看任务列表
curl http://localhost:8080/api/tasks
```

---

## 详细部署步骤

### Step 1: 准备环境

#### 1.1 安装 Docker

```bash
# Ubuntu
sudo apt-get update
sudo apt-get install -y docker.io docker-compose
sudo systemctl start docker
sudo systemctl enable docker

# 添加当前用户到 docker 组
sudo usermod -aG docker $USER
newgrp docker
```

#### 1.2 安装 Go (编译时)

```bash
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

#### 1.3 安装依赖工具

```bash
# golangci-lint (代码检查)
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.55.2

# swag (Swagger 文档生成，可选)
go install github.com/swaggo/swag/cmd/swag@latest
```

---

### Step 2: 数据库配置

#### 2.1 MySQL 安装与配置

```bash
# 安装 MySQL
sudo apt-get install -y mysql-server

# 启动 MySQL
sudo systemctl start mysql
sudo systemctl enable mysql

# 配置 root 密码
sudo mysql_secure_installation
```

#### 2.2 创建数据库

```sql
-- 登录 MySQL
mysql -u root -p

-- 创建数据库
CREATE DATABASE apk_analysis CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 创建用户
CREATE USER 'apk_user'@'localhost' IDENTIFIED BY 'strong_password';
GRANT ALL PRIVILEGES ON apk_analysis.* TO 'apk_user'@'localhost';
FLUSH PRIVILEGES;

-- 退出
EXIT;
```

#### 2.3 导入表结构

```bash
# 使用 GORM 自动迁移 (推荐)
# 首次启动时会自动创建表

# 或手动执行 SQL (如果需要)
mysql -u apk_user -p apk_analysis < sql/schema.sql
```

---

### Step 3: 消息队列配置

#### 3.1 RabbitMQ 安装

```bash
# 使用 Docker 安装 (推荐)
docker run -d --name rabbitmq \
  -p 5672:5672 \
  -p 15672:15672 \
  -e RABBITMQ_DEFAULT_USER=user \
  -e RABBITMQ_DEFAULT_PASS=password \
  rabbitmq:3.11-management

# 或使用 apt 安装
sudo apt-get install -y rabbitmq-server
sudo systemctl start rabbitmq-server
sudo systemctl enable rabbitmq-server

# 启用管理插件
sudo rabbitmq-plugins enable rabbitmq_management
```

#### 3.2 配置队列

```bash
# 访问管理界面
# http://localhost:15672 (user/password)

# 或使用 rabbitmqadmin 命令行
rabbitmqadmin declare queue name=apk_tasks durable=true
```

---

### Step 4: 缓存服务配置

#### 4.1 Redis 安装

```bash
# 使用 Docker (推荐)
docker run -d --name redis \
  -p 6379:6379 \
  redis:7.2-alpine

# 或使用 apt 安装
sudo apt-get install -y redis-server
sudo systemctl start redis
sudo systemctl enable redis
```

#### 4.2 Redis 配置优化

```bash
# 编辑配置文件
sudo vim /etc/redis/redis.conf

# 关键配置
maxmemory 2gb
maxmemory-policy allkeys-lru
appendonly yes
appendfsync everysec

# 重启 Redis
sudo systemctl restart redis
```

---

### Step 5: 应用部署

#### 5.1 下载依赖

```bash
cd apk-analysis-go
go mod download
go mod tidy
```

#### 5.2 编译二进制

```bash
make build

# 输出: bin/server
```

#### 5.3 配置文件

创建 `configs/config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "production"  # debug/release/production

database:
  type: "mysql"  # mysql/sqlite
  mysql:
    host: "localhost"
    port: 3306
    user: "apk_user"
    password: "strong_password"
    database: "apk_analysis"
    max_idle_conns: 10
    max_open_conns: 100
    conn_max_lifetime: 3600  # seconds
  sqlite:
    path: "./data/tasks.db"

queue:
  rabbitmq:
    host: "localhost"
    port: 5672
    user: "user"
    password: "password"
    vhost: "/"
    queue_name: "apk_tasks"

cache:
  redis:
    host: "localhost"
    port: 6379
    password: ""
    db: 0
    pool_size: 10

logging:
  level: "info"  # debug/info/warn/error
  output: "stdout"  # stdout/file
  file_path: "./logs/app.log"

monitoring:
  prometheus:
    enabled: true
    port: 9090
  pprof:
    enabled: true
    port: 6060
```

#### 5.4 启动服务

```bash
# 前台运行
./bin/server --config ./configs/config.yaml

# 后台运行
nohup ./bin/server --config ./configs/config.yaml > logs/server.log 2>&1 &

# 查看日志
tail -f logs/server.log
```

---

### Step 6: Systemd 服务配置 (推荐)

#### 6.1 创建 Systemd 服务文件

```bash
sudo vim /etc/systemd/system/apk-analysis.service
```

**内容**:
```ini
[Unit]
Description=APK Analysis Platform Server
After=network.target mysql.service rabbitmq-server.service redis.service
Wants=mysql.service rabbitmq-server.service redis.service

[Service]
Type=simple
User=apk
Group=apk
WorkingDirectory=/opt/apk-analysis-go
ExecStart=/opt/apk-analysis-go/bin/server --config /opt/apk-analysis-go/configs/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=apk-analysis

# 资源限制
LimitNOFILE=65536
LimitNPROC=4096

# 环境变量
Environment="GO_ENV=production"

[Install]
WantedBy=multi-user.target
```

#### 6.2 启动服务

```bash
# 重新加载 systemd
sudo systemctl daemon-reload

# 启动服务
sudo systemctl start apk-analysis

# 设置开机自启
sudo systemctl enable apk-analysis

# 查看状态
sudo systemctl status apk-analysis

# 查看日志
sudo journalctl -u apk-analysis -f
```

---

## Docker 部署

### Docker Compose 配置

创建 `docker-compose.yml`:

```yaml
version: '3.8'

services:
  # 主应用
  apk-analysis:
    build: .
    container_name: apk-analysis-server
    ports:
      - "8080:8080"
      - "9090:9090"  # Prometheus metrics
    environment:
      - DB_TYPE=mysql
      - MYSQL_HOST=mysql
      - MYSQL_PORT=3306
      - MYSQL_USER=apk_user
      - MYSQL_PASS=strong_password
      - MYSQL_DB=apk_analysis
      - RABBITMQ_HOST=rabbitmq
      - REDIS_HOST=redis
    depends_on:
      - mysql
      - rabbitmq
      - redis
    restart: unless-stopped
    volumes:
      - ./configs:/app/configs
      - ./logs:/app/logs
      - ./results:/app/results

  # MySQL
  mysql:
    image: mysql:8.0
    container_name: apk-analysis-mysql
    environment:
      MYSQL_ROOT_PASSWORD: root_password
      MYSQL_DATABASE: apk_analysis
      MYSQL_USER: apk_user
      MYSQL_PASSWORD: strong_password
    ports:
      - "3306:3306"
    volumes:
      - mysql-data:/var/lib/mysql
    restart: unless-stopped

  # RabbitMQ
  rabbitmq:
    image: rabbitmq:3.11-management
    container_name: apk-analysis-rabbitmq
    environment:
      RABBITMQ_DEFAULT_USER: user
      RABBITMQ_DEFAULT_PASS: password
    ports:
      - "5672:5672"
      - "15672:15672"
    volumes:
      - rabbitmq-data:/var/lib/rabbitmq
    restart: unless-stopped

  # Redis
  redis:
    image: redis:7.2-alpine
    container_name: apk-analysis-redis
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    restart: unless-stopped

  # Prometheus
  prometheus:
    image: prom/prometheus:latest
    container_name: apk-analysis-prometheus
    ports:
      - "9091:9090"
    volumes:
      - ./deployments/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
    restart: unless-stopped

  # Grafana
  grafana:
    image: grafana/grafana:latest
    container_name: apk-analysis-grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-data:/var/lib/grafana
      - ./deployments/grafana/dashboards:/etc/grafana/provisioning/dashboards
    restart: unless-stopped

volumes:
  mysql-data:
  rabbitmq-data:
  redis-data:
  prometheus-data:
  grafana-data:
```

### 启动 Docker 环境

```bash
# 构建并启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f apk-analysis

# 停止服务
docker-compose down

# 停止并删除数据卷
docker-compose down -v
```

---

## 生产环境部署

### 1. 反向代理 (Nginx)

#### 1.1 安装 Nginx

```bash
sudo apt-get install -y nginx
```

#### 1.2 配置站点

```bash
sudo vim /etc/nginx/sites-available/apk-analysis
```

**配置内容**:
```nginx
upstream apk_analysis_backend {
    server 127.0.0.1:8080;
    # 如果有多个实例，添加负载均衡
    # server 127.0.0.1:8081;
    # server 127.0.0.1:8082;
}

server {
    listen 80;
    server_name apk-analysis.example.com;

    # 重定向到 HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name apk-analysis.example.com;

    # SSL 证书
    ssl_certificate /etc/letsencrypt/live/apk-analysis.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/apk-analysis.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # 日志
    access_log /var/log/nginx/apk-analysis-access.log;
    error_log /var/log/nginx/apk-analysis-error.log;

    # 客户端上传限制
    client_max_body_size 100M;

    # API 代理
    location /api/ {
        proxy_pass http://apk_analysis_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }

    # 健康检查
    location /api/health {
        proxy_pass http://apk_analysis_backend;
        access_log off;
    }

    # Prometheus metrics (限制访问)
    location /metrics {
        proxy_pass http://127.0.0.1:9090;
        allow 10.0.0.0/8;
        deny all;
    }
}
```

#### 1.3 启用站点

```bash
sudo ln -s /etc/nginx/sites-available/apk-analysis /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

---

### 2. SSL 证书 (Let's Encrypt)

```bash
# 安装 certbot
sudo apt-get install -y certbot python3-certbot-nginx

# 获取证书
sudo certbot --nginx -d apk-analysis.example.com

# 自动续期
sudo crontab -e
# 添加: 0 3 * * * certbot renew --quiet
```

---

### 3. 防火墙配置

```bash
# 使用 ufw
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 80/tcp    # HTTP
sudo ufw allow 443/tcp   # HTTPS
sudo ufw enable

# 或使用 iptables
sudo iptables -A INPUT -p tcp --dport 80 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 443 -j ACCEPT
sudo iptables-save > /etc/iptables/rules.v4
```

---

### 4. 日志管理 (Logrotate)

```bash
sudo vim /etc/logrotate.d/apk-analysis
```

**配置**:
```
/opt/apk-analysis-go/logs/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0640 apk apk
    sharedscripts
    postrotate
        systemctl reload apk-analysis > /dev/null 2>&1 || true
    endscript
}
```

---

## 监控与运维

### 1. Prometheus 监控

访问: `http://localhost:9091`

**关键指标**:
- `apk_analysis_tasks_total` - 总任务数
- `apk_analysis_tasks_in_progress` - 进行中任务数
- `apk_analysis_task_duration_seconds` - 任务执行时长
- `apk_analysis_http_requests_total` - HTTP 请求总数
- `apk_analysis_http_request_duration_seconds` - HTTP 请求延迟

### 2. Grafana 仪表盘

访问: `http://localhost:3000` (admin/admin)

**配置步骤**:
1. 添加 Prometheus 数据源
2. 导入预设仪表盘: `deployments/grafana/dashboards/apk-analysis.json`
3. 查看实时监控

### 3. 健康检查

```bash
# 应用健康检查
curl http://localhost:8080/api/health

# 数据库连接检查
mysql -u apk_user -p -e "SELECT 1"

# RabbitMQ 检查
rabbitmqctl status

# Redis 检查
redis-cli ping
```

### 4. 性能分析 (pprof)

```bash
# CPU 性能分析
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 内存分析
go tool pprof http://localhost:6060/debug/pprof/heap

# goroutine 分析
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

---

## 常见问题

### Q1: 数据库连接失败

**症状**: 启动时报错 `dial tcp: connection refused`

**解决方案**:
```bash
# 检查 MySQL 是否运行
sudo systemctl status mysql

# 检查防火墙
sudo ufw status

# 检查配置文件中的数据库地址
vim configs/config.yaml
```

---

### Q2: RabbitMQ 连接超时

**症状**: `connection timeout` 错误

**解决方案**:
```bash
# 检查 RabbitMQ 状态
sudo rabbitmqctl status

# 重启 RabbitMQ
sudo systemctl restart rabbitmq-server

# 检查队列
rabbitmqadmin list queues
```

---

### Q3: 内存占用过高

**症状**: 进程内存占用 >2GB

**解决方案**:
```bash
# 使用 pprof 分析内存
go tool pprof http://localhost:6060/debug/pprof/heap

# 调整数据库连接池
# 修改 config.yaml:
# max_idle_conns: 5
# max_open_conns: 50

# 重启服务
sudo systemctl restart apk-analysis
```

---

### Q4: 端口被占用

**症状**: `bind: address already in use`

**解决方案**:
```bash
# 查找占用端口的进程
sudo lsof -i :8080

# 杀死进程
sudo kill -9 <PID>

# 或更换端口
vim configs/config.yaml
# 修改 server.port
```

---

## 升级指南

### 1. 备份数据

```bash
# 备份数据库
mysqldump -u apk_user -p apk_analysis > backup_$(date +%Y%m%d).sql

# 备份配置文件
cp -r configs configs.backup

# 备份结果文件
tar -czf results_backup_$(date +%Y%m%d).tar.gz results/
```

### 2. 停止服务

```bash
sudo systemctl stop apk-analysis
```

### 3. 更新代码

```bash
git pull origin main

# 或下载新版本
wget https://github.com/your-org/apk-analysis-go/releases/download/v1.1.0/apk-analysis-go-v1.1.0.tar.gz
tar -xzf apk-analysis-go-v1.1.0.tar.gz
```

### 4. 编译新版本

```bash
make build
```

### 5. 数据库迁移 (如有)

```bash
# GORM 自动迁移会在启动时执行
# 或手动执行 SQL
mysql -u apk_user -p apk_analysis < migrations/v1.1.0.sql
```

### 6. 启动服务

```bash
sudo systemctl start apk-analysis
sudo systemctl status apk-analysis
```

### 7. 验证升级

```bash
# 检查版本
curl http://localhost:8080/api/health | jq .version

# 检查日志
sudo journalctl -u apk-analysis -n 100
```

---

## 安全建议

### 1. 数据库安全

- ✅ 使用强密码
- ✅ 限制远程访问
- ✅ 定期备份
- ✅ 启用 SSL/TLS 连接

### 2. API 安全

- ✅ 启用 HTTPS
- ✅ 实施 API 认证 (JWT/OAuth)
- ✅ 限制请求频率 (Rate Limiting)
- ✅ 输入验证和过滤

### 3. 系统安全

- ✅ 定期更新系统和软件包
- ✅ 使用防火墙限制端口访问
- ✅ 配置 SELinux/AppArmor
- ✅ 监控异常访问日志

---

## 性能优化

### 1. 数据库优化

```sql
-- 添加索引
CREATE INDEX idx_status ON apk_tasks(status);
CREATE INDEX idx_created_at ON apk_tasks(created_at);

-- 定期清理旧数据
DELETE FROM apk_tasks WHERE created_at < DATE_SUB(NOW(), INTERVAL 90 DAY);
```

### 2. 连接池优化

```yaml
database:
  mysql:
    max_idle_conns: 20
    max_open_conns: 100
    conn_max_lifetime: 3600
```

### 3. 缓存策略

```yaml
cache:
  redis:
    pool_size: 20
    ttl: 3600  # 1 hour
```

---

## 联系支持

- **文档**: https://docs.apk-analysis.com
- **问题反馈**: https://github.com/your-org/apk-analysis-go/issues
- **Email**: support@apk-analysis.com

---

**最后更新**: 2025-11-05
**维护者**: APK Analysis Team
