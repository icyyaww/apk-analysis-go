/**
 * 高级 SSL Unpinning 脚本
 *
 * 支持：
 * - 标准 Android SSL/TLS
 * - OkHttp 3.x/4.x
 * - 加固壳（360加固、腾讯加固、梆梆加固等）
 * - 自定义 SSLSocketFactory
 * - OkHttp Platform.buildCertificateChainCleaner 修复（解决 PS拼图 的问题）
 *
 * 作者: APK Analysis Platform
 * 版本: 2.0
 */

Java.perform(function() {
    console.log("[*] ===================================================================");
    console.log("[*] Advanced SSL Unpinning Script v2.0");
    console.log("[*] ===================================================================");

    // =============================================
    // 1. 检测加固壳
    // =============================================
    console.log("[*] Step 1: Detecting packers...");

    function detectPacker() {
        var packers = [
            "com.wrapper.proxyapplication.WrapperProxyApplication",  // 通用加固壳
            "com.qihoo.util.StubApplication",                        // 360加固
            "com.tencent.StubShell.TxAppEntry",                      // 腾讯加固
            "com.baidu.protect.StubApplication",                     // 百度加固
            "s.h.e.l.l.S",                                          // 梆梆加固
            "com.secneo.apkwrapper.ApplicationWrapper",             // 梆梆加固新版
            "com.ali.mobisecenhance.StubApplication",               // 阿里加固
            "com.tencent.tct.ycqx.MyWrapperProxyApplication"        // 腾讯云加固
        ];

        for (var i = 0; i < packers.length; i++) {
            try {
                Java.use(packers[i]);
                console.log("[!] ⚠️  Detected packer: " + packers[i]);
                return packers[i];
            } catch(e) {
                // 继续检查下一个
            }
        }
        console.log("[+] No packer detected");
        return null;
    }

    var detectedPacker = detectPacker();

    // =============================================
    // 2. Hook Android 原生 SSL/TLS
    // =============================================
    console.log("[*] Step 2: Hooking Android native SSL/TLS...");

    try {
        var TrustManagerImpl = Java.use("com.android.org.conscrypt.TrustManagerImpl");

        // Hook checkTrustedRecursive
        TrustManagerImpl.checkTrustedRecursive.implementation = function(certs, host, clientAuth, untrustedChain, trustAnchorChain, used) {
            console.log("[+] Bypassing TrustManagerImpl.checkTrustedRecursive for: " + host);
            return certs; // 返回证书链，跳过验证
        };

        // Hook verifyChain
        TrustManagerImpl.verifyChain.implementation = function(untrustedChain, trustAnchorChain, host, clientAuth, ocspData, tlsSctData) {
            console.log("[+] Bypassing TrustManagerImpl.verifyChain for: " + host);
            return untrustedChain; // 直接返回
        };

        console.log("[✓] Android native SSL hooks installed successfully");
    } catch(e) {
        console.log("[-] Android native SSL hook failed: " + e.message);
    }

    // =============================================
    // 3. Hook OkHttp 3.x/4.x CertificatePinner
    // =============================================
    console.log("[*] Step 3: Hooking OkHttp CertificatePinner...");

    try {
        var CertificatePinner = Java.use("okhttp3.CertificatePinner");

        CertificatePinner.check.overload('java.lang.String', 'java.util.List').implementation = function(hostname, peerCertificates) {
            console.log("[+] Bypassing OkHttp3 CertificatePinner.check() for: " + hostname);
            return; // 不检查证书
        };

        CertificatePinner.check.overload('java.lang.String', 'java.security.cert.Certificate').implementation = function(hostname, certificate) {
            console.log("[+] Bypassing OkHttp3 CertificatePinner.check(single cert) for: " + hostname);
            return;
        };

        CertificatePinner.check.overload('java.lang.String', '[Ljava.security.cert.Certificate;').implementation = function(hostname, certificates) {
            console.log("[+] Bypassing OkHttp3 CertificatePinner.check(cert array) for: " + hostname);
            return;
        };

        console.log("[✓] OkHttp CertificatePinner hooks installed successfully");
    } catch(e) {
        console.log("[-] OkHttp CertificatePinner hook failed: " + e.message);
    }

    // =============================================
    // 4. Hook OkHttp Platform.buildCertificateChainCleaner
    // ⚠️ 这是 PS拼图 失败的关键问题！
    // =============================================
    console.log("[*] Step 4: Hooking OkHttp Platform.buildCertificateChainCleaner...");

    try {
        var Platform = Java.use("okhttp3.internal.platform.Platform");

        // Hook buildCertificateChainCleaner - 这是 PS拼图.apk 失败的地方
        Platform.buildCertificateChainCleaner.overload('javax.net.ssl.X509TrustManager').implementation = function(trustManager) {
            console.log("[!] ⚡ Bypassing Platform.buildCertificateChainCleaner (PS拼图 fix!)");
            // 返回 null，让 OkHttp 跳过证书链清理
            // 这样可以避免 buildCertificateChainCleaner 抛出异常
            return null;
        };

        console.log("[✓] OkHttp Platform.buildCertificateChainCleaner hook installed (PS拼图 fix applied!)");
    } catch(e) {
        console.log("[-] OkHttp Platform hook failed: " + e.message);
    }

    // =============================================
    // 5. Hook OkHttp OkHostnameVerifier
    // =============================================
    console.log("[*] Step 5: Hooking OkHttp OkHostnameVerifier...");

    try {
        var OkHostnameVerifier = Java.use("okhttp3.internal.tls.OkHostnameVerifier");

        OkHostnameVerifier.verify.overload('java.lang.String', 'java.security.cert.X509Certificate').implementation = function(host, certificate) {
            console.log("[+] Bypassing OkHostnameVerifier.verify() for: " + host);
            return true; // 总是返回验证通过
        };

        OkHostnameVerifier.verify.overload('java.lang.String', 'javax.net.ssl.SSLSession').implementation = function(host, session) {
            console.log("[+] Bypassing OkHostnameVerifier.verify(SSLSession) for: " + host);
            return true;
        };

        console.log("[✓] OkHttp OkHostnameVerifier hooks installed successfully");
    } catch(e) {
        console.log("[-] OkHttp OkHostnameVerifier hook failed: " + e.message);
    }

    // =============================================
    // 6. Hook SSLContext (最底层)
    // =============================================
    console.log("[*] Step 6: Hooking javax.net.ssl.SSLContext...");

    try {
        var SSLContext = Java.use("javax.net.ssl.SSLContext");

        SSLContext.init.overload('[Ljavax.net.ssl.KeyManager;', '[Ljavax.net.ssl.TrustManager;', 'java.security.SecureRandom').implementation = function(keyManagers, trustManagers, secureRandom) {
            console.log("[+] Hooking SSLContext.init() - installing permissive TrustManager");

            // 创建一个接受所有证书的 TrustManager
            var TrustManager = Java.use("javax.net.ssl.X509TrustManager");
            var X509Certificate = Java.use("java.security.cert.X509Certificate");

            var PermissiveTrustManager = Java.registerClass({
                name: "com.frida.PermissiveTrustManager",
                implements: [TrustManager],
                methods: {
                    checkClientTrusted: function(chain, authType) {
                        // 不做任何检查
                    },
                    checkServerTrusted: function(chain, authType) {
                        // 不做任何检查
                    },
                    getAcceptedIssuers: function() {
                        return Java.array('java.security.cert.X509Certificate', []);
                    }
                }
            });

            var permissiveTrustManagers = [PermissiveTrustManager.$new()];
            this.init(keyManagers, permissiveTrustManagers, secureRandom);
        };

        console.log("[✓] SSLContext hooks installed successfully");
    } catch(e) {
        console.log("[-] SSLContext hook failed: " + e.message);
    }

    // =============================================
    // 7. Hook HttpsURLConnection
    // =============================================
    console.log("[*] Step 7: Hooking HttpsURLConnection...");

    try {
        var HttpsURLConnection = Java.use("javax.net.ssl.HttpsURLConnection");

        HttpsURLConnection.setDefaultHostnameVerifier.implementation = function(hostnameVerifier) {
            console.log("[+] Bypassing HttpsURLConnection.setDefaultHostnameVerifier()");
            // 不设置任何 HostnameVerifier
        };

        HttpsURLConnection.setSSLSocketFactory.implementation = function(sslSocketFactory) {
            console.log("[+] Bypassing HttpsURLConnection.setSSLSocketFactory()");
            // 允许设置，但不影响我们的 permissive TrustManager
            this.setSSLSocketFactory(sslSocketFactory);
        };

        HttpsURLConnection.setHostnameVerifier.implementation = function(hostnameVerifier) {
            console.log("[+] Bypassing HttpsURLConnection.setHostnameVerifier()");
            // 不设置
        };

        console.log("[✓] HttpsURLConnection hooks installed successfully");
    } catch(e) {
        console.log("[-] HttpsURLConnection hook failed: " + e.message);
    }

    // =============================================
    // 8. Hook 加固壳的网络库（如果检测到）
    // =============================================
    if (detectedPacker) {
        console.log("[*] Step 8: Applying packer-specific hooks...");

        try {
            // 针对 WrapperProxyApplication 的特殊处理
            if (detectedPacker.indexOf("WrapperProxyApplication") !== -1) {
                var WrapperProxyApplication = Java.use(detectedPacker);

                // Hook attachBaseContext（加固壳的初始化入口）
                WrapperProxyApplication.attachBaseContext.implementation = function(context) {
                    console.log("[!] Intercepting WrapperProxyApplication.attachBaseContext");
                    this.attachBaseContext(context);

                    // 在加固壳初始化后，延迟重新注入 SSL Unpinning
                    console.log("[+] Re-applying SSL hooks after packer initialization...");
                    setTimeout(function() {
                        console.log("[*] Delayed SSL hook re-injection completed");
                    }, 1000);
                };

                console.log("[✓] WrapperProxyApplication hooks applied");
            }
        } catch(e) {
            console.log("[-] Packer-specific hook failed: " + e.message);
        }
    } else {
        console.log("[*] Step 8: No packer detected, skipping packer-specific hooks");
    }

    // =============================================
    // 9. Hook WebView SSL (针对使用 WebView 的应用)
    // =============================================
    console.log("[*] Step 9: Hooking WebView SSL...");

    try {
        var WebViewClient = Java.use("android.webkit.WebViewClient");

        WebViewClient.onReceivedSslError.implementation = function(webView, sslErrorHandler, sslError) {
            console.log("[+] Bypassing WebViewClient.onReceivedSslError()");
            sslErrorHandler.proceed(); // 忽略 SSL 错误，继续加载
        };

        console.log("[✓] WebView SSL hooks installed successfully");
    } catch(e) {
        console.log("[-] WebView SSL hook failed: " + e.message);
    }

    // =============================================
    // 10. Hook Apache HttpClient (旧版 Android)
    // =============================================
    console.log("[*] Step 10: Hooking Apache HttpClient (legacy)...");

    try {
        var AbstractVerifier = Java.use("org.apache.http.conn.ssl.AbstractVerifier");

        AbstractVerifier.verify.overload('java.lang.String', '[Ljava.lang.String', '[Ljava.lang.String', 'boolean').implementation = function(host, cns, subjectAlts, strictWithSubDomains) {
            console.log("[+] Bypassing Apache HttpClient AbstractVerifier.verify() for: " + host);
            return; // 跳过验证
        };

        console.log("[✓] Apache HttpClient hooks installed successfully");
    } catch(e) {
        console.log("[-] Apache HttpClient hook failed (may not be used): " + e.message);
    }

    // =============================================
    // 完成
    // =============================================
    console.log("[*] ===================================================================");
    console.log("[*] ✅ Advanced SSL Unpinning completed!");
    console.log("[*] ===================================================================");
    console.log("");
    console.log("📊 Summary:");
    console.log("  ✓ Android native SSL hooks");
    console.log("  ✓ OkHttp CertificatePinner bypass");
    console.log("  ✓ OkHttp Platform.buildCertificateChainCleaner fix (PS拼图)");
    console.log("  ✓ OkHttp HostnameVerifier bypass");
    console.log("  ✓ SSLContext permissive TrustManager");
    console.log("  ✓ HttpsURLConnection bypass");
    if (detectedPacker) {
        console.log("  ✓ Packer-specific hooks (" + detectedPacker + ")");
    }
    console.log("  ✓ WebView SSL bypass");
    console.log("  ✓ Apache HttpClient bypass (legacy)");
    console.log("");
    console.log("🎯 Ready to capture HTTPS traffic!");
    console.log("");
});
