package packer

// GetBuiltinRules 获取内置壳规则库
func GetBuiltinRules() []PackerRule {
	return []PackerRule{
		// ==================== 国产加固 (高优先级) ====================
		{
			Name:       "360加固",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libjiagu.so", "libjiagu_x86.so", "libjiagu_a64.so", "libjiagu_x64.so"},
			Strings:    []string{"com.qihoo.util", "com.stub.StubApp", "com.qihoo360.replugin"},
			ClassNames: []string{"com.stub.StubApp", "com.qihoo.util.QHClassLoader"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     100,
		},
		{
			Name:       "腾讯乐固",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libshell.so", "libshellx.so", "libtxmsecurity.so", "libshella-2.10.3.4.so", "libshellx-2.10.3.4.so"},
			Strings:    []string{"com.tencent.StubShell", "com.tencent.bugly", "com.tencent.mm.sdk"},
			ClassNames: []string{"com.tencent.StubShell.TxAppEntry"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     100,
		},
		{
			Name:       "爱加密",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libexec.so", "libexecmain.so", "ijiami.ajm"},
			Strings:    []string{"ijiami", "s.h.e.l.l", "com.shell.SuperApplication"},
			ClassNames: []string{"com.shell.SuperApplication"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     100,
		},
		{
			Name:       "梆梆加固",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libDexHelper.so", "libDexHelper-x86.so", "libSecShell.so", "libSecShell-x86.so"},
			Strings:    []string{"com.secneo.apkwrapper", "com.bangcle"},
			ClassNames: []string{"com.secneo.apkwrapper.ApplicationWrapper"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     100,
		},
		{
			Name:       "娜迦加固",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libnaga.so", "libddog.so", "libedog.so"},
			Strings:    []string{"com.nagapt.protect", "com.naga"},
			ClassNames: []string{"com.nagapt.protect.StubApplication"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     95,
		},
		{
			Name:       "网易易盾",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libnesec.so", "libNetHTProtect.so"},
			Strings:    []string{"com.netease.nis", "com.netease.htprotect"},
			ClassNames: []string{"com.netease.nis.wrapper.MyApplication"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     95,
		},
		{
			Name:       "阿里聚安全",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libmobisec.so", "libsgmain.so", "libsgsecuritybody.so"},
			Strings:    []string{"com.alibaba.wireless.security", "com.taobao.wireless.security"},
			ClassNames: []string{"com.alibaba.wireless.security.open.SecurityGuardManager"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     95,
		},
		{
			Name:       "百度加固",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libbaiduprotect.so", "libcocklogic.so"},
			Strings:    []string{"com.baidu.protect", "com.baidu.cloudacc"},
			ClassNames: []string{"com.baidu.protect.StubApplication"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     90,
		},
		{
			Name:       "通付盾",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libegis.so", "libNSaferOnly.so"},
			Strings:    []string{"com.payegis", "com.tongfudun"},
			ClassNames: []string{"com.payegis.protect.StubApp"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     90,
		},
		{
			Name:       "瑞星加固",
			Type:       PackerTypeNative,
			NativeLibs: []string{"librsjia.so", "librsdec.so"},
			Strings:    []string{"com.rsshield", "com.rising.shield"},
			ClassNames: []string{"com.rsshield.RsApplication"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     85,
		},
		{
			Name:       "几维安全",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libkwscmm.so", "libkwscr.so"},
			Strings:    []string{"com.kiwisec", "cn.kiwisec"},
			ClassNames: []string{"com.kiwisec.android.loader.KWLoader"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     85,
		},
		{
			Name:       "顶像加固",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libx3g.so", "libdxoptimizer.so"},
			Strings:    []string{"com.dingxiang.mobile", "com.dx.mobile"},
			ClassNames: []string{"com.dingxiang.mobile.ShieldApp"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     85,
		},
		// ==================== 国际加固 ====================
		{
			Name:       "DexGuard",
			Type:       PackerTypeDexEncrypt,
			NativeLibs: []string{},
			Strings:    []string{"DexGuard", "GuardSquare"},
			ClassNames: []string{"o.Oo", "o.OoO", "o.oOo", "o.OOo"},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaClassLoader,
			Priority:     80,
		},
		{
			Name:       "DexProtector",
			Type:       PackerTypeVMP,
			NativeLibs: []string{"libdexprotector.so"},
			Strings:    []string{"liblxz.dexprotector", "DexProtector"},
			ClassNames: []string{},
			CanUnpack:    false,
			UnpackMethod: UnpackMethodManual,
			Priority:     80,
		},
		{
			Name:       "Arxan",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libArxanJNI.so", "libArxan.so"},
			Strings:    []string{"com.arxan", "Arxan"},
			ClassNames: []string{},
			CanUnpack:    false,
			UnpackMethod: UnpackMethodManual,
			Priority:     75,
		},
		{
			Name:       "AppSealing",
			Type:       PackerTypeNative,
			NativeLibs: []string{"libAppSealing.so", "libAppSealingCore.so"},
			Strings:    []string{"AppSealing", "com.appsealing"},
			ClassNames: []string{},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     75,
		},
		// ==================== 通用特征 (低优先级) ====================
		{
			Name: "未知壳 (DEX异常小)",
			Type: PackerTypeUnknown,
			FileSize: FileSizeRule{
				DEXMaxKB:    100, // DEX小于100KB可疑
				NativeMinMB: 0,
			},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     10,
		},
		{
			Name: "未知壳 (Native库异常大)",
			Type: PackerTypeUnknown,
			FileSize: FileSizeRule{
				DEXMaxKB:    0,
				NativeMinMB: 10, // Native库超过10MB可疑
			},
			CanUnpack:    true,
			UnpackMethod: UnpackMethodFridaDEXDump,
			Priority:     10,
		},
	}
}

// GetHighPriorityPackerNames 获取高优先级的壳名称列表 (用于日志和报告)
func GetHighPriorityPackerNames() []string {
	return []string{
		"360加固",
		"腾讯乐固",
		"爱加密",
		"梆梆加固",
		"娜迦加固",
		"网易易盾",
		"阿里聚安全",
		"百度加固",
		"通付盾",
		"DexGuard",
		"DexProtector",
	}
}

// IsSupportedPacker 检查是否为已知支持的加固
func IsSupportedPacker(name string) bool {
	supported := GetHighPriorityPackerNames()
	for _, s := range supported {
		if s == name {
			return true
		}
	}
	return false
}
