package worker

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apk-analysis/apk-analysis-go/internal/adb"
	"github.com/apk-analysis/apk-analysis-go/internal/ai"
	"github.com/sirupsen/logrus"
)

// ============================================
// 页面类型枚举
// ============================================

// PageType 页面类型
type PageType int

const (
	PageTypeUnknown    PageType = iota
	PageTypePermission          // 权限申请弹窗
	PageTypeAgreement           // 用户协议/隐私政策
	PageTypeLogin               // 登录页面
	PageTypeGuide               // 引导页/轮播页
	PageTypeMainUI              // 主界面
	PageTypeAd                  // 广告页/开屏广告
	PageTypeUpdate              // 更新提示弹窗
)

// String 页面类型字符串表示
func (p PageType) String() string {
	switch p {
	case PageTypePermission:
		return "permission"
	case PageTypeAgreement:
		return "agreement"
	case PageTypeLogin:
		return "login"
	case PageTypeGuide:
		return "guide"
	case PageTypeMainUI:
		return "main_ui"
	case PageTypeAd:
		return "ad"
	case PageTypeUpdate:
		return "update"
	default:
		return "unknown"
	}
}

// ============================================
// 登录页面处理策略
// ============================================

// LoginPageStrategy 登录页面处理策略结果
type LoginPageStrategy struct {
	CanBypass    bool        // 是否可以绕过登录
	BypassMethod string      // 绕过方式: skip_button, close_button, bottom_tab, back_button, one_click_login
	Actions      []ai.Action // 执行的操作
	NeedLogin    bool        // 是否必须登录
	LoginType    string      // 登录类型（如果需要登录）
}

// GuidanceConfig 引导配置
type GuidanceConfig struct {
	Enabled          bool // 是否启用引导
	MaxRounds        int  // 最大引导轮数
	RoundTimeoutSec  int  // 每轮超时秒数
	StableThreshold  int  // 页面稳定阈值
	SaveScreenshots  bool // 是否保存引导截图
	AutoLoginEnabled bool // 是否启用自动登录
}

// DefaultGuidanceConfig 默认引导配置
func DefaultGuidanceConfig() *GuidanceConfig {
	return &GuidanceConfig{
		Enabled:          true,
		MaxRounds:        15,
		RoundTimeoutSec:  10,
		StableThreshold:  3,
		SaveScreenshots:  true,
		AutoLoginEnabled: false,
	}
}

// GuidanceResult 引导结果
type GuidanceResult struct {
	Success          bool      // 是否成功进入主界面
	RoundsExecuted   int       // 执行的轮数
	FinalPageType    PageType  // 最终页面类型
	LoginRequired    bool      // 是否需要登录
	LoginBypassUsed  string    // 使用的登录绕过方式
	PagesEncountered []string  // 遇到的页面类型列表
	Duration         time.Duration // 引导耗时
}

// ============================================
// 关键词定义
// ============================================

var (
	// 权限相关关键词
	permissionKeywords = []string{
		"允许", "allow", "同意", "agree", "授权", "grant",
		"仅在使用中允许", "始终允许", "仅此一次",
		"while using", "only this time", "always allow",
		"权限", "permission",
	}

	// 用户协议关键词
	agreementKeywords = []string{
		"用户协议", "隐私政策", "服务条款", "同意并继续",
		"user agreement", "privacy policy", "terms of service",
		"我已阅读", "i have read", "agree and continue",
		"隐私保护", "个人信息", "privacy",
	}

	// 跳过登录关键词
	skipLoginKeywords = []string{
		"跳过", "skip", "稀后", "later", "游客", "guest",
		"暂不登录", "先逛逛", "随便看看", "试用", "体验",
		"暂不", "以后再说", "不了", "直接进入", "立即体验",
		"免登录", "visitor", "browse", "explore",
		"进入首页", "先看看", "暂不注册",
	}

	// 登录页面特征关键词
	loginPageKeywords = []string{
		"登录", "login", "sign in", "signin",
		"手机号", "phone", "验证码", "captcha",
		"密码", "password", "账号", "account",
		"注册", "register", "sign up",
	}

	// 引导页关键词
	guideKeywords = []string{
		"下一步", "next", "开始体验", "start", "进入", "enter",
		"了解更多", "learn more", "立即体验", "开始使用",
		"完成", "done", "finish", "got it", "知道了",
	}

	// 主界面关键词
	mainUIKeywords = []string{
		"首页", "home", "推荐", "发现", "我的", "mine",
		"消息", "message", "搜索", "search", "热门",
		"关注", "动态", "广场", "社区",
	}

	// 广告/开屏关键词
	adKeywords = []string{
		"跳过广告", "skip ad", "广告", "ad",
		"秒后跳过", "s后跳过", "跳过 ",
	}

	// 更新提示关键词
	updateKeywords = []string{
		"立即更新", "稍后更新", "暂不更新", "update",
		"新版本", "new version", "升级", "upgrade",
		"以后再说", "暂不升级", "取消更新",
	}

	// 同意/确认关键词（通用）- 用于协议页面
	// 注意：按优先级排序，越具体的关键词越靠前
	agreeKeywords = []string{
		"同意并继续", "我同意", "同意以上", "我已阅读并同意",
		"agree and continue", "i agree",
		"同意", "agree", "确定", "ok", "好的",
		"accept", "继续", "continue",
		"我知道了", "知道了", "got it", "i understand",
	}

	// 禁止点击的关键词
	forbiddenKeywords = []string{
		"拒绝", "deny", "refuse", "不同意", "disagree",
		"禁止", "forbid", "否", "取消", "cancel",
		"退出登录", "sign out", "logout", "退出",
	}

	// 关闭按钮关键词/模式
	closeButtonPatterns = []string{
		`content-desc="[^"]*关闭[^"]*"`,
		`content-desc="[^"]*close[^"]*"`,
		`resource-id="[^"]*close[^"]*"`,
		`resource-id="[^"]*dismiss[^"]*"`,
		`resource-id="[^"]*btn_close[^"]*"`,
		`resource-id="[^"]*iv_close[^"]*"`,
		`resource-id="[^"]*img_close[^"]*"`,
		`resource-id="[^"]*icon_close[^"]*"`,
	}
)

// ============================================
// UI 元素查找方法
// ============================================

// GuidanceUIElement 引导阶段使用的UI元素
type GuidanceUIElement struct {
	Text       string
	ResourceID string
	Class      string
	Package    string
	Bounds     [4]int // x1, y1, x2, y2
	Center     [2]int
	Clickable  bool
}

// findClickableElementByText 在 UI XML 中查找包含指定文本的可点击元素
func findClickableElementByText(uiXML string, text string) *GuidanceUIElement {
	// 构建正则模式，匹配包含指定文本的node
	// 注意：text属性可能包含text，也可能在content-desc中
	patterns := []string{
		// 匹配 text 属性
		fmt.Sprintf(`<node[^>]*text="[^"]*(?i)%s[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"[^>]*>`, regexp.QuoteMeta(text)),
		// 匹配 content-desc 属性
		fmt.Sprintf(`<node[^>]*content-desc="[^"]*(?i)%s[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"[^>]*>`, regexp.QuoteMeta(text)),
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(uiXML)

		if len(matches) >= 5 {
			x1, _ := strconv.Atoi(matches[1])
			y1, _ := strconv.Atoi(matches[2])
			x2, _ := strconv.Atoi(matches[3])
			y2, _ := strconv.Atoi(matches[4])

			return &GuidanceUIElement{
				Text:   text,
				Bounds: [4]int{x1, y1, x2, y2},
				Center: [2]int{(x1 + x2) / 2, (y1 + y2) / 2},
			}
		}
	}

	return nil
}

// findElementByPattern 通过正则模式查找元素
func findElementByPattern(uiXML string, pattern string) *GuidanceUIElement {
	// 首先匹配模式
	re := regexp.MustCompile(pattern)
	if !re.MatchString(uiXML) {
		return nil
	}

	// 找到匹配的node，提取bounds
	// 构建完整的node匹配模式
	nodePattern := fmt.Sprintf(`<node[^>]*%s[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"[^>]*>`, pattern)
	nodeRe := regexp.MustCompile(nodePattern)
	matches := nodeRe.FindStringSubmatch(uiXML)

	if len(matches) >= 5 {
		x1, _ := strconv.Atoi(matches[1])
		y1, _ := strconv.Atoi(matches[2])
		x2, _ := strconv.Atoi(matches[3])
		y2, _ := strconv.Atoi(matches[4])

		return &GuidanceUIElement{
			Bounds: [4]int{x1, y1, x2, y2},
			Center: [2]int{(x1 + x2) / 2, (y1 + y2) / 2},
		}
	}

	return nil
}

// findElementsByKeywords 查找包含任意关键词的元素
func findElementsByKeywords(uiXML string, keywords []string) []*GuidanceUIElement {
	var elements []*GuidanceUIElement

	for _, kw := range keywords {
		if elem := findClickableElementByText(uiXML, kw); elem != nil {
			elements = append(elements, elem)
		}
	}

	return elements
}

// findCheckboxNearAgreementText 查找协议文本附近的复选框
// 通过位置关系检测：找到包含"协议"/"政策"/"同意"文本的元素，然后查找其左侧的小型可点击元素
func findCheckboxNearAgreementText(uiXML string) *GuidanceUIElement {
	// 查找包含协议相关文本的元素
	agreementTextPatterns := []string{
		`text="[^"]*同意[^"]*协议[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`,
		`text="[^"]*同意[^"]*政策[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`,
		`text="[^"]*用户协议[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`,
		`text="[^"]*隐私政策[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`,
	}

	var agreementY int = -1
	for _, pattern := range agreementTextPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(uiXML)
		if len(matches) >= 5 {
			y1, _ := strconv.Atoi(matches[2])
			y2, _ := strconv.Atoi(matches[4])
			agreementY = (y1 + y2) / 2
			break
		}
	}

	if agreementY < 0 {
		return nil
	}

	// 查找同一行（Y坐标相近）的小型可点击元素
	// 复选框通常是小尺寸（宽高<150px）且在协议文本左侧
	nodePattern := `<node[^>]*clickable="true"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"[^>]*>`
	re := regexp.MustCompile(nodePattern)
	matches := re.FindAllStringSubmatch(uiXML, -1)

	for _, match := range matches {
		if len(match) >= 5 {
			x1, _ := strconv.Atoi(match[1])
			y1, _ := strconv.Atoi(match[2])
			x2, _ := strconv.Atoi(match[3])
			y2, _ := strconv.Atoi(match[4])

			width := x2 - x1
			height := y2 - y1
			centerY := (y1 + y2) / 2

			// 检查条件：
			// 1. 小尺寸（宽高都<150px）
			// 2. Y坐标与协议文本相近（差值<100px）
			// 3. 在屏幕左侧（x1 < 400）
			if width < 150 && height < 150 && width > 20 && height > 20 {
				if abs(centerY-agreementY) < 100 && x1 < 400 {
					return &GuidanceUIElement{
						Bounds: [4]int{x1, y1, x2, y2},
						Center: [2]int{(x1 + x2) / 2, (y1 + y2) / 2},
					}
				}
			}
		}
	}

	return nil
}

// ============================================
// 辅助函数
// ============================================

// abs 返回整数的绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ============================================
// 页面类型检测方法
// ============================================

// containsAnyKeyword 检查文本是否包含任意关键词
func containsAnyKeyword(text string, keywords []string) bool {
	textLower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(textLower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// countKeywordMatches 统计关键词匹配数量
func countKeywordMatches(text string, keywords []string) int {
	textLower := strings.ToLower(text)
	count := 0
	for _, kw := range keywords {
		if strings.Contains(textLower, strings.ToLower(kw)) {
			count++
		}
	}
	return count
}

// detectPageType 检测页面类型
func detectPageType(uiXML string) PageType {
	xmlLower := strings.ToLower(uiXML)

	// 1. 检测系统级权限弹窗（仅限系统包名）
	// 注意：必须是系统权限控制器的包名，避免将应用内的隐私协议误判为权限弹窗
	isSystemPermission := strings.Contains(xmlLower, "com.android.permissioncontroller") ||
		strings.Contains(xmlLower, "com.android.packageinstaller") ||
		strings.Contains(xmlLower, "com.lbe.security") || // 某些厂商的权限管理
		strings.Contains(xmlLower, "com.miui.securitycenter") || // 小米
		strings.Contains(xmlLower, "com.huawei.systemmanager") || // 华为
		strings.Contains(xmlLower, "com.coloros.safecenter") // OPPO

	if isSystemPermission {
		return PageTypePermission
	}

	// 2. 检测用户协议/隐私政策页面（优先于其他检测）
	// 这类页面通常包含"隐私"、"协议"、"条款"等关键词，以及"同意并继续"按钮
	agreementScore := countKeywordMatches(xmlLower, agreementKeywords)
	if agreementScore >= 2 {
		return PageTypeAgreement
	}
	// 特殊情况：包含"同意并继续"或"我已阅读"的页面很可能是协议页面
	if containsAnyKeyword(xmlLower, []string{"同意并继续", "我已阅读并同意", "同意以上协议", "agree and continue"}) {
		return PageTypeAgreement
	}

	// 3. 检测更新弹窗
	if countKeywordMatches(xmlLower, updateKeywords) >= 2 {
		return PageTypeUpdate
	}

	// 4. 检测广告/开屏
	if containsAnyKeyword(xmlLower, adKeywords) {
		return PageTypeAd
	}

	// 5. 检测应用内的权限请求提示（非系统弹窗）
	// 这类页面通常是应用自己展示的权限说明，包含"权限"关键词
	if containsAnyKeyword(xmlLower, []string{"permission", "权限"}) &&
		containsAnyKeyword(xmlLower, permissionKeywords) {
		return PageTypePermission
	}

	// 5. 检测登录页面
	if countKeywordMatches(xmlLower, loginPageKeywords) >= 2 {
		return PageTypeLogin
	}

	// 6. 检测引导页
	if containsAnyKeyword(xmlLower, guideKeywords) ||
		strings.Contains(xmlLower, "viewpager") ||
		strings.Contains(xmlLower, "indicator") ||
		strings.Contains(xmlLower, "banner") {
		// 排除已经是主界面的情况
		if countKeywordMatches(xmlLower, mainUIKeywords) < 2 {
			return PageTypeGuide
		}
	}

	// 7. 检测主界面（降低阈值，增加检测特征）
	mainUIScore := countKeywordMatches(xmlLower, mainUIKeywords)

	// 检测底部导航栏（强特征，+3分）
	hasBottomNav := strings.Contains(xmlLower, "bottomnavigation") ||
		strings.Contains(xmlLower, "tablayout") ||
		strings.Contains(xmlLower, "navigation_bar") ||
		strings.Contains(xmlLower, "tab_layout") ||
		strings.Contains(xmlLower, "bottom_bar") ||
		strings.Contains(xmlLower, "main_tab") ||
		strings.Contains(xmlLower, "home_tab") ||
		strings.Contains(xmlLower, "tab_container")

	if hasBottomNav {
		mainUIScore += 3
	}

	// 检测内容列表（RecyclerView/ListView 有内容）
	hasContentList := strings.Contains(xmlLower, "recyclerview") ||
		strings.Contains(xmlLower, "listview") ||
		strings.Contains(xmlLower, "gridview")

	if hasContentList {
		mainUIScore += 1
	}

	// 有底部导航就基本确定是主界面
	if hasBottomNav && mainUIScore >= 3 {
		return PageTypeMainUI
	}

	// 没有底部导航，但有足够多的主界面特征
	if mainUIScore >= 3 {
		return PageTypeMainUI
	}

	return PageTypeUnknown
}

// isLoginPage 检测是否为登录页面
func isLoginPage(xmlLower string) bool {
	return countKeywordMatches(xmlLower, loginPageKeywords) >= 2
}

// hasGuidanceElements 检测页面是否包含引导类元素（弹窗/协议/权限等）
// 返回 true 表示页面有需要处理的引导元素
func hasGuidanceElements(uiXML string) bool {
	xmlLower := strings.ToLower(uiXML)

	// 检测系统权限弹窗
	if strings.Contains(xmlLower, "com.android.permissioncontroller") ||
		strings.Contains(xmlLower, "com.android.packageinstaller") ||
		strings.Contains(xmlLower, "com.lbe.security") ||
		strings.Contains(xmlLower, "com.miui.securitycenter") ||
		strings.Contains(xmlLower, "com.huawei.systemmanager") ||
		strings.Contains(xmlLower, "com.coloros.safecenter") {
		return true
	}

	// 检测协议/隐私相关
	if countKeywordMatches(xmlLower, agreementKeywords) >= 2 {
		return true
	}
	if containsAnyKeyword(xmlLower, []string{"同意并继续", "我已阅读并同意", "同意以上协议"}) {
		return true
	}

	// 检测更新弹窗
	if countKeywordMatches(xmlLower, updateKeywords) >= 2 {
		return true
	}

	// 检测广告
	if containsAnyKeyword(xmlLower, adKeywords) {
		return true
	}

	// 检测登录页面（登录弹窗或强制登录）
	if countKeywordMatches(xmlLower, loginPageKeywords) >= 3 {
		return true
	}

	// 检测引导页（明显的引导元素）
	if containsAnyKeyword(xmlLower, []string{"开始体验", "立即体验", "开始使用", "下一步", "进入应用"}) {
		return true
	}

	return false
}

// isUsablePage 检测页面是否已经可以正常使用（不需要继续引导）
func isUsablePage(uiXML string) bool {
	xmlLower := strings.ToLower(uiXML)

	// 1. 有底部导航栏 - 强信号
	hasBottomNav := strings.Contains(xmlLower, "bottomnavigation") ||
		strings.Contains(xmlLower, "tablayout") ||
		strings.Contains(xmlLower, "navigation_bar") ||
		strings.Contains(xmlLower, "tab_layout") ||
		strings.Contains(xmlLower, "bottom_bar") ||
		strings.Contains(xmlLower, "main_tab") ||
		strings.Contains(xmlLower, "home_tab")

	// 2. 有内容列表 - 中信号
	hasContentList := strings.Contains(xmlLower, "recyclerview") ||
		strings.Contains(xmlLower, "listview") ||
		strings.Contains(xmlLower, "gridview")

	// 3. 有多个主界面关键词 - 中信号
	mainUIScore := countKeywordMatches(xmlLower, mainUIKeywords)

	// 4. 没有引导元素
	noGuidance := !hasGuidanceElements(uiXML)

	// 判断条件：
	// - 有底部导航 + 无引导元素 → 可用
	// - 有内容列表 + 主界面关键词 >= 2 + 无引导元素 → 可用
	if hasBottomNav && noGuidance {
		return true
	}

	if hasContentList && mainUIScore >= 2 && noGuidance {
		return true
	}

	return false
}

// ============================================
// 登录页面处理方法
// ============================================

// handleLoginPage 处理登录页面（多策略）
func (o *Orchestrator) handleLoginPage(ctx context.Context, uiXML string, packageName string, adbClient *adb.Client) LoginPageStrategy {
	result := LoginPageStrategy{}
	xmlLower := strings.ToLower(uiXML)

	o.logger.Info("开始分析登录页面，寻找绕过方式")

	// ========== 策略1: 查找跳过/游客选项（最优先） ==========
	o.logger.Debug("策略1: 查找跳过/游客选项")
	for _, kw := range skipLoginKeywords {
		if elem := findClickableElementByText(uiXML, kw); elem != nil {
			o.logger.WithField("keyword", kw).Info("找到跳过登录按钮")
			result.CanBypass = true
			result.BypassMethod = "skip_button"
			result.Actions = []ai.Action{{
				Type:     "click",
				X:        elem.Center[0],
				Y:        elem.Center[1],
				Reason:   fmt.Sprintf("点击跳过登录: %s", kw),
				Priority: 14,
			}}
			return result
		}
	}

	// ========== 策略2: 查找关闭按钮（弹窗式登录） ==========
	o.logger.Debug("策略2: 查找关闭按钮")
	for _, pattern := range closeButtonPatterns {
		if elem := findElementByPattern(uiXML, pattern); elem != nil {
			// 验证是在右上角区域（x > 屏幕宽度70%，y < 屏幕高度20%）
			if elem.Center[0] > 750 && elem.Center[1] < 400 {
				o.logger.Info("找到右上角关闭按钮")
				result.CanBypass = true
				result.BypassMethod = "close_button"
				result.Actions = []ai.Action{{
					Type:     "click",
					X:        elem.Center[0],
					Y:        elem.Center[1],
					Reason:   "点击关闭按钮关闭登录弹窗",
					Priority: 13,
				}}
				return result
			}
		}
	}

	// ========== 策略3: 查找其他入口（底部Tab/侧边栏） ==========
	o.logger.Debug("策略3: 查找底部Tab入口")
	if strings.Contains(xmlLower, "bottomnavigation") ||
		strings.Contains(xmlLower, "tablayout") ||
		strings.Contains(xmlLower, "navigation_bar") ||
		strings.Contains(xmlLower, "tab_layout") {

		// 寻找非"我的"的Tab（"我的"通常需要登录）
		tabKeywords := []string{"首页", "home", "发现", "discover", "推荐", "热门", "广场", "社区", "商城", "分类"}
		for _, kw := range tabKeywords {
			if elem := findClickableElementByText(uiXML, kw); elem != nil {
				// 验证在底部区域（y > 屏幕高度80%）
				if elem.Center[1] > 1800 {
					o.logger.WithField("tab", kw).Info("找到底部Tab入口")
					result.CanBypass = true
					result.BypassMethod = "bottom_tab"
					result.Actions = []ai.Action{{
						Type:     "click",
						X:        elem.Center[0],
						Y:        elem.Center[1],
						Reason:   fmt.Sprintf("点击底部Tab绕过登录: %s", kw),
						Priority: 12,
					}}
					return result
				}
			}
		}
	}

	// ========== 策略4: 尝试按返回键 ==========
	o.logger.Debug("策略4: 尝试按返回键")
	result.CanBypass = true
	result.BypassMethod = "back_button"
	result.Actions = []ai.Action{{
		Type:     "keyevent",
		Value:    "4", // KEYCODE_BACK
		Reason:   "按返回键尝试跳过登录",
		Priority: 10,
	}}

	// ========== 策略5: 本机号码一键登录（可选） ==========
	// 注意：这个策略需要在配置中启用
	oneClickKeywords := []string{
		"本机号码一键登录", "一键登录", "本机号码登录",
		"手机号一键登录", "运营商登录", "快捷登录",
	}

	for _, kw := range oneClickKeywords {
		if elem := findClickableElementByText(uiXML, kw); elem != nil {
			o.logger.WithField("method", kw).Info("找到一键登录选项")
			// 一键登录作为备选，不覆盖返回键策略
			// 如果返回键失败，可以考虑尝试一键登录
			break
		}
	}

	return result
}

// handleLoginPageWithRetry 带重试的登录页面处理
func (o *Orchestrator) handleLoginPageWithRetry(
	ctx context.Context,
	uiXML string,
	packageName string,
	adbClient *adb.Client,
	taskID string,
) (bool, string, error) {

	strategy := o.handleLoginPage(ctx, uiXML, packageName, adbClient)

	if !strategy.CanBypass {
		// 完全无法绕过
		o.logger.Warn("无法找到任何绕过登录的方式")
		return false, "no_bypass_option", nil
	}

	// 执行绕过操作
	for _, action := range strategy.Actions {
		if action.Type == "keyevent" {
			// 按键操作
			keycode := action.Value
			_, err := adbClient.Shell(ctx, fmt.Sprintf("input keyevent %s", keycode))
			if err != nil {
				o.logger.WithError(err).Warn("按键操作失败")
			}
		} else if action.Type == "click" {
			if err := adbClient.TapScreen(ctx, action.X, action.Y); err != nil {
				o.logger.WithError(err).Warn("点击操作失败")
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}

	// 等待页面变化
	time.Sleep(2 * time.Second)

	// 【关键】检查是否退出了应用（返回键可能导致退出）
	currentPkg, err := adbClient.GetForegroundPackage(ctx)
	if err == nil && currentPkg != packageName {
		// 退出了应用，这不是真正的"绕过"
		o.logger.WithFields(logrus.Fields{
			"method":      strategy.BypassMethod,
			"current_pkg": currentPkg,
			"target_pkg":  packageName,
		}).Warn("绕过登录失败，操作导致应用退出")
		return false, strategy.BypassMethod + "_app_exited", nil
	}

	// 验证是否成功绕过
	newUIXML, err := o.dumpUIHierarchy(ctx, adbClient)
	if err != nil {
		return false, strategy.BypassMethod, err
	}

	newPageType := detectPageType(newUIXML)

	if newPageType == PageTypeLogin || isLoginPage(strings.ToLower(newUIXML)) {
		// 仍然在登录页
		o.logger.WithField("method", strategy.BypassMethod).Warn("绕过登录失败，仍在登录页")

		if strategy.BypassMethod == "back_button" {
			// 返回键失败，标记为强制登录
			return false, strategy.BypassMethod + "_failed", nil
		}
		return false, strategy.BypassMethod + "_failed", nil
	}

	// 成功绕过
	o.logger.WithField("method", strategy.BypassMethod).Info("成功绕过登录页面")
	return true, strategy.BypassMethod, nil
}

// ============================================
// 辅助方法
// ============================================

// getLauncherActivity 获取应用的启动 Activity
func (o *Orchestrator) getLauncherActivity(ctx context.Context, packageName string, adbClient *adb.Client) (string, error) {
	// 方法1: 使用 dumpsys 获取
	cmd := fmt.Sprintf("dumpsys package %s | grep -A 10 'android.intent.action.MAIN' | grep -E 'Activity|activity'", packageName)
	output, err := adbClient.Shell(ctx, cmd)
	if err == nil && output != "" {
		// 解析输出，找到 Activity 名称
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, packageName) && strings.Contains(line, "Activity") {
				// 提取 Activity 名称
				parts := strings.Split(line, "/")
				if len(parts) >= 2 {
					activityName := strings.TrimSpace(parts[1])
					activityName = strings.Split(activityName, " ")[0]
					if activityName != "" {
						return activityName, nil
					}
				}
			}
		}
	}

	// 方法2: 使用 pm dump 获取
	cmd2 := fmt.Sprintf("pm dump %s | grep -A 5 'android.intent.category.LAUNCHER'", packageName)
	output2, err := adbClient.Shell(ctx, cmd2)
	if err == nil && output2 != "" {
		re := regexp.MustCompile(`([a-zA-Z0-9_.]+Activity)`)
		matches := re.FindStringSubmatch(output2)
		if len(matches) > 0 {
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("未找到 Launcher Activity")
}

// recoverToApp 恢复到目标应用
func (o *Orchestrator) recoverToApp(ctx context.Context, packageName string, adbClient *adb.Client) error {
	o.logger.WithField("package", packageName).Info("尝试恢复到目标应用")

	// 尝试1: 按返回键
	_, _ = adbClient.Shell(ctx, "input keyevent 4")
	time.Sleep(1 * time.Second)

	// 检查是否恢复
	currentPkg, _ := adbClient.GetForegroundPackage(ctx)
	if currentPkg == packageName {
		o.logger.Info("通过返回键恢复成功")
		return nil
	}

	// 尝试2: 使用 monkey 重新启动
	_, err := adbClient.Shell(ctx, fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", packageName))
	if err != nil {
		return fmt.Errorf("无法恢复到应用: %w", err)
	}

	time.Sleep(2 * time.Second)
	return nil
}

// dumpUIHierarchy 获取 UI 层级 XML
func (o *Orchestrator) dumpUIHierarchy(ctx context.Context, adbClient *adb.Client) (string, error) {
	// 执行 uiautomator dump
	_, err := adbClient.Shell(ctx, "uiautomator dump /sdcard/window_dump.xml")
	if err != nil {
		return "", fmt.Errorf("dump UI 失败: %w", err)
	}

	// 读取 XML 内容
	xmlContent, err := adbClient.Shell(ctx, "cat /sdcard/window_dump.xml")
	if err != nil {
		return "", fmt.Errorf("读取 UI XML 失败: %w", err)
	}

	return xmlContent, nil
}

// takeGuidanceScreenshot 截图（引导阶段专用）
func (o *Orchestrator) takeGuidanceScreenshot(ctx context.Context, savePath string, adbClient *adb.Client) error {
	// 确保目录存在
	dir := filepath.Dir(savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 使用 ADB 客户端的截图方法
	return adbClient.Screenshot(ctx, savePath)
}

// hashUIXML 计算 UI XML 的简单哈希（用于检测页面变化）
func hashUIXML(xml string) string {
	// 移除动态内容，只保留结构
	re := regexp.MustCompile(`bounds="\[[^\]]+\]"`)
	normalized := re.ReplaceAllString(xml, "")

	// 移除时间戳等
	re2 := regexp.MustCompile(`\d{2}:\d{2}`)
	normalized = re2.ReplaceAllString(normalized, "")

	hash := md5.Sum([]byte(normalized))
	return fmt.Sprintf("%x", hash)[:16]
}

// executeGuidanceAction 执行引导操作
func (o *Orchestrator) executeGuidanceAction(ctx context.Context, action ai.Action, adbClient *adb.Client) error {
	o.logger.WithFields(logrus.Fields{
		"type":   action.Type,
		"reason": action.Reason,
	}).Debug("执行引导操作")

	switch action.Type {
	case "click":
		return adbClient.TapScreen(ctx, action.X, action.Y)

	case "keyevent":
		_, err := adbClient.Shell(ctx, fmt.Sprintf("input keyevent %s", action.Value))
		return err

	case "scroll":
		var cmd string
		switch action.Direction {
		case "left":
			cmd = "input swipe 800 1200 200 1200 300"
		case "right":
			cmd = "input swipe 200 1200 800 1200 300"
		case "up":
			cmd = "input swipe 540 500 540 1500 300"
		case "down":
			cmd = "input swipe 540 1500 540 500 300"
		default:
			cmd = "input swipe 540 1500 540 500 300" // 默认向下
		}
		_, err := adbClient.Shell(ctx, cmd)
		return err

	case "back":
		// 按返回键 (keyevent 4)
		return adbClient.PressBack(ctx)

	default:
		return fmt.Errorf("未知操作类型: %s", action.Type)
	}
}

// ============================================
// 主引导方法
// ============================================

// analyzeGuidancePage 分析引导页面并生成操作
func (o *Orchestrator) analyzeGuidancePage(
	ctx context.Context,
	uiXML string,
	packageName string,
	adbClient *adb.Client,
	taskID string,
) (PageType, []ai.Action, bool) {
	xmlLower := strings.ToLower(uiXML)
	var actions []ai.Action
	shouldContinue := true

	pageType := detectPageType(uiXML)

	// 详细日志：用于调试页面识别
	agreementScore := countKeywordMatches(xmlLower, agreementKeywords)
	permissionScore := countKeywordMatches(xmlLower, permissionKeywords)
	loginScore := countKeywordMatches(xmlLower, loginPageKeywords)

	o.logger.WithFields(logrus.Fields{
		"page_type":        pageType.String(),
		"agreement_score":  agreementScore,
		"permission_score": permissionScore,
		"login_score":      loginScore,
	}).Info("检测到页面类型")

	switch pageType {
	case PageTypePermission:
		// 权限弹窗：点击允许
		for _, kw := range permissionKeywords {
			if elem := findClickableElementByText(uiXML, kw); elem != nil {
				actions = append(actions, ai.Action{
					Type:     "click",
					X:        elem.Center[0],
					Y:        elem.Center[1],
					Reason:   fmt.Sprintf("点击权限允许: %s", kw),
					Priority: 16,
				})
				return pageType, actions[:1], true // 权限弹窗只返回一个操作
			}
		}

	case PageTypeAgreement:
		// 用户协议页面：点击"同意"按钮或勾选复选框
		// 注意：不要点击"隐私政策"、"用户协议"等超链接文案，那些会跳转到详情页

		// 首先检查是否是年龄验证/监护人确认页面
		// 这类页面有：监护人同意、已满14岁、游客模式 等按钮
		ageVerifyKeywords := []string{
			"游客模式", "游客登录", "游客", "guest",
			"已满14岁", "已满14", "已满16岁", "已满18岁",
			"我已成年", "已成年", "我已满",
			"进入体验", "立即体验", "开始体验",
		}

		for _, kw := range ageVerifyKeywords {
			if elem := findClickableElementByText(uiXML, kw); elem != nil {
				actions = append(actions, ai.Action{
					Type:     "click",
					X:        elem.Center[0],
					Y:        elem.Center[1],
					Reason:   fmt.Sprintf("年龄验证/游客模式: %s", kw),
					Priority: 17, // 最高优先级
				})
				o.logger.WithFields(logrus.Fields{
					"keyword": kw,
					"x":       elem.Center[0],
					"y":       elem.Center[1],
				}).Info("找到年龄验证/游客按钮")
				return pageType, actions[:1], true
			}
		}

		// 查找"监护人同意"按钮（次优先）
		guardianKeywords := []string{"监护人同意", "家长同意", "监护人确认"}
		for _, kw := range guardianKeywords {
			if elem := findClickableElementByText(uiXML, kw); elem != nil {
				actions = append(actions, ai.Action{
					Type:     "click",
					X:        elem.Center[0],
					Y:        elem.Center[1],
					Reason:   fmt.Sprintf("监护人确认: %s", kw),
					Priority: 16,
				})
				o.logger.WithFields(logrus.Fields{
					"keyword": kw,
					"x":       elem.Center[0],
					"y":       elem.Center[1],
				}).Info("找到监护人同意按钮")
				return pageType, actions[:1], true
			}
		}

		// 优先查找明确的同意按钮（完整文案，避免匹配到链接中的"同意"）
		explicitAgreeKeywords := []string{
			"同意并继续", "我同意", "同意以上协议", "我已阅读并同意",
			"agree and continue", "i agree", "accept",
			"确定", "好的", "进入",
		}

		for _, kw := range explicitAgreeKeywords {
			if elem := findClickableElementByText(uiXML, kw); elem != nil {
				actions = append(actions, ai.Action{
					Type:     "click",
					X:        elem.Center[0],
					Y:        elem.Center[1],
					Reason:   fmt.Sprintf("同意协议: %s", kw),
					Priority: 15,
				})
				o.logger.WithFields(logrus.Fields{
					"keyword": kw,
					"x":       elem.Center[0],
					"y":       elem.Center[1],
				}).Info("找到明确的同意按钮")
				return pageType, actions[:1], true
			}
		}

		// 【重要】首先检测是否是登录页面
		// 登录页面特征：有登录/试用/注册等按钮，底部有协议复选框
		// 登录页面不应该由引导阶段处理，应该交给 AI 交互阶段
		isLoginPage := containsAnyKeyword(xmlLower, []string{
			"试用", "微信登录", "qq登录", "手机号登录", "验证码登录",
			"其他方式登录", "一键登录",
		})

		if isLoginPage {
			o.logger.Info("检测到登录页面，跳过引导操作，交给AI交互处理")
			return pageType, nil, false // 返回空操作，让引导阶段结束
		}

		// 非登录页面的协议页面：查找复选框并点击
		// 方法1：通过class/id名称匹配
		checkboxPatterns := []string{
			`class="[^"]*CheckBox[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`,
			`resource-id="[^"]*checkbox[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`,
			`resource-id="[^"]*check[^"]*"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`,
		}

		for _, pattern := range checkboxPatterns {
			if elem := findElementByPattern(uiXML, pattern); elem != nil {
				actions = append(actions, ai.Action{
					Type:     "click",
					X:        elem.Center[0],
					Y:        elem.Center[1],
					Reason:   "勾选协议复选框",
					Priority: 14,
				})
				o.logger.WithFields(logrus.Fields{
					"x": elem.Center[0],
					"y": elem.Center[1],
				}).Info("找到协议复选框")
				return pageType, actions[:1], true
			}
		}

		// 方法2：查找协议文本附近的小型可点击元素（很可能是复选框）
		// 仅在纯协议页面（无登录按钮）时使用
		if elem := findCheckboxNearAgreementText(uiXML); elem != nil {
			actions = append(actions, ai.Action{
				Type:     "click",
				X:        elem.Center[0],
				Y:        elem.Center[1],
				Reason:   "勾选协议复选框（通过位置检测）",
				Priority: 14,
			})
			o.logger.WithFields(logrus.Fields{
				"x": elem.Center[0],
				"y": elem.Center[1],
			}).Info("找到协议复选框（通过位置检测）")
			return pageType, actions[:1], true
		}

		// 最后尝试：如果页面是隐私详情页（只有文本没有按钮），按返回键
		if !containsAnyKeyword(xmlLower, []string{"同意并继续", "我同意", "确定", "游客", "监护人", "登录", "注册"}) {
			o.logger.Info("可能是隐私详情页，尝试返回")
			actions = append(actions, ai.Action{
				Type:     "back",
				Reason:   "从隐私详情页返回",
				Priority: 10,
			})
			return pageType, actions[:1], true
		}

	case PageTypeUpdate:
		// 更新弹窗：点击稍后/取消
		updateSkipKeywords := []string{"稍后更新", "暂不更新", "以后再说", "取消", "暂不升级", "下次再说"}
		for _, kw := range updateSkipKeywords {
			if elem := findClickableElementByText(uiXML, kw); elem != nil {
				actions = append(actions, ai.Action{
					Type:     "click",
					X:        elem.Center[0],
					Y:        elem.Center[1],
					Reason:   fmt.Sprintf("跳过更新: %s", kw),
					Priority: 14,
				})
				return pageType, actions[:1], true
			}
		}

	case PageTypeAd:
		// 广告：点击跳过
		adSkipKeywords := []string{"跳过", "skip", "关闭", "×"}
		for _, kw := range adSkipKeywords {
			if elem := findClickableElementByText(uiXML, kw); elem != nil {
				actions = append(actions, ai.Action{
					Type:     "click",
					X:        elem.Center[0],
					Y:        elem.Center[1],
					Reason:   fmt.Sprintf("跳过广告: %s", kw),
					Priority: 15,
				})
				return pageType, actions[:1], true
			}
		}
		// 等待广告自动结束
		time.Sleep(3 * time.Second)

	case PageTypeLogin:
		// 登录页面：使用多策略处理
		bypassed, method, err := o.handleLoginPageWithRetry(ctx, uiXML, packageName, adbClient, taskID)
		if err != nil {
			o.logger.WithError(err).Warn("处理登录页面出错")
		}

		if !bypassed {
			// 无法绕过登录
			o.logger.WithField("method", method).Warn("无法绕过登录页面，结束智能引导")
			shouldContinue = false
			return pageType, nil, shouldContinue
		}

		// 成功绕过，返回空操作（操作已在 handleLoginPageWithRetry 中执行）
		return pageType, nil, true

	case PageTypeGuide:
		// 引导页：寻找按钮或滑动
		for _, kw := range guideKeywords {
			if elem := findClickableElementByText(uiXML, kw); elem != nil {
				actions = append(actions, ai.Action{
					Type:     "click",
					X:        elem.Center[0],
					Y:        elem.Center[1],
					Reason:   fmt.Sprintf("点击引导按钮: %s", kw),
					Priority: 12,
				})
				return pageType, actions, true
			}
		}
		// 没有明确按钮，尝试滑动
		actions = append(actions, ai.Action{
			Type:      "scroll",
			Direction: "left",
			Reason:    "滑动引导页",
			Priority:  10,
		})
		return pageType, actions, true

	case PageTypeMainUI:
		// 已到主界面，结束引导
		shouldContinue = false
		return pageType, nil, shouldContinue

	default:
		// 未知页面：尝试查找通用的同意/继续按钮
		for _, kw := range agreeKeywords {
			if elem := findClickableElementByText(uiXML, kw); elem != nil {
				if !containsAnyKeyword(strings.ToLower(kw), forbiddenKeywords) {
					actions = append(actions, ai.Action{
						Type:     "click",
						X:        elem.Center[0],
						Y:        elem.Center[1],
						Reason:   fmt.Sprintf("点击通用按钮: %s", kw),
						Priority: 8,
					})
					break
				}
			}
		}
	}

	// 如果有滚动区域，添加滚动操作
	if strings.Contains(xmlLower, "scrollview") || strings.Contains(xmlLower, "recyclerview") {
		if len(actions) == 0 {
			actions = append(actions, ai.Action{
				Type:      "scroll",
				Direction: "down",
				Reason:    "向下滚动探索",
				Priority:  5,
			})
		}
	}

	return pageType, actions, shouldContinue
}

// runAppLaunchGuidance 启动应用并完成智能引导
func (o *Orchestrator) runAppLaunchGuidance(
	ctx context.Context,
	taskID string,
	packageName string,
	adbClient *adb.Client,
) (*GuidanceResult, error) {
	startTime := time.Now()
	result := &GuidanceResult{
		PagesEncountered: []string{},
	}

	config := DefaultGuidanceConfig()

	o.logger.WithFields(logrus.Fields{
		"task_id":      taskID,
		"package_name": packageName,
		"max_rounds":   config.MaxRounds,
	}).Info("开始启动应用并执行智能引导")

	// 1. 获取主 Activity 并启动应用
	launcherActivity, err := o.getLauncherActivity(ctx, packageName, adbClient)
	if err != nil {
		o.logger.WithError(err).Warn("无法获取 Launcher Activity，尝试使用 monkey 启动")
		_, err = adbClient.Shell(ctx, fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", packageName))
		if err != nil {
			return result, fmt.Errorf("启动应用失败: %w", err)
		}
	} else {
		o.logger.WithField("launcher", launcherActivity).Info("找到 Launcher Activity")
		component := fmt.Sprintf("%s/%s", packageName, launcherActivity)
		if err := adbClient.StartActivity(ctx, component); err != nil {
			// 启动失败，尝试 monkey
			o.logger.WithError(err).Warn("启动 Activity 失败，尝试 monkey")
			_, _ = adbClient.Shell(ctx, fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", packageName))
		}
	}

	// 等待应用启动
	time.Sleep(3 * time.Second)

	// 2. 智能引导循环
	var lastUIHash string
	stableCount := 0           // 页面稳定计数（兜底机制）
	noGuidanceCount := 0       // 连续无引导元素计数
	const noGuidanceThreshold = 2 // 连续2轮无引导元素则结束

	taskDir := filepath.Join(o.resultsDir, taskID)
	guidanceDir := filepath.Join(taskDir, "guidance")

	// 创建引导截图目录
	if config.SaveScreenshots {
		_ = os.MkdirAll(guidanceDir, 0755)
	}

	for round := 1; round <= config.MaxRounds; round++ {
		result.RoundsExecuted = round

		o.logger.WithField("round", round).Info("智能引导轮次")

		// 2.1 检查是否在目标应用中
		currentPackage, err := adbClient.GetForegroundPackage(ctx)
		if err == nil && currentPackage != packageName {
			o.logger.WithFields(logrus.Fields{
				"current": currentPackage,
				"target":  packageName,
			}).Warn("应用已退出，尝试恢复")

			if err := o.recoverToApp(ctx, packageName, adbClient); err != nil {
				o.logger.WithError(err).Error("无法恢复到目标应用")
				break
			}
			time.Sleep(2 * time.Second)
			noGuidanceCount = 0 // 恢复后重置计数
			continue
		}

		// 2.2 截图（用于记录）
		if config.SaveScreenshots {
			screenshotPath := filepath.Join(guidanceDir, fmt.Sprintf("round_%02d.png", round))
			if err := o.takeGuidanceScreenshot(ctx, screenshotPath, adbClient); err != nil {
				o.logger.WithError(err).Debug("保存引导截图失败")
			}
		}

		// 2.3 获取 UI 层级
		uiXML, err := o.dumpUIHierarchy(ctx, adbClient)
		if err != nil {
			o.logger.WithError(err).Warn("Dump UI 失败")
			time.Sleep(2 * time.Second)
			continue
		}

		// 2.4 【新增】快速检测：页面是否已可正常使用
		if isUsablePage(uiXML) {
			o.logger.Info("检测到页面已可正常使用（有内容+无弹窗），引导完成")
			result.Success = true
			result.FinalPageType = PageTypeMainUI
			break
		}

		// 2.5 【新增】检测是否有引导元素
		hasGuidance := hasGuidanceElements(uiXML)
		if !hasGuidance {
			noGuidanceCount++
			o.logger.WithField("no_guidance_count", noGuidanceCount).Debug("当前页面无引导元素")

			// 连续N轮无引导元素，认为引导完成
			if noGuidanceCount >= noGuidanceThreshold {
				o.logger.WithField("threshold", noGuidanceThreshold).Info("连续多轮无引导元素，引导完成")
				result.Success = true
				result.FinalPageType = PageTypeMainUI
				break
			}
		} else {
			noGuidanceCount = 0 // 有引导元素，重置计数
		}

		// 2.6 计算 UI Hash，检测页面是否变化（兜底机制）
		currentHash := hashUIXML(uiXML)
		if currentHash == lastUIHash {
			stableCount++
			o.logger.WithField("stable_count", stableCount).Debug("页面未变化")
			// 稳定性阈值提高到5，作为兜底
			if stableCount >= 5 {
				o.logger.Info("页面长时间稳定，引导完成（兜底）")
				result.Success = true
				break
			}
		} else {
			stableCount = 0
			lastUIHash = currentHash
		}

		// 2.7 分析页面并生成操作
		pageType, actions, shouldContinue := o.analyzeGuidancePage(ctx, uiXML, packageName, adbClient, taskID)

		result.PagesEncountered = append(result.PagesEncountered, pageType.String())
		result.FinalPageType = pageType

		o.logger.WithFields(logrus.Fields{
			"page_type":       pageType.String(),
			"action_count":    len(actions),
			"should_continue": shouldContinue,
			"has_guidance":    hasGuidance,
		}).Info("页面分析结果")

		// 2.8 检查是否应该继续
		if !shouldContinue {
			if pageType == PageTypeMainUI {
				result.Success = true
				o.logger.Info("已进入主界面，引导完成")
			} else if pageType == PageTypeLogin {
				result.LoginRequired = true
				o.logger.Warn("遇到强制登录页面，结束引导")
			}
			break
		}

		// 2.9 执行操作
		if len(actions) > 0 {
			for _, action := range actions {
				o.logger.WithFields(logrus.Fields{
					"type":   action.Type,
					"reason": action.Reason,
				}).Info("执行引导操作")

				if err := o.executeGuidanceAction(ctx, action, adbClient); err != nil {
					o.logger.WithError(err).Warn("执行操作失败")
				}
				time.Sleep(1 * time.Second)
			}
			// 执行了操作后重置稳定计数，因为页面可能会变化
			stableCount = 0
			noGuidanceCount = 0 // 执行操作后也重置无引导计数
		} else if pageType == PageTypeUnknown && hasGuidance {
			// 未知页面但有引导元素，尝试滑动
			o.logger.Debug("未知页面有引导元素，尝试左滑")
			_, _ = adbClient.Shell(ctx, "input swipe 800 1200 200 1200 300")
			stableCount = 0
		}
		// 如果是未知页面且无引导元素，不做任何操作，等待下一轮检测

		time.Sleep(2 * time.Second)
	}

	result.Duration = time.Since(startTime)

	o.logger.WithFields(logrus.Fields{
		"success":        result.Success,
		"rounds":         result.RoundsExecuted,
		"final_page":     result.FinalPageType.String(),
		"login_required": result.LoginRequired,
		"duration_ms":    result.Duration.Milliseconds(),
	}).Info("智能引导阶段完成")

	return result, nil
}
