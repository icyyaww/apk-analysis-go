-- 迁移: 添加静态和动态分析完成标记字段
-- 目的: 实现精确的域名分析触发机制，确保静态+动态分析都完成后才触发域名分析
-- 日期: 2025-01-18

ALTER TABLE apk_tasks
ADD COLUMN static_analysis_completed BOOLEAN DEFAULT FALSE COMMENT '静态分析完成标记' AFTER install_result,
ADD COLUMN dynamic_analysis_completed BOOLEAN DEFAULT FALSE COMMENT '动态分析完成标记' AFTER static_analysis_completed;

-- 为已完成的任务更新标记（补充历史数据）
-- 如果任务状态是 completed 且有 MobSF 报告，则认为静态分析完成
UPDATE apk_tasks t
INNER JOIN task_mobsf_reports m ON t.id = m.task_id
SET t.static_analysis_completed = TRUE
WHERE t.status = 'completed'
  AND m.status = 'completed';

-- 如果任务状态是 completed 且有 Activity 数据，则认为动态分析完成
UPDATE apk_tasks t
INNER JOIN task_activities a ON t.id = a.task_id
SET t.dynamic_analysis_completed = TRUE
WHERE t.status = 'completed';

-- 说明：
-- 1. static_analysis_completed: MobSF 静态分析完成时设置为 true
-- 2. dynamic_analysis_completed: Orchestrator 动态分析完成时设置为 true
-- 3. 域名分析触发条件: static_analysis_completed AND dynamic_analysis_completed = TRUE
-- 4. 这样可以避免在其中一个分析完成时就触发域名分析导致数据不完整的问题
