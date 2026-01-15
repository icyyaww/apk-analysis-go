# 动态 URL 存储方案设计

> **文档版本**: 1.0
> **创建日期**: 2025-12-18
> **状态**: 待实施

---

## 1. 问题背景

### 1.1 当前问题

动态分析期间捕获的流量（URL）没有可靠地存储到数据库中，导致域名分析时无法获取完整的动态 URL。

**当前数据流**：

```
动态分析执行
    ↓
flows.jsonl (文件) ←── API 读取展示 ✅
    ↓
activity_details_json (数据库) ←── 只存了部分流量 ❌
    ↓
域名分析 ←── 读不到完整动态 URL ❌
    ↓
主域名识别错误（只基于静态 URL）
```

### 1.2 问题表现

| 数据来源 | 存储位置 | 动态 URL 数量 | 问题 |
|---------|---------|--------------|------|
| `flows.jsonl` 文件 | 文件系统 | 完整 (如 26 条) | 文件可能被清理 |
| `activity_details_json` | 数据库 | 不完整 (如 1 条) | 只存 Activity 遍历期间的流量 |
| `url_analysis_dynamic` | 数据库 | 空 | 未使用 |

### 1.3 影响

1. **域名分析不准确**：动态流量中访问最多的域名未被识别为主域名
2. **数据不持久**：`flows.jsonl` 文件被清理后，动态流量数据丢失
3. **查询不便**：无法通过数据库查询历史任务的动态流量

---

## 2. 设计目标

1. **完整性**：存储所有动态捕获的流量 URL
2. **持久性**：数据存储在数据库中，不依赖文件系统
3. **可查询**：支持按任务、域名、时间等维度查询
4. **兼容性**：不影响现有的 `flows.jsonl` 文件写入逻辑
5. **性能**：批量写入，减少数据库压力

---

## 3. 数据库设计

### 3.1 新建 `task_flows` 表

存储完整的流量记录，每条流量一行。

```sql
CREATE TABLE task_flows (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    task_id VARCHAR(36) NOT NULL COMMENT '任务 ID',
    url VARCHAR(2048) NOT NULL COMMENT '完整 URL',
    host VARCHAR(255) NOT NULL COMMENT '主机名/域名',
    port INT DEFAULT 443 COMMENT '端口',
    path VARCHAR(1024) COMMENT 'URL 路径',
    method VARCHAR(10) DEFAULT 'GET' COMMENT 'HTTP 方法',
    scheme VARCHAR(10) DEFAULT 'https' COMMENT '协议 (http/https)',
    status_code INT COMMENT 'HTTP 状态码',
    content_type VARCHAR(128) COMMENT '响应内容类型',
    request_size INT COMMENT '请求大小 (bytes)',
    response_size INT COMMENT '响应大小 (bytes)',
    timestamp DECIMAL(16,6) COMMENT 'Unix 时间戳 (秒.微秒)',
    activity VARCHAR(255) COMMENT '关联的 Activity 名称',
    source VARCHAR(20) DEFAULT 'mitmproxy' COMMENT '来源: mitmproxy/frida',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '记录创建时间',

    INDEX idx_task_id (task_id),
    INDEX idx_host (host),
    INDEX idx_task_host (task_id, host),
    INDEX idx_task_timestamp (task_id, timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='任务动态流量记录表';
```

**字段说明**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | BIGINT | 是 | 自增主键 |
| task_id | VARCHAR(36) | 是 | 关联的任务 ID |
| url | VARCHAR(2048) | 是 | 完整的请求 URL |
| host | VARCHAR(255) | 是 | 主机名，用于域名分析 |
| port | INT | 否 | 端口号，默认 443 |
| path | VARCHAR(1024) | 否 | URL 路径部分 |
| method | VARCHAR(10) | 否 | HTTP 方法：GET/POST/PUT 等 |
| scheme | VARCHAR(10) | 否 | 协议：http 或 https |
| status_code | INT | 否 | HTTP 响应状态码 |
| content_type | VARCHAR(128) | 否 | 响应的 Content-Type |
| request_size | INT | 否 | 请求体大小 |
| response_size | INT | 否 | 响应体大小 |
| timestamp | DECIMAL(16,6) | 否 | 请求发生的时间戳 |
| activity | VARCHAR(255) | 否 | 当前正在遍历的 Activity |
| source | VARCHAR(20) | 否 | 数据来源 |
| created_at | TIMESTAMP | 是 | 记录入库时间 |

### 3.2 复用 `url_analysis_dynamic` 字段

在现有的 `task_domain_analysis` 表中，`url_analysis_dynamic` 字段用于存储去重汇总后的动态 URL。

**数据格式 (JSON)**：

```json
{
  "urls": [
    "https://pic.dsylm.com/api.php/Index/upGrade",
    "https://pic.dsylm.com/api.php/Comment/rollcomments",
    "https://dualstack-arestapi.amap.com/sdk/compliance/params"
  ],
  "domains": {
    "dsylm.com": {
      "count": 18,
      "subdomains": ["pic.dsylm.com"]
    },
    "amap.com": {
      "count": 6,
      "subdomains": ["dualstack-arestapi.amap.com", "dualstack-mpsapi.amap.com"]
    }
  },
  "total_count": 26,
  "unique_url_count": 24,
  "unique_domain_count": 5,
  "captured_at": "2025-12-18T10:00:00Z"
}
```

---

## 4. 数据流设计

### 4.1 整体流程

```
┌─────────────────────────────────────────────────────────────────┐
│                        动态分析执行                              │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  Activity 遍历 / Monkey 测试 / 后台监控                          │
│                                                                  │
│  每次从 mitmproxy 获取流量后:                                    │
│    1. 写入 flows.jsonl 文件 (保持现有逻辑)                       │
│    2. 批量写入 task_flows 表 (新增) ✅                           │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  动态分析完成 (completeTask 之前)                                │
│                                                                  │
│  汇总动态 URL:                                                   │
│    1. 从 task_flows 表读取该任务的所有流量                       │
│    2. 去重、统计域名                                             │
│    3. 存入 url_analysis_dynamic 字段                            │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  域名分析 (AnalyzeTask)                                          │
│                                                                  │
│  读取动态 URL (优先级):                                          │
│    1. 从 url_analysis_dynamic 字段读取 (推荐，已汇总)            │
│    2. 从 task_flows 表读取 (备选，完整数据)                      │
│    3. 从 flows.jsonl 文件读取 (兜底，文件可能不存在)             │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  主域名识别                                                      │
│                                                                  │
│  合并静态 URL + 动态 URL → 评分排序 → 选择主域名                 │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 写入时机

| 阶段 | 写入目标 | 触发条件 |
|------|---------|---------|
| Activity 遍历 | `task_flows` 表 | 每次调用 `collectFlows()` 后 |
| Monkey 测试 | `task_flows` 表 | Monkey 执行完成后 |
| 后台监控 | `task_flows` 表 | 监控阶段结束后 |
| 动态分析完成 | `url_analysis_dynamic` 字段 | `completeTask()` 调用前 |

### 4.3 读取优先级

```go
func (s *AnalysisService) extractDynamicURLs(task *domain.Task) []string {
    // 优先级 1: 从 url_analysis_dynamic 字段读取（已汇总，最快）
    if urls := s.readFromURLAnalysisDynamic(task); len(urls) > 0 {
        return urls
    }

    // 优先级 2: 从 task_flows 表读取（完整数据）
    if urls := s.readFromTaskFlowsTable(task.ID); len(urls) > 0 {
        return urls
    }

    // 优先级 3: 从 activity_details_json 读取（部分数据）
    if urls := s.readFromActivityDetails(task); len(urls) > 0 {
        return urls
    }

    // 优先级 4: 从 flows.jsonl 文件读取（兜底）
    return s.readFromFlowsFile(task.ID)
}
```

---

## 5. 代码修改清单

### 5.1 新增文件

| 文件路径 | 说明 |
|---------|------|
| `internal/domain/flow.go` | 定义 `TaskFlow` 结构体 |
| `internal/repository/flow_repo.go` | 流量数据访问层 |

### 5.2 修改文件

| 文件路径 | 修改内容 |
|---------|---------|
| `internal/repository/database.go` | 添加 `task_flows` 表自动迁移 |
| `internal/worker/orchestrator.go` | 收集流量时调用 `SaveFlows()` |
| `internal/worker/orchestrator.go` | `completeTask()` 前调用汇总方法 |
| `internal/domainanalysis/service.go` | 修改 `extractDynamicURLs()` 读取优先级 |

### 5.3 详细修改说明

#### 5.3.1 新增 `internal/domain/flow.go`

```go
package domain

import "time"

// TaskFlow 任务流量记录
type TaskFlow struct {
    ID           int64     `gorm:"primaryKey;autoIncrement"`
    TaskID       string    `gorm:"type:varchar(36);not null;index"`
    URL          string    `gorm:"type:varchar(2048);not null"`
    Host         string    `gorm:"type:varchar(255);not null;index"`
    Port         int       `gorm:"default:443"`
    Path         string    `gorm:"type:varchar(1024)"`
    Method       string    `gorm:"type:varchar(10);default:'GET'"`
    Scheme       string    `gorm:"type:varchar(10);default:'https'"`
    StatusCode   int       `gorm:"column:status_code"`
    ContentType  string    `gorm:"type:varchar(128)"`
    RequestSize  int       `gorm:"column:request_size"`
    ResponseSize int       `gorm:"column:response_size"`
    Timestamp    float64   `gorm:"type:decimal(16,6)"`
    Activity     string    `gorm:"type:varchar(255)"`
    Source       string    `gorm:"type:varchar(20);default:'mitmproxy'"`
    CreatedAt    time.Time `gorm:"autoCreateTime"`
}

func (TaskFlow) TableName() string {
    return "task_flows"
}
```

#### 5.3.2 新增 `internal/repository/flow_repo.go`

```go
package repository

import (
    "context"
    "gorm.io/gorm"
    "apk-analysis-go/internal/domain"
)

type FlowRepository interface {
    SaveFlows(ctx context.Context, flows []*domain.TaskFlow) error
    GetFlowsByTaskID(ctx context.Context, taskID string) ([]*domain.TaskFlow, error)
    GetUniqueURLsByTaskID(ctx context.Context, taskID string) ([]string, error)
    GetDomainStatsByTaskID(ctx context.Context, taskID string) (map[string]int, error)
    DeleteFlowsByTaskID(ctx context.Context, taskID string) error
}

type flowRepo struct {
    db *gorm.DB
}

func NewFlowRepository(db *gorm.DB) FlowRepository {
    return &flowRepo{db: db}
}

// SaveFlows 批量保存流量记录
func (r *flowRepo) SaveFlows(ctx context.Context, flows []*domain.TaskFlow) error {
    if len(flows) == 0 {
        return nil
    }
    // 批量插入，每批 100 条
    return r.db.WithContext(ctx).CreateInBatches(flows, 100).Error
}

// GetFlowsByTaskID 获取任务的所有流量记录
func (r *flowRepo) GetFlowsByTaskID(ctx context.Context, taskID string) ([]*domain.TaskFlow, error) {
    var flows []*domain.TaskFlow
    err := r.db.WithContext(ctx).
        Where("task_id = ?", taskID).
        Order("timestamp ASC").
        Find(&flows).Error
    return flows, err
}

// GetUniqueURLsByTaskID 获取任务的去重 URL 列表
func (r *flowRepo) GetUniqueURLsByTaskID(ctx context.Context, taskID string) ([]string, error) {
    var urls []string
    err := r.db.WithContext(ctx).
        Model(&domain.TaskFlow{}).
        Where("task_id = ?", taskID).
        Distinct("url").
        Pluck("url", &urls).Error
    return urls, err
}

// GetDomainStatsByTaskID 获取任务的域名统计
func (r *flowRepo) GetDomainStatsByTaskID(ctx context.Context, taskID string) (map[string]int, error) {
    type DomainCount struct {
        Host  string
        Count int
    }
    var results []DomainCount

    err := r.db.WithContext(ctx).
        Model(&domain.TaskFlow{}).
        Select("host, COUNT(*) as count").
        Where("task_id = ?", taskID).
        Group("host").
        Scan(&results).Error

    if err != nil {
        return nil, err
    }

    stats := make(map[string]int)
    for _, r := range results {
        stats[r.Host] = r.Count
    }
    return stats, nil
}

// DeleteFlowsByTaskID 删除任务的所有流量记录
func (r *flowRepo) DeleteFlowsByTaskID(ctx context.Context, taskID string) error {
    return r.db.WithContext(ctx).
        Where("task_id = ?", taskID).
        Delete(&domain.TaskFlow{}).Error
}
```

#### 5.3.3 修改 `internal/worker/orchestrator.go`

```go
// 在收集流量的地方添加数据库写入

func (o *Orchestrator) collectAndSaveFlows(ctx context.Context, taskID, activity string) ([]*flow.FlowRecord, error) {
    // 1. 从 mitmproxy 获取流量
    flows, err := o.flowParser.GetFlows(...)
    if err != nil {
        return nil, err
    }

    // 2. 写入文件（保持现有逻辑）
    flowsPath := filepath.Join(o.resultsDir, taskID, "flows.jsonl")
    o.appendFlowsToFile(flowsPath, flows)

    // 3. 写入数据库（新增）
    taskFlows := make([]*domain.TaskFlow, 0, len(flows))
    for _, f := range flows {
        taskFlows = append(taskFlows, &domain.TaskFlow{
            TaskID:    taskID,
            URL:       f.URL,
            Host:      f.Host,
            Port:      f.Port,
            Path:      f.Path,
            Method:    f.Method,
            Scheme:    f.Scheme,
            Timestamp: f.Timestamp,
            Activity:  activity,
            Source:    "mitmproxy",
        })
    }

    if err := o.flowRepo.SaveFlows(ctx, taskFlows); err != nil {
        o.logger.WithError(err).Warn("Failed to save flows to database")
        // 不影响主流程，只记录警告
    }

    return flows, nil
}

// 在 completeTask 之前添加汇总逻辑
func (o *Orchestrator) summarizeDynamicURLs(ctx context.Context, taskID string) error {
    // 从 task_flows 表读取并汇总
    urls, err := o.flowRepo.GetUniqueURLsByTaskID(ctx, taskID)
    if err != nil {
        return err
    }

    domainStats, err := o.flowRepo.GetDomainStatsByTaskID(ctx, taskID)
    if err != nil {
        return err
    }

    // 构建汇总 JSON
    summary := map[string]interface{}{
        "urls":               urls,
        "domains":            domainStats,
        "total_count":        len(urls),
        "unique_domain_count": len(domainStats),
        "captured_at":        time.Now().Format(time.RFC3339),
    }

    summaryJSON, _ := json.Marshal(summary)

    // 更新 url_analysis_dynamic 字段
    return o.taskRepo.UpdateDynamicURLSummary(ctx, taskID, string(summaryJSON))
}
```

#### 5.3.4 修改 `internal/domainanalysis/service.go`

```go
// extractDynamicURLs 从多个来源提取动态 URL（按优先级）
func (s *AnalysisService) extractDynamicURLs(task *appDomain.Task) []string {
    urlSet := make(map[string]bool)
    urls := []string{}

    // 优先级 1: 从 url_analysis_dynamic 字段读取
    if task.DomainAnalysis != nil && task.DomainAnalysis.URLAnalysisDynamic != "" {
        var summary map[string]interface{}
        if err := json.Unmarshal([]byte(task.DomainAnalysis.URLAnalysisDynamic), &summary); err == nil {
            if urlList, ok := summary["urls"].([]interface{}); ok {
                for _, u := range urlList {
                    if urlStr, ok := u.(string); ok && urlStr != "" && !urlSet[urlStr] {
                        urlSet[urlStr] = true
                        urls = append(urls, urlStr)
                    }
                }
            }
        }
        if len(urls) > 0 {
            s.logger.WithFields(logrus.Fields{
                "task_id": task.ID,
                "count":   len(urls),
                "source":  "url_analysis_dynamic",
            }).Info("Loaded dynamic URLs from summary field")
            return urls
        }
    }

    // 优先级 2: 从 task_flows 表读取
    if flowURLs, err := s.flowRepo.GetUniqueURLsByTaskID(context.Background(), task.ID); err == nil && len(flowURLs) > 0 {
        for _, u := range flowURLs {
            if !urlSet[u] {
                urlSet[u] = true
                urls = append(urls, u)
            }
        }
        s.logger.WithFields(logrus.Fields{
            "task_id": task.ID,
            "count":   len(urls),
            "source":  "task_flows_table",
        }).Info("Loaded dynamic URLs from task_flows table")
        return urls
    }

    // 优先级 3: 从 activity_details_json 读取（现有逻辑）
    // ...

    // 优先级 4: 从 flows.jsonl 文件读取（现有逻辑）
    // ...

    return urls
}
```

---

## 6. 迁移计划

### 6.1 数据库迁移

```sql
-- 1. 创建新表
CREATE TABLE task_flows (...);

-- 2. 为现有任务补充数据（可选，从 flows.jsonl 文件导入）
-- 此步骤可以通过迁移工具完成
```

### 6.2 代码部署步骤

1. **合并代码**：将新增和修改的代码合并到主分支
2. **数据库迁移**：执行建表 SQL
3. **重启服务**：重启 apk-analysis-server
4. **验证**：提交新任务，验证流量是否正确存储

### 6.3 兼容性说明

- 现有的 `flows.jsonl` 文件写入逻辑**保持不变**
- 新增数据库存储是**增量功能**，不影响现有流程
- 域名分析会**优先使用数据库数据**，兜底使用文件

---

## 7. 性能考虑

### 7.1 写入性能

- 使用**批量插入** (`CreateInBatches`)，每批 100 条
- 异步写入（可选），不阻塞主流程
- 写入失败只记录警告，不影响任务执行

### 7.2 查询性能

- 添加了 `task_id`、`host`、`task_host` 复合索引
- 汇总数据存储在 `url_analysis_dynamic`，避免每次查询全表

### 7.3 存储空间

- 预估每个任务 50-200 条流量记录
- 每条记录约 500 字节
- 每个任务约 25-100 KB

---

## 8. 测试要点

### 8.1 功能测试

- [ ] 流量正确写入 `task_flows` 表
- [ ] `url_analysis_dynamic` 字段正确汇总
- [ ] 域名分析能正确读取动态 URL
- [ ] 主域名识别结果正确

### 8.2 边界测试

- [ ] 无动态流量时的处理
- [ ] 大量流量（1000+）时的性能
- [ ] 数据库写入失败时的降级处理

### 8.3 兼容性测试

- [ ] 现有任务的域名分析不受影响
- [ ] `flows.jsonl` 文件仍能正常生成
- [ ] API 返回数据格式不变

---

## 9. 参考资料

- 现有代码：`internal/domainanalysis/service.go` - `extractDynamicURLs()`
- 现有代码：`internal/worker/orchestrator.go` - `appendFlowsToFile()`
- 数据库表：`task_domain_analysis` - `url_analysis_dynamic` 字段
