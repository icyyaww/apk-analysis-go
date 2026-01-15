-- ============================================
-- Migration: 添加壳检测和动态脱壳相关表和字段
-- Date: 2026-01-13
-- Description: 支持APK壳检测和动态脱壳功能
-- ============================================

-- 1. 扩展 task_static_reports 表，添加壳检测相关字段
ALTER TABLE task_static_reports
ADD COLUMN IF NOT EXISTS is_packed BOOLEAN DEFAULT FALSE COMMENT '是否检测到加壳',
ADD COLUMN IF NOT EXISTS packer_name VARCHAR(100) COMMENT '壳名称',
ADD COLUMN IF NOT EXISTS packer_type VARCHAR(50) COMMENT '壳类型: native/dex_encrypt/vmp/unknown',
ADD COLUMN IF NOT EXISTS packer_confidence DECIMAL(3,2) COMMENT '壳检测置信度 0.00-1.00',
ADD COLUMN IF NOT EXISTS packer_indicators JSON COMMENT '壳检测特征列表',
ADD COLUMN IF NOT EXISTS needs_dynamic_unpacking BOOLEAN DEFAULT FALSE COMMENT '是否需要动态脱壳',
ADD COLUMN IF NOT EXISTS packer_detection_duration_ms INT COMMENT '壳检测耗时(毫秒)';

-- 2. 创建脱壳结果表
CREATE TABLE IF NOT EXISTS task_unpacking_results (
    id INT PRIMARY KEY AUTO_INCREMENT,
    task_id VARCHAR(36) NOT NULL COMMENT '关联任务ID',

    -- 脱壳状态
    status VARCHAR(50) NOT NULL COMMENT '脱壳状态: pending/running/success/failed/timeout/skipped',
    method VARCHAR(50) COMMENT '脱壳方法: frida_dex_dump/frida_class_loader/manual',

    -- 脱壳结果
    dumped_dex_count INT DEFAULT 0 COMMENT 'Dump的DEX文件数量',
    dumped_dex_paths JSON COMMENT 'Dump的DEX文件路径列表',
    merged_dex_path VARCHAR(500) COMMENT '合并后的DEX文件路径',
    total_size BIGINT DEFAULT 0 COMMENT 'DEX文件总大小(字节)',
    duration_ms INT COMMENT '脱壳耗时(毫秒)',

    -- 错误信息
    error_message TEXT COMMENT '错误信息',

    -- 时间戳
    started_at TIMESTAMP NULL COMMENT '开始时间',
    completed_at TIMESTAMP NULL COMMENT '完成时间',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',

    -- 外键和索引
    FOREIGN KEY (task_id) REFERENCES apk_tasks(id) ON DELETE CASCADE,
    UNIQUE KEY uk_task_id (task_id),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='任务脱壳结果表';

-- 3. 创建索引优化查询
CREATE INDEX IF NOT EXISTS idx_static_is_packed ON task_static_reports(is_packed);
CREATE INDEX IF NOT EXISTS idx_static_packer_name ON task_static_reports(packer_name);

-- 4. 添加壳统计视图 (可选)
CREATE OR REPLACE VIEW v_packer_statistics AS
SELECT
    packer_name,
    packer_type,
    COUNT(*) AS total_count,
    AVG(packer_confidence) AS avg_confidence,
    SUM(CASE WHEN needs_dynamic_unpacking = TRUE THEN 1 ELSE 0 END) AS needs_unpack_count
FROM task_static_reports
WHERE is_packed = TRUE
GROUP BY packer_name, packer_type
ORDER BY total_count DESC;

-- 5. 添加脱壳成功率视图 (可选)
CREATE OR REPLACE VIEW v_unpacking_statistics AS
SELECT
    method,
    COUNT(*) AS total_attempts,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS success_count,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_count,
    SUM(CASE WHEN status = 'timeout' THEN 1 ELSE 0 END) AS timeout_count,
    SUM(CASE WHEN status = 'skipped' THEN 1 ELSE 0 END) AS skipped_count,
    AVG(duration_ms) AS avg_duration_ms,
    AVG(dumped_dex_count) AS avg_dex_count
FROM task_unpacking_results
GROUP BY method;
