# 域名评分计算规则文档

## 目录

1. [概述](#概述)
2. [Go版本评分规则（生产环境）](#go版本评分规则生产环境)
3. [Python MVP版本评分规则（参考实现）](#python-mvp版本评分规则参考实现)
4. [两版本对比](#两版本对比)
5. [实际案例](#实际案例)
6. [优化建议](#优化建议)

---

## 概述

### 目标
从APK应用中提取的大量域名中，智能识别出**主域名**（Primary Domain），即应用的核心业务域名。

### 方法
通过多维度评分机制，综合分析：
- 域名与应用包名的匹配度
- URL访问频率
- 动态流量特征
- SDK过滤（排除第三方SDK域名）
- 路径多样性（仅Python版本）
- 子域名数量（仅Python版本）
- API/认证特征（仅Python版本）

### 数据来源
- **静态分析**：MobSF报告中的域名列表
- **动态分析**：应用运行时的网络流量捕获（mitmproxy）

---

## Go版本评分规则（生产环境）

### 评分维度

#### 1. 包名匹配（Package Match）
**权重：50分（固定）**

**计算逻辑：**
```go
// 判断域名是否包含包名的任意部分
packageParts := strings.Split(packageName, ".")
for _, part := range packageParts {
    if strings.Contains(domain, part) {
        score += 50
        break
    }
}
```

**示例：**
- 包名：`com.jd.jrapp`
- 域名：`jdpay.com` → 包含 `jd` → **+50分**
- 域名：`example.com` → 不包含 → **+0分**

#### 2. URL请求频率（Request Count）
**权重：0-50分（线性，上限50）**

**计算逻辑：**
```go
score += min(requestCount, 50)
```

**示例：**
- 请求237次 → **+50分**（上限）
- 请求30次 → **+30分**
- 请求5次 → **+5分**

#### 3. 动态流量（Dynamic Traffic）
**权重：10分（固定）**

**计算逻辑：**
```go
// 域名来自动态捕获的流量
if fromDynamicURLs {
    score += 10
}
```

**示例：**
- 在mitmproxy捕获的流量中 → **+10分**
- 仅在MobSF静态分析中 → **+0分**

#### 4. SDK过滤（SDK Penalty）
**权重：-30分（惩罚）**

**计算逻辑：**
```go
// 匹配SDK规则库中的域名模式
if matchesSDKPattern(domain) {
    score -= 30
}
```

**示例SDK域名：**
- `googletagmanager.com` → 广告追踪 → **-30分**
- `firebase.google.com` → 统计分析 → **-30分**
- `alipay.com` → 支付SDK → **-30分**

### 总分计算

```
总分 = 包名匹配(0/50) + URL频率(0-50) + 动态流量(0/10) - SDK惩罚(0/30)
范围：-30 ~ 110 分
```

### 置信度计算

```go
confidence := "高" if score >= 80
confidence := "中" if 50 <= score < 80
confidence := "低" if score < 50
```

### 完整代码示例

```go
func (a *DomainAnalyzer) scoreDomain(
    domain string,
    packageName string,
    requestCount int,
    fromDynamicURLs bool,
) int {
    score := 0

    // 1. 包名匹配（50分）
    packageParts := strings.Split(packageName, ".")
    for _, part := range packageParts {
        if len(part) > 2 && strings.Contains(domain, part) {
            score += 50
            break
        }
    }

    // 2. URL频率（0-50分）
    score += min(requestCount, 50)

    // 3. 动态流量（10分）
    if fromDynamicURLs {
        score += 10
    }

    // 4. SDK过滤（-30分）
    if a.sdkManager.IsSDKDomain(domain) {
        score -= 30
    }

    return score
}
```

---

## Python MVP版本评分规则（参考实现）

### 评分维度

#### 1. 包名匹配（Package Match）
**权重：0-10分（渐变）**

**计算逻辑：**
```python
# 计算包名与域名的匹配度（Levenshtein距离）
package_parts = package_name.split('.')
best_match_ratio = 0.0

for part in package_parts:
    if len(part) > 2:
        ratio = fuzz.ratio(part, domain_name) / 100
        best_match_ratio = max(best_match_ratio, ratio)

score += best_match_ratio * 10  # 0-10分
```

**示例：**
- `jd` 与 `jd.com`：完全匹配 → **+10分**
- `jingdong` 与 `jd.com`：部分匹配 → **+6分**
- `example` 与 `jd.com`：无匹配 → **+0分**

#### 2. URL访问频率（Frequency）
**权重：0-5分（归一化）**

**计算逻辑：**
```python
max_count = max(stats['count'] for stats in all_domains)
freq_score = (current_count / max_count) * 5
score += freq_score
```

**示例：**
- 当前域名237次，最大237次 → `(237/237)*5` → **+5分**
- 当前域名118次，最大237次 → `(118/237)*5` → **+2.49分**

#### 3. 路径多样性（Path Diversity）
**权重：0-3分（归一化）**

**计算逻辑：**
```python
unique_paths = set(url.path for url in domain_urls)
max_paths = max(len(stats['paths']) for stats in all_domains)
path_score = (len(unique_paths) / max_paths) * 3
score += path_score
```

**示例：**
- 路径：`['/api/user', '/api/order', '/api/pay', ...]` → 多样性高 → **+3分**
- 路径：`['/index.html']` → 多样性低 → **+0.5分**

#### 4. 子域名数量（Subdomain Count）
**权重：0-2分（归一化）**

**计算逻辑：**
```python
subdomains = set(extract_subdomain(url) for url in domain_urls)
max_subdomains = max(len(stats['subdomains']) for stats in all_domains)
subdomain_score = (len(subdomains) / max_subdomains) * 2
score += subdomain_score
```

**示例：**
- 子域名：`['api', 'www', 'cdn', 'static']` → **+2分**
- 子域名：`['www']` → **+0.5分**

#### 5. 动态流量（Dynamic Traffic）
**权重：0-3分（阶梯）**

**计算逻辑：**
```python
if is_dynamic:
    score += 2  # 基础分
    if dynamic_count > 5:
        score += 1  # 高频奖励
```

**示例：**
- 动态流量20次 → **+3分**
- 动态流量3次 → **+2分**
- 无动态流量 → **+0分**

#### 6. API特征（API Features）
**权重：3分（固定）**

**检测规则：**
```python
api_patterns = ['/api/', '/v1/', '/v2/', '/rest/', '/graphql']
if any(pattern in url.path for pattern in api_patterns):
    score += 3
```

**示例：**
- `https://api.jd.com/v1/user` → **+3分**
- `https://www.jd.com/index.html` → **+0分**

#### 7. 认证特征（Auth Features）
**权重：1分（固定）**

**检测规则：**
```python
auth_patterns = ['/login', '/auth', '/oauth', '/token']
if any(pattern in url.path for pattern in auth_patterns):
    score += 1
```

**示例：**
- `https://auth.jd.com/login` → **+1分**
- `https://www.jd.com/product` → **+0分**

#### 8. CDN惩罚（CDN Penalty）
**权重：-2分（惩罚）**

**检测规则：**
```python
cdn_patterns = ['cdn', 'static', 'img', 'assets']
if any(pattern in domain for pattern in cdn_patterns):
    score -= 2
```

**示例：**
- `cdn.jd.com` → **-2分**
- `api.jd.com` → **+0分**

### 总分计算

```python
总分 = 包名匹配(0-10) + 频率(0-5) + 路径多样性(0-3) +
      子域名(0-2) + 动态流量(0-3) + API(0-3) +
      认证(0-1) - CDN惩罚(0-2)

理论范围：-2 ~ 27 分
实际范围：0 ~ 22 分（常见）
```

### 置信度计算

```python
# 归一化到 0-1 范围
confidence = min(score / 22, 1.0)

# 分级
if confidence >= 0.8:    # ≥17.6分
    level = "高"
elif confidence >= 0.5:  # ≥11分
    level = "中"
else:
    level = "低"
```

### 完整代码示例

```python
def _score_domain(self, domain: str, stats: Dict, global_stats: Dict) -> float:
    score = 0.0

    # 1. 包名匹配（0-10分）
    if stats['package_match']:
        score += stats['package_score'] * 10

    # 2. 频率（0-5分）
    freq_score = (stats['count'] / global_stats['max_count']) * 5
    score += freq_score

    # 3. 路径多样性（0-3分）
    path_score = (len(stats['paths']) / global_stats['max_paths']) * 3
    score += path_score

    # 4. 子域名（0-2分）
    subdomain_score = (len(stats['subdomains']) / global_stats['max_subdomains']) * 2
    score += subdomain_score

    # 5. 动态流量（0-3分）
    if stats['is_dynamic']:
        score += 2
        if stats.get('dynamic_count', 0) > 5:
            score += 1

    # 6. API特征（3分）
    if stats['is_api']:
        score += 3.0

    # 7. 认证特征（1分）
    if stats['has_auth']:
        score += 1.0

    # 8. CDN惩罚（-2分）
    if stats['is_cdn']:
        score -= 2.0

    return max(score, 0.0)  # 确保非负
```

---

## 两版本对比

| 维度 | Go版本 | Python MVP版本 | 差异说明 |
|------|--------|----------------|----------|
| **包名匹配** | 50分（boolean） | 0-10分（渐变） | Go版本采用全或无，Python版本考虑相似度 |
| **URL频率** | 0-50分（绝对值） | 0-5分（归一化） | Go版本直接计数，Python版本相对排名 |
| **动态流量** | 10分（boolean） | 0-3分（阶梯） | Go版本简化，Python版本区分频次 |
| **SDK过滤** | -30分（严格） | 无 | Go版本有专门SDK规则库 |
| **路径多样性** | 无 | 0-3分 | Python版本特有 |
| **子域名数量** | 无 | 0-2分 | Python版本特有 |
| **API特征** | 无 | 3分 | Python版本特有 |
| **认证特征** | 无 | 1分 | Python版本特有 |
| **CDN惩罚** | 无 | -2分 | Python版本特有 |
| **总分范围** | -30 ~ 110 | 0 ~ 22 | Go版本分数区间更大 |
| **置信度** | 分级（高/中/低） | 归一化（0-1） | Python版本更精细 |

### 设计哲学

- **Go版本**：简单高效，强调包名匹配和访问频率，严格过滤SDK
- **Python MVP版本**：细粒度分析，多维度特征，适合研究和优化

---

## 实际案例

### 案例1：京东金融 APP

**基本信息：**
- 包名：`com.jd.jrapp`
- 主域名：`jd.com`
- URL总数：237次
- 动态流量：有

#### Go版本计算

```
1. 包名匹配：jd.com 包含 "jd"          → +50分
2. URL频率：237次（超过上限50）         → +50分
3. 动态流量：来自mitmproxy捕获          → +10分
4. SDK过滤：jd.com 不是SDK域名          → +0分
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
总分：110分
置信度：高
```

#### Python MVP版本计算

```
1. 包名匹配：fuzz.ratio("jd", "jd") = 100%  → +10分
2. URL频率：237/237 = 1.0                    → +5分
3. 路径多样性：15个路径/15个（最大）         → +3分
4. 子域名：4个/4个（最大）                   → +2分
5. 动态流量：20次（>5）                      → +3分
6. API特征：包含/api/路径                    → +3分
7. 认证特征：包含/oauth/路径                 → +1分
8. CDN惩罚：无CDN关键词                      → +0分
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
总分：27分
置信度：min(27/22, 1.0) = 1.0（满分）
```

### 案例2：第三方SDK域名

**基本信息：**
- 域名：`firebase.google.com`
- 包名：`com.jd.jrapp`
- URL总数：45次

#### Go版本计算

```
1. 包名匹配：不包含"jd"                  → +0分
2. URL频率：45次                         → +45分
3. 动态流量：有                          → +10分
4. SDK过滤：匹配Firebase SDK规则         → -30分
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
总分：25分
置信度：低（被SDK过滤有效降低）
```

#### Python MVP版本计算

```
1. 包名匹配：无匹配                      → +0分
2. URL频率：45/237 = 0.19                → +0.95分
3. 路径多样性：3个路径/15个              → +0.6分
4. 子域名：1个/4个                       → +0.5分
5. 动态流量：8次（>5）                   → +3分
6. API特征：包含/v1/路径                 → +3分
7. 认证特征：无                          → +0分
8. CDN惩罚：无                           → +0分
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
总分：8.05分
置信度：8.05/22 = 0.37（低）
```

### 案例3：CDN域名

**基本信息：**
- 域名：`cdn.jd.com`
- 包名：`com.jd.jrapp`
- URL总数：120次

#### Go版本计算

```
1. 包名匹配：包含"jd"                    → +50分
2. URL频率：120次（上限50）              → +50分
3. 动态流量：有                          → +10分
4. SDK过滤：不是SDK（CDN不在SDK库）      → +0分
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
总分：110分
置信度：高（Go版本未识别CDN）
```

#### Python MVP版本计算

```
1. 包名匹配：fuzz.ratio("jd", "jd") = 100%  → +10分
2. URL频率：120/237 = 0.51                   → +2.55分
3. 路径多样性：2个路径/15个                  → +0.4分
4. 子域名：1个/4个                           → +0.5分
5. 动态流量：15次（>5）                      → +3分
6. API特征：无                               → +0分
7. 认证特征：无                              → +0分
8. CDN惩罚：包含"cdn"关键词                  → -2分
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
总分：14.45分
置信度：14.45/22 = 0.66（中）
```

**分析：** Python版本通过CDN惩罚机制降低了CDN域名的置信度。

---

## 优化建议

### 针对Go版本

#### 1. 增加路径多样性分析
**问题：** 当前只考虑请求次数，无法区分单一路径重复请求 vs 多路径访问

**建议：**
```go
// 新增维度：路径多样性（0-10分）
uniquePaths := extractUniquePaths(urls)
pathScore := min(len(uniquePaths), 10)
score += pathScore
```

**效果：**
- `jd.com` 有15个不同API路径 → +10分
- `cdn.jd.com` 只有1个图片路径 → +1分

#### 2. CDN域名识别
**问题：** CDN域名可能获得高分（如案例3）

**建议：**
```go
cdnPatterns := []string{"cdn", "static", "img", "assets", "cache"}
if matchesPattern(domain, cdnPatterns) {
    score -= 20  // CDN惩罚
}
```

#### 3. API特征检测
**问题：** 无法区分API域名和普通网页域名

**建议：**
```go
apiPatterns := []string{"/api/", "/v1/", "/v2/", "/rest/", "/graphql"}
if hasAPIPattern(urls, apiPatterns) {
    score += 15  // API加分
}
```

#### 4. 包名匹配渐变评分
**问题：** 全或无评分，无法体现相似度差异

**建议：**
```go
// 使用模糊匹配（Levenshtein距离）
matchRatio := fuzzyMatch(packagePart, domain)
score += int(matchRatio * 50)  // 0-50分渐变
```

**效果：**
- `jd.com` vs `jd` → 100%匹配 → +50分
- `jingdong.com` vs `jd` → 60%匹配 → +30分

### 针对Python MVP版本

#### 1. 添加SDK过滤机制
**问题：** 缺少对第三方SDK域名的有效过滤

**建议：**
```python
# 新增SDK规则库
sdk_patterns = ['firebase', 'googletagmanager', 'facebook', 'umeng']
if matches_sdk(domain, sdk_patterns):
    score -= 10  # SDK严重惩罚
```

#### 2. 提高包名匹配权重
**问题：** 包名匹配仅10分，权重可能过低

**建议：**
```python
# 调整为0-15分
score += package_match_ratio * 15
```

#### 3. 动态调整总分基准
**问题：** 固定22分基准，不同应用特征差异大

**建议：**
```python
# 根据应用类型动态调整
if app_type == "电商":
    max_score = 25  # 电商应用API特征多
elif app_type == "工具":
    max_score = 18  # 工具应用特征少

confidence = min(score / max_score, 1.0)
```

### 通用优化方向

1. **机器学习方法**
   - 收集人工标注的主域名数据
   - 训练分类模型（Random Forest / XGBoost）
   - 特征工程：当前所有维度作为输入

2. **白名单机制**
   - 维护知名应用的主域名白名单
   - 包名 → 主域名映射表
   - 直接命中白名单，跳过评分

3. **用户反馈循环**
   - 允许用户标记正确/错误的主域名
   - 持续优化评分权重

4. **多候选返回**
   - 返回TOP 3候选域名及评分
   - 提供人工复核接口

---

## 附录

### Go版本代码位置
- 评分逻辑：`/internal/domainanalysis/analyzer.go`
- SDK管理：`/internal/domainanalysis/sdk_manager.go`
- 服务层：`/internal/domainanalysis/service.go`

### Python MVP版本代码位置
- 评分逻辑：`/home/icyyaww/project/动态apk解析/apk-analysis-mvp/orchestrator/domain_analyzer.py`

### 参考文档
- MobSF API文档：https://mobsf.github.io/docs/
- mitmproxy文档：https://docs.mitmproxy.org/

---

**文档版本：** v1.0
**最后更新：** 2025-01-13
**维护者：** APK Analysis Team
