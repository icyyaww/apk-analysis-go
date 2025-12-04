# APK Analysis Platform - 架构文档

> **版本**: 1.0.0 (Go 重构版本)
> **最后更新**: 2025-11-05
> **架构风格**: 微服务 + 分层架构 + 领域驱动设计 (DDD)

---

## 📋 目录

- [系统概览](#系统概览)
- [技术栈](#技术栈)
- [架构设计](#架构设计)
- [模块详解](#模块详解)
- [数据流](#数据流)
- [设计模式](#设计模式)
- [性能优化](#性能优化)
- [安全设计](#安全设计)
- [扩展性](#扩展性)

---

## 系统概览

### 项目简介

APK Analysis Platform 是一个**自动化 Android 应用分析系统**,用于:
- 自动化 APK 安装与 Activity 遍历
- 网络流量捕获与归因分析
- MobSF 静态安全分析集成
- AI 智能交互与 UI 自动化
- 域名分析与备案查询
- 实时监控与性能分析

### 核心特性

| 特性 | 说明 | 技术实现 |
|------|------|---------|
| **高性能** | 并发处理 10+ 任务 | Goroutine Worker Pool |
| **高可用** | 99.9% SLA | 健康检查 + 自动恢复 |
| **可扩展** | 水平扩展支持 | 微服务架构 + 消息队列 |
| **可观测** | 实时监控与追踪 | Prometheus + Grafana + pprof |
| **智能化** | AI 驱动的 UI 交互 | 智谱 GLM-4V 多模态 AI |

---

## 技术栈

### 后端技术

| 组件 | 技术选型 | 版本 | 用途 |
|------|---------|------|------|
| **编程语言** | Go | 1.21+ | 高性能后端服务 |
| **Web 框架** | Gin | 1.9+ | HTTP 服务与路由 |
| **ORM** | GORM | 1.25+ | 数据库操作 |
| **数据库** | MySQL / SQLite | 8.0+ / 3.x | 数据持久化 |
| **消息队列** | RabbitMQ | 3.11+ | 异步任务处理 |
| **缓存** | Redis | 7.0+ | 分布式缓存 |
| **日志** | Logrus | 1.9+ | 结构化日志 |
| **配置** | Viper | 1.16+ | 配置管理 |

### 监控与运维

| 组件 | 技术选型 | 用途 |
|------|---------|------|
| **监控** | Prometheus | 指标采集 |
| **可视化** | Grafana | 仪表盘展示 |
| **性能分析** | pprof | CPU/内存分析 |
| **追踪** | OpenTelemetry (计划) | 分布式追踪 |

### 外部服务

| 服务 | 用途 | 集成方式 |
|------|------|---------|
| **MobSF** | 静态安全分析 | HTTP API |
| **智谱 AI** | 多模态 AI 分析 | SDK |
| **ADB** | Android 设备控制 | 命令行 |
| **MitmProxy** | 流量拦截 | JSONL 文件 |

---

## 架构设计

### 整体架构图

```
┌──────────────────────────────────────────────────────────────────┐
│                         Client Layer                              │
│  ┌────────────┐    ┌────────────┐    ┌────────────┐             │
│  │  Dashboard │    │  API Client│    │  Monitoring│             │
│  │  (Web UI)  │    │   (SDK)    │    │  (Grafana) │             │
│  └──────┬─────┘    └──────┬─────┘    └──────┬─────┘             │
└─────────┼──────────────────┼──────────────────┼──────────────────┘
          │                  │                  │
          ▼                  ▼                  ▼
┌──────────────────────────────────────────────────────────────────┐
│                      API Gateway Layer                            │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                    Gin HTTP Server                         │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  │  │
│  │  │ Handlers │  │Middleware│  │  Router  │  │  Metrics │  │  │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘  │  │
│  └────────────────────────────────────────────────────────────┘  │
└─────────┼──────────────────┼──────────────────┼──────────────────┘
          │                  │                  │
          ▼                  ▼                  ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Business Logic Layer                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │
│  │TaskService   │  │WorkerPool    │  │QueueService  │           │
│  │              │  │              │  │              │           │
│  │ - CreateTask │  │ - Workers    │  │ - Publish    │           │
│  │ - GetTask    │  │ - Scheduler  │  │ - Consume    │           │
│  │ - UpdateTask │  │ - Executor   │  │ - Retry      │           │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘           │
│         │                  │                  │                   │
│         ▼                  ▼                  ▼                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │
│  │ADBClient     │  │MobSFClient   │  │AIClient      │           │
│  │              │  │              │  │              │           │
│  │ - Connect    │  │ - Upload     │  │ - Analyze    │           │
│  │ - Install    │  │ - Scan       │  │ - Action     │           │
│  │ - Screenshot │  │ - Report     │  │ - Interact   │           │
│  └──────────────┘  └──────────────┘  └──────────────┘           │
└─────────┼──────────────────┼──────────────────┼──────────────────┘
          │                  │                  │
          ▼                  ▼                  ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Data Access Layer                             │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                    Repository Layer                        │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │  │
│  │  │TaskRepository│  │ActivityRepo  │  │DomainRepo    │    │  │
│  │  │              │  │              │  │              │    │  │
│  │  │ - CRUD Ops   │  │ - CRUD Ops   │  │ - CRUD Ops   │    │  │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘    │  │
│  └─────────┼──────────────────┼──────────────────┼────────────┘  │
└────────────┼──────────────────┼──────────────────┼───────────────┘
             │                  │                  │
             ▼                  ▼                  ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Infrastructure Layer                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐         │
│  │  MySQL   │  │ RabbitMQ │  │  Redis   │  │ External │         │
│  │ Database │  │  Queue   │  │  Cache   │  │ Services │         │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘         │
└──────────────────────────────────────────────────────────────────┘
```

---

### 分层架构详解

#### 1. API Gateway Layer (API 网关层)

**职责**:
- HTTP 请求路由
- 请求验证与参数解析
- 中间件处理 (日志、CORS、限流等)
- 响应格式化

**核心组件**:
```
internal/api/
├── router.go           # 路由注册
├── middleware/         # 中间件
│   ├── logger.go       # 日志中间件
│   ├── cors.go         # CORS 中间件
│   ├── prometheus.go   # 监控中间件
│   └── recovery.go     # 错误恢复
└── handlers/           # 请求处理器
    ├── task_handler.go # 任务相关 API
    └── health_handler.go # 健康检查
```

---

#### 2. Business Logic Layer (业务逻辑层)

**职责**:
- 核心业务逻辑实现
- 任务编排与调度
- 第三方服务集成
- 业务规则校验

**核心服务**:

**TaskService** (`internal/service/task_service.go`):
```go
type TaskService interface {
    CreateTask(ctx context.Context, apkName string) (*domain.Task, error)
    GetTask(ctx context.Context, id string) (*domain.Task, error)
    UpdateTaskStatus(ctx context.Context, id string, status domain.TaskStatus) error
    UpdateTaskProgress(ctx context.Context, id string, percent int, step string) error
    DeleteTask(ctx context.Context, id string) error
    ListRecentTasks(ctx context.Context, limit int) ([]*domain.Task, error)
    GetTasksByStatus(ctx context.Context, status domain.TaskStatus) ([]*domain.Task, error)
    GetTaskStatistics(ctx context.Context) (map[string]int64, error)
}
```

**WorkerPool** (`internal/worker/worker_pool.go`):
```go
type WorkerPool struct {
    workers       int
    taskQueue     chan *domain.Task
    stopChan      chan struct{}
    wg            sync.WaitGroup
    executor      TaskExecutor
}

func (wp *WorkerPool) Start()
func (wp *WorkerPool) Stop()
func (wp *WorkerPool) Submit(task *domain.Task) error
```

**QueueService** (`internal/queue/queue_service.go`):
```go
type QueueService interface {
    Publish(task *domain.Task) error
    Consume(handler MessageHandler) error
    Close() error
}
```

---

#### 3. Data Access Layer (数据访问层)

**职责**:
- 数据库 CRUD 操作
- 查询优化
- 事务管理
- 数据映射

**Repository 接口** (`internal/repository/task_repository.go`):
```go
type TaskRepository interface {
    Create(ctx context.Context, task *domain.Task) error
    Update(ctx context.Context, task *domain.Task) error
    FindByID(ctx context.Context, id string) (*domain.Task, error)
    ListRecent(ctx context.Context, limit int) ([]*domain.Task, error)
    Delete(ctx context.Context, id string) error
    UpdateStatus(ctx context.Context, id string, status domain.TaskStatus) error
    UpdateProgress(ctx context.Context, id string, percent int, step string) error
    FindByStatus(ctx context.Context, status domain.TaskStatus) ([]*domain.Task, error)
    CountByStatus(ctx context.Context, status domain.TaskStatus) (int64, error)
}
```

---

#### 4. Infrastructure Layer (基础设施层)

**职责**:
- 数据库连接管理
- 消息队列连接
- 缓存管理
- 外部服务接口

**组件**:
- MySQL / SQLite 数据库
- RabbitMQ 消息队列
- Redis 缓存
- MobSF、智谱 AI 等外部服务

---

## 模块详解

### 1. Domain Layer (领域层)

**路径**: `internal/domain/`

**核心实体**:

#### Task (任务实体)
```go
type Task struct {
    ID              string       `gorm:"primaryKey"`
    APKName         string       `gorm:"type:varchar(255)"`
    PackageName     string       `gorm:"type:varchar(255)"`
    Status          TaskStatus   `gorm:"type:varchar(50)"`
    CreatedAt       time.Time    `gorm:"type:datetime(6)"`
    StartedAt       *time.Time   `gorm:"type:datetime(6)"`
    CompletedAt     *time.Time   `gorm:"type:datetime(6)"`
    CurrentStep     string       `gorm:"type:varchar(255)"`
    ProgressPercent int          `gorm:"type:int"`
    ErrorMessage    string       `gorm:"type:text"`

    // 关联关系
    Activities      []TaskActivity       `gorm:"foreignKey:TaskID"`
    MobSFReport     *TaskMobSFReport     `gorm:"foreignKey:TaskID"`
    DomainAnalysis  *TaskDomainAnalysis  `gorm:"foreignKey:TaskID"`
    AILogs          *TaskAILog           `gorm:"foreignKey:TaskID"`
}
```

#### TaskStatus (任务状态枚举)
```go
type TaskStatus string

const (
    TaskStatusQueued     TaskStatus = "queued"
    TaskStatusInstalling TaskStatus = "installing"
    TaskStatusRunning    TaskStatus = "running"
    TaskStatusCollecting TaskStatus = "collecting"
    TaskStatusCompleted  TaskStatus = "completed"
    TaskStatusFailed     TaskStatus = "failed"
    TaskStatusCancelled  TaskStatus = "cancelled"
)
```

---

### 2. Worker Pool (工作池)

**设计目标**:
- 并发处理多个 APK 分析任务
- 资源隔离 (每个任务独立 goroutine)
- 任务队列管理
- 优雅关闭

**实现细节**:

```go
type WorkerPool struct {
    workers       int                    // Worker 数量
    taskQueue     chan *domain.Task      // 任务队列
    stopChan      chan struct{}          // 停止信号
    wg            sync.WaitGroup         // 等待组
    executor      TaskExecutor           // 任务执行器
    mu            sync.RWMutex           // 读写锁
    activeWorkers int                    // 活跃 Worker 数
}

func NewWorkerPool(workers int, queueSize int, executor TaskExecutor) *WorkerPool {
    return &WorkerPool{
        workers:   workers,
        taskQueue: make(chan *domain.Task, queueSize),
        stopChan:  make(chan struct{}),
        executor:  executor,
    }
}

func (wp *WorkerPool) Start() {
    for i := 0; i < wp.workers; i++ {
        wp.wg.Add(1)
        go wp.worker(i)
    }
}

func (wp *WorkerPool) worker(id int) {
    defer wp.wg.Done()

    for {
        select {
        case task := <-wp.taskQueue:
            wp.incrementActive()
            wp.executor.Execute(task)
            wp.decrementActive()
        case <-wp.stopChan:
            return
        }
    }
}
```

**并发控制**:
- 使用 buffered channel 限制并发数
- 信号量模式控制资源访问
- Context 传递超时和取消信号

---

### 3. ADB Client (Android 设备控制)

**路径**: `internal/adb/adb_client.go`

**功能**:
- 设备连接与断开
- APK 安装与卸载
- Activity 启动
- 屏幕截图
- UI Hierarchy 提取

**关键方法**:
```go
type ADBClient interface {
    Connect(deviceID string) error
    Disconnect() error
    InstallAPK(apkPath string) error
    UninstallAPK(packageName string) error
    StartActivity(component string) error
    Screenshot(outputPath string) error
    DumpUIHierarchy(outputPath string) error
    ExecuteShellCommand(command string) (string, error)
}
```

**实现示例**:
```go
func (c *ADBClientImpl) StartActivity(component string) error {
    cmd := exec.Command("adb", "shell", "am", "start", "-n", component)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("failed to start activity: %w, output: %s", err, output)
    }
    return nil
}
```

---

### 4. MobSF Client (静态分析集成)

**路径**: `internal/mobsf/mobsf_client.go`

**功能**:
- APK 上传
- 静态扫描触发
- 扫描状态轮询
- 报告获取

**API 集成**:
```go
type MobSFClient interface {
    UploadAPK(apkPath string) (hash string, err error)
    Scan(hash string) error
    GetScanStatus(hash string) (status string, err error)
    GetReport(hash string) (*MobSFReport, error)
}

// 上传 APK
func (c *MobSFClientImpl) UploadAPK(apkPath string) (string, error) {
    file, err := os.Open(apkPath)
    if err != nil {
        return "", err
    }
    defer file.Close()

    body := &bytes.Buffer{}
    writer := multipart.NewWriter(body)

    part, _ := writer.CreateFormFile("file", filepath.Base(apkPath))
    io.Copy(part, file)
    writer.Close()

    req, _ := http.NewRequest("POST", c.baseURL+"/api/v1/upload", body)
    req.Header.Set("Content-Type", writer.FormDataContentType())
    req.Header.Set("Authorization", c.apiKey)

    resp, err := c.httpClient.Do(req)
    // ... 解析响应
}
```

**队列化处理**:
- 单线程执行避免 MobSF OOM
- 任务队列缓冲
- 健康检查与重试机制

---

### 5. AI Client (智能交互)

**路径**: `internal/ai/ai_client.go`

**功能**:
- 截图上传与分析
- UI 元素识别
- 交互策略生成
- 动作执行

**流程**:
```
1. 截图 → Base64 编码
2. 调用智谱 GLM-4V API
3. 解析返回的交互建议
4. 执行点击/输入/滑动
5. 记录交互日志
```

**实现**:
```go
type AIClient interface {
    AnalyzeUI(screenshotPath string) (*UIAnalysisResult, error)
    GenerateActions(analysis *UIAnalysisResult) ([]*Action, error)
    ExecuteAction(action *Action) error
}

type UIAnalysisResult struct {
    Buttons       []UIElement
    InputFields   []UIElement
    ImportantElements []UIElement
}

type Action struct {
    Type   ActionType  // click, input, swipe
    Target UIElement
    Value  string      // 用于 input 动作
}
```

---

### 6. Flow Attribution (流量归因)

**路径**: `internal/flow/flow_attribution.go`

**原理**:
- 基于时间戳 (timestamp) 归因
- 增量读取 JSONL 文件
- Activity 执行前后标记

**算法**:
```go
type FlowAttribution struct {
    flowFilePath string
    lastIndex    int
}

func (fa *FlowAttribution) AttributeToActivity(activityName string) ([]*Flow, error) {
    // 1. 读取当前行索引
    currentIndex := fa.getCurrentLineCount()

    // 2. 读取增量流量 (从 lastIndex 到 currentIndex)
    flows := fa.readLines(fa.lastIndex, currentIndex)

    // 3. 更新索引
    fa.lastIndex = currentIndex

    // 4. 返回归因流量
    return flows, nil
}
```

**数据格式** (JSONL):
```json
{"ts": 1730649023.456, "method": "GET", "url": "https://api.example.com/init"}
{"ts": 1730649024.789, "method": "POST", "url": "https://analytics.example.com/track"}
```

---

### 7. Domain Analysis (域名分析)

**路径**: `internal/domain_analysis/domain_analyzer.go`

**功能**:
- 主域名识别 (从动态流量 + 静态代码)
- 域名备案查询
- IP 归属地查询

**主域名识别算法**:
```go
func (da *DomainAnalyzer) IdentifyPrimaryDomain(packageName string, flows []*Flow, mobsfReport *MobSFReport) (string, float64) {
    candidates := make(map[string]int)

    // 1. 从动态流量中提取域名
    for _, flow := range flows {
        domain := extractDomain(flow.URL)
        candidates[domain]++
    }

    // 2. 从 MobSF 报告中提取域名
    for domain := range mobsfReport.Domains {
        candidates[domain]++
    }

    // 3. 计算匹配度
    scores := make(map[string]float64)
    for domain, count := range candidates {
        score := 0.0

        // 包名匹配 (+50%)
        if containsPackageKeyword(domain, packageName) {
            score += 0.5
        }

        // 请求次数 (+最多50%)
        score += min(float64(count)/100, 0.5)

        scores[domain] = score
    }

    // 4. 返回最高分域名
    return findMaxScore(scores)
}
```

---

## 数据流

### 任务执行完整流程

```
1. 用户上传 APK
   ↓
2. API Handler 接收请求
   ↓
3. TaskService.CreateTask()
   - 生成 UUID
   - 保存到数据库 (status: queued)
   ↓
4. QueueService.Publish()
   - 发送任务到 RabbitMQ
   ↓
5. WorkerPool.Consume()
   - Worker 从队列获取任务
   ↓
6. TaskExecutor.Execute()
   ├─ ADB.InstallAPK()
   ├─ Frida SSL Unpinning (可选)
   ├─ MobSF.UploadAndScan() (异步)
   ├─ Activity 遍历:
   │  ├─ ADB.StartActivity()
   │  ├─ ADB.Screenshot()
   │  ├─ AI.AnalyzeUI() (可选)
   │  ├─ AI.ExecuteActions()
   │  └─ FlowAttribution.Attribute()
   ├─ DomainAnalysis.Analyze()
   └─ TaskService.UpdateStatus(completed)
   ↓
7. 结果存储
   - 数据库: 任务状态、Activity 详情
   - 文件系统: 截图、UI Hierarchy、流量 JSONL
```

---

### 数据流图

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ HTTP Request
       ▼
┌──────────────────┐
│  API Handler     │
└──────┬───────────┘
       │ CreateTask()
       ▼
┌──────────────────┐      ┌──────────────────┐
│  TaskService     │─────>│  TaskRepository  │
└──────┬───────────┘      └────────┬─────────┘
       │                           │
       │ Publish                   │ INSERT
       ▼                           ▼
┌──────────────────┐      ┌──────────────────┐
│  QueueService    │      │  MySQL Database  │
│   (RabbitMQ)     │      └──────────────────┘
└──────┬───────────┘
       │ Consume
       ▼
┌──────────────────┐
│   WorkerPool     │
└──────┬───────────┘
       │ Execute
       ▼
┌──────────────────────────────────────┐
│         TaskExecutor                 │
│  ┌────────────┐  ┌────────────┐     │
│  │ ADB Client │  │MobSF Client│     │
│  └────────────┘  └────────────┘     │
│  ┌────────────┐  ┌────────────┐     │
│  │ AI Client  │  │Flow Attrib.│     │
│  └────────────┘  └────────────┘     │
└──────────────────┬───────────────────┘
                   │ Update
                   ▼
          ┌──────────────────┐
          │  TaskRepository  │
          └──────────────────┘
```

---

## 设计模式

### 1. Repository Pattern (仓储模式)

**目的**: 分离数据访问逻辑与业务逻辑

**实现**:
```go
// 接口定义
type TaskRepository interface {
    Create(ctx context.Context, task *domain.Task) error
    FindByID(ctx context.Context, id string) (*domain.Task, error)
}

// 具体实现
type TaskRepositoryImpl struct {
    db *gorm.DB
}

func (r *TaskRepositoryImpl) Create(ctx context.Context, task *domain.Task) error {
    return r.db.WithContext(ctx).Create(task).Error
}
```

**优点**:
- 易于测试 (可 Mock)
- 数据库切换方便 (MySQL ↔ SQLite)
- 关注点分离

---

### 2. Dependency Injection (依赖注入)

**目的**: 降低耦合,提高可测试性

**实现**:
```go
type TaskService struct {
    repo   repository.TaskRepository  // 注入 Repository
    logger *logrus.Logger              // 注入 Logger
}

func NewTaskService(repo repository.TaskRepository, logger *logrus.Logger) *TaskService {
    return &TaskService{
        repo:   repo,
        logger: logger,
    }
}
```

**优点**:
- 易于单元测试 (注入 Mock 对象)
- 松耦合
- 易于替换实现

---

### 3. Worker Pool Pattern (工作池模式)

**目的**: 控制并发数,复用 goroutine

**实现**:
```go
type WorkerPool struct {
    workers   int
    taskQueue chan *domain.Task
}

func (wp *WorkerPool) Start() {
    for i := 0; i < wp.workers; i++ {
        go wp.worker(i)
    }
}

func (wp *WorkerPool) Submit(task *domain.Task) error {
    wp.taskQueue <- task
    return nil
}
```

**优点**:
- 限制并发数 (避免资源耗尽)
- goroutine 复用 (减少创建开销)
- 任务队列缓冲

---

### 4. Factory Pattern (工厂模式)

**目的**: 创建复杂对象

**实现**:
```go
func NewTaskExecutor(adbClient adb.ADBClient, mobsfClient mobsf.MobSFClient, aiClient ai.AIClient) *TaskExecutor {
    return &TaskExecutor{
        adb:   adbClient,
        mobsf: mobsfClient,
        ai:    aiClient,
    }
}
```

---

### 5. Strategy Pattern (策略模式)

**目的**: 不同分析策略可切换

**实现**:
```go
type ActivityFilterStrategy interface {
    Filter(activities []string) []string
}

type SmartFilterStrategy struct{}
func (s *SmartFilterStrategy) Filter(activities []string) []string {
    // 智能过滤逻辑
}

type SimpleFilterStrategy struct{}
func (s *SimpleFilterStrategy) Filter(activities []string) []string {
    // 简单过滤逻辑
}
```

---

## 性能优化

### 1. 数据库优化

**索引设计**:
```sql
CREATE INDEX idx_status ON apk_tasks(status);
CREATE INDEX idx_created_at ON apk_tasks(created_at);
CREATE INDEX idx_package_name ON apk_tasks(package_name);
```

**连接池配置**:
```go
db.DB().SetMaxIdleConns(10)
db.DB().SetMaxOpenConns(100)
db.DB().SetConnMaxLifetime(time.Hour)
```

**批量操作**:
```go
// 批量插入
db.CreateInBatches(tasks, 100)
```

---

### 2. 并发优化

**Goroutine 池化**:
- Worker Pool 限制并发数
- 避免无限创建 goroutine

**Channel 缓冲**:
```go
taskQueue := make(chan *domain.Task, 100) // 缓冲 100 个任务
```

**读写锁**:
```go
var mu sync.RWMutex

// 读操作
mu.RLock()
value := sharedMap[key]
mu.RUnlock()

// 写操作
mu.Lock()
sharedMap[key] = newValue
mu.Unlock()
```

---

### 3. 内存优化

**对象池 (sync.Pool)**:
```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

buf := bufferPool.Get().(*bytes.Buffer)
defer bufferPool.Put(buf)
```

**流式读取**:
```go
// 避免一次性加载大文件
scanner := bufio.NewScanner(file)
for scanner.Scan() {
    line := scanner.Text()
    // 处理每一行
}
```

---

### 4. 缓存策略

**Redis 缓存**:
```go
// 缓存任务信息
func (s *TaskService) GetTask(ctx context.Context, id string) (*domain.Task, error) {
    // 1. 尝试从 Redis 获取
    cached, err := s.redis.Get(ctx, "task:"+id).Result()
    if err == nil {
        var task domain.Task
        json.Unmarshal([]byte(cached), &task)
        return &task, nil
    }

    // 2. 从数据库查询
    task, err := s.repo.FindByID(ctx, id)
    if err != nil {
        return nil, err
    }

    // 3. 写入缓存
    data, _ := json.Marshal(task)
    s.redis.Set(ctx, "task:"+id, data, time.Hour)

    return task, nil
}
```

---

## 安全设计

### 1. 输入验证

```go
func (h *TaskHandler) CreateTask(c *gin.Context) {
    var req CreateTaskRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "Invalid request"})
        return
    }

    // 验证 APK 名称
    if !isValidAPKName(req.APKName) {
        c.JSON(400, gin.H{"error": "Invalid APK name"})
        return
    }
}
```

---

### 2. SQL 注入防护

**使用 GORM Prepared Statement**:
```go
// 安全: 参数化查询
db.Where("id = ?", userInput).First(&task)

// 危险: 字符串拼接
db.Where("id = '" + userInput + "'").First(&task) // ❌
```

---

### 3. 错误处理

```go
func (s *TaskService) GetTask(ctx context.Context, id string) (*domain.Task, error) {
    task, err := s.repo.FindByID(ctx, id)
    if err != nil {
        // 不要泄露内部错误信息
        s.logger.WithError(err).Error("Failed to get task")
        return nil, fmt.Errorf("task not found")
    }
    return task, nil
}
```

---

### 4. 日志脱敏

```go
func (s *TaskService) LogTaskCreated(task *domain.Task) {
    // 不要记录敏感信息
    s.logger.WithFields(logrus.Fields{
        "task_id": task.ID,
        "status":  task.Status,
        // "api_key": task.APIKey, // ❌ 不要记录 API Key
    }).Info("Task created")
}
```

---

## 扩展性

### 1. 水平扩展

**无状态设计**:
- 服务无状态,可任意扩容
- 通过负载均衡分发请求

**分布式任务队列**:
- RabbitMQ 支持多消费者
- 多实例竞争消费任务

**配置示例**:
```yaml
# 部署多个实例
docker-compose scale apk-analysis=3
```

---

### 2. 微服务拆分 (未来)

**当前**: 单体应用
**未来**: 微服务

```
apk-analysis-go (单体)
    ↓
    拆分
    ↓
┌─────────────────────────────────┐
│ task-service     (任务管理)      │
│ worker-service   (任务执行)      │
│ analysis-service (分析服务)      │
│ api-gateway      (API 网关)      │
└─────────────────────────────────┘
```

---

### 3. 插件化架构 (计划)

**目标**: 支持自定义分析插件

```go
type AnalysisPlugin interface {
    Name() string
    Execute(task *domain.Task) error
}

type PluginManager struct {
    plugins map[string]AnalysisPlugin
}

func (pm *PluginManager) Register(plugin AnalysisPlugin) {
    pm.plugins[plugin.Name()] = plugin
}
```

---

## 监控指标

### Prometheus 指标

```
# 任务相关
apk_analysis_tasks_total{status="completed"}
apk_analysis_tasks_in_progress
apk_analysis_task_duration_seconds

# HTTP 相关
apk_analysis_http_requests_total{method="GET", path="/api/tasks"}
apk_analysis_http_request_duration_seconds

# Worker Pool
apk_analysis_worker_pool_size
apk_analysis_worker_pool_active
apk_analysis_worker_pool_queue_size

# 数据库
apk_analysis_db_connections_open
apk_analysis_db_connections_idle
apk_analysis_db_query_duration_seconds
```

---

## 总结

### 核心优势

| 优势 | 说明 |
|------|------|
| **高性能** | Go 并发模型 + Worker Pool + 连接池优化 |
| **高可用** | 健康检查 + 自动恢复 + 优雅关闭 |
| **可扩展** | 分层架构 + 依赖注入 + 微服务预留 |
| **可维护** | 清晰分层 + 设计模式 + 完善测试 |
| **可观测** | Prometheus + Grafana + pprof + 结构化日志 |

---

### Python → Go 重构收益

| 指标 | Python 版本 | Go 版本 | 提升 |
|------|------------|---------|------|
| **内存占用** | ~5.5 GB | ~1.8 GB | **-67%** |
| **并发处理** | 1-2 任务 | 10+ 任务 | **5-10x** |
| **启动时间** | ~10s | ~2s | **5x** |
| **CPU 利用率** | 25-30% | 15-20% | **30% ↓** |
| **代码可维护性** | 中 | 高 | **显著提升** |

---

**最后更新**: 2025-11-05
**维护者**: APK Analysis Team
