# APK 分析平台 Go 版本 - API 端点文档

> **版本**: v1.0.0
> **基础 URL**: `http://localhost:8080/api`
> **更新时间**: 2025-11-05

---

## 📋 目录

- [系统监控](#系统监控)
- [任务管理](#任务管理)
- [流量分析](#流量分析)
- [文件服务](#文件服务)
- [MobSF 报告](#mobsf-报告)
- [SDK 规则管理](#sdk-规则管理)

---

## 系统监控

### 健康检查
**GET** `/api/health`

**响应示例**:
```json
{
  "status": "ok",
  "version": "1.0.0"
}
```

### 系统统计
**GET** `/api/stats`

**响应示例**:
```json
{
  "total_tasks": 150,
  "status_breakdown": {
    "queued": 5,
    "installing": 2,
    "running": 3,
    "collecting": 1,
    "completed": 120,
    "failed": 15,
    "cancelled": 4
  }
}
```

---

## 任务管理

### 获取任务列表
**GET** `/api/tasks?limit=50`

**查询参数**:
- `limit` (可选): 返回任务数量, 默认 50

**响应示例**:
```json
[
  {
    "id": "c4d540c2-2ed9-49bf-8ec4-8ad595ae2142",
    "apk_name": "知乎.apk",
    "package_name": "com.zhihu.android",
    "status": "completed",
    "created_at": "2025-11-03T08:30:15.123456",
    "created_at_cst": "2025/11/03 16:30:15",
    "started_at": "2025-11-03T08:30:30.456789",
    "completed_at": "2025-11-03T08:42:00.789012",
    "completed_at_cst": "2025/11/03 16:42:00",
    "current_step": "任务完成",
    "progress_percent": 100,
    "error_message": null,
    "should_stop": false,
    "launcher_activity": "com.zhihu.android/.app.ui.activity.MainActivity",
    "activities": "[\"com.zhihu...MainActivity\", \"...\"]",
    "mobsf_status": "completed",
    "mobsf_score": 72,
    "primary_domain": "{\"primary_domain\": \"zhihu.com\", \"confidence\": 0.95}",
    "domain_beian_status": "[{\"domain\":\"zhihu.com\",\"beian_info\":{\"status\":\"registered\"}}]"
  }
]
```

### 获取单个任务详情
**GET** `/api/tasks/:id`

**路径参数**:
- `id`: 任务 ID (UUID)

**响应**: 同任务列表单项

**错误响应**:
```json
{
  "error": "任务不存在"
}
```
状态码: 404

### 删除任务
**DELETE** `/api/tasks/:id`

**路径参数**:
- `id`: 任务 ID (UUID)

**响应示例**:
```json
{
  "success": true,
  "message": "任务删除成功"
}
```

**错误响应**:
```json
{
  "error": "删除任务失败"
}
```
状态码: 500

### 停止任务
**POST** `/api/tasks/:id/stop`

**路径参数**:
- `id`: 任务 ID (UUID)

**响应示例**:
```json
{
  "success": true,
  "message": "任务已标记为停止"
}
```

**说明**: 任务不会立即停止, 而是在完成当前 Activity 后停止

---

## 流量分析

### 获取任务的所有 URL
**GET** `/api/tasks/:id/urls`

**路径参数**:
- `id`: 任务 ID (UUID)

**响应示例**:
```json
[
  {
    "url": "https://api.zhihu.com/v4/me",
    "host": "api.zhihu.com",
    "path": "/v4/me",
    "method": "GET"
  },
  {
    "url": "https://www.zhihu.com/api/v4/columns/",
    "host": "www.zhihu.com",
    "path": "/api/v4/columns/",
    "method": "POST"
  }
]
```

### 获取特定 Activity 的 URL
**GET** `/api/tasks/:id/activities/:name/urls`

**路径参数**:
- `id`: 任务 ID (UUID)
- `name`: Activity 名称 (URL 编码)

**响应示例**:
```json
[
  {
    "url": "https://api.zhihu.com/v4/me",
    "host": "api.zhihu.com",
    "path": "/v4/me",
    "method": "GET"
  }
]
```

---

## 文件服务

### 获取截图
**GET** `/api/tasks/:id/screenshot/:filename`

**路径参数**:
- `id`: 任务 ID (UUID)
- `filename`: 截图文件名 (如 `001_MainActivity_initial.png`)

**响应**: PNG 图片文件

**状态码**:
- 200: 成功
- 404: 文件不存在

### 列出所有截图
**GET** `/api/tasks/:id/screenshots`

**路径参数**:
- `id`: 任务 ID (UUID)

**响应示例**:
```json
[
  "001_MainActivity_initial.png",
  "001_MainActivity_after.png",
  "002_LoginActivity_initial.png"
]
```

### 获取 UI 层级 XML (解析后)
**GET** `/api/tasks/:id/ui_hierarchy/:filename`

**路径参数**:
- `id`: 任务 ID (UUID)
- `filename`: UI 层级文件名 (如 `001_MainActivity.xml`)

**响应示例**:
```json
{
  "rotation": 0,
  "root": {
    "index": 0,
    "text": "",
    "resource_id": "",
    "class": "android.widget.FrameLayout",
    "package": "com.zhihu.android",
    "bounds": "[0,0][1080,2340]",
    "clickable": false,
    "enabled": true,
    "children": [
      {
        "index": 0,
        "text": "登录",
        "resource_id": "com.zhihu.android:id/btn_login",
        "class": "android.widget.Button",
        "bounds": "[100,800][980,950]",
        "clickable": true,
        "enabled": true
      }
    ]
  }
}
```

### 下载流量数据
**GET** `/api/tasks/:id/flows`

**路径参数**:
- `id`: 任务 ID (UUID)

**响应**: JSONL 文件下载

**Content-Type**: `application/jsonl`

**Content-Disposition**: `attachment; filename=flows_{task_id_prefix}.jsonl`

---

## MobSF 报告

### 获取 MobSF 分析报告
**GET** `/api/tasks/:id/mobsf`

**路径参数**:
- `id`: 任务 ID (UUID)

**响应示例**:
```json
{
  "task_id": "c4d540c2-2ed9-49bf-8ec4-8ad595ae2142",
  "status": "completed",
  "hash": "abc123def456...",
  "score": 72,
  "app_name": "知乎",
  "scanned_at": "2025-11-03T08:35:00",
  "report": "{...完整 MobSF JSON 报告...}"
}
```

**错误响应**:
```json
{
  "error": "MobSF 报告不存在"
}
```
状态码: 404

---

## SDK 规则管理

### 获取 SDK 规则列表
**GET** `/api/sdk_rules?page=1&limit=50&category=ad&status=active&search=keyword`

**查询参数**:
- `page` (可选): 页码, 默认 1
- `limit` (可选): 每页数量, 默认 50
- `category` (可选): 分类过滤 (ad/analytics/push/payment/social/cdn/cloud/other)
- `status` (可选): 状态过滤 (active/pending/disabled)
- `search` (可选): 搜索关键词

**响应示例**:
```json
{
  "rules": [
    {
      "id": 1,
      "domain": "doubleclick.net",
      "category": "ad",
      "provider": "Google Ads",
      "description": "Google 广告服务",
      "source": "builtin",
      "confidence": 1.00,
      "status": "active",
      "discover_count": 152,
      "priority": 90,
      "created_at": "2025-10-01T00:00:00"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 1,
    "pages": 1
  }
}
```

### 获取待审核规则
**GET** `/api/sdk_rules/pending`

**响应示例**:
```json
{
  "rules": [],
  "total": 0
}
```

### 创建 SDK 规则
**POST** `/api/sdk_rules`

**请求体**:
```json
{
  "domain": "example.com",
  "category": "ad",
  "provider": "Example Provider",
  "description": "示例 SDK",
  "confidence": 0.95
}
```

**必填字段**:
- `domain`: 域名
- `category`: 分类

**响应示例**:
```json
{
  "success": true,
  "message": "SDK 规则创建成功"
}
```

**错误响应**:
```json
{
  "error": "参数错误: domain is required"
}
```
状态码: 400

### 更新 SDK 规则
**PUT** `/api/sdk_rules/:id`

**路径参数**:
- `id`: 规则 ID

**请求体**:
```json
{
  "category": "analytics",
  "provider": "Updated Provider",
  "description": "更新后的描述",
  "confidence": 0.98,
  "status": "active"
}
```

**响应示例**:
```json
{
  "success": true,
  "message": "SDK 规则更新成功"
}
```

### 审核通过 SDK 规则
**POST** `/api/sdk_rules/:id/approve`

**路径参数**:
- `id`: 规则 ID

**响应示例**:
```json
{
  "success": true,
  "message": "SDK 规则已审核通过"
}
```

### 拒绝 SDK 规则
**POST** `/api/sdk_rules/:id/reject`

**路径参数**:
- `id`: 规则 ID

**响应示例**:
```json
{
  "success": true,
  "message": "SDK 规则已拒绝"
}
```

### 删除 SDK 规则
**DELETE** `/api/sdk_rules/:id`

**路径参数**:
- `id`: 规则 ID

**响应示例**:
```json
{
  "success": true,
  "message": "SDK 规则删除成功"
}
```

### 获取 SDK 统计信息
**GET** `/api/sdk_rules/statistics`

**响应示例**:
```json
{
  "total_rules": 0,
  "active_rules": 0,
  "pending_rules": 0,
  "by_category": {
    "ad": 0,
    "analytics": 0,
    "push": 0,
    "payment": 0,
    "social": 0,
    "cdn": 0,
    "cloud": 0,
    "other": 0
  },
  "by_source": {
    "builtin": 0,
    "discovered": 0,
    "manual": 0
  }
}
```

### 获取 SDK 分类列表
**GET** `/api/sdk_rules/categories`

**响应示例**:
```json
[
  {"value": "ad", "label": "广告", "color": "#f44336"},
  {"value": "analytics", "label": "统计分析", "color": "#2196f3"},
  {"value": "push", "label": "消息推送", "color": "#4caf50"},
  {"value": "payment", "label": "支付", "color": "#ff9800"},
  {"value": "social", "label": "社交分享", "color": "#9c27b0"},
  {"value": "cdn", "label": "CDN", "color": "#00bcd4"},
  {"value": "cloud", "label": "云服务", "color": "#607d8b"},
  {"value": "other", "label": "其他", "color": "#9e9e9e"}
]
```

---

## HTTP 状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 成功 |
| 201 | 创建成功 |
| 204 | 成功 (无内容) |
| 400 | 请求参数错误 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

---

## 通用响应格式

### 成功响应
```json
{
  "success": true,
  "message": "操作成功",
  "data": {...}
}
```

### 错误响应
```json
{
  "error": "错误描述"
}
```

---

## 时间格式

- **数据库存储**: UTC ISO 8601 格式 (`2025-11-03T08:30:15.123456`)
- **API 响应**: 同时返回 UTC 和 CST 格式
  - `created_at`: UTC 时间
  - `created_at_cst`: CST 时间 (`2025/11/03 16:30:15`)

---

## CORS 支持

所有 API 端点支持跨域请求:
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Authorization`

---

## 实现状态

### ✅ 已实现 (20+ 端点)

**系统监控** (2):
- `/api/health` - 健康检查
- `/api/stats` - 系统统计

**任务管理** (4):
- `/api/tasks` - 任务列表
- `/api/tasks/:id` - 任务详情
- `/api/tasks/:id` (DELETE) - 删除任务
- `/api/tasks/:id/stop` - 停止任务

**流量分析** (2):
- `/api/tasks/:id/urls` - 任务 URL
- `/api/tasks/:id/activities/:name/urls` - Activity URL

**文件服务** (4):
- `/api/tasks/:id/screenshot/:filename` - 获取截图
- `/api/tasks/:id/screenshots` - 列出截图
- `/api/tasks/:id/ui_hierarchy/:filename` - UI 层级
- `/api/tasks/:id/flows` - 流量数据下载

**MobSF 报告** (1):
- `/api/tasks/:id/mobsf` - MobSF 报告

**SDK 规则** (9):
- `/api/sdk_rules` (GET) - 规则列表
- `/api/sdk_rules` (POST) - 创建规则
- `/api/sdk_rules/:id` (PUT) - 更新规则
- `/api/sdk_rules/:id` (DELETE) - 删除规则
- `/api/sdk_rules/pending` - 待审核
- `/api/sdk_rules/statistics` - 统计信息
- `/api/sdk_rules/categories` - 分类列表
- `/api/sdk_rules/:id/approve` - 审核通过
- `/api/sdk_rules/:id/reject` - 拒绝

### 🔲 待实现 (需要完整业务逻辑)

- JSON 解析逻辑 (ActivityDetailsJSON → URLs 提取)
- SDK 规则 Repository 实现
- Activity/MobSF/Domain Repository 实现

---

**最后更新**: 2025-11-05
**维护者**: APK Analysis Team
