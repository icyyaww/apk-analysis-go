# AI智能交互集成文档

## 📚 目录
1. [功能概述](#功能概述)
2. [架构设计](#架构设计)
3. [模块说明](#模块说明)
4. [集成方案](#集成方案)
5. [配置说明](#配置说明)
6. [使用示例](#使用示例)
7. [最佳实践](#最佳实践)
8. [故障排查](#故障排查)

---

## 功能概述

### 🎯 核心功能

将Python项目(`apk-analysis-mvp`)中的AI智能交互逻辑移植到Go项目，实现：

1. **AI驱动的UI分析**: 使用智谱GLM-4-Flash模型分析Android UI元素
2. **智能交互策略**: AI生成高质量的点击、输入、滚动动作
3. **优先级驱动执行**: 按优先级执行动作(16级优先级系统)
4. **多层防护机制**: 确保不执行危险操作(返回/退出/拒绝)
5. **降级策略**: AI失败时使用规则引擎兜底
6. **智能点击工具**: 通过文本识别点击按钮、自动同意隐私政策

### ✨ 主要特性

- **纯文本模式**: 使用UI XML而非截图，成本更低速度更快
- **动态操作数**: 根据UI复杂度自动调整交互次数(3-20个)
- **上下文感知**: 识别登录页、权限弹窗、主Activity等场景
- **流量归因**: 实时监控每个动作触发的网络请求
- **禁止关键词过滤**: 三层防护机制避免退出APP

---

## 架构设计

### 📦 模块结构

```
internal/ai/
├── client.go              # AI客户端 (智谱GLM-4-Flash API)
├── analyzer.go            # UI分析器 (已有)
├── ui_parser.go           # UI XML解析器 (新增)
├── interaction_engine.go  # 交互引擎 (新增)
├── smart_click.go         # 智能点击工具 (新增)
└── README.md              # 模块文档
```

### 🔄 交互流程

```
Activity启动
    ↓
提取UI Hierarchy (XML)
    ↓
解析UI元素 (UIParser)
    ↓
AI分析生成策略 (InteractionEngine)
    ↓
动态循环执行动作
    ├─ 执行动作 (点击/输入/滚动)
    ├─ 监控流量归因
    ├─ 截图记录状态
    ├─ 重新dump UI
    └─ 重新分析 → 继续循环
    ↓
返回Activity详情 + AI交互结果
```

### 🏗️ 数据流

```
executeActivity()
    ↓
1. adbClient.StartActivity(component)
2. adbClient.DumpUIHierarchy(xmlPath)
3. uiData := ai.ParseUIXML(xmlPath)
4. actions := interactionEngine.PlanActions(uiData, activity, category)
5. FOR each action:
      - interactionEngine.ExecuteAction(action, adbClient)
      - adbClient.Screenshot(screenshotPath)
      - attributor.AttributeFlows(startTime, endTime)
      - adbClient.DumpUIHierarchy(nextXmlPath)  # 重新分析
6. RETURN activityDetail
```

---

## 模块说明

### 1. UI元素解析器 (`ui_parser.go`)

#### 数据结构

```go
// UIElement UI元素
type UIElement struct {
    Text       string   `json:"text"`
    ResourceID string   `json:"resource_id"`
    Class      string   `json:"class"`
    Bounds     [4]int   `json:"bounds"`     // [left, top, right, bottom]
    Center     [2]int   `json:"center"`     // [x, y]
    Clickable  bool     `json:"clickable"`
    Scrollable bool     `json:"scrollable"`
    Label      string   `json:"label,omitempty"` // Switch/CheckBox的关联标签
}

// UIData UI数据
type UIData struct {
    ClickableElements []UIElement `json:"clickable_elements"`
    InputFields       []UIElement `json:"input_fields"`
    ScrollableViews   []UIElement `json:"scrollable_views"`
}
```

#### 核心功能

```go
// 解析UI XML文件
uiData, err := ai.ParseUIXML("/path/to/ui_hierarchy.xml")

// 计算最大操作次数 (根据UI复杂度)
maxActions := ai.CalculateMaxActions(uiData)
```

**智能标签关联**: 自动为Switch/CheckBox查找左侧文本标签
**动态操作数计算**: 5个元素以下全点，5-15个点80%，15+个点60%(最多20个)

---

### 2. 交互引擎 (`interaction_engine.go`)

#### 核心API

```go
// 创建交互引擎
engine := ai.NewInteractionEngine(apiKey, logger)

// 分析UI并生成交互策略
actions, err := engine.PlanActions(ctx, uiData, "MainActivity", "社交应用")

// 执行单个动作
err := engine.ExecuteAction(ctx, action, adbClient)
```

#### Action数据结构

```go
type Action struct {
    Type      string `json:"type"`      // click, input, scroll
    X         int    `json:"x,omitempty"`
    Y         int    `json:"y,omitempty"`
    Value     string `json:"value,omitempty"`     // input值
    Direction string `json:"direction,omitempty"` // scroll方向
    Reason    string `json:"reason"`
    Priority  int    `json:"priority"` // 1-16优先级
}
```

#### 优先级体系

| 优先级 | 场景 | 示例 |
|-------|------|------|
| 16 | 系统"Open"按钮 | App Info页面的"Open"按钮 |
| 15 | 权限同意 | 同意/允许/确定/Accept/Allow |
| 14 | 跳过登录 | 跳过/游客模式/试用/体验 |
| 10-12 | 高价值操作 | 搜索/刷新/分享/查看详情 |
| 8 | 滚动浏览 | 向下滚动加载更多内容 |
| 0 | 禁止操作 | 返回/退出/拒绝/取消 |

#### 禁止关键词列表

```go
forbiddenKeywords := []string{
    "返回", "back", "关闭", "close",
    "退出", "exit", "quit",
    "拒绝", "deny", "refuse", "不同意", "disagree",
    "禁止", "forbid", "否", "no", "取消", "cancel",
    "退出登录", "sign out", "logout",
}
```

---

### 3. 智能点击工具 (`smart_click.go`)

#### 核心API

```go
// 创建智能点击器
clicker := ai.NewSmartClicker(logger)

// 通过文本查找并点击按钮
success, err := clicker.ClickButtonByText(ctx, adbClient,
    []string{"同意", "确定", "OK"}, 3)

// 自动点击隐私政策同意按钮
success, err := clicker.AutoClickPrivacyAgreement(ctx, adbClient, 5)

// 点击指定坐标
err := clicker.ClickCoordinate(ctx, adbClient, 540, 1000)

// 滑动屏幕
err := clicker.SwipeScreen(ctx, adbClient, "down", 300)
```

#### 隐私政策自动同意策略

1. **策略1**: 先查找并勾选复选框（"我已阅读"等）
2. **策略2**: UI Automator文本查找点击（"同意"等）
3. **策略3**: 尝试常见坐标位置（经验值）

---

### 4. AI客户端 (`client.go`)

#### 新增方法

```go
// 分析纯文本提示词 (不需要图片)
response, err := aiClient.AnalyzeText(ctx, prompt)
```

**模型选择**: `glm-4-flash` (纯文本,更快更便宜,适合UI XML分析)

---

## 集成方案

### 方案1: 最小侵入式集成 (推荐)

在`orchestrator.go`的`executeActivity`方法中增强AI交互：

```go
// executeActivity 执行单个 Activity
func (o *Orchestrator) executeActivity(...) map[string]interface{} {
    // ... 现有代码 ...

    // 3. UI Hierarchy (所有 Activity)
    uiHierarchyFile := fmt.Sprintf("%03d_%s.xml", index+1, o.shortActivityName(activity))
    uiHierarchyPath := filepath.Join(uiHierarchyDir, uiHierarchyFile)
    if err := adbClient.DumpUIHierarchy(ctx, uiHierarchyPath); err != nil {
        o.logger.WithError(err).Warn("UI hierarchy dump failed")
    } else {
        detail["ui_hierarchy_file"] = uiHierarchyFile

        // ===== 新增: AI智能交互 =====
        if o.aiEnabled {
            aiResult := o.performAIInteraction(ctx, activity, uiHierarchyPath,
                screenshotDir, adbClient, startTime)
            if aiResult != nil {
                detail["ai_interaction"] = aiResult
            }
        }
    }

    // ... 现有代码 (保留performDeepExploration作为降级方案) ...
}

// performAIInteraction 执行AI智能交互
func (o *Orchestrator) performAIInteraction(
    ctx context.Context,
    activity string,
    uiXMLPath string,
    screenshotDir string,
    adbClient *adb.Client,
    activityStartTime time.Time,
) map[string]interface{} {
    result := map[string]interface{}{
        "success": false,
        "actions_executed": 0,
        "error": nil,
    }

    // 1. 解析UI XML
    uiData, err := ai.ParseUIXML(uiXMLPath)
    if err != nil {
        result["error"] = fmt.Sprintf("Failed to parse UI XML: %v", err)
        return result
    }

    // 2. 检查是否有可交互元素
    if len(uiData.ClickableElements) == 0 && len(uiData.InputFields) == 0 {
        result["error"] = "No interactive elements found"
        return result
    }

    // 3. 创建交互引擎
    apiKey := os.Getenv("GLM_API_KEY")
    if apiKey == "" {
        result["error"] = "GLM_API_KEY not set"
        return result
    }
    engine := ai.NewInteractionEngine(apiKey, o.logger)

    // 4. 生成交互策略
    actions, err := engine.PlanActions(ctx, uiData, activity, "通用应用")
    if err != nil {
        o.logger.WithError(err).Warn("Failed to plan actions, using fallback")
        return result
    }

    if len(actions) == 0 {
        result["error"] = "No actions generated"
        return result
    }

    // 5. 执行动作 (动态循环模式)
    detailedActions := []map[string]interface{}{}
    maxIterations := ai.CalculateMaxActions(uiData)

    for i := 0; i < maxIterations && i < len(actions); i++ {
        action := actions[i]

        o.logger.WithFields(logrus.Fields{
            "iteration": i + 1,
            "type":      action.Type,
            "priority":  action.Priority,
            "reason":    action.Reason,
        }).Info("Executing AI action")

        // 执行动作
        actionStart := time.Now()
        if err := engine.ExecuteAction(ctx, action, adbClient); err != nil {
            o.logger.WithError(err).Warn("Action execution failed")
            continue
        }

        // 等待UI稳定
        time.Sleep(2 * time.Second)

        // 截图
        screenshotFile := fmt.Sprintf("ai_action_%s_%d.png",
            o.shortActivityName(activity), i+1)
        screenshotPath := filepath.Join(screenshotDir, screenshotFile)
        if err := adbClient.Screenshot(ctx, screenshotPath); err != nil {
            o.logger.WithError(err).Warn("Screenshot failed")
        }

        // 记录动作详情
        actionDetail := map[string]interface{}{
            "type":     action.Type,
            "reason":   action.Reason,
            "priority": action.Priority,
            "screenshot": screenshotFile,
            "duration_ms": time.Since(actionStart).Milliseconds(),
        }

        if action.Type == "click" {
            actionDetail["x"] = action.X
            actionDetail["y"] = action.Y
        } else if action.Type == "input" {
            actionDetail["x"] = action.X
            actionDetail["y"] = action.Y
            actionDetail["value"] = action.Value
        } else if action.Type == "scroll" {
            actionDetail["direction"] = action.Direction
        }

        detailedActions = append(detailedActions, actionDetail)

        // 如果是最高优先级动作(同意/允许),执行后停止
        if action.Priority >= 15 {
            o.logger.Info("High-priority action completed, stopping interaction")
            break
        }

        // 重新dump UI (可选,用于动态重新分析)
        // ... 实现类似Python版本的动态循环 ...
    }

    result["success"] = true
    result["actions_executed"] = len(detailedActions)
    result["actions"] = detailedActions

    return result
}
```

### 方案2: 可选启用式集成

通过环境变量控制是否启用AI智能交互：

```go
// 在Orchestrator结构体中添加
type Orchestrator struct {
    // ... 现有字段 ...
    aiInteractionEnabled bool
    interactionEngine    *ai.InteractionEngine
    smartClicker         *ai.SmartClicker
}

// 在NewOrchestrator中初始化
func NewOrchestrator(...) *Orchestrator {
    // ... 现有代码 ...

    // AI智能交互初始化
    aiInteractionEnabled := os.Getenv("AI_INTERACTION_ENABLED") == "true"
    var interactionEngine *ai.InteractionEngine
    var smartClicker *ai.SmartClicker

    if aiInteractionEnabled {
        apiKey := os.Getenv("GLM_API_KEY")
        if apiKey != "" {
            interactionEngine = ai.NewInteractionEngine(apiKey, logger)
            smartClicker = ai.NewSmartClicker(logger)
            logger.Info("AI smart interaction enabled")
        } else {
            logger.Warn("AI_INTERACTION_ENABLED=true but GLM_API_KEY not set")
            aiInteractionEnabled = false
        }
    }

    return &Orchestrator{
        // ... 现有字段 ...
        aiInteractionEnabled: aiInteractionEnabled,
        interactionEngine:    interactionEngine,
        smartClicker:         smartClicker,
    }
}
```

### 方案3: 与现有performDeepExploration并存

保留现有的`performDeepExploration`作为降级方案：

```go
if o.aiInteractionEnabled && o.interactionEngine != nil {
    // 尝试AI智能交互
    aiResult := o.performAIInteraction(...)
    if aiResult != nil && aiResult["success"].(bool) {
        detail["ai_interaction"] = aiResult
        o.logger.Info("AI interaction completed successfully")
    } else {
        // AI失败,降级到传统深度探索
        o.logger.Warn("AI interaction failed, falling back to deep exploration")
        o.performDeepExploration(ctx, activity, adbClient)
    }
} else {
    // AI未启用,使用传统深度探索
    o.performDeepExploration(ctx, activity, adbClient)
}
```

---

## 配置说明

### 环境变量

```bash
# AI交互总开关
AI_INTERACTION_ENABLED=true

# 智谱AI API密钥 (必需)
GLM_API_KEY=your_zhipu_api_key_here

# 每个Activity最大操作次数 (可选,默认20)
AI_MAX_ACTIONS_PER_ACTIVITY=20

# AI模型选择 (可选,默认glm-4-flash)
GLM_MODEL=glm-4-flash

# AI API超时时间 (可选,默认60秒)
AI_API_TIMEOUT=60
```

### Docker Compose配置

```yaml
services:
  apk-analysis-server:
    environment:
      - AI_INTERACTION_ENABLED=true
      - GLM_API_KEY=${GLM_API_KEY}
      - AI_MAX_ACTIONS_PER_ACTIVITY=20
```

### 本地开发

```bash
# 设置环境变量
export GLM_API_KEY="your_api_key"
export AI_INTERACTION_ENABLED=true

# 运行服务
go run ./cmd/server
```

---

## 使用示例

### 示例1: 基本使用

```go
package main

import (
    "context"
    "github.com/apk-analysis/apk-analysis-go/internal/ai"
    "github.com/apk-analysis/apk-analysis-go/internal/adb"
    "github.com/sirupsen/logrus"
)

func main() {
    logger := logrus.New()
    ctx := context.Background()

    // 创建ADB客户端
    adbClient := adb.NewClient("android-emulator:5555", 30*time.Second, logger)

    // 创建交互引擎
    engine := ai.NewInteractionEngine("your_api_key", logger)

    // 解析UI XML
    uiData, _ := ai.ParseUIXML("/path/to/ui_hierarchy.xml")

    // 生成交互策略
    actions, _ := engine.PlanActions(ctx, uiData, "MainActivity", "社交应用")

    // 执行动作
    for _, action := range actions {
        engine.ExecuteAction(ctx, action, adbClient)
        time.Sleep(2 * time.Second)

        // 如果是最高优先级动作,停止
        if action.Priority >= 15 {
            break
        }
    }
}
```

### 示例2: 智能点击

```go
// 自动点击隐私政策
clicker := ai.NewSmartClicker(logger)
success, _ := clicker.AutoClickPrivacyAgreement(ctx, adbClient, 5)

if success {
    logger.Info("Privacy agreement accepted")
} else {
    logger.Warn("Failed to accept privacy agreement")
}
```

### 示例3: 完整流程

```go
// 1. 启动Activity
adbClient.StartActivity(ctx, "com.example.app/.MainActivity")
time.Sleep(3 * time.Second)

// 2. Dump UI Hierarchy
uiXMLPath := "/tmp/ui_hierarchy.xml"
adbClient.DumpUIHierarchy(ctx, uiXMLPath)

// 3. 解析UI元素
uiData, _ := ai.ParseUIXML(uiXMLPath)

// 4. 生成交互策略
engine := ai.NewInteractionEngine(apiKey, logger)
actions, _ := engine.PlanActions(ctx, uiData, "MainActivity", "通用应用")

// 5. 动态循环执行
for i := 0; i < len(actions); i++ {
    action := actions[i]

    // 执行动作
    engine.ExecuteAction(ctx, action, adbClient)
    time.Sleep(2 * time.Second)

    // 截图
    adbClient.Screenshot(ctx, fmt.Sprintf("/tmp/screenshot_%d.png", i))

    // 重新dump UI (动态重新分析)
    adbClient.DumpUIHierarchy(ctx, uiXMLPath)
    uiData, _ = ai.ParseUIXML(uiXMLPath)

    // 重新生成策略
    actions, _ = engine.PlanActions(ctx, uiData, "MainActivity", "通用应用")

    // 如果没有可交互元素或高优先级动作完成,停止
    if len(actions) == 0 || action.Priority >= 15 {
        break
    }
}
```

---

## 最佳实践

### 1. 错误处理

```go
// 优雅降级
aiResult := o.performAIInteraction(...)
if aiResult == nil || !aiResult["success"].(bool) {
    // 降级到传统深度探索
    o.performDeepExploration(ctx, activity, adbClient)
}
```

### 2. 日志记录

```go
o.logger.WithFields(logrus.Fields{
    "activity":   activity,
    "actions":    len(actions),
    "max_actions": maxActions,
}).Info("AI interaction plan generated")
```

### 3. 超时控制

```go
// 为AI调用设置超时
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()

actions, err := engine.PlanActions(ctx, uiData, activity, category)
```

### 4. 资源清理

```go
// 确保临时文件被清理
defer os.Remove(uiXMLPath)
defer os.Remove(screenshotPath)
```

### 5. 并发安全

```go
// ADB命令已由DeviceManager处理设备级锁
// AI调用本身是无状态的,多任务并发安全
```

---

## 故障排查

### 问题1: AI未生成任何动作

**可能原因**:
- UI XML解析失败
- 没有可交互元素
- AI API调用失败

**解决方法**:
```go
// 检查UI数据
if len(uiData.ClickableElements) == 0 {
    logger.Warn("No clickable elements found")
}

// 检查API密钥
if os.Getenv("GLM_API_KEY") == "" {
    logger.Error("GLM_API_KEY not set")
}

// 使用降级策略
actions := engine.fallbackStrategy(uiData, activity)
```

### 问题2: 动作执行失败

**可能原因**:
- 坐标超出屏幕范围
- UI已变化
- ADB命令执行失败

**解决方法**:
```go
// 验证坐标
if action.X < 0 || action.X > 1080 || action.Y < 0 || action.Y > 2340 {
    logger.Warn("Invalid coordinates")
    continue
}

// 捕获错误并继续
if err := engine.ExecuteAction(ctx, action, adbClient); err != nil {
    logger.WithError(err).Warn("Action failed, continuing")
    continue
}
```

### 问题3: AI返回禁止操作

**可能原因**:
- Prompt构建不当
- 响应过滤失败

**解决方法**:
```go
// 三层防护已实现
// 1. Prompt中明确禁止
// 2. 解析响应时过滤
// 3. 执行前验证坐标对应元素
```

### 问题4: 性能问题

**可能原因**:
- AI调用耗时长
- 动态循环次数过多

**解决方法**:
```go
// 限制最大操作次数
maxActions := ai.CalculateMaxActions(uiData)
if maxActions > 10 {
    maxActions = 10
}

// 设置超时
ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
defer cancel()
```

---

## 附录

### A. 优先级完整列表

| 优先级 | 场景 | 关键词 | 处理方式 |
|-------|------|--------|---------|
| 16 | 系统Open按钮 | open, 打开, launch, 启动 | 立即执行,执行后停止 |
| 15 | 权限同意 | 同意, 允许, 确定, accept, allow, agree, ok | 立即执行,执行后停止 |
| 14 | 跳过登录 | 跳过, 游客, 试用, skip, guest, trial | 立即执行,执行后停止 |
| 10-12 | 高价值操作 | 搜索, 刷新, 分享, search, refresh, share | 正常执行 |
| 8 | 滚动浏览 | 滚动, scroll | 正常执行 |
| 5 | 普通按钮 | 其他可点击元素 | 正常执行 |
| 0 | 禁止操作 | 返回, 退出, 拒绝, back, exit, deny | 跳过不执行 |

### B. Python vs Go实现对比

| 功能 | Python实现 | Go实现 | 状态 |
|------|-----------|--------|------|
| UI XML解析 | xml.etree.ElementTree | encoding/xml | ✅ 完成 |
| AI调用 | zhipu SDK | 原生HTTP | ✅ 完成 |
| 动作执行 | subprocess | adb.Client | ✅ 完成 |
| 动态循环 | while循环 + AI重新分析 | for循环 + 重新dump UI | ✅ 完成 |
| 智能点击 | regex + XML解析 | regex + 字符串匹配 | ✅ 完成 |
| 降级策略 | 规则引擎 | 规则引擎 | ✅ 完成 |

### C. API成本估算

**GLM-4-Flash定价** (纯文本模式):
- 输入: ¥0.001/1K tokens
- 输出: ¥0.001/1K tokens

**单个Activity估算**:
- Prompt: ~1500 tokens
- 响应: ~500 tokens
- 成本: ~¥0.002/Activity

**单个APK估算** (假设10个Activity):
- 总成本: ~¥0.02

**对比**:
- GLM-4V-Flash (带图片): ~¥0.1/Activity
- **节省成本**: 80%

---

## 更新日志

### v1.0.0 (2025-11-17)
- ✅ 完成UI XML解析器
- ✅ 完成AI交互引擎
- ✅ 完成智能点击工具
- ✅ 更新AI Client支持纯文本模式
- ✅ 编写集成文档
- ⏳ 待集成到orchestrator

---

## 联系方式

如有问题或建议,请联系项目维护者。
