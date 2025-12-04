/**
 * Universal SSL Unpinning Script for Android
 * Supports multiple SSL pinning implementations
 */

Java.perform(function() {
    console.log("[*] SSL Unpinning script loaded");

    // ============================================
    // 1. OkHttp 3.x CertificatePinner
    // ============================================
    try {
        var CertificatePinner = Java.use("okhttp3.CertificatePinner");
        CertificatePinner.check.overload('java.lang.String', 'java.util.List').implementation = function(hostname, peerCertificates) {
            console.log("[+] OkHttp3 CertificatePinner.check() bypassed for: " + hostname);
            return;
        };
        console.log("[+] OkHttp3 CertificatePinner hooked");
    } catch (e) {
        console.log("[-] OkHttp3 CertificatePinner not found");
    }

    // ============================================
    // 2. TrustManager (javax.net.ssl)
    // ============================================
    try {
        var X509TrustManager = Java.use('javax.net.ssl.X509TrustManager');
        var SSLContext = Java.use('javax.net.ssl.SSLContext');

        // 创建一个接受所有证书的 TrustManager
        var TrustManager = Java.registerClass({
            name: 'com.fake.TrustManager',
            implements: [X509TrustManager],
            methods: {
                checkClientTrusted: function(chain, authType) {},
                checkServerTrusted: function(chain, authType) {},
                getAcceptedIssuers: function() {
                    return [];
                }
            }
        });

        // Hook SSLContext.init()
        SSLContext.init.overload('[Ljavax.net.ssl.KeyManager;', '[Ljavax.net.ssl.TrustManager;', 'java.security.SecureRandom').implementation = function(keyManager, trustManager, secureRandom) {
            console.log("[+] SSLContext.init() called, replacing TrustManager");
            this.init(keyManager, [TrustManager.$new()], secureRandom);
        };

        console.log("[+] TrustManager hooked");
    } catch (e) {
        console.log("[-] TrustManager hook failed: " + e);
    }

    // ============================================
    // 3. WebViewClient (Android WebView)
    // ============================================
    try {
        var WebViewClient = Java.use("android.webkit.WebViewClient");
        WebViewClient.onReceivedSslError.implementation = function(view, handler, error) {
            console.log("[+] WebViewClient.onReceivedSslError() bypassed");
            handler.proceed();
        };
        console.log("[+] WebViewClient hooked");
    } catch (e) {
        console.log("[-] WebViewClient not found");
    }

    // ============================================
    // 4. Apache HttpClient (Legacy)
    // ============================================
    try {
        var AbstractVerifier = Java.use("org.apache.http.conn.ssl.AbstractVerifier");
        AbstractVerifier.verify.overload('java.lang.String', '[Ljava.lang.String', '[Ljava.lang.String', 'boolean').implementation = function(host, cns, subjectAlts, strictWithSubDomains) {
            console.log("[+] Apache HttpClient SSL verification bypassed for: " + host);
            return;
        };
        console.log("[+] Apache HttpClient hooked");
    } catch (e) {
        console.log("[-] Apache HttpClient not found");
    }

    // ============================================
    // 5. HostnameVerifier
    // ============================================
    try {
        var HostnameVerifier = Java.use("javax.net.ssl.HostnameVerifier");
        HostnameVerifier.verify.overload('java.lang.String', 'javax.net.ssl.SSLSession').implementation = function(hostname, session) {
            console.log("[+] HostnameVerifier.verify() bypassed for: " + hostname);
            return true;
        };
        console.log("[+] HostnameVerifier hooked");
    } catch (e) {
        console.log("[-] HostnameVerifier hook failed");
    }

    // ============================================
    // 6. Conscrypt (Android's SSL provider)
    // ============================================
    try {
        var ConscryptFileDescriptorSocket = Java.use("com.android.org.conscrypt.ConscryptFileDescriptorSocket");
        ConscryptFileDescriptorSocket.verifyCertificateChain.implementation = function(certChain, authMethod) {
            console.log("[+] Conscrypt certificate verification bypassed");
        };
        console.log("[+] Conscrypt hooked");
    } catch (e) {
        console.log("[-] Conscrypt not found");
    }

    // ============================================
    // 7. Network Security Config Bypass
    // ============================================
    try {
        var NetworkSecurityConfig = Java.use("android.security.net.config.ApplicationConfig");
        NetworkSecurityConfig.isCleartextTrafficPermitted.implementation = function() {
            console.log("[+] Network Security Config: cleartext traffic allowed");
            return true;
        };
        console.log("[+] Network Security Config hooked");
    } catch (e) {
        console.log("[-] Network Security Config not found");
    }

    console.log("[*] SSL Unpinning hooks installed successfully");
});
