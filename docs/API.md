# APK Analysis Platform - API 文档

> **版本**: 1.0.0
> **基础 URL**: `http://localhost:8080/api`
> **协议**: HTTP/HTTPS
> **数据格式**: JSON

---

## 📋 目录

- [认证](#认证)
- [通用响应格式](#通用响应格式)
- [错误码](#错误码)
- [API 端点](#api-端点)
  - [任务管理](#任务管理)
  - [系统统计](#系统统计)
  - [健康检查](#健康检查)
- [数据模型](#数据模型)
- [示例代码](#示例代码)

---

## 认证

当前版本暂不需要认证。

**计划支持** (未来版本):
- Bearer Token 认证
- API Key 认证
- OAuth 2.0

---

## 通用响应格式

### 成功响应

```json
{
  "data": { ... },
  "timestamp": "2025-11-05T12:00:00Z"
}
```

### 错误响应

```json
{
  "error": "错误信息描述",
  "code": 404,
  "timestamp": "2025-11-05T12:00:00Z"
}
```

---

## 错误码

| HTTP 状态码 | 说明 | 示例 |
|-----------|------|------|
| **200** | 成功 | 请求成功处理 |
| **201** | 创建成功 | 资源创建成功 |
| **400** | 请求参数错误 | 缺少必需参数 |
| **404** | 资源不存在 | 任务 ID 不存在 |
| **500** | 服务器内部错误 | 数据库连接失败 |
| **503** | 服务不可用 | 服务维护中 |

---

## API 端点

### 任务管理

#### 1. 获取任务列表

**端点**: `GET /api/tasks`

**描述**: 获取最近的任务列表

**查询参数**:

| 参数 | 类型 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| `limit` | integer | 否 | 50 | 返回数量限制 (1-100) |
| `status` | string | 否 | - | 按状态过滤 (queued/running/completed/failed/cancelled) |

**请求示例**:

```bash
# 获取最近 50 个任务
curl http://localhost:8080/api/tasks

# 获取最近 10 个任务
curl http://localhost:8080/api/tasks?limit=10

# 获取所有正在运行的任务
curl http://localhost:8080/api/tasks?status=running
```

**响应示例**:

```json
[
  {
    "id": "c4d540c2-2ed9-49bf-8ec4-8ad595ae2142",
    "apk_name": "zhihu.apk",
    "package_name": "com.zhihu.android",
    "status": "completed",
    "created_at": "2025-11-05T08:30:15.123456Z",
    "started_at": "2025-11-05T08:30:30.456789Z",
    "completed_at": "2025-11-05T08:42:00.789012Z",
    "current_step": "任务完成",
    "progress_percent": 100,
    "launcher_activity": "com.zhihu.android/.app.ui.activity.MainActivity",
    "activities": ["com.zhihu.android.MainActivity", "com.zhihu.android.LoginActivity"],
    "mobsf_status": "completed",
    "mobsf_score": 72
  }
]
```

---

#### 2. 获取任务详情

**端点**: `GET /api/tasks/{id}`

**描述**: 根据任务 ID 获取完整的任务信息

**路径参数**:

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `id` | string (UUID) | 是 | 任务唯一标识符 |

**请求示例**:

```bash
curl http://localhost:8080/api/tasks/c4d540c2-2ed9-49bf-8ec4-8ad595ae2142
```

**响应示例**:

```json
{
  "id": "c4d540c2-2ed9-49bf-8ec4-8ad595ae2142",
  "apk_name": "zhihu.apk",
  "package_name": "com.zhihu.android",
  "status": "completed",
  "created_at": "2025-11-05T08:30:15.123456Z",
  "started_at": "2025-11-05T08:30:30.456789Z",
  "completed_at": "2025-11-05T08:42:00.789012Z",
  "current_step": "任务完成",
  "progress_percent": 100,
  "error_message": null,
  "device_connected": true,
  "launcher_activity": "com.zhihu.android/.app.ui.activity.MainActivity",
  "activities": [
    "com.zhihu.android.MainActivity",
    "com.zhihu.android.LoginActivity",
    "com.zhihu.android.ProfileActivity"
  ],
  "mobsf_status": "completed",
  "mobsf_score": 72
}
```

**错误响应** (404):

```json
{
  "error": "任务不存在",
  "code": 404
}
```

---

#### 3. 删除任务

**端点**: `DELETE /api/tasks/{id}`

**描述**: 删除指定任务及其相关数据

**路径参数**:

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `id` | string (UUID) | 是 | 任务唯一标识符 |

**请求示例**:

```bash
curl -X DELETE http://localhost:8080/api/tasks/c4d540c2-2ed9-49bf-8ec4-8ad595ae2142
```

**响应示例** (200):

```json
{
  "message": "任务已成功删除"
}
```

**错误响应** (404):

```json
{
  "error": "任务不存在",
  "code": 404
}
```

---

#### 4. 停止任务

**端点**: `POST /api/tasks/{id}/stop`

**描述**: 停止正在运行的任务

**路径参数**:

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `id` | string (UUID) | 是 | 任务唯一标识符 |

**请求示例**:

```bash
curl -X POST http://localhost:8080/api/tasks/c4d540c2-2ed9-49bf-8ec4-8ad595ae2142/stop
```

**响应示例** (200):

```json
{
  "message": "任务已停止"
}
```

**错误响应** (400):

```json
{
  "error": "任务不在运行状态",
  "code": 400
}
```

---

### 系统统计

#### 5. 获取系统统计

**端点**: `GET /api/stats`

**描述**: 获取系统整体统计信息，包括各状态任务数量

**请求示例**:

```bash
curl http://localhost:8080/api/stats
```

**响应示例**:

```json
{
  "total_tasks": 150,
  "queued": 5,
  "running": 2,
  "completed": 138,
  "failed": 5,
  "cancelled": 0
}
```

---

### 健康检查

#### 6. 健康检查

**端点**: `GET /api/health`

**描述**: 检查服务健康状态

**请求示例**:

```bash
curl http://localhost:8080/api/health
```

**响应示例** (200):

```json
{
  "status": "ok",
  "timestamp": "2025-11-05T12:00:00Z",
  "components": {
    "database": "ok",
    "rabbitmq": "ok",
    "redis": "ok"
  }
}
```

**错误响应** (503):

```json
{
  "status": "degraded",
  "timestamp": "2025-11-05T12:00:00Z",
  "components": {
    "database": "ok",
    "rabbitmq": "down",
    "redis": "ok"
  }
}
```

---

## 数据模型

### Task (任务)

```typescript
interface Task {
  id: string;                    // UUID v4
  apk_name: string;              // APK 文件名
  package_name: string;          // 应用包名
  status: TaskStatus;            // 任务状态
  created_at: string;            // 创建时间 (ISO 8601)
  started_at: string | null;     // 开始时间
  completed_at: string | null;   // 完成时间
  current_step: string;          // 当前执行步骤
  progress_percent: number;      // 进度百分比 (0-100)
  error_message: string | null;  // 错误信息
  device_connected: boolean;     // 设备连接状态
  launcher_activity: string;     // 主 Activity
  activities: string[];          // Activity 列表
  mobsf_status: string;          // MobSF 状态
  mobsf_score: number;           // MobSF 评分 (0-100)
}
```

### TaskStatus (任务状态)

```typescript
enum TaskStatus {
  Queued = "queued",           // 已入队
  Installing = "installing",   // 安装中
  Running = "running",         // 运行中
  Collecting = "collecting",   // 收集数据中
  Completed = "completed",     // 已完成
  Failed = "failed",           // 失败
  Cancelled = "cancelled",     // 已取消
}
```

### Stats (统计信息)

```typescript
interface Stats {
  total_tasks: number;    // 总任务数
  queued: number;         // 排队中
  running: number;        // 运行中
  completed: number;      // 已完成
  failed: number;         // 失败
  cancelled: number;      // 已取消
}
```

---

## 示例代码

### JavaScript (fetch)

```javascript
// 获取任务列表
async function getTasks() {
  const response = await fetch('http://localhost:8080/api/tasks?limit=10');
  const tasks = await response.json();
  console.log(tasks);
}

// 获取任务详情
async function getTask(taskId) {
  const response = await fetch(`http://localhost:8080/api/tasks/${taskId}`);
  const task = await response.json();
  console.log(task);
}

// 停止任务
async function stopTask(taskId) {
  const response = await fetch(`http://localhost:8080/api/tasks/${taskId}/stop`, {
    method: 'POST'
  });
  const result = await response.json();
  console.log(result);
}

// 删除任务
async function deleteTask(taskId) {
  const response = await fetch(`http://localhost:8080/api/tasks/${taskId}`, {
    method: 'DELETE'
  });
  const result = await response.json();
  console.log(result);
}
```

---

### Python (requests)

```python
import requests

BASE_URL = "http://localhost:8080/api"

# 获取任务列表
def get_tasks(limit=50):
    response = requests.get(f"{BASE_URL}/tasks", params={"limit": limit})
    return response.json()

# 获取任务详情
def get_task(task_id):
    response = requests.get(f"{BASE_URL}/tasks/{task_id}")
    return response.json()

# 停止任务
def stop_task(task_id):
    response = requests.post(f"{BASE_URL}/tasks/{task_id}/stop")
    return response.json()

# 删除任务
def delete_task(task_id):
    response = requests.delete(f"{BASE_URL}/tasks/{task_id}")
    return response.json()

# 获取系统统计
def get_stats():
    response = requests.get(f"{BASE_URL}/stats")
    return response.json()

# 示例使用
if __name__ == "__main__":
    tasks = get_tasks(limit=10)
    print(f"获取到 {len(tasks)} 个任务")

    if tasks:
        task_id = tasks[0]["id"]
        task = get_task(task_id)
        print(f"任务详情: {task}")
```

---

### cURL

```bash
#!/bin/bash

BASE_URL="http://localhost:8080/api"

# 获取任务列表
curl -X GET "$BASE_URL/tasks?limit=10"

# 获取任务详情
TASK_ID="c4d540c2-2ed9-49bf-8ec4-8ad595ae2142"
curl -X GET "$BASE_URL/tasks/$TASK_ID"

# 停止任务
curl -X POST "$BASE_URL/tasks/$TASK_ID/stop"

# 删除任务
curl -X DELETE "$BASE_URL/tasks/$TASK_ID"

# 获取系统统计
curl -X GET "$BASE_URL/stats"

# 健康检查
curl -X GET "$BASE_URL/health"
```

---

### Go (标准库)

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

const baseURL = "http://localhost:8080/api"

type Task struct {
    ID              string   `json:"id"`
    APKName         string   `json:"apk_name"`
    PackageName     string   `json:"package_name"`
    Status          string   `json:"status"`
    CreatedAt       string   `json:"created_at"`
    ProgressPercent int      `json:"progress_percent"`
    CurrentStep     string   `json:"current_step"`
}

// 获取任务列表
func getTasks(limit int) ([]Task, error) {
    url := fmt.Sprintf("%s/tasks?limit=%d", baseURL, limit)
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var tasks []Task
    if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
        return nil, err
    }
    return tasks, nil
}

// 获取任务详情
func getTask(taskID string) (*Task, error) {
    url := fmt.Sprintf("%s/tasks/%s", baseURL, taskID)
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var task Task
    if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
        return nil, err
    }
    return &task, nil
}

func main() {
    tasks, err := getTasks(10)
    if err != nil {
        panic(err)
    }
    fmt.Printf("获取到 %d 个任务\n", len(tasks))

    if len(tasks) > 0 {
        task, _ := getTask(tasks[0].ID)
        fmt.Printf("任务详情: %+v\n", task)
    }
}
```

---

## 限流策略 (计划)

未来版本将实施以下限流策略:

| 端点 | 限制 | 窗口 |
|------|------|------|
| `GET /api/tasks` | 100 请求 | 1 分钟 |
| `GET /api/tasks/{id}` | 200 请求 | 1 分钟 |
| `DELETE /api/tasks/{id}` | 10 请求 | 1 分钟 |
| `POST /api/tasks/{id}/stop` | 20 请求 | 1 分钟 |

---

## Webhook (计划)

未来版本将支持 Webhook 通知:

**事件类型**:
- `task.created` - 任务创建
- `task.started` - 任务开始
- `task.completed` - 任务完成
- `task.failed` - 任务失败

**Webhook 请求格式**:
```json
{
  "event": "task.completed",
  "task_id": "c4d540c2-2ed9-49bf-8ec4-8ad595ae2142",
  "timestamp": "2025-11-05T12:00:00Z",
  "data": {
    "status": "completed",
    "progress_percent": 100
  }
}
```

---

## 版本历史

| 版本 | 发布日期 | 变更说明 |
|------|---------|---------|
| 1.0.0 | 2025-11-05 | 初始版本 |

---

## 联系支持

- **在线文档**: https://docs.apk-analysis.com
- **问题反馈**: https://github.com/your-org/apk-analysis-go/issues
- **Email**: support@apk-analysis.com

---

**最后更新**: 2025-11-05
**维护者**: APK Analysis Team
