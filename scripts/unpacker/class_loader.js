/**
 * ClassLoader DEX Dumper for Android
 * 专门针对 DexGuard 等使用动态ClassLoader加载的加固
 *
 * 原理：
 * 1. Hook defineClass / loadClass 等类加载方法
 * 2. 在类被实际加载时dump对应的DEX
 * 3. 监控所有ClassLoader的创建和使用
 */

Java.perform(function() {
    console.log("[*] ClassLoader DEX Dumper loaded");

    var dumpDir = "/data/local/tmp/dex_dump/";
    var dexCount = 0;
    var processedClassLoaders = [];

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
    // 1. Hook ClassLoader.loadClass
    // ============================================
    try {
        var ClassLoader = Java.use('java.lang.ClassLoader');

        ClassLoader.loadClass.overload('java.lang.String').implementation = function(className) {
            // 检查是否是新的ClassLoader
            var clKey = this.toString();
            if (processedClassLoaders.indexOf(clKey) < 0) {
                processedClassLoaders.push(clKey);
                console.log("[*] New ClassLoader detected: " + clKey);
                tryDumpClassLoaderDex(this);
            }
            return this.loadClass(className);
        };

        ClassLoader.loadClass.overload('java.lang.String', 'boolean').implementation = function(className, resolve) {
            var clKey = this.toString();
            if (processedClassLoaders.indexOf(clKey) < 0) {
                processedClassLoaders.push(clKey);
                console.log("[*] New ClassLoader detected: " + clKey);
                tryDumpClassLoaderDex(this);
            }
            return this.loadClass(className, resolve);
        };

        console.log("[+] ClassLoader.loadClass hooked");
    } catch(e) {
        console.log("[-] ClassLoader hook failed: " + e);
    }

    // ============================================
    // 2. Hook BaseDexClassLoader
    // ============================================
    try {
        var BaseDexClassLoader = Java.use('dalvik.system.BaseDexClassLoader');

        // Hook构造函数
        BaseDexClassLoader.$init.overload('java.lang.String', 'java.io.File', 'java.lang.String', 'java.lang.ClassLoader')
            .implementation = function(dexPath, optimizedDirectory, librarySearchPath, parent) {
                console.log("\n[+] ========== BaseDexClassLoader Created ==========");
                console.log("[+] DEX Path: " + dexPath);

                this.$init(dexPath, optimizedDirectory, librarySearchPath, parent);

                // 尝试dump DEX
                setTimeout(function() {
                    tryDumpFromPathList(this);
                }.bind(this), 1000);

                return;
            };

        console.log("[+] BaseDexClassLoader hooked");
    } catch(e) {
        console.log("[-] BaseDexClassLoader hook failed: " + e);
    }

    // ============================================
    // 3. 监控 DexPathList.dexElements
    // ============================================
    try {
        var DexPathList = Java.use('dalvik.system.DexPathList');
        var DexPathList_Element = Java.use('dalvik.system.DexPathList$Element');

        // 获取dexElements字段
        var dexElementsField = DexPathList.class.getDeclaredField('dexElements');
        dexElementsField.setAccessible(true);

        console.log("[+] DexPathList reflection setup completed");
    } catch(e) {
        console.log("[-] DexPathList reflection failed: " + e);
    }

    // ============================================
    // 4. 定期扫描所有ClassLoader
    // ============================================
    setInterval(function() {
        scanAllClassLoaders();
    }, 5000);

    // ============================================
    // Helper Functions
    // ============================================

    function tryDumpClassLoaderDex(classLoader) {
        try {
            var clClass = classLoader.getClass();
            var clName = clClass.getName();

            console.log("[*] Analyzing ClassLoader: " + clName);

            // 检查是否是BaseDexClassLoader的子类
            if (clName.indexOf("DexClassLoader") > -1 ||
                clName.indexOf("PathClassLoader") > -1 ||
                clName.indexOf("InMemoryDexClassLoader") > -1) {

                tryDumpFromPathList(classLoader);
            }

            // 检查自定义ClassLoader
            if (clName.indexOf("com.stub") > -1 ||
                clName.indexOf("com.tencent") > -1 ||
                clName.indexOf("com.qihoo") > -1 ||
                clName.indexOf("shell") > -1) {

                console.log("[!] Detected potential packer ClassLoader: " + clName);
                tryDumpFromPathList(classLoader);
            }

        } catch(e) {
            console.log("[-] Failed to analyze ClassLoader: " + e);
        }
    }

    function tryDumpFromPathList(classLoader) {
        try {
            // 获取pathList字段
            var pathListField = Java.use('dalvik.system.BaseDexClassLoader').class.getDeclaredField('pathList');
            pathListField.setAccessible(true);

            var pathList = pathListField.get(classLoader);
            if (pathList == null) {
                console.log("[-] pathList is null");
                return;
            }

            // 获取dexElements字段
            var dexElementsField = Java.use('dalvik.system.DexPathList').class.getDeclaredField('dexElements');
            dexElementsField.setAccessible(true);

            var dexElements = dexElementsField.get(pathList);
            if (dexElements == null) {
                console.log("[-] dexElements is null");
                return;
            }

            console.log("[*] Found " + dexElements.length + " dexElements");

            // 遍历每个dexElement
            for (var i = 0; i < dexElements.length; i++) {
                var element = dexElements[i];
                if (element == null) continue;

                try {
                    // 获取dexFile字段
                    var dexFileField = element.getClass().getDeclaredField('dexFile');
                    dexFileField.setAccessible(true);
                    var dexFile = dexFileField.get(element);

                    if (dexFile != null) {
                        dumpDexFile(dexFile, i);
                    }
                } catch(e) {
                    // 尝试其他字段名
                    try {
                        var pathField = element.getClass().getDeclaredField('path');
                        pathField.setAccessible(true);
                        var path = pathField.get(element);
                        if (path != null) {
                            console.log("[*] DEX path: " + path);
                            dumpDexFromPath(path.toString());
                        }
                    } catch(e2) {}
                }
            }

        } catch(e) {
            console.log("[-] Failed to dump from pathList: " + e);
        }
    }

    function dumpDexFile(dexFile, index) {
        try {
            var DexFile = Java.use('dalvik.system.DexFile');

            // 获取DEX文件名
            var fileName = dexFile.getName();
            console.log("[*] Processing DexFile: " + fileName);

            // 尝试获取原始DEX数据
            // 方法1: 使用反射获取mCookie并读取内存
            try {
                var mCookieField = DexFile.class.getDeclaredField('mCookie');
                mCookieField.setAccessible(true);
                var mCookie = mCookieField.get(dexFile);

                if (mCookie != null) {
                    console.log("[*] Found mCookie, attempting native dump...");
                    // mCookie包含native指针，需要通过native方式读取
                    // 这里简化处理，直接读取文件
                }
            } catch(e) {}

            // 方法2: 如果是文件路径，直接读取文件
            if (fileName != null && fileName.length > 0) {
                dumpDexFromPath(fileName);
            }

        } catch(e) {
            console.log("[-] Failed to dump DexFile: " + e);
        }
    }

    function dumpDexFromPath(path) {
        try {
            var File = Java.use('java.io.File');
            var FileInputStream = Java.use('java.io.FileInputStream');
            var FileOutputStream = Java.use('java.io.FileOutputStream');

            var file = File.$new(path);
            if (!file.exists() || !file.canRead()) {
                console.log("[-] Cannot access file: " + path);
                return;
            }

            var fileSize = file.length();
            if (fileSize < 112) {
                console.log("[-] File too small: " + fileSize);
                return;
            }

            // 读取文件
            var fis = FileInputStream.$new(file);
            var bytes = Java.array('byte', new Array(parseInt(fileSize)).fill(0));
            fis.read(bytes);
            fis.close();

            // 验证DEX magic
            if (bytes[0] != 0x64 || bytes[1] != 0x65 || bytes[2] != 0x78) {
                console.log("[-] Invalid DEX magic");
                return;
            }

            // 写入dump文件
            var outPath = dumpDir + "classloader_dex_" + dexCount + ".dex";
            var fos = FileOutputStream.$new(outPath);
            fos.write(bytes);
            fos.close();

            console.log("[+] DEX dumped to: " + outPath + " (" + fileSize + " bytes)");
            dexCount++;

        } catch(e) {
            console.log("[-] Failed to dump DEX from path: " + e);
        }
    }

    function scanAllClassLoaders() {
        try {
            // 枚举所有已加载的类，获取它们的ClassLoader
            Java.enumerateLoadedClasses({
                onMatch: function(className) {
                    try {
                        var cls = Java.use(className);
                        var classLoader = cls.class.getClassLoader();

                        if (classLoader != null) {
                            var clKey = classLoader.toString();
                            if (processedClassLoaders.indexOf(clKey) < 0) {
                                processedClassLoaders.push(clKey);
                                console.log("[*] Found new ClassLoader via class scan: " + clKey);
                                tryDumpClassLoaderDex(classLoader);
                            }
                        }
                    } catch(e) {}
                },
                onComplete: function() {}
            });
        } catch(e) {
            console.log("[-] Class scan failed: " + e);
        }
    }

    console.log("[*] ClassLoader DEX Dumper hooks installed");
    console.log("[*] Monitoring ClassLoader activities...");
    console.log("[*] Dump directory: " + dumpDir);
});
