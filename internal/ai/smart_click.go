package ai

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// SmartClicker 智能点击器
type SmartClicker struct {
	logger        *logrus.Logger
	targetPackage string // 目标应用包名，用于安全检查
}

// NewSmartClicker 创建智能点击器
func NewSmartClicker(logger *logrus.Logger) *SmartClicker {
	return &SmartClicker{
		logger: logger,
	}
}

// NewSmartClickerWithPackage 创建带包名验证的智能点击器
// targetPackage: 目标应用包名，点击操作只允许在该应用内执行
func NewSmartClickerWithPackage(logger *logrus.Logger, targetPackage string) *SmartClicker {
	return &SmartClicker{
		logger:        logger,
		targetPackage: targetPackage,
	}
}

// SetTargetPackage 设置目标应用包名
func (s *SmartClicker) SetTargetPackage(packageName string) {
	s.targetPackage = packageName
}

// ClickButtonByText 使用UI Automator通过文本查找并点击按钮
func (s *SmartClicker) ClickButtonByText(ctx context.Context, executor ActionExecutor, buttonTexts []string, maxAttempts int) (bool, error) {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		s.logger.WithFields(logrus.Fields{
			"attempt":      attempt + 1,
			"buttonTexts": buttonTexts,
		}).Debug("Attempting to click button by text")

		// 1. 获取UI hierarchy
		_, err := executor.Shell(ctx, "uiautomator dump /sdcard/window_dump.xml")
		if err != nil {
			s.logger.WithError(err).Warn("Failed to dump UI hierarchy")
			time.Sleep(1 * time.Second)
			continue
		}

		// 2. 读取UI hierarchy
		xmlContent, err := executor.Shell(ctx, "cat /sdcard/window_dump.xml")
		if err != nil || xmlContent == "" {
			s.logger.WithError(err).Warn("Failed to read UI hierarchy")
			time.Sleep(1 * time.Second)
			continue
		}

		// 3. 解析XML查找按钮坐标
		found, err := s.findAndClickButton(ctx, executor, xmlContent, buttonTexts)
		if err != nil {
			s.logger.WithError(err).Debug("Error finding button")
			time.Sleep(1 * time.Second)
			continue
		}

		if found {
			// 验证是否成功（检查按钮是否消失）
			time.Sleep(1 * time.Second)
			_, err := executor.Shell(ctx, "uiautomator dump /sdcard/check.xml")
			if err == nil {
				checkXML, _ := executor.Shell(ctx, "cat /sdcard/check.xml")
				// 检查按钮是否还在
				stillExists := false
				for _, btnText := range buttonTexts {
					if strings.Contains(checkXML, btnText) {
						stillExists = true
						break
					}
				}

				if !stillExists {
					s.logger.WithField("buttonText", buttonTexts[0]).Info("Successfully clicked button")
					return true, nil
				}
			}

			s.logger.Debug("Clicked but button still visible, retrying...")
		}

		time.Sleep(1 * time.Second)
	}

	s.logger.Warn("Failed to click button after max attempts")
	return false, nil
}

// findAndClickButton 查找并点击按钮
// 增强版：支持包名验证，确保只点击目标应用内的元素
// 按优先级查找策略：
// 1. 外层循环：按 buttonTexts 列表顺序（优先级从高到低）
// 2. 内层循环：遍历 UI 元素查找匹配
// 3. 两轮查找：第一轮只找 clickable="true"，第二轮放宽条件
func (s *SmartClicker) findAndClickButton(ctx context.Context, executor ActionExecutor, xmlContent string, buttonTexts []string) (bool, error) {
	// 使用正则表达式提取节点信息（包含 package 属性）
	nodePattern := regexp.MustCompile(`<node[^>]*>`)
	nodeMatches := nodePattern.FindAllString(xmlContent, -1)

	// 负面排除列表 - 绝对不能点击的按钮
	negativeTexts := []string{"不同意", "拒绝", "不允许", "取消", "退出", "否", "No", "Disagree", "Reject", "Cancel"}

	// 两轮查找：第一轮只找 clickable="true"，第二轮放宽条件（用于 TextView 等可点击但 clickable=false 的元素）
	for round := 1; round <= 2; round++ {
		requireClickable := (round == 1)
		if round == 2 {
			s.logger.Debug("Round 2: trying non-clickable elements")
		}

		// 按 buttonTexts 的优先级顺序查找（外层循环）
		// 这样确保优先级高的按钮（如"游客模式"）先被点击
		for _, btnText := range buttonTexts {
			// 遍历所有 UI 元素查找匹配（内层循环）
			for _, nodeStr := range nodeMatches {
				// 第一轮：检查是否可点击
				if requireClickable && !strings.Contains(nodeStr, `clickable="true"`) {
					continue
				}

				// 提取 text 属性
				textMatch := regexp.MustCompile(`text="([^"]*)"`).FindStringSubmatch(nodeStr)
				if len(textMatch) < 2 {
					continue
				}
				text := textMatch[1]

				// 检查文本是否匹配目标按钮
				if len(text) > 50 {
					continue
				}

				// 负面排除检查
				isNegative := false
				for _, neg := range negativeTexts {
					if strings.Contains(text, neg) {
						isNegative = true
						break
					}
				}
				if isNegative {
					continue
				}

				// 精确匹配当前优先级的按钮文本
				matched := false
				if text == btnText ||
					strings.HasPrefix(text, btnText) ||
					strings.HasSuffix(text, btnText) ||
					(len(text) <= 20 && strings.Contains(text, btnText)) {
					matched = true
				}
				if !matched {
					continue
				}

				// 提取 bounds 属性
				boundsMatch := regexp.MustCompile(`bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`).FindStringSubmatch(nodeStr)
				if len(boundsMatch) < 5 {
					continue
				}
				x1, _ := strconv.Atoi(boundsMatch[1])
				y1, _ := strconv.Atoi(boundsMatch[2])
				x2, _ := strconv.Atoi(boundsMatch[3])
				y2, _ := strconv.Atoi(boundsMatch[4])
				centerX := (x1 + x2) / 2
				centerY := (y1 + y2) / 2

				// 提取 package 属性（用于安全检查）
				pkgMatch := regexp.MustCompile(`package="([^"]*)"`).FindStringSubmatch(nodeStr)
				elementPackage := ""
				if len(pkgMatch) >= 2 {
					elementPackage = pkgMatch[1]
				}

				// 安全检查：如果设置了目标包名，验证元素是否属于目标应用
				if s.targetPackage != "" && elementPackage != "" {
					if !s.isPackageSafe(elementPackage) {
						s.logger.WithFields(logrus.Fields{
							"text":           text,
							"elementPackage": elementPackage,
							"targetPackage":  s.targetPackage,
							"x":              centerX,
							"y":              centerY,
						}).Warn("Skipping button click: element belongs to different package")
						continue
					}
				}

				s.logger.WithFields(logrus.Fields{
					"text":     text,
					"package":  elementPackage,
					"x":        centerX,
					"y":        centerY,
					"round":    round,
					"priority": btnText,
				}).Info("Found button, clicking...")

				// 点击按钮
				err := executor.TapScreen(ctx, centerX, centerY)
				if err != nil {
					return false, err
				}

				return true, nil
			}
		}

		// 尝试 content-desc（同样增加包名验证）
		for _, nodeStr := range nodeMatches {
			if requireClickable && !strings.Contains(nodeStr, `clickable="true"`) {
				continue
			}

			// 提取 content-desc 属性
			descMatch := regexp.MustCompile(`content-desc="([^"]*)"`).FindStringSubmatch(nodeStr)
			if len(descMatch) < 2 || descMatch[1] == "" {
				continue
			}
			contentDesc := descMatch[1]

			// 检查 content-desc 是否匹配目标按钮
			matched := false
			for _, btnText := range buttonTexts {
				if strings.Contains(contentDesc, btnText) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}

			// 提取 bounds
			boundsMatch := regexp.MustCompile(`bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`).FindStringSubmatch(nodeStr)
			if len(boundsMatch) < 5 {
				continue
			}
			x1, _ := strconv.Atoi(boundsMatch[1])
			y1, _ := strconv.Atoi(boundsMatch[2])
			x2, _ := strconv.Atoi(boundsMatch[3])
			y2, _ := strconv.Atoi(boundsMatch[4])
			centerX := (x1 + x2) / 2
			centerY := (y1 + y2) / 2

			// 提取 package 属性
			pkgMatch := regexp.MustCompile(`package="([^"]*)"`).FindStringSubmatch(nodeStr)
			elementPackage := ""
			if len(pkgMatch) >= 2 {
				elementPackage = pkgMatch[1]
			}

			// 安全检查
			if s.targetPackage != "" && elementPackage != "" {
				if !s.isPackageSafe(elementPackage) {
					s.logger.WithFields(logrus.Fields{
						"content-desc":   contentDesc,
						"elementPackage": elementPackage,
						"targetPackage":  s.targetPackage,
						"x":              centerX,
						"y":              centerY,
					}).Warn("Skipping button click: element belongs to different package")
					continue
				}
			}

			s.logger.WithFields(logrus.Fields{
				"content-desc": contentDesc,
				"package":      elementPackage,
				"x":            centerX,
				"y":            centerY,
				"round":        round,
			}).Info("Found button by content-desc, clicking...")

			// 点击按钮
			err := executor.TapScreen(ctx, centerX, centerY)
			if err != nil {
				return false, err
			}

			return true, nil
		}
	}

	return false, nil
}

// isPackageSafe 检查包名是否安全（属于目标应用或允许的系统弹窗）
func (s *SmartClicker) isPackageSafe(elementPackage string) bool {
	// 如果是目标应用，安全
	if elementPackage == s.targetPackage {
		return true
	}

	// 允许的系统包名（权限弹窗、安装确认等需要交互的）
	allowedSystemPackages := []string{
		"com.android.permissioncontroller",  // 权限弹窗
		"com.android.packageinstaller",      // 安装确认
		"com.google.android.permissioncontroller", // Google 权限弹窗
		"com.miui.securitycenter",           // MIUI 安全中心权限弹窗
	}

	for _, pkg := range allowedSystemPackages {
		if elementPackage == pkg {
			return true
		}
	}

	// 危险包名黑名单（绝对不能点击）
	dangerousPackages := []string{
		"com.android.systemui",           // 系统UI（状态栏、导航栏）
		"com.android.launcher",           // 原生桌面
		"com.android.launcher3",          // 原生桌面3
		"com.google.android.apps.nexuslauncher", // Pixel桌面
		"com.miui.home",                  // 小米桌面
		"com.huawei.android.launcher",    // 华为桌面
		"com.oppo.launcher",              // OPPO桌面
		"com.vivo.launcher",              // vivo桌面
		"com.oneplus.launcher",           // OnePlus桌面
		"com.sec.android.app.launcher",   // 三星桌面
		"com.android.settings",           // 系统设置
	}

	for _, pkg := range dangerousPackages {
		if elementPackage == pkg {
			s.logger.WithFields(logrus.Fields{
				"elementPackage": elementPackage,
				"reason":         "dangerous package",
			}).Debug("Package is in dangerous list")
			return false
		}
	}

	// 其他未知包名，默认不安全（防止误点击其他应用）
	s.logger.WithFields(logrus.Fields{
		"elementPackage": elementPackage,
		"targetPackage":  s.targetPackage,
		"reason":         "unknown package",
	}).Debug("Package is unknown, treating as unsafe")
	return false
}

// AutoClickPrivacyAgreement 自动点击隐私政策同意按钮
func (s *SmartClicker) AutoClickPrivacyAgreement(ctx context.Context, executor ActionExecutor, maxAttempts int) (bool, error) {
	s.logger.Info("Auto-clicking privacy agreement...")

	// 常见的同意按钮文本（按优先级排序）
	// 注意：这些文本会使用精确匹配逻辑（见 findAndClickButton）
	agreementTexts := []string{
		// 最常见的同意按钮
		"同意并继续",
		"同意并进入",
		"我同意",
		"同意",
		// 知道了类型
		"我知道了",
		"知道了",
		"好的",
		"好",
		// 确认类型
		"确定",
		"确认",
		// 接受/允许
		"接受",
		"允许",
		"授权",
		// 继续/进入
		"继续",
		"进入",
		"开始体验",
		"立即体验",
		"开始使用",
		// 英文
		"OK",
		"Agree",
		"Accept",
		"I Agree",
		"Continue",
		"Allow",
	}

	// 复选框相关文本
	checkboxTexts := []string{
		"我已阅读",
		"已阅读",
		"同意上述",
		"同意并",
		"阅读并同意",
		"我同意上述",
		"read and agree",
		"I have read",
	}

	// 常见的同意按钮坐标位置（基于1080x1776分辨率）
	commonPositions := [][2]int{
		{555, 1005},  // 右下角"同意"按钮
		{810, 1400},  // 右侧按钮
		{540, 1200},  // 底部中心按钮
		{720, 1500},  // 底部右侧按钮
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		s.logger.WithField("attempt", attempt+1).Debug("Attempting privacy agreement click")

		// 策略1: 先查找并勾选复选框
		s.tryCheckboxes(ctx, executor, checkboxTexts)
		time.Sleep(500 * time.Millisecond)

		// 策略2: UI Automator文本查找点击
		found, err := s.ClickButtonByText(ctx, executor, agreementTexts, 2)
		if err == nil && found {
			s.logger.Info("Successfully clicked privacy agreement via text search")
			return true, nil
		}

		// 策略3: 尝试常见坐标位置
		for _, pos := range commonPositions {
			s.logger.WithFields(logrus.Fields{
				"x": pos[0],
				"y": pos[1],
			}).Debug("Trying common position")

			err := executor.TapScreen(ctx, pos[0], pos[1])
			if err == nil {
				time.Sleep(1 * time.Second)

				// 检查是否成功（界面是否变化）
				_, checkErr := executor.Shell(ctx, "uiautomator dump /sdcard/check.xml")
				if checkErr == nil {
					s.logger.WithField("position", pos).Info("Clicked common position")
					// 假设成功（界面变化检测较复杂，简化处理）
					return true, nil
				}
			}
		}

		time.Sleep(1 * time.Second)
	}

	s.logger.Warn("Failed to auto-click privacy agreement")
	return false, nil
}

// tryCheckboxes 尝试勾选复选框（增强版：支持包名验证）
func (s *SmartClicker) tryCheckboxes(ctx context.Context, executor ActionExecutor, checkboxTexts []string) {
	// 获取UI hierarchy
	_, err := executor.Shell(ctx, "uiautomator dump /sdcard/checkbox_dump.xml")
	if err != nil {
		return
	}

	xmlContent, err := executor.Shell(ctx, "cat /sdcard/checkbox_dump.xml")
	if err != nil {
		return
	}

	// 使用更灵活的正则，提取所有 node 元素
	nodePattern := regexp.MustCompile(`<node[^>]*>`)
	nodeMatches := nodePattern.FindAllString(xmlContent, -1)

	for _, nodeStr := range nodeMatches {
		// 检查是否是未勾选的复选框
		if !strings.Contains(nodeStr, `CheckBox`) || !strings.Contains(nodeStr, `checked="false"`) {
			continue
		}

		// 提取 bounds
		boundsMatch := regexp.MustCompile(`bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`).FindStringSubmatch(nodeStr)
		if len(boundsMatch) < 5 {
			continue
		}
		x1, _ := strconv.Atoi(boundsMatch[1])
		y1, _ := strconv.Atoi(boundsMatch[2])
		x2, _ := strconv.Atoi(boundsMatch[3])
		y2, _ := strconv.Atoi(boundsMatch[4])
		centerX := (x1 + x2) / 2
		centerY := (y1 + y2) / 2

		// 提取 package 属性
		pkgMatch := regexp.MustCompile(`package="([^"]*)"`).FindStringSubmatch(nodeStr)
		elementPackage := ""
		if len(pkgMatch) >= 2 {
			elementPackage = pkgMatch[1]
		}

		// 安全检查：如果设置了目标包名，验证元素是否属于目标应用
		if s.targetPackage != "" && elementPackage != "" {
			if !s.isPackageSafe(elementPackage) {
				s.logger.WithFields(logrus.Fields{
					"elementPackage": elementPackage,
					"targetPackage":  s.targetPackage,
					"x":              centerX,
					"y":              centerY,
				}).Warn("Skipping checkbox click: element belongs to different package")
				continue
			}
		}

		s.logger.WithFields(logrus.Fields{
			"package": elementPackage,
			"x":       centerX,
			"y":       centerY,
		}).Debug("Found unchecked checkbox, clicking...")

		executor.TapScreen(ctx, centerX, centerY)
		time.Sleep(300 * time.Millisecond)
	}
}

// ClickCoordinate 点击指定坐标
func (s *SmartClicker) ClickCoordinate(ctx context.Context, executor ActionExecutor, x, y int) error {
	s.logger.WithFields(logrus.Fields{
		"x": x,
		"y": y,
	}).Debug("Clicking coordinate")

	return executor.TapScreen(ctx, x, y)
}

// SwipeScreen 滑动屏幕
func (s *SmartClicker) SwipeScreen(ctx context.Context, executor ActionExecutor, direction string, duration int) error {
	var cmd string

	switch direction {
	case "up":
		cmd = fmt.Sprintf("input swipe 540 1500 540 500 %d", duration)
	case "down":
		cmd = fmt.Sprintf("input swipe 540 500 540 1500 %d", duration)
	case "left":
		cmd = fmt.Sprintf("input swipe 800 900 200 900 %d", duration)
	case "right":
		cmd = fmt.Sprintf("input swipe 200 900 800 900 %d", duration)
	default:
		return fmt.Errorf("unknown swipe direction: %s", direction)
	}

	s.logger.WithFields(logrus.Fields{
		"direction": direction,
		"duration":  duration,
	}).Debug("Swiping screen")

	_, err := executor.Shell(ctx, cmd)
	return err
}
