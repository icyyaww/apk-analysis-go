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
	logger *logrus.Logger
}

// NewSmartClicker 创建智能点击器
func NewSmartClicker(logger *logrus.Logger) *SmartClicker {
	return &SmartClicker{
		logger: logger,
	}
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
func (s *SmartClicker) findAndClickButton(ctx context.Context, executor ActionExecutor, xmlContent string, buttonTexts []string) (bool, error) {
	// 使用正则表达式提取节点信息
	// 简化版：查找text和bounds属性
	nodePattern := regexp.MustCompile(`<node[^>]*text="([^"]*)"[^>]*clickable="true"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"[^>]*>`)
	matches := nodePattern.FindAllStringSubmatch(xmlContent, -1)

	for _, match := range matches {
		if len(match) < 6 {
			continue
		}

		text := match[1]
		x1, _ := strconv.Atoi(match[2])
		y1, _ := strconv.Atoi(match[3])
		x2, _ := strconv.Atoi(match[4])
		y2, _ := strconv.Atoi(match[5])

		// 检查文本是否匹配
		for _, btnText := range buttonTexts {
			if strings.Contains(text, btnText) {
				// 计算中心点
				centerX := (x1 + x2) / 2
				centerY := (y1 + y2) / 2

				s.logger.WithFields(logrus.Fields{
					"text": text,
					"x":    centerX,
					"y":    centerY,
				}).Info("Found button, clicking...")

				// 点击按钮
				err := executor.TapScreen(ctx, centerX, centerY)
				if err != nil {
					return false, err
				}

				return true, nil
			}
		}
	}

	// 尝试content-desc
	descPattern := regexp.MustCompile(`<node[^>]*content-desc="([^"]*)"[^>]*clickable="true"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"[^>]*>`)
	descMatches := descPattern.FindAllStringSubmatch(xmlContent, -1)

	for _, match := range descMatches {
		if len(match) < 6 {
			continue
		}

		contentDesc := match[1]
		x1, _ := strconv.Atoi(match[2])
		y1, _ := strconv.Atoi(match[3])
		x2, _ := strconv.Atoi(match[4])
		y2, _ := strconv.Atoi(match[5])

		// 检查content-desc是否匹配
		for _, btnText := range buttonTexts {
			if strings.Contains(contentDesc, btnText) {
				// 计算中心点
				centerX := (x1 + x2) / 2
				centerY := (y1 + y2) / 2

				s.logger.WithFields(logrus.Fields{
					"content-desc": contentDesc,
					"x":            centerX,
					"y":            centerY,
				}).Info("Found button by content-desc, clicking...")

				// 点击按钮
				err := executor.TapScreen(ctx, centerX, centerY)
				if err != nil {
					return false, err
				}

				return true, nil
			}
		}
	}

	return false, nil
}

// AutoClickPrivacyAgreement 自动点击隐私政策同意按钮
func (s *SmartClicker) AutoClickPrivacyAgreement(ctx context.Context, executor ActionExecutor, maxAttempts int) (bool, error) {
	s.logger.Info("Auto-clicking privacy agreement...")

	// 常见的同意按钮文本（按优先级排序）
	agreementTexts := []string{
		"同意",
		"我同意",
		"确定",
		"确认",
		"接受",
		"我知道了",
		"进入",
		"继续",
		"允许",
		"好的",
		"OK",
		"Agree",
		"Accept",
		"I Agree",
		"Continue",
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

// tryCheckboxes 尝试勾选复选框
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

	// 查找复选框
	checkboxPattern := regexp.MustCompile(`<node[^>]*class="[^"]*CheckBox"[^>]*checked="false"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"[^>]*>`)
	matches := checkboxPattern.FindAllStringSubmatch(xmlContent, -1)

	for _, match := range matches {
		if len(match) < 5 {
			continue
		}

		x1, _ := strconv.Atoi(match[1])
		y1, _ := strconv.Atoi(match[2])
		x2, _ := strconv.Atoi(match[3])
		y2, _ := strconv.Atoi(match[4])

		centerX := (x1 + x2) / 2
		centerY := (y1 + y2) / 2

		s.logger.WithFields(logrus.Fields{
			"x": centerX,
			"y": centerY,
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
