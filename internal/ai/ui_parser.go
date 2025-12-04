package ai

import (
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// UIElement UI元素
type UIElement struct {
	Text       string   `json:"text"`
	ResourceID string   `json:"resource_id"`
	Class      string   `json:"class"`
	Package    string   `json:"package,omitempty"` // 元素所属包名（用于安全检查）
	Bounds     [4]int   `json:"bounds"`            // [left, top, right, bottom]
	Center     [2]int   `json:"center"`            // [x, y]
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

// UINode XML节点
type UINode struct {
	XMLName      xml.Name `xml:"node"`
	Text         string   `xml:"text,attr"`
	ResourceID   string   `xml:"resource-id,attr"`
	Class        string   `xml:"class,attr"`
	Package      string   `xml:"package,attr"` // 元素所属包名（用于安全检查）
	Clickable    string   `xml:"clickable,attr"`
	Scrollable   string   `xml:"scrollable,attr"`
	Bounds       string   `xml:"bounds,attr"`
	ContentDesc  string   `xml:"content-desc,attr"`
	Children     []UINode `xml:"node"`
}

// UIHierarchy UI层级（支持 <hierarchy> 根元素）
type UIHierarchy struct {
	XMLName  xml.Name `xml:"hierarchy"`
	Rotation string   `xml:"rotation,attr"`
	Nodes    []UINode `xml:"node"`
}

// ParseUIXML 解析UI XML文件
func ParseUIXML(xmlPath string) (*UIData, error) {
	// 读取文件
	data, err := os.ReadFile(xmlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read UI XML: %w", err)
	}

	uiData := &UIData{
		ClickableElements: []UIElement{},
		InputFields:       []UIElement{},
		ScrollableViews:   []UIElement{},
	}

	// 收集所有文本节点（用于后续关联标签）
	allTextViews := []UIElement{}

	// 遍历所有节点
	var traverse func(node UINode)
	traverse = func(node UINode) {
		elem := parseUIElement(node)
		if elem == nil {
			for _, child := range node.Children {
				traverse(child)
			}
			return
		}

		// 分类元素
		if elem.Clickable {
			uiData.ClickableElements = append(uiData.ClickableElements, *elem)
		}

		if strings.Contains(elem.Class, "EditText") {
			uiData.InputFields = append(uiData.InputFields, *elem)
		}

		if elem.Scrollable {
			uiData.ScrollableViews = append(uiData.ScrollableViews, *elem)
		}

		// 收集文本节点
		if elem.Text != "" && strings.Contains(elem.Class, "Text") {
			allTextViews = append(allTextViews, *elem)
		}

		// 递归处理子节点
		for _, child := range node.Children {
			traverse(child)
		}
	}

	// 尝试解析为 <hierarchy> 根元素（Android UIAutomator 标准格式）
	var hierarchy UIHierarchy
	if err := xml.Unmarshal(data, &hierarchy); err == nil && len(hierarchy.Nodes) > 0 {
		// 成功解析为 hierarchy，遍历所有根节点
		for _, rootNode := range hierarchy.Nodes {
			traverse(rootNode)
		}
	} else {
		// 回退：尝试解析为 <node> 根元素（兼容旧格式）
		var root UINode
		if err := xml.Unmarshal(data, &root); err != nil {
			return nil, fmt.Errorf("failed to parse UI XML (tried both hierarchy and node): %w", err)
		}
		traverse(root)
	}

	// 为Switch/CheckBox关联标签
	associateLabels(uiData.ClickableElements, allTextViews)

	return uiData, nil
}

// parseUIElement 解析单个UI元素
func parseUIElement(node UINode) *UIElement {
	bounds := parseBounds(node.Bounds)
	if bounds == nil {
		return nil
	}

	center := [2]int{
		(bounds[0] + bounds[2]) / 2,
		(bounds[1] + bounds[3]) / 2,
	}

	// 获取文本：优先使用节点自身的text，如果为空则递归查找子节点的text
	text := strings.TrimSpace(node.Text)
	if text == "" && node.ContentDesc != "" {
		text = strings.TrimSpace(node.ContentDesc)
	}
	if text == "" {
		text = extractChildText(node)
	}

	return &UIElement{
		Text:       text,
		ResourceID: node.ResourceID,
		Class:      node.Class,
		Package:    node.Package,
		Bounds:     *bounds,
		Center:     center,
		Clickable:  node.Clickable == "true",
		Scrollable: node.Scrollable == "true",
	}
}

// extractChildText 递归提取子节点中的文本（用于可点击容器）
func extractChildText(node UINode) string {
	// 优先查找直接子节点的text
	for _, child := range node.Children {
		if childText := strings.TrimSpace(child.Text); childText != "" {
			return childText
		}
		if childText := strings.TrimSpace(child.ContentDesc); childText != "" {
			return childText
		}
	}

	// 如果直接子节点没有text，递归查找
	for _, child := range node.Children {
		if childText := extractChildText(child); childText != "" {
			return childText
		}
	}

	return ""
}

// parseBounds 解析bounds字符串: "[100,200][300,400]" -> [100, 200, 300, 400]
func parseBounds(boundsStr string) *[4]int {
	re := regexp.MustCompile(`\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)
	matches := re.FindStringSubmatch(boundsStr)
	if len(matches) != 5 {
		return nil
	}

	var bounds [4]int
	for i := 0; i < 4; i++ {
		val, err := strconv.Atoi(matches[i+1])
		if err != nil {
			return nil
		}
		bounds[i] = val
	}

	return &bounds
}

// associateLabels 为Switch/CheckBox关联旁边的文本标签
func associateLabels(clickableElements []UIElement, textViews []UIElement) {
	for i := range clickableElements {
		elem := &clickableElements[i]

		// 只处理Switch和CheckBox
		if !strings.Contains(elem.Class, "Switch") && !strings.Contains(elem.Class, "CheckBox") {
			continue
		}

		// 如果元素本身有文本，优先使用
		if elem.Text != "" {
			elem.Label = elem.Text
			continue
		}

		// 查找左侧的文本标签
		switchCenterY := elem.Center[1]
		var bestCandidate *UIElement
		minDistance := 1000000

		for j := range textViews {
			tv := &textViews[j]
			tvCenterY := tv.Center[1]

			// 条件1: 在同一水平线上 (Y坐标相近, 容差±50px)
			if abs(tvCenterY-switchCenterY) > 50 {
				continue
			}

			// 条件2: 位于Switch左侧
			if tv.Bounds[2] > elem.Bounds[0] {
				continue
			}

			// 条件3: 有有效文本
			if tv.Text == "" || len(tv.Text) < 2 {
				continue
			}

			// 计算水平距离
			distance := elem.Bounds[0] - tv.Bounds[2]

			// 选择距离最近的
			if distance < minDistance {
				minDistance = distance
				bestCandidate = tv
			}
		}

		if bestCandidate != nil {
			elem.Label = bestCandidate.Text
		}
	}
}

// abs 绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// CalculateMaxActions 根据UI元素数量动态计算最大操作次数
func CalculateMaxActions(uiData *UIData) int {
	clickableCount := len(uiData.ClickableElements)
	inputCount := len(uiData.InputFields)
	scrollableCount := len(uiData.ScrollableViews)

	// 基础操作数 = 可点击元素数 + 输入框数
	baseActions := clickableCount + inputCount

	// 如果有滚动视图，增加滚动操作
	if scrollableCount > 0 {
		scrollOps := scrollableCount * 2
		if scrollOps > 5 {
			scrollOps = 5
		}
		baseActions += scrollOps
	}

	// 动态范围
	var maxActions int
	switch {
	case baseActions <= 5:
		maxActions = baseActions
	case baseActions <= 15:
		maxActions = int(float64(baseActions) * 0.8)
	default:
		maxActions = int(float64(baseActions) * 0.6)
		if maxActions > 20 {
			maxActions = 20
		}
	}

	// 最小3个操作
	if maxActions < 3 {
		maxActions = 3
	}

	return maxActions
}

// ========== 操作安全检查相关函数 ==========

// 系统包名黑名单（这些包名的元素点击可能导致应用退出）
var dangerousPackages = []string{
	"com.android.systemui",           // 系统UI（状态栏、导航栏）
	"com.android.launcher",           // 原生桌面
	"com.android.launcher3",          // 原生桌面3
	"com.google.android.apps.nexuslauncher", // Pixel桌面
	"com.google.android.gms",         // Google服务弹窗
	"com.android.packageinstaller",   // 安装器
	"com.android.permissioncontroller", // 权限弹窗
	"com.miui.home",                  // 小米桌面
	"com.huawei.android.launcher",    // 华为桌面
	"com.oppo.launcher",              // OPPO桌面
	"com.vivo.launcher",              // vivo桌面
	"com.coloros.safecenter",         // ColorOS安全中心
	"com.oneplus.launcher",           // OnePlus桌面
	"com.sec.android.app.launcher",   // 三星桌面
	"com.android.settings",           // 系统设置
}

// FindElementByCoords 根据坐标查找对应的UI元素
// 从 XML 内容解析并找到包含指定坐标的最小元素
func FindElementByCoords(xmlContent string, x, y int) (*UIElement, error) {
	if xmlContent == "" {
		return nil, fmt.Errorf("empty XML content")
	}

	// 尝试解析为 hierarchy 格式
	var hierarchy UIHierarchy
	var rootNodes []UINode

	if err := xml.Unmarshal([]byte(xmlContent), &hierarchy); err == nil && len(hierarchy.Nodes) > 0 {
		rootNodes = hierarchy.Nodes
	} else {
		// 回退：尝试解析为 node 根元素
		var root UINode
		if err := xml.Unmarshal([]byte(xmlContent), &root); err != nil {
			return nil, fmt.Errorf("failed to parse UI XML: %w", err)
		}
		rootNodes = []UINode{root}
	}

	// 递归查找包含坐标的最小元素
	var foundElement *UIElement
	var minArea int = -1

	var findElement func(node UINode)
	findElement = func(node UINode) {
		bounds := parseBounds(node.Bounds)
		if bounds == nil {
			// 继续遍历子节点
			for _, child := range node.Children {
				findElement(child)
			}
			return
		}

		// 检查坐标是否在元素边界内
		if x >= bounds[0] && x <= bounds[2] && y >= bounds[1] && y <= bounds[3] {
			// 计算元素面积
			area := (bounds[2] - bounds[0]) * (bounds[3] - bounds[1])

			// 选择面积最小的元素（最精确匹配）
			if minArea == -1 || area < minArea {
				minArea = area
				center := [2]int{
					(bounds[0] + bounds[2]) / 2,
					(bounds[1] + bounds[3]) / 2,
				}
				text := strings.TrimSpace(node.Text)
				if text == "" && node.ContentDesc != "" {
					text = strings.TrimSpace(node.ContentDesc)
				}
				foundElement = &UIElement{
					Text:       text,
					ResourceID: node.ResourceID,
					Class:      node.Class,
					Package:    node.Package,
					Bounds:     *bounds,
					Center:     center,
					Clickable:  node.Clickable == "true",
					Scrollable: node.Scrollable == "true",
				}
			}
		}

		// 继续遍历子节点
		for _, child := range node.Children {
			findElement(child)
		}
	}

	for _, rootNode := range rootNodes {
		findElement(rootNode)
	}

	if foundElement == nil {
		return nil, fmt.Errorf("no element found at coordinates (%d, %d)", x, y)
	}

	return foundElement, nil
}

// IsElementSafe 检查元素是否安全（属于目标应用）
// 返回 true 表示安全可以执行操作，false 表示危险应跳过
func IsElementSafe(element *UIElement, targetPackage string) bool {
	if element == nil {
		return false
	}

	// 如果元素没有 package 属性，检查是否在危险区域（如屏幕边缘）
	if element.Package == "" {
		// 检查是否在屏幕边缘（可能是导航栏区域）
		// 通常导航栏在屏幕底部约100px区域
		// 状态栏在屏幕顶部约100px区域
		// 这里不做强制拦截，因为有些应用的元素确实没有package属性
		return true
	}

	// 如果元素属于目标应用，安全
	if element.Package == targetPackage {
		return true
	}

	// 检查是否在黑名单中
	for _, dangerousPkg := range dangerousPackages {
		if element.Package == dangerousPkg {
			return false
		}
		// 前缀匹配（如 com.miui.* 匹配 com.miui.home）
		if strings.HasSuffix(dangerousPkg, "*") {
			prefix := strings.TrimSuffix(dangerousPkg, "*")
			if strings.HasPrefix(element.Package, prefix) {
				return false
			}
		}
	}

	// 其他应用（可能是悬浮窗、广告等），默认不安全
	// 但不是完全拒绝，因为有些场景需要处理权限弹窗等
	// 这里返回 false，让调用方决定是否允许
	return false
}

// IsDangerousZone 检查坐标是否在危险区域（导航栏、状态栏等）
// screenWidth 和 screenHeight 是屏幕尺寸
func IsDangerousZone(x, y, screenWidth, screenHeight int) bool {
	// 状态栏区域（顶部约75px）
	if y < 75 {
		return true
	}

	// 导航栏区域（底部约150px，包含虚拟按键）
	if y > screenHeight-150 {
		return true
	}

	// 屏幕边缘（左右各20px，可能是手势区域）
	if x < 20 || x > screenWidth-20 {
		return true
	}

	return false
}

// ParseUIXMLContent 从 XML 内容字符串解析 UI 数据
// 与 ParseUIXML 类似，但接受字符串而不是文件路径
func ParseUIXMLContent(xmlContent string) (*UIData, error) {
	if xmlContent == "" {
		return nil, fmt.Errorf("empty XML content")
	}

	uiData := &UIData{
		ClickableElements: []UIElement{},
		InputFields:       []UIElement{},
		ScrollableViews:   []UIElement{},
	}

	allTextViews := []UIElement{}

	var traverse func(node UINode)
	traverse = func(node UINode) {
		elem := parseUIElement(node)
		if elem == nil {
			for _, child := range node.Children {
				traverse(child)
			}
			return
		}

		if elem.Clickable {
			uiData.ClickableElements = append(uiData.ClickableElements, *elem)
		}

		if strings.Contains(elem.Class, "EditText") {
			uiData.InputFields = append(uiData.InputFields, *elem)
		}

		if elem.Scrollable {
			uiData.ScrollableViews = append(uiData.ScrollableViews, *elem)
		}

		if elem.Text != "" && strings.Contains(elem.Class, "Text") {
			allTextViews = append(allTextViews, *elem)
		}

		for _, child := range node.Children {
			traverse(child)
		}
	}

	var hierarchy UIHierarchy
	if err := xml.Unmarshal([]byte(xmlContent), &hierarchy); err == nil && len(hierarchy.Nodes) > 0 {
		for _, rootNode := range hierarchy.Nodes {
			traverse(rootNode)
		}
	} else {
		var root UINode
		if err := xml.Unmarshal([]byte(xmlContent), &root); err != nil {
			return nil, fmt.Errorf("failed to parse UI XML: %w", err)
		}
		traverse(root)
	}

	associateLabels(uiData.ClickableElements, allTextViews)

	return uiData, nil
}
