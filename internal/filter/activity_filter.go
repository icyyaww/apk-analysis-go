package filter

import (
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

// ActivityFilter Activity 过滤器
type ActivityFilter struct {
	logger         *logrus.Logger
	packageName    string
	corePatterns   []*regexp.Regexp
	skipPatterns   []*regexp.Regexp
	sdkRules       []string
	minNameLength  int
	maxTestPercent float64
}

// NewActivityFilter 创建 Activity 过滤器
func NewActivityFilter(packageName string, logger *logrus.Logger) *ActivityFilter {
	// 核心 Activity 模式
	corePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i).*MainActivity.*`),
		regexp.MustCompile(`(?i).*LoginActivity.*`),
		regexp.MustCompile(`(?i).*HomeActivity.*`),
		regexp.MustCompile(`(?i).*WelcomeActivity.*`),
		regexp.MustCompile(`(?i).*SplashActivity.*`),
		regexp.MustCompile(`(?i).*\.main\.`),
		regexp.MustCompile(`(?i).*\.login\.`),
		regexp.MustCompile(`(?i).*\.home\.`),
	}

	// 跳过的 Activity 模式
	skipPatterns := []*regexp.Regexp{
		// 系统和测试
		regexp.MustCompile(`(?i).*Test.*Activity.*`),
		regexp.MustCompile(`(?i).*Debug.*Activity.*`),
		regexp.MustCompile(`(?i).*Demo.*Activity.*`),
		regexp.MustCompile(`(?i).*Sample.*Activity.*`),
		regexp.MustCompile(`(?i).*\.test\..*`),
		regexp.MustCompile(`(?i).*\.debug\..*`),

		// 资源和配置类（确定不是 Activity）
		regexp.MustCompile(`.*R\$.*`),           // R$string, R$drawable
		regexp.MustCompile(`.*BuildConfig\$.*`), // BuildConfig$1

		// 注意：移除了过于严格的规则：
		// - 不再过滤 `^[a-z](\.[a-z])+$`（单字母包名）
		// - 不再过滤 `.*\$.*`（所有内部类）
		// 这些规则会在 isMeaninglessName() 中更精确地处理
	}

	// 内嵌 SDK 关键字（过滤内嵌到应用包名下的第三方 SDK Activity）
	// 例如：com.xiachufang.umeng.PushActivity -> 包含 ".umeng."，应该被过滤
	// 策略：只过滤纯统计/推送SDK，保留可能有业务逻辑的SDK（支付、分享、地图等）
	sdkRules := []string{
		// 纯统计分析SDK
		".umeng.",     // 友盟统计：com.xiachufang.umeng.xxx
		".sensorsdata.", // 神策数据
		".talkingdata.", // TalkingData
		".growingio.",   // GrowingIO

		// 纯推送SDK
		".jpush.",     // 极光推送：com.xiachufang.jpush.xxx
		".igexin.",    // 个推

		// 注意：移除了以下规则以放宽限制：
		//   - .mob. (保留，可能是ShareSDK等有业务逻辑的库)
		//   - .google. (保留，可能是zxing、地图等有用的库)
		//   - .facebook. (保留，社交分享可能有业务逻辑)
		//   - .alipay. (保留，支付功能有业务逻辑)
		//   - .weibo. (保留，社交分享可能有业务逻辑)
		//   - .tencent. (保留，QQ/微信分享可能有业务逻辑)
		//   - .firebase. (保留，可能是Auth、Storage等有业务逻辑的服务)
	}

	return &ActivityFilter{
		logger:         logger,
		packageName:    packageName,
		corePatterns:   corePatterns,
		skipPatterns:   skipPatterns,
		sdkRules:       sdkRules,
		minNameLength:  10,
		maxTestPercent: 0.25, // 最多 25% 测试 Activity
	}
}

// FilterResult 过滤结果
type FilterResult struct {
	TotalActivities   int
	SelectedCount     int
	FilteredCount     int
	SelectedList      []string
	FilteredList      []string
	CoreActivities    []string
	FilterReasons     map[string]string
}

// Filter 过滤 Activity 列表
func (f *ActivityFilter) Filter(activities []string) *FilterResult {
	result := &FilterResult{
		TotalActivities: len(activities),
		SelectedList:    []string{},
		FilteredList:    []string{},
		CoreActivities:  []string{},
		FilterReasons:   make(map[string]string),
	}

	for _, activity := range activities {
		reason := f.shouldSkip(activity)
		if reason != "" {
			result.FilteredList = append(result.FilteredList, activity)
			result.FilterReasons[activity] = reason
		} else {
			result.SelectedList = append(result.SelectedList, activity)

			// 标记核心 Activity
			if f.isCoreActivity(activity) {
				result.CoreActivities = append(result.CoreActivities, activity)
			}
		}
	}

	result.SelectedCount = len(result.SelectedList)
	result.FilteredCount = len(result.FilteredList)

	f.logger.WithFields(logrus.Fields{
		"total":    result.TotalActivities,
		"selected": result.SelectedCount,
		"filtered": result.FilteredCount,
		"core":     len(result.CoreActivities),
	}).Info("Activity filtering completed")

	return result
}

// shouldSkip 判断是否应跳过 Activity
func (f *ActivityFilter) shouldSkip(activity string) string {
	// 1. 检查是否为第三方 SDK 的 Activity
	// 策略：检查是否包含已知的第三方 SDK 特征关键字
	// 例如：com.umeng.xxx, com.alipay.xxx, com.tencent.xxx
	if f.isThirdPartySDK(activity) {
		return "third_party_sdk"
	}

	// 注意：移除了包名匹配限制（原第137-139行）
	// 原因：允许执行更多 Activity，不再严格要求必须以应用包名开头
	// 只过滤明确属于第三方 SDK 的 Activity

	// 2. 检查是否包含内嵌 SDK 特征关键字（内嵌到应用包名下的 SDK）
	// 例如：com.xiachufang.umeng.PushActivity 包含 ".umeng."
	for _, sdkKeyword := range f.sdkRules {
		if strings.Contains(activity, sdkKeyword) {
			return "embedded_sdk"
		}
	}

	// 3. 检查混淆类名和跳过模式
	for _, pattern := range f.skipPatterns {
		if pattern.MatchString(activity) {
			return "obfuscated_or_test"
		}
	}

	// 4. 检查名称长度 (太短的可能是混淆类)
	lastPart := activity
	if idx := strings.LastIndex(activity, "."); idx >= 0 {
		lastPart = activity[idx+1:]
	}
	if len(lastPart) < 2 {
		return "name_too_short"
	}

	// 5. 检查是否为无意义的混淆类名（单字母或纯数字）
	// 例如：com.xiachufang.a, com.xiachufang.A, com.xiachufang.a1
	if f.isMeaninglessName(lastPart) {
		return "meaningless_name"
	}

	return ""
}

// isThirdPartySDK 判断 Activity 是否为第三方 SDK
// 检查包名前缀是否匹配已知的第三方 SDK
// 策略：只过滤明确的推送/统计/广告SDK，保留可能包含业务逻辑的库（如二维码、分享、支付等）
func (f *ActivityFilter) isThirdPartySDK(activity string) bool {
	// 只过滤纯推送/统计/广告SDK（这些通常不包含业务逻辑）
	// 注意：移除了以下SDK以放宽限制：
	//   - com.google.* (保留，可能是zxing等有用的库)
	//   - com.alipay.* (保留，支付功能可能有业务逻辑)
	//   - com.sina.weibo.* (保留，社交分享可能有业务逻辑)
	//   - com.tencent.* (保留，QQ/微信分享可能有业务逻辑)
	//   - com.facebook.* (保留，社交分享可能有业务逻辑)
	//   - com.baidu.* (保留，可能是地图等有用的服务)
	//   - 手机厂商SDK (保留，可能有推送以外的功能)
	thirdPartyPrefixes := []string{
		// 纯统计分析SDK
		"com.umeng.",          // 友盟统计
		"com.sensorsdata.",    // 神策数据
		"com.talkingdata.",    // TalkingData
		"com.growingio.",      // GrowingIO
		"io.fabric.",          // Fabric/Crashlytics
		"com.crashlytics.",    // Crashlytics
		"com.adjust.",         // Adjust
		"com.appsflyer.",      // AppsFlyer

		// 纯推送SDK
		"cn.jpush.",           // 极光推送
		"com.igexin.",         // 个推
		"com.google.firebase.messaging.", // Firebase 推送（更精确的匹配）

		// MobSDK相关（但不是所有com.mob开头的）
		"com.mob.tools.",      // MobTools
		"com.mob.moblink.",    // MobLink
		"com.mob.pushsdk.",    // Mob推送SDK
	}

	for _, prefix := range thirdPartyPrefixes {
		if strings.HasPrefix(activity, prefix) {
			return true
		}
	}

	return false
}

// isAppActivity 判断 Activity 是否属于应用（支持包名不完全匹配）
func (f *ActivityFilter) isAppActivity(activity string) bool {
	if f.packageName == "" {
		return true // 无包名限制
	}

	// 策略1: 完全匹配 - Activity 以应用包名开头
	// 例如：com.xiachufang.MainActivity (包名 com.xiachufang)
	if strings.HasPrefix(activity, f.packageName) {
		return true
	}

	// 策略2: 提取共同包名段，检查是否有关联
	// 例如：
	//   应用包名: com.anshan.bsd
	//   Activity: com.purang.bsd.ui.MainActivity
	//   共同段: "bsd" (最后一段)
	appParts := strings.Split(f.packageName, ".")
	activityParts := strings.Split(activity, ".")

	// 至少需要 3 段（如 com.example.app）
	if len(appParts) < 3 || len(activityParts) < 3 {
		return false
	}

	// 检查最后一段是否匹配（通常是应用标识符）
	// com.anshan.bsd vs com.purang.bsd -> "bsd" 匹配
	lastAppPart := appParts[len(appParts)-1]
	if len(lastAppPart) >= 3 { // 至少 3 个字符才有意义
		for i := 0; i < len(activityParts) && i < 4; i++ {
			if activityParts[i] == lastAppPart {
				return true
			}
		}
	}

	// 策略3: 检查是否有多个共同段
	// 例如：com.example.app.android vs com.example.app.ios
	//       共同段: ["com", "example", "app"]
	commonCount := 0
	for i := 0; i < len(appParts) && i < len(activityParts); i++ {
		if appParts[i] == activityParts[i] {
			commonCount++
		}
	}

	// 如果前 N 段都匹配（如 com.example），认为是同一应用
	if commonCount >= 2 {
		return true
	}

	return false
}

// isCoreActivity 判断是否为核心 Activity
func (f *ActivityFilter) isCoreActivity(activity string) bool {
	for _, pattern := range f.corePatterns {
		if pattern.MatchString(activity) {
			return true
		}
	}
	return false
}

// isMeaninglessName 判断是否为无意义的类名（混淆特征）
func (f *ActivityFilter) isMeaninglessName(name string) bool {
	// 1. 单字母类名：a, A, b, B (无论大小写)
	if len(name) == 1 {
		return true
	}

	// 2. 纯数字类名：123, 456
	isAllDigits := true
	for _, ch := range name {
		if ch < '0' || ch > '9' {
			isAllDigits = false
			break
		}
	}
	if isAllDigits {
		return true
	}

	// 3. 单字母+数字组合（常见混淆模式）：a1, A2, b3
	// 例如：com.xiachufang.a1, com.xiachufang.B2
	if len(name) == 2 {
		firstChar := name[0]
		secondChar := name[1]

		// 第一个字符是字母，第二个是数字
		isFirstLetter := (firstChar >= 'a' && firstChar <= 'z') || (firstChar >= 'A' && firstChar <= 'Z')
		isSecondDigit := secondChar >= '0' && secondChar <= '9'

		if isFirstLetter && isSecondDigit {
			return true
		}
	}

	// 4. 全小写单字母串（长度 <= 3）：abc, xyz
	// 但排除常见缩写：api, app, ui, web
	if len(name) <= 3 {
		isAllLowercase := true
		for _, ch := range name {
			if ch < 'a' || ch > 'z' {
				isAllLowercase = false
				break
			}
		}

		if isAllLowercase {
			// 白名单：常见的有意义缩写
			meaningfulAbbr := []string{"api", "app", "ui", "web", "sdk", "ads"}
			for _, abbr := range meaningfulAbbr {
				if name == abbr {
					return false
				}
			}
			return true
		}
	}

	return false
}

// GetFilterReport 生成过滤报告
func (f *ActivityFilter) GetFilterReport(result *FilterResult) map[string]interface{} {
	// 统计过滤原因
	reasonCounts := make(map[string]int)
	for _, reason := range result.FilterReasons {
		reasonCounts[reason]++
	}

	savings := 0.0
	if result.TotalActivities > 0 {
		savings = float64(result.FilteredCount) / float64(result.TotalActivities) * 100
	}

	return map[string]interface{}{
		"total_activities":      result.TotalActivities,
		"filtered_out":          result.FilteredCount,
		"selected_for_testing":  result.SelectedCount,
		"core_activities_count": len(result.CoreActivities),
		"time_savings_percent":  savings,
		"filter_reasons":        reasonCounts,
		"skipped_activities":    result.FilteredList[:min(10, len(result.FilteredList))], // 前 10 个
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
