# 性能监控与优化指南

## 📊 概述

本文档介绍 APK Analysis Platform 的性能监控工具使用方法,包括 pprof 性能分析、内存监控、以及性能优化最佳实践。

---

## 🔍 1. pprof 性能分析

### 1.1 可用端点

pprof 端点仅在非生产环境 (`mode != "release"`) 下可用:

| 端点 | 功能 | 用途 |
|------|------|------|
| `/debug/pprof/` | 概览页面 | 查看所有可用的 profile |
| `/debug/pprof/profile` | CPU Profile | 分析 CPU 热点 |
| `/debug/pprof/heap` | 堆内存 Profile | 分析内存分配 |
| `/debug/pprof/goroutine` | Goroutine Profile | 检测 Goroutine 泄漏 |
| `/debug/pprof/allocs` | 分配 Profile | 分析内存分配次数 |
| `/debug/pprof/block` | 阻塞 Profile | 分析阻塞操作 |
| `/debug/pprof/mutex` | 互斥锁 Profile | 分析锁竞争 |
| `/debug/pprof/trace` | 执行跟踪 | 全局事件跟踪 |

### 1.2 CPU 性能分析

#### 基础使用
```bash
# 采集 30 秒的 CPU profile
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30

# 等待采集完成后,进入交互式界面
(pprof) top 10  # 查看 CPU 占用前 10 的函数
(pprof) list executeActivity  # 查看具体函数代码
(pprof) web  # 生成火焰图 (需要安装 graphviz)
```

#### 生成火焰图
```bash
# 1. 采集 profile
curl -o cpu.prof http://localhost:8080/debug/pprof/profile?seconds=30

# 2. 生成 SVG 火焰图
go tool pprof -http=:9090 cpu.prof

# 3. 浏览器打开 http://localhost:9090 查看可视化图表
```

#### 常用命令
```bash
# 查看函数调用关系
(pprof) top -cum  # 按累计时间排序
(pprof) top -flat # 按函数自身时间排序

# 查看调用图
(pprof) web executeActivity  # 以 executeActivity 为中心的调用图

# 查看源代码
(pprof) list executeActivity  # 显示函数源码和耗时

# 导出报告
(pprof) pdf  # 生成 PDF 报告
(pprof) png  # 生成 PNG 图片
```

### 1.3 内存分析

#### 堆内存分析
```bash
# 采集堆内存 profile
go tool pprof http://localhost:8080/debug/pprof/heap

# 分析内存分配
(pprof) top -alloc_space    # 按累计分配内存排序
(pprof) top -inuse_space    # 按当前使用内存排序
(pprof) list parseJSONL     # 查看具体函数的内存分配
```

#### 内存泄漏检测
```bash
# 1. 采集基线 heap profile
curl -o heap_baseline.prof http://localhost:8080/debug/pprof/heap

# 2. 执行若干任务后,再次采集
curl -o heap_after.prof http://localhost:8080/debug/pprof/heap

# 3. 对比差异
go tool pprof -base heap_baseline.prof heap_after.prof

# 4. 分析增长的内存
(pprof) top -alloc_space
(pprof) list suspiciousFunction
```

### 1.4 Goroutine 泄漏检测

```bash
# 查看当前所有 Goroutine
go tool pprof http://localhost:8080/debug/pprof/goroutine

# 分析 Goroutine 数量
(pprof) top 10  # 按 Goroutine 数量排序

# 查看堆栈信息
(pprof) traces  # 显示所有 Goroutine 堆栈

# 查看特定函数的 Goroutine
(pprof) list workerLoop
```

#### 持续监控 Goroutine 数量
```bash
# 每秒输出 Goroutine 数量
while true; do
    curl -s http://localhost:8080/metrics | jq .memory.goroutines
    sleep 1
done

# 预期结果:
# - 空闲状态: ~25
# - 单任务: ~30
# - 10 并发任务: ~80
# 如果持续增长 → Goroutine 泄漏
```

### 1.5 执行跟踪 (Trace)

```bash
# 1. 采集 5 秒的 trace
wget -O trace.out http://localhost:8080/debug/pprof/trace?seconds=5

# 2. 打开 trace 可视化
go tool trace trace.out

# 3. 浏览器会自动打开,可以查看:
# - Goroutine 调度情况
# - 系统调用耗时
# - 网络 I/O 阻塞
# - GC 事件
```

---

## 📈 2. 内存监控

### 2.1 实时内存查询

#### HTTP API
```bash
# 获取当前内存统计
curl http://localhost:8080/metrics

# 响应示例:
{
  "memory": {
    "alloc": 52428800,        // 当前分配 (字节)
    "total_alloc": 1048576000, // 累计分配
    "sys": 104857600,          // 系统内存
    "num_gc": 12,              // GC 次数
    "goroutines": 25,          // Goroutine 数量
    "alloc_mb": 50,            // 当前分配 (MB)
    "sys_mb": 100              // 系统内存 (MB)
  }
}
```

#### 使用 jq 过滤
```bash
# 只查看内存 MB 值
curl -s http://localhost:8080/metrics | jq '.memory | {alloc_mb, sys_mb, goroutines}'

# 监控内存使用率
watch -n 1 'curl -s http://localhost:8080/metrics | jq .memory.alloc_mb'
```

### 2.2 手动触发 GC

```bash
# 触发垃圾回收
curl -X POST http://localhost:8080/debug/gc

# 响应:
{
  "message": "GC triggered successfully"
}

# 验证内存释放
curl -s http://localhost:8080/metrics | jq .memory.alloc_mb
```

### 2.3 内存告警

内存监控器每 30 秒自动检查内存使用,当超过 **1.5GB** 时会输出警告日志:

```log
time="2025-11-05T10:30:00+08:00" level=warning msg="High memory usage detected" alloc_mb=1600 sys_mb=2048
```

**处理建议**:
1. 查看 pprof heap profile 定位内存热点
2. 检查是否有大对象未释放
3. 考虑手动触发 GC
4. 检查 Goroutine 是否泄漏

---

## 🚀 3. 性能优化最佳实践

### 3.1 数据库连接池

当前配置 (已优化):
```go
MaxIdleConns: 10           // 空闲连接数
MaxOpenConns: 50           // 最大连接数
ConnMaxLifetime: 1h        // 连接最长存活时间
ConnMaxIdleTime: 10m       // 空闲连接超时
```

**监控指标**:
```bash
# 查看数据库连接状态
mysql> show processlist;

# 预期连接数: 10-50 之间
# 如果经常达到 50 → 考虑增加 MaxOpenConns
# 如果大部分时间 < 10 → 考虑减少 MaxIdleConns
```

### 3.2 流式 JSONL 处理

#### 问题场景
```go
// ❌ 错误: 全量加载到内存
data, _ := os.ReadFile("flows.jsonl")  // 500MB 文件 → OOM
lines := strings.Split(string(data), "\n")
for _, line := range lines {
    var flow FlowData
    json.Unmarshal([]byte(line), &flow)
}
```

#### 优化方案
```go
// ✅ 正确: 流式处理
reader, _ := utils.NewStreamJSONLReader("flows.jsonl")
defer reader.Close()

for {
    data, err := reader.ReadNext()
    if err == io.EOF {
        break
    }

    // 处理单条数据
    processFlow(data)
}
```

**内存效果**:
- 优化前: 500MB 文件 → 峰值内存 ~1.2GB
- 优化后: 500MB 文件 → 峰值内存 ~50MB

### 3.3 字符串拼接优化

#### 问题场景
```go
// ❌ 错误: 频繁字符串拼接
var result string
for i := 0; i < 1000; i++ {
    result += fmt.Sprintf("line_%d\n", i)  // 每次都重新分配内存
}
```

#### 优化方案
```go
// ✅ 正确: 使用 strings.Builder
var builder strings.Builder
builder.Grow(1000 * 10)  // 预分配容量

for i := 0; i < 1000; i++ {
    builder.WriteString(fmt.Sprintf("line_%d\n", i))
}
result := builder.String()
```

### 3.4 对象池 (sync.Pool)

#### 适用场景
- 频繁创建/销毁的临时对象
- 大对象 (如 buffer)

#### 示例
```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

func processData(data []byte) {
    // 从池中获取 buffer
    buf := bufferPool.Get().(*bytes.Buffer)
    defer func() {
        buf.Reset()
        bufferPool.Put(buf)  // 归还到池中
    }()

    buf.Write(data)
    // ... 处理逻辑
}
```

---

## 📊 4. 性能基准

### 4.1 内存使用基准

| 场景 | 目标值 | 实际值 | 状态 |
|------|--------|--------|------|
| 服务启动 | < 100MB | ~50MB | ✅ |
| 空闲状态 | < 100MB | ~60MB | ✅ |
| 单任务执行 | < 500MB | ~400MB | ✅ |
| 并发 10 任务 | < 2GB | ~1.8GB | ✅ |
| 流式处理 500MB JSONL | < 100MB | ~50MB | ✅ |

### 4.2 API 响应延迟基准

| 端点 | P50 | P95 | P99 |
|------|-----|-----|-----|
| GET /api/tasks | 10ms | 30ms | 50ms |
| GET /api/tasks/:id | 5ms | 15ms | 25ms |
| GET /api/tasks/:id/urls | 50ms | 120ms | 200ms |
| GET /metrics | 5ms | 10ms | 15ms |
| DELETE /api/tasks/:id | 100ms | 300ms | 500ms |

### 4.3 Goroutine 基准

| 场景 | 期望数量 | 说明 |
|------|----------|------|
| 服务启动 | ~25 | 基础 Goroutines |
| 单任务执行 | ~30 | +5 任务相关 |
| 并发 10 任务 | ~80 | +55 任务相关 |
| 任务完成后 5 分钟 | ~25 | 应恢复到基础数量 |

---

## 🔧 5. 故障排查

### 5.1 内存持续增长

**症状**: `alloc_mb` 持续上升,GC 后无法回落

**排查步骤**:
```bash
# 1. 采集 heap profile
curl -o heap.prof http://localhost:8080/debug/pprof/heap

# 2. 分析内存占用
go tool pprof heap.prof
(pprof) top -inuse_space  # 查看当前占用内存最多的函数
(pprof) list suspiciousFunc

# 3. 常见原因:
# - 大对象未释放 (检查全局变量/缓存)
# - Goroutine 泄漏 (检查 goroutine profile)
# - 第三方库内存泄漏 (检查调用栈)
```

### 5.2 CPU 使用率过高

**症状**: CPU 使用率 > 80%

**排查步骤**:
```bash
# 1. 采集 CPU profile
curl -o cpu.prof http://localhost:8080/debug/pprof/profile?seconds=30

# 2. 分析热点
go tool pprof cpu.prof
(pprof) top -cum  # 查看累计 CPU 时间
(pprof) web       # 可视化调用图

# 3. 常见热点:
# - JSON 解析 (考虑使用 jsoniter)
# - 正则表达式 (考虑预编译)
# - 字符串拼接 (使用 strings.Builder)
```

### 5.3 Goroutine 泄漏

**症状**: Goroutine 数量持续增长

**排查步骤**:
```bash
# 1. 查看 Goroutine 堆栈
go tool pprof http://localhost:8080/debug/pprof/goroutine

(pprof) top 10    # 查看 Goroutine 数量最多的函数
(pprof) traces    # 查看完整堆栈

# 2. 常见原因:
# - channel 阻塞未关闭
# - 无限循环未退出
# - Context 未取消
```

**修复示例**:
```go
// ❌ 错误: channel 阻塞
go func() {
    for data := range ch {  // 如果 ch 永不关闭 → Goroutine 泄漏
        process(data)
    }
}()

// ✅ 正确: 使用 context 控制生命周期
go func() {
    for {
        select {
        case data := <-ch:
            process(data)
        case <-ctx.Done():
            return  // 退出 Goroutine
        }
    }
}()
```

---

## 📚 6. 参考资料

### 官方文档
- [Go pprof 文档](https://pkg.go.dev/net/http/pprof)
- [Go 性能分析博客](https://go.dev/blog/pprof)
- [Go 内存模型](https://go.dev/ref/mem)

### 工具安装
```bash
# graphviz (用于生成可视化图表)
# Ubuntu/Debian
sudo apt-get install graphviz

# macOS
brew install graphviz

# CentOS/RHEL
sudo yum install graphviz
```

### 推荐阅读
- [High Performance Go Workshop](https://dave.cheney.net/high-performance-go-workshop/gopherchina-2019.html)
- [Go 性能优化实战](https://github.com/dgryski/go-perfbook)
- [GORM 性能优化](https://gorm.io/docs/performance.html)

---

## ✅ 检查清单

定期检查以下指标:

- [ ] 内存使用 < 2GB (并发 10 任务)
- [ ] 空闲状态内存 < 100MB
- [ ] Goroutine 数量稳定 (任务完成后恢复基线)
- [ ] 数据库连接数 < 50
- [ ] API P95 响应 < 200ms
- [ ] GC 暂停时间 < 10ms
- [ ] 无 Goroutine 泄漏
- [ ] 无内存泄漏
