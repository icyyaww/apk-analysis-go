/**
 * Universal DEX Dumper for Android
 * Supports: 360加固, 腾讯乐固, 爱加密, 梆梆加固, 娜迦加固, 网易易盾, 阿里聚安全 等
 *
 * 原理：
 * 1. Hook DexClassLoader 和 InMemoryDexClassLoader 的构造函数
 * 2. 当加固壳在运行时解密并加载真实DEX时，拦截并dump到本地
 * 3. Hook Native层的 art::DexFile::OpenMemory 捕获内存中的DEX
 */

Java.perform(function() {
    console.log("[*] Universal DEX Dumper v2.0 loaded");
    console.log("[*] Target package: " + Java.use("android.app.ActivityThread").currentPackageName());

    var dumpDir = "/data/local/tmp/dex_dump/";
    var dexCount = 0;
    var dumpedHashes = [];  // 用于去重

    // 确保输出目录存在
    try {
        var File = Java.use("java.io.File");
        var dir = File.$new(dumpDir);
        if (!dir.exists()) {
            dir.mkdirs();
        }
    } catch(e) {
        console.log("[-] Failed to create dump directory: " + e);
    }

    // ============================================
    // 1. Hook DexClassLoader (Android 4.x - 13)
    // ============================================
    try {
        var DexClassLoader = Java.use('dalvik.system.DexClassLoader');
        DexClassLoader.$init.overload('java.lang.String', 'java.lang.String', 'java.lang.String', 'java.lang.ClassLoader')
            .implementation = function(dexPath, optimizedDirectory, librarySearchPath, parent) {
                console.log("\n[+] ========== DexClassLoader ==========");
                console.log("[+] DEX Path: " + dexPath);
                console.log("[+] Optimized Dir: " + optimizedDirectory);
                console.log("[+] Library Path: " + librarySearchPath);

                // Dump DEX文件
                if (dexPath != null && dexPath.toString().length > 0) {
                    dumpDexFile(dexPath.toString(), "DexClassLoader");
                }

                return this.$init(dexPath, optimizedDirectory, librarySearchPath, parent);
            };
        console.log("[+] DexClassLoader hooked successfully");
    } catch(e) {
        console.log("[-] DexClassLoader hook failed: " + e);
    }

    // ============================================
    // 2. Hook PathClassLoader
    // ============================================
    try {
        var PathClassLoader = Java.use('dalvik.system.PathClassLoader');
        PathClassLoader.$init.overload('java.lang.String', 'java.lang.ClassLoader')
            .implementation = function(dexPath, parent) {
                console.log("\n[+] ========== PathClassLoader ==========");
                console.log("[+] DEX Path: " + dexPath);

                if (dexPath != null && dexPath.toString().indexOf("classes") > -1) {
                    dumpDexFile(dexPath.toString(), "PathClassLoader");
                }

                return this.$init(dexPath, parent);
            };
        console.log("[+] PathClassLoader hooked successfully");
    } catch(e) {
        console.log("[-] PathClassLoader hook failed: " + e);
    }

    // ============================================
    // 3. Hook InMemoryDexClassLoader (Android 8+)
    // ============================================
    try {
        var InMemoryDexClassLoader = Java.use('dalvik.system.InMemoryDexClassLoader');

        // 单个ByteBuffer
        try {
            InMemoryDexClassLoader.$init.overload('java.nio.ByteBuffer', 'java.lang.ClassLoader')
                .implementation = function(buffer, parent) {
                    console.log("\n[+] ========== InMemoryDexClassLoader (single) ==========");
                    console.log("[+] Buffer size: " + buffer.remaining());

                    dumpByteBuffer(buffer, "InMemoryDexClassLoader_single");

                    return this.$init(buffer, parent);
                };
            console.log("[+] InMemoryDexClassLoader (single) hooked");
        } catch(e) {}

        // ByteBuffer数组
        try {
            InMemoryDexClassLoader.$init.overload('[Ljava.nio.ByteBuffer;', 'java.lang.ClassLoader')
                .implementation = function(buffers, parent) {
                    console.log("\n[+] ========== InMemoryDexClassLoader (array) ==========");
                    console.log("[+] Buffer count: " + buffers.length);

                    for (var i = 0; i < buffers.length; i++) {
                        if (buffers[i] != null) {
                            console.log("[+] Dumping buffer " + i + ", size: " + buffers[i].remaining());
                            dumpByteBuffer(buffers[i], "InMemoryDexClassLoader_array_" + i);
                        }
                    }

                    return this.$init(buffers, parent);
                };
            console.log("[+] InMemoryDexClassLoader (array) hooked");
        } catch(e) {}

    } catch(e) {
        console.log("[-] InMemoryDexClassLoader not found (Android < 8?): " + e);
    }

    // ============================================
    // 4. Hook BaseDexClassLoader.pathList (更底层)
    // ============================================
    try {
        var BaseDexClassLoader = Java.use('dalvik.system.BaseDexClassLoader');
        var DexPathList = Java.use('dalvik.system.DexPathList');

        // Hook findClass来捕获类加载
        BaseDexClassLoader.findClass.implementation = function(name) {
            // 只在关键类加载时打印
            if (name.indexOf("Application") > -1 || name.indexOf("Activity") > -1) {
                console.log("[*] Loading class: " + name);
            }
            return this.findClass(name);
        };
    } catch(e) {
        console.log("[-] BaseDexClassLoader hook failed: " + e);
    }

    // ============================================
    // 5. Hook Native层 OpenMemory (libart.so)
    // ============================================
    try {
        // Android 7-10: _ZN3art7DexFile10OpenMemoryEPKhjRKNSt3__112basic_stringIcNS3_11char_traitsIcEENS3_9allocatorIcEEEEjPNS_6MemMapEPKNS_10OatDexFileEPS9_
        // Android 11+: 签名可能不同

        var libart = Process.findModuleByName("libart.so");
        if (libart) {
            var symbols = libart.enumerateExports();
            for (var i = 0; i < symbols.length; i++) {
                var symbol = symbols[i];
                if (symbol.name.indexOf("DexFile") > -1 && symbol.name.indexOf("OpenMemory") > -1) {
                    console.log("[*] Found OpenMemory symbol: " + symbol.name);

                    Interceptor.attach(symbol.address, {
                        onEnter: function(args) {
                            this.base = args[0];
                            this.size = args[1].toInt32();
                        },
                        onLeave: function(retval) {
                            if (this.size > 0x70 && this.size < 100 * 1024 * 1024) {  // 112B - 100MB
                                console.log("[+] Native OpenMemory: base=" + this.base + " size=" + this.size);
                                dumpNativeMemory(this.base, this.size, "native_OpenMemory");
                            }
                        }
                    });
                    break;
                }
            }
        }
    } catch(e) {
        console.log("[-] Native hook setup failed: " + e);
    }

    // ============================================
    // 6. 延迟扫描内存中的DEX (兜底方案)
    // ============================================
    setTimeout(function() {
        console.log("\n[*] Starting delayed memory scan for DEX files...");
        scanMemoryForDex();
    }, 10000);  // 10秒后扫描

    // ============================================
    // Helper Functions
    // ============================================

    function dumpDexFile(path, source) {
        try {
            var FileInputStream = Java.use('java.io.FileInputStream');
            var FileOutputStream = Java.use('java.io.FileOutputStream');
            var file = Java.use('java.io.File').$new(path);

            if (!file.exists() || !file.canRead()) {
                console.log("[-] Cannot read file: " + path);
                return;
            }

            var fileSize = file.length();
            if (fileSize < 112) {
                console.log("[-] File too small to be DEX: " + fileSize);
                return;
            }

            // 读取文件内容
            var fis = FileInputStream.$new(file);
            var bytes = Java.array('byte', new Array(parseInt(fileSize)).fill(0));
            fis.read(bytes);
            fis.close();

            // 检查是否为有效DEX
            if (!isValidDexBytes(bytes)) {
                console.log("[-] Not a valid DEX file: " + path);
                return;
            }

            // 计算哈希去重
            var hash = calculateHash(bytes);
            if (dumpedHashes.indexOf(hash) >= 0) {
                console.log("[*] DEX already dumped (hash: " + hash.substring(0, 8) + ")");
                return;
            }
            dumpedHashes.push(hash);

            // 写入文件
            var outPath = dumpDir + "dex_" + dexCount + "_" + source + ".dex";
            var fos = FileOutputStream.$new(outPath);
            fos.write(bytes);
            fos.close();

            console.log("[+] DEX dumped to: " + outPath + " (" + fileSize + " bytes)");
            dexCount++;

        } catch(e) {
            console.log("[-] Failed to dump DEX file: " + e);
        }
    }

    function dumpByteBuffer(buffer, source) {
        try {
            var size = buffer.remaining();
            if (size < 112) {
                console.log("[-] Buffer too small: " + size);
                return;
            }

            // 读取ByteBuffer内容
            var bytes = Java.array('byte', new Array(size).fill(0));
            var position = buffer.position();
            buffer.get(bytes);
            buffer.position(position);  // 恢复位置

            // 检查是否为有效DEX
            if (!isValidDexBytes(bytes)) {
                console.log("[-] Not a valid DEX in buffer");
                return;
            }

            // 计算哈希去重
            var hash = calculateHash(bytes);
            if (dumpedHashes.indexOf(hash) >= 0) {
                console.log("[*] DEX already dumped (hash: " + hash.substring(0, 8) + ")");
                return;
            }
            dumpedHashes.push(hash);

            // 写入文件
            var outPath = dumpDir + "memory_dex_" + dexCount + "_" + source + ".dex";
            var FileOutputStream = Java.use('java.io.FileOutputStream');
            var fos = FileOutputStream.$new(outPath);
            fos.write(bytes);
            fos.close();

            console.log("[+] Memory DEX dumped to: " + outPath + " (" + size + " bytes)");
            dexCount++;

        } catch(e) {
            console.log("[-] Failed to dump ByteBuffer: " + e);
        }
    }

    function dumpNativeMemory(base, size, source) {
        try {
            // 读取Native内存
            var data = Memory.readByteArray(base, size);
            if (data == null) {
                console.log("[-] Failed to read native memory");
                return;
            }

            // 转换为Java byte数组进行验证
            var bytes = new Uint8Array(data);

            // 检查DEX magic
            if (bytes[0] != 0x64 || bytes[1] != 0x65 || bytes[2] != 0x78 || bytes[3] != 0x0a) {
                console.log("[-] Invalid DEX magic in native memory");
                return;
            }

            // 写入文件
            var outPath = dumpDir + "native_dex_" + dexCount + "_" + source + ".dex";
            var file = new File(outPath, "wb");
            file.write(data);
            file.close();

            console.log("[+] Native DEX dumped to: " + outPath + " (" + size + " bytes)");
            dexCount++;

        } catch(e) {
            console.log("[-] Failed to dump native memory: " + e);
        }
    }

    function scanMemoryForDex() {
        try {
            // 扫描内存区域寻找DEX文件
            var ranges = Process.enumerateRanges({
                protection: 'r--',
                coalesce: true
            });

            console.log("[*] Scanning " + ranges.length + " memory ranges...");

            for (var i = 0; i < ranges.length; i++) {
                var range = ranges[i];
                if (range.size < 1024 * 1024 || range.size > 100 * 1024 * 1024) {
                    continue;  // 跳过太小或太大的区域
                }

                try {
                    // 搜索DEX magic "dex\n"
                    var pattern = "64 65 78 0a";  // "dex\n"
                    var matches = Memory.scanSync(range.base, range.size, pattern);

                    for (var j = 0; j < matches.length; j++) {
                        var match = matches[j];

                        // 读取DEX header获取文件大小
                        var header = Memory.readByteArray(match.address, 112);
                        if (header == null) continue;

                        var headerBytes = new Uint8Array(header);

                        // 从header获取file_size (offset 32, 4 bytes, little-endian)
                        var fileSize = headerBytes[32] | (headerBytes[33] << 8) |
                                      (headerBytes[34] << 16) | (headerBytes[35] << 24);

                        if (fileSize > 112 && fileSize < range.size) {
                            console.log("[*] Found potential DEX at " + match.address + ", size: " + fileSize);
                            dumpNativeMemory(match.address, fileSize, "memory_scan");
                        }
                    }
                } catch(e) {
                    // 忽略无法访问的内存区域
                }
            }

            console.log("[*] Memory scan completed. Total DEX dumped: " + dexCount);

        } catch(e) {
            console.log("[-] Memory scan failed: " + e);
        }
    }

    function isValidDexBytes(bytes) {
        if (bytes.length < 112) return false;

        // 检查magic: "dex\n"
        if (bytes[0] != 0x64 || bytes[1] != 0x65 || bytes[2] != 0x78 || bytes[3] != 0x0a) {
            return false;
        }
        return true;
    }

    function calculateHash(bytes) {
        try {
            var MessageDigest = Java.use('java.security.MessageDigest');
            var md = MessageDigest.getInstance("MD5");
            md.update(bytes);
            var digest = md.digest();

            var hex = "";
            for (var i = 0; i < digest.length; i++) {
                var b = digest[i] & 0xff;
                hex += (b < 16 ? "0" : "") + b.toString(16);
            }
            return hex;
        } catch(e) {
            return Math.random().toString();
        }
    }

    console.log("[*] DEX Dumper hooks installed successfully");
    console.log("[*] Waiting for DEX loading...");
    console.log("[*] Dump directory: " + dumpDir);
});
