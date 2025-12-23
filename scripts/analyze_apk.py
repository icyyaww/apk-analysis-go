#!/usr/bin/env python3
"""
Androguard 深度分析器
输入：APK 路径
输出：JSON 格式的深度分析结果

依赖：
    pip install androguard>=3.4.0
"""

import sys
import json
import re
import traceback
from datetime import datetime
import warnings
import logging

# 禁用所有警告信息
warnings.filterwarnings('ignore')

# 禁用 Androguard 的日志输出
logging.getLogger('androguard').setLevel(logging.CRITICAL)
logging.getLogger('androguard.core.apk').setLevel(logging.CRITICAL)
logging.getLogger('androguard.core.axml').setLevel(logging.CRITICAL)
logging.getLogger('androguard.core.dex').setLevel(logging.CRITICAL)

# 禁用 loguru 日志（如果存在）
try:
    from loguru import logger
    logger.disable('androguard')
except ImportError:
    pass

try:
    # Androguard 4.x 使用新的导入路径
    from androguard.core.apk import APK
    from androguard.core.dex import DEX
except ImportError:
    try:
        # Androguard 3.x 兼容
        from androguard.core.bytecodes.apk import APK
        from androguard.core.bytecodes.dvm import DalvikVMFormat as DEX
    except ImportError:
        print(json.dumps({"error": "androguard not installed. Run: pip install androguard"}))
        sys.exit(1)


def deep_analyze(apk_path):
    """深度分析 APK"""

    start_time = datetime.now()

    # 基础 APK 对象（快速）
    apk = APK(apk_path)

    # 不使用 AnalyzeAPK（太慢），直接解析 DEX 字符串
    # a, d, dx = AnalyzeAPK(apk_path)  # 已移除

    result = {
        "urls": [],
        "domains": [],
        "strings": [],
        "native_libs": [],
        "certificates": {},
        "api_calls": [],
        "analysis_duration_ms": 0
    }

    # ========== 1. 提取 URL（从 DEX 字符串常量） ==========
    url_pattern = re.compile(r'https?://[^\s"\'<>)}\]]+', re.IGNORECASE)
    urls = set()
    all_dex_strings = []  # 保存所有 DEX 字符串，供后续使用

    # 遍历所有 DEX 文件，直接提取字符串（不反编译代码）
    for dex_name in apk.get_dex_names():
        try:
            dex_bytes = apk.get_file(dex_name)
            dex = DEX(dex_bytes)

            for string_value in dex.get_strings():
                string_str = str(string_value)
                all_dex_strings.append(string_str)

                if re.match(r'https?://', string_str, re.IGNORECASE):
                    # 清理 URL（移除尾部的特殊字符）
                    clean_url = re.sub(r'[.,;:!?\'")\]}>]+$', '', string_str)
                    if len(clean_url) > 10:  # 最短 URL 长度
                        urls.add(clean_url)
        except Exception:
            pass  # 静默错误，避免污染 JSON 输出

    # ========== 3. 提取 URL（从资源文件）NEW! ==========
    try:
        files = apk.get_files()
        # 只搜索文本文件（XML, JSON, TXT, properties等）
        text_extensions = ['.xml', '.json', '.txt', '.properties', '.html', '.js', '.css']
        text_files = [f for f in files if any(f.endswith(ext) for ext in text_extensions)]

        # 搜索所有文本文件（无限制）
        for file_path in text_files:
            try:
                # 跳过明显的系统文件
                if any(skip in file_path for skip in ['schemas.android.com', 'AndroidManifest']):
                    continue

                content = apk.get_file(file_path).decode('utf-8', errors='ignore')
                found_urls = url_pattern.findall(content)

                for url in found_urls:
                    # 过滤掉 Android 系统 schema URL
                    if 'schemas.android.com' in url:
                        continue

                    clean_url = re.sub(r'[.,;:!?\'")\]}>]+$', '', url)
                    if len(clean_url) > 10 and not clean_url.endswith('/apk/res'):
                        urls.add(clean_url)
            except:
                continue
    except Exception as e:
        pass  # 静默错误，避免污染 JSON 输出

    result["urls"] = sorted(list(urls))

    # ========== 4. 提取域名（过滤无关域名） ==========
    # 无关域名的黑名单（开源库、标准文档、学术网站等）
    domain_blacklist = [
        'apache.org', 'w3.org', 'mozilla.org', 'google.com', 'github.com',
        'stackoverflow.com', 'wikipedia.org', 'adobe.com', 'microsoft.com',
        'creativecommons.org', 'color.org', 'jpeg.org', 'ietf.org',
        'brucelindbloom.com', 'poynton.com', 'rjwagner49.com', 'unicode.org',
        'xmlpull.org', 'schemas.android.com', 'developer.android.com',
        'example.com', 'test.com', 'localhost', 'bootcss.com',
        'bugzil.la', 'bugzilla.mozilla.org', 'crbug.com',
        'apple.com', 'gstatic.com', 'googleapis.com', 'firebase.google.com',
        'ubc.ca', 'wolframalpha.com', 'zachstronaut.com', 'math.ubc.ca',
        '51purse.com'  # PDF阅读器示例域名
    ]

    domains = set()
    for url in urls:
        match = re.search(r'https?://([^/:?#]+)', url)
        if match:
            domain = match.group(1)
            # 移除端口号
            domain = re.sub(r':\d+$', '', domain)

            # 过滤无效域名
            if domain in ['a', 'b', 'c']:  # 单字母域名
                continue

            # 过滤黑名单中的域名
            if any(blacklist in domain for blacklist in domain_blacklist):
                continue

            # 过滤明显的路径片段（误识别为域名的）
            if '\\' in domain or domain.startswith('/'):
                continue

            domains.add(domain)

    result["domains"] = sorted(list(domains))

    # ========== 5. 提取敏感字符串（可选） ==========
    sensitive_patterns = [
        r'api[_-]?key',
        r'secret',
        r'token',
        r'password',
        r'access[_-]?token',
        r'private[_-]?key',
    ]

    sensitive_strings = set()
    try:
        # 使用之前收集的 DEX 字符串
        for string_str in all_dex_strings:
            for pattern in sensitive_patterns:
                if re.search(pattern, string_str, re.IGNORECASE):
                    # 限制长度，避免过长字符串
                    if 10 < len(string_str) < 200:
                        sensitive_strings.add(string_str)
                        break
    except Exception as e:
        pass  # 静默错误，避免污染 JSON 输出

    result["strings"] = sorted(list(sensitive_strings))[:50]  # 限制数量

    # ========== 6. 提取 Native 库 ==========
    try:
        result["native_libs"] = apk.get_libraries()
    except Exception as e:
        result["native_libs"] = []  # 静默错误，避免污染 JSON 输出

    # ========== 7. 提取证书信息 ==========
    try:
        certs = apk.get_certificates()
        if certs and len(certs) > 0:
            cert = certs[0]  # asn1crypto.x509.Certificate 对象

            # 解析 subject 获取 CN (开发者) 和 O (公司)
            subject_dict = {}
            issuer_dict = {}

            # 从 subject 提取字段
            if hasattr(cert, 'subject') and cert.subject:
                for rdn in cert.subject.chosen:
                    for attr in rdn:
                        attr_type = attr['type'].human_friendly
                        attr_value = attr['value'].native
                        subject_dict[attr_type] = attr_value

            # 从 issuer 提取字段
            if hasattr(cert, 'issuer') and cert.issuer:
                for rdn in cert.issuer.chosen:
                    for attr in rdn:
                        attr_type = attr['type'].human_friendly
                        attr_value = attr['value'].native
                        issuer_dict[attr_type] = attr_value

            result["certificates"] = {
                "subject": cert.subject.human_friendly if hasattr(cert.subject, 'human_friendly') else "",
                "issuer": cert.issuer.human_friendly if hasattr(cert.issuer, 'human_friendly') else "",
                "serial": str(cert.serial_number) if hasattr(cert, 'serial_number') else "",
                "not_before": cert.not_valid_before.isoformat() if hasattr(cert, 'not_valid_before') and cert.not_valid_before else "",
                "not_after": cert.not_valid_after.isoformat() if hasattr(cert, 'not_valid_after') and cert.not_valid_after else "",
                # 直接提供解析后的字段，方便 Go 端使用
                "developer": subject_dict.get("Common Name", ""),
                "company": subject_dict.get("Organization", ""),
                "organization_unit": subject_dict.get("Organizational Unit", ""),
                "country": subject_dict.get("Country", ""),
                "state": subject_dict.get("State/Province", ""),
                "locality": subject_dict.get("Locality", ""),
            }
    except Exception as e:
        result["certificates"] = {}  # 静默错误，避免污染 JSON 输出

    # ========== 8. 提取敏感 API 调用（从字符串中匹配） ==========
    # 不再遍历方法（太慢），改为从字符串中查找 API 引用
    sensitive_apis = [
        "getLocation",
        "getDeviceId",
        "getSubscriberId",
        "getSimSerialNumber",
        "getLine1Number",
        "execSQL",
        "rawQuery",
        "getSharedPreferences",
        "sendTextMessage",
    ]

    api_calls = []
    try:
        found_apis = set()
        for string_str in all_dex_strings:
            for api in sensitive_apis:
                if api in string_str and api not in found_apis:
                    api_calls.append({
                        "api": api,
                        "found_in": string_str[:100] if len(string_str) > 100 else string_str,
                    })
                    found_apis.add(api)
                    if len(api_calls) >= 50:
                        break
            if len(api_calls) >= 50:
                break
    except Exception as e:
        pass  # 静默错误，避免污染 JSON 输出

    result["api_calls"] = api_calls[:50]

    # ========== 9. 提取基础信息 ==========
    basic_info = {}
    try:
        basic_info["package_name"] = apk.get_package() or ""
        basic_info["version_name"] = apk.get_androidversion_name() or ""
        basic_info["version_code"] = str(apk.get_androidversion_code()) if apk.get_androidversion_code() else ""
        basic_info["app_name"] = apk.get_app_name() or ""
        basic_info["min_sdk"] = str(apk.get_min_sdk_version()) if apk.get_min_sdk_version() else ""
        basic_info["target_sdk"] = str(apk.get_target_sdk_version()) if apk.get_target_sdk_version() else ""
        basic_info["main_activity"] = apk.get_main_activity() or ""
    except Exception as e:
        pass  # 静默错误，避免污染 JSON 输出

    # ========== 9.1 aapt2 回退：如果 Androguard 解析失败，尝试用 aapt2 ==========
    if not basic_info.get("package_name"):
        try:
            import subprocess
            # 尝试使用 aapt2 dump badging
            aapt_result = subprocess.run(
                ["aapt2", "dump", "badging", apk_path],
                capture_output=True,
                text=True,
                timeout=30
            )
            if aapt_result.returncode == 0:
                output = aapt_result.stdout
                # 解析 package: name='xxx' versionCode='xxx' versionName='xxx'
                # re 已在文件顶部导入
                pkg_match = re.search(r"package:\s*name='([^']+)'", output)
                if pkg_match:
                    basic_info["package_name"] = pkg_match.group(1)
                ver_code_match = re.search(r"versionCode='([^']+)'", output)
                if ver_code_match:
                    basic_info["version_code"] = ver_code_match.group(1)
                ver_name_match = re.search(r"versionName='([^']+)'", output)
                if ver_name_match:
                    basic_info["version_name"] = ver_name_match.group(1)
                # application-label:'xxx'
                app_name_match = re.search(r"application-label:'([^']+)'", output)
                if app_name_match:
                    basic_info["app_name"] = app_name_match.group(1)
                # sdkVersion:'xx'
                min_sdk_match = re.search(r"sdkVersion:'([^']+)'", output)
                if min_sdk_match:
                    basic_info["min_sdk"] = min_sdk_match.group(1)
                # targetSdkVersion:'xx'
                target_sdk_match = re.search(r"targetSdkVersion:'([^']+)'", output)
                if target_sdk_match:
                    basic_info["target_sdk"] = target_sdk_match.group(1)
                # launchable-activity: name='xxx'
                main_activity_match = re.search(r"launchable-activity:\s*name='([^']+)'", output)
                if main_activity_match:
                    basic_info["main_activity"] = main_activity_match.group(1)
        except Exception:
            pass  # aapt2 也失败了，静默

    result["basic_info"] = basic_info

    # ========== 10. 记录耗时 ==========
    duration = (datetime.now() - start_time).total_seconds() * 1000
    result["analysis_duration_ms"] = int(duration)

    return result


def main():
    """主函数"""
    if len(sys.argv) != 2:
        print(json.dumps({
            "error": "Usage: python androguard_deep_analyzer.py <apk_path>"
        }))
        sys.exit(1)

    apk_path = sys.argv[1]

    try:
        result = deep_analyze(apk_path)
        print(json.dumps(result, ensure_ascii=False, indent=2))
    except Exception as e:
        error_result = {
            "error": str(e),
            "traceback": traceback.format_exc()
        }
        print(json.dumps(error_result, ensure_ascii=False, indent=2))
        sys.exit(1)


if __name__ == "__main__":
    main()
