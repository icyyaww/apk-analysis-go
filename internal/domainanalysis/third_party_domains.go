package domainanalysis

// ThirdPartyDomains 第三方SDK域名集合
// 参考 MVP 项目的完整域名库
var ThirdPartyDomains = map[string]bool{
	// ========== 社交平台 ==========
	"qq.com":          true,
	"weixin.qq.com":   true,
	"wechat.com":      true,
	"facebook.com":    true,
	"twitter.com":     true,
	"instagram.com":   true,
	"linkedin.com":    true,
	"snapchat.com":    true,
	"weibo.com":       true,
	"weibo.cn":        true,
	"douyin.com":      true,
	"tiktok.com":      true,

	// ========== 支付平台 ==========
	"alipay.com":  true,
	"taobao.com":  true,
	"alibaba.com": true,
	"alicdn.com":  true,
	"95516.com":   true, // 银联
	"paypal.com":  true,
	"stripe.com":  true,

	// ========== 搜索引擎 ==========
	"baidu.com":              true,
	"bdimg.com":              true,
	"bdstatic.com":           true,
	"google.com":             true,
	"googleapis.com":         true,
	"gstatic.com":            true,
	"googleusercontent.com":  true,
	"bing.com":               true,
	"live.com":               true,

	// ========== 云服务商 ==========
	"aliyuncs.com":         true,
	"tencent.com":          true,
	"gtimg.com":            true,
	"qpic.cn":              true,
	"qcloud.com":           true,
	"myqcloud.com":         true,
	"tencent-cloud.com":    true,
	"tencentcloud.com":     true,
	"cloud.tencent.com":    true,
	"amazonaws.com":        true,
	"awsstatic.com":        true,
	"azure.com":            true,
	"azureedge.net":        true,
	"microsoft.com":        true,
	"windows.net":          true,
	"cloudflare.com":       true,
	"cloudflareinsights.com": true,
	"digitalocean.com":     true,
	"digitaloceanspaces.com": true,

	// ========== 统计分析SDK ==========
	"umeng.com":                  true,
	"cnzz.com":                   true,
	"google-analytics.com":       true,
	"googletagmanager.com":       true,
	"app-measurement.com":        true,
	"app-analytics-services.com": true,
	"segment.com":                true,
	"mixpanel.com":               true,
	"amplitude.com":              true,
	"sensorsdata.cn":             true,
	"growingio.com":              true,
	"adobe.com":                  true,
	"adobedtm.com":               true,
	"omtrdc.net":                 true,
	"2o7.net":                    true,
	"demdex.net":                 true,
	"hotjar.com":                 true,
	"crazyegg.com":               true,
	"optimizely.com":             true,
	"chartbeat.com":              true,
	"quantcast.com":              true,

	// ========== 崩溃监控/错误追踪 ==========
	"bugly.qq.com":   true,
	"bugly.com":      true,
	"crashlytics.com": true,
	"fabric.io":      true,
	"sentry.io":      true,
	"sentry.com":     true,
	"rollbar.com":    true,

	// ========== 广告平台 ==========
	"doubleclick.net":       true,
	"googlesyndication.com": true,
	"googleadservices.com":  true,
	"admob.com":             true,
	"facebook.net":          true,
	"fbcdn.net":             true,
	"bytedance.com":         true,
	"pstatp.com":            true,
	"snssdk.com":            true,
	"tanx.com":              true,
	"mmstat.com":            true,
	"admaster.com.cn":       true,
	"miaozhen.com":          true,

	// ========== 推送服务 ==========
	"jpush.cn":      true,
	"jiguang.cn":    true,
	"getui.com":     true,
	"gepush.com":    true,
	"getui.net":     true,
	"igexin.com":    true,
	"firebase.com":  true,
	"firebaseio.com": true,
	"onesignal.com": true,

	// ========== 地图定位 ==========
	"amap.com":  true,
	"gaode.com": true,

	// ========== CDN服务 ==========
	"akamai.com":       true,
	"akamaiedge.net":   true,
	"akamaihd.net":     true,
	"fastly.net":       true,
	"cloudfront.net":   true,
	"cdninstagram.com": true,
	"cdnfacebook.com":  true,

	// ========== 开发平台 ==========
	"github.com":            true,
	"githubusercontent.com":  true,
	"githubassets.com":      true,
	"gitlab.com":            true,
	"gitlab.io":             true,
	"googlesource.com":      true,
	"sourceforge.net":       true,
	"sf.net":                true,
	"npmjs.com":             true,
	"npmjs.org":             true,
	"jsdelivr.net":          true,
	"unpkg.com":             true,

	// ========== 开源框架/库官方域名 ==========
	"slf4j.org":         true,
	"apache.org":        true,
	"eclipse.org":       true,
	"mozilla.org":       true,
	"w3.org":            true,
	"json.org":          true,
	"sqlite.org":        true,
	"postgresql.org":    true,
	"mysql.com":         true,
	"redis.io":          true,
	"mongodb.com":       true,
	"rabbitmq.com":      true,
	"elastic.co":        true,
	"elasticsearch.com": true,
	"jetbrains.com":     true,
	"android.com":       true,
	"developer.android.com": true,
	"apple.com":         true,
	"icloud.com":        true,

	// ========== 手机厂商SDK/服务 ==========
	"huawei.com":      true,
	"hicloud.com":     true,
	"vmall.com":       true,
	"xiaomi.com":      true,
	"mi.com":          true,
	"miui.com":        true,
	"oppo.com":        true,
	"heytap.com":      true,
	"vivo.com":        true,
	"vivoglobal.com":  true,
	"meizu.com":       true,
	"flyme.cn":        true,
	"oneplus.com":     true,
	"oneplus.cn":      true,
	"samsung.com":     true,
	"samsungcloud.com": true,
	"motorola.com":    true,
	"lenovo.com":      true,

	// ========== APM性能监控 ==========
	"tingyun.com":       true,
	"networkbench.com":  true,
	"newrelic.com":      true,
	"appdynamics.com":   true,
	"dynatrace.com":     true,
	"datadog.com":       true,
	"datadoghq.com":     true,

	// ========== 归因/深度链接 ==========
	"appsflyer.com": true,
	"adjust.com":    true,
	"branch.io":     true,
	"kochava.com":   true,
	"singular.net":  true,
	"tenjin.com":    true,

	// ========== 营销自动化/用户参与 ==========
	"moengage.com":  true,
	"clevertap.com": true,
	"braze.com":     true,
	"appboy.com":    true,
	"leanplum.com":  true,
	"localytics.com": true,

	// ========== 推送/通信服务 ==========
	"urbanairship.com": true,
	"twilio.com":       true,
	"sendgrid.com":     true,
	"mailchimp.com":    true,
	"pusher.com":       true,
	"pubnub.com":       true,

	// ========== 视频/直播服务 ==========
	"vimeo.com":       true,
	"vimeocdn.com":    true,
	"youtube.com":     true,
	"ytimg.com":       true,
	"googlevideo.com": true,
	"jwplayer.com":    true,
	"brightcove.com":  true,
	"brightcove.net":  true,
	"wistia.com":      true,
	"wistia.net":      true,

	// ========== 支付/金融SDK ==========
	"stripe.network":         true,
	"braintreegateway.com":   true,
	"braintreepayments.com":  true,
	"square.com":             true,
	"squareup.com":           true,
	"adyen.com":              true,

	// ========== 安全/验证服务 ==========
	"recaptcha.net": true,
	"auth0.com":     true,
	"okta.com":      true,
	"onelogin.com":  true,

	// ========== 图片/媒体处理 ==========
	"imgur.com":              true,
	"cloudinary.com":         true,
	"imgix.net":              true,
	"fastly-insights.com":    true,

	// ========== 其他第三方服务 ==========
	"zendesk.com":   true,
	"intercom.io":   true,
	"intercom.com":  true,
	"drift.com":     true,
	"freshdesk.com": true,
	"helpshift.com": true,
}

// ExcludeKeywords 排除关键词(广告/统计/SDK) - 仅用于子域名级别检查
var ExcludeKeywords = []string{
	// 统计分析服务
	"analytics", "track", "log", "metrics", "stat", "monitor",
	"sensors", "tongji", "statistics",
	"bugly", "firebase", "crashlytics", "sentry",
	"umeng", "cnzz",
	// 广告服务
	"ad", "ads", "adx", "doubleclick", "admob",
	"advertisement", "guanggao", "advert", "banner",
	// 社交平台
	"weibo", "douyin", "tiktok",
}

// CDNKeywords CDN特征关键词
var CDNKeywords = []string{
	"cdn", "static", "img", "image", "asset", "resource", "cache",
	"assets", "res", "images", "js", "css",
}

// APIKeywords API特征关键词
var APIKeywords = []string{
	"api", "app", "service", "server", "gateway", "interface",
	"rest", "graphql", "rpc", "backend", "gw",
}
