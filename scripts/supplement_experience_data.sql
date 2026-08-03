\set ON_ERROR_STOP on

BEGIN;

SELECT pg_advisory_xact_lock(hashtext('taskpilot:supplement-experience-data:v1'));

DO $schema_check$
BEGIN
    IF to_regclass('public.users') IS NULL
        OR to_regclass('public.documents') IS NULL
        OR to_regclass('public.parse_jobs') IS NULL
        OR to_regclass('public.parse_results') IS NULL
        OR to_regclass('public.projects') IS NULL
        OR to_regclass('public.tasks') IS NULL THEN
        RAISE EXCEPTION 'TaskPilot schema is incomplete; run the database migration first';
    END IF;
END
$schema_check$;

CREATE TEMP TABLE requested_experience_users (
    email TEXT PRIMARY KEY,
    is_primary BOOLEAN NOT NULL
) ON COMMIT DROP;

INSERT INTO requested_experience_users (email, is_primary)
VALUES (LOWER(BTRIM(:'primary_email')), TRUE);

INSERT INTO requested_experience_users (email, is_primary)
SELECT LOWER(BTRIM(email)), FALSE
FROM regexp_split_to_table(:'additional_emails', ',') AS email
WHERE BTRIM(email) <> ''
ON CONFLICT (email) DO UPDATE
SET is_primary = requested_experience_users.is_primary OR EXCLUDED.is_primary;

DO $target_check$
DECLARE
    invalid_emails TEXT;
    missing_emails TEXT;
    inactive_emails TEXT;
BEGIN
    SELECT string_agg(email, ', ' ORDER BY email)
    INTO invalid_emails
    FROM requested_experience_users
    WHERE email = ''
        OR email !~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$';

    IF invalid_emails IS NOT NULL THEN
        RAISE EXCEPTION 'invalid target email(s): %', invalid_emails;
    END IF;

    SELECT string_agg(r.email, ', ' ORDER BY r.email)
    INTO missing_emails
    FROM requested_experience_users r
    LEFT JOIN users u ON LOWER(u.email) = r.email
    WHERE u.id IS NULL;

    IF missing_emails IS NOT NULL THEN
        RAISE EXCEPTION 'target user(s) do not exist: %. This script never creates users', missing_emails;
    END IF;

    SELECT string_agg(r.email, ', ' ORDER BY r.email)
    INTO inactive_emails
    FROM requested_experience_users r
    JOIN users u ON LOWER(u.email) = r.email
    WHERE u.status <> 1;

    IF inactive_emails IS NOT NULL THEN
        RAISE EXCEPTION 'target user(s) are inactive: %', inactive_emails;
    END IF;
END
$target_check$;

CREATE TEMP TABLE experience_seed_outcome (
    user_id BIGINT NOT NULL,
    email TEXT NOT NULL,
    seed_key TEXT NOT NULL,
    created BOOLEAN NOT NULL
) ON COMMIT DROP;

CREATE OR REPLACE FUNCTION pg_temp.add_experience_dataset(
    target_user_id BIGINT,
    seed_key TEXT,
    document_title TEXT,
    document_text TEXT,
    result_summary TEXT,
    deadline_after INTERVAL,
    deliverables JSONB,
    key_requirements JSONB,
    risk_warnings JSONB,
    task_specs JSONB,
    confirmed BOOLEAN,
    project_name TEXT,
    project_description TEXT,
    project_status TEXT,
    created_before INTERVAL
) RETURNS BOOLEAN
LANGUAGE plpgsql
AS $function$
DECLARE
    seed_model TEXT := 'taskpilot-experience-seed/' || seed_key;
    created_time TIMESTAMPTZ := CURRENT_TIMESTAMP - created_before;
    target_deadline TIMESTAMPTZ := CURRENT_TIMESTAMP + deadline_after;
    generated_tasks JSONB;
    document_id BIGINT;
    parse_job_id BIGINT;
    parse_result_id BIGINT;
    project_id BIGINT;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM parse_results
        WHERE user_id = target_user_id
            AND ai_model = seed_model
    ) THEN
        RETURN FALSE;
    END IF;

    SELECT COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'title', task.value->>'title',
                'description', task.value->>'description',
                'priority', task.value->>'priority',
                'deadline', CURRENT_TIMESTAMP + make_interval(days => (task.value->>'deadline_days')::INT)
            )
            ORDER BY task.ordinality
        ),
        '[]'::jsonb
    )
    INTO generated_tasks
    FROM jsonb_array_elements(task_specs) WITH ORDINALITY AS task(value, ordinality);

    INSERT INTO documents (
        user_id, source_type, title, text_input, raw_text, status, created_at, updated_at
    ) VALUES (
        target_user_id, 'text', document_title, document_text, document_text,
        'ready', created_time, created_time
    ) RETURNING id INTO document_id;

    INSERT INTO parse_jobs (
        user_id, document_id, job_type, status, retry_count,
        started_at, finished_at, created_at, updated_at
    ) VALUES (
        target_user_id, document_id, 'ai_parse', 'success', 0,
        created_time, created_time + INTERVAL '16 seconds',
        created_time, created_time + INTERVAL '16 seconds'
    ) RETURNING id INTO parse_job_id;

    INSERT INTO parse_results (
        user_id, document_id, parse_job_id, title, summary, deadline,
        deliverables, key_requirements, risk_warnings, generated_tasks,
        ai_model, version, is_confirmed, created_at, updated_at
    ) VALUES (
        target_user_id, document_id, parse_job_id, document_title, result_summary,
        target_deadline, deliverables, key_requirements, risk_warnings, generated_tasks,
        seed_model, 1, confirmed, created_time + INTERVAL '18 seconds',
        created_time + INTERVAL '18 seconds'
    ) RETURNING id INTO parse_result_id;

    IF project_name IS NOT NULL THEN
        IF NOT confirmed THEN
            RAISE EXCEPTION 'seed % cannot create a project from an unconfirmed result', seed_key;
        END IF;
        IF project_status NOT IN ('active', 'archived') THEN
            RAISE EXCEPTION 'seed % has invalid project status %', seed_key, project_status;
        END IF;

        INSERT INTO projects (
            user_id, source_document_id, parse_result_id, name, description,
            deadline, status, version, created_at, updated_at
        ) VALUES (
            target_user_id, document_id, parse_result_id, project_name,
            project_description, target_deadline, project_status, 1,
            created_time + INTERVAL '1 minute', created_time + INTERVAL '1 minute'
        ) RETURNING id INTO project_id;

        INSERT INTO tasks (
            project_id, user_id, source_parse_result_id, title, description,
            status, priority, deadline, sort_order, source_type, version,
            created_at, updated_at
        )
        SELECT
            project_id,
            target_user_id,
            parse_result_id,
            task.value->>'title',
            task.value->>'description',
            task.value->>'status',
            task.value->>'priority',
            CURRENT_TIMESTAMP + make_interval(days => (task.value->>'deadline_days')::INT),
            (task.ordinality - 1)::INT,
            'ai',
            1,
            created_time + INTERVAL '1 minute',
            CASE
                WHEN task.value->>'status' = 'done' THEN created_time + INTERVAL '2 days'
                WHEN task.value->>'status' = 'doing' THEN CURRENT_TIMESTAMP - INTERVAL '2 hours'
                ELSE created_time + INTERVAL '1 minute'
            END
        FROM jsonb_array_elements(task_specs) WITH ORDINALITY AS task(value, ordinality);
    END IF;

    RETURN TRUE;
END
$function$;

DO $seed$
DECLARE
    target RECORD;
    was_created BOOLEAN;
BEGIN
    FOR target IN
        SELECT u.id AS user_id, LOWER(u.email) AS email
        FROM requested_experience_users r
        JOIN users u ON LOWER(u.email) = r.email
        ORDER BY r.is_primary DESC, r.email
    LOOP
        was_created := pg_temp.add_experience_dataset(
            target.user_id,
            'innovation-contest-v1',
            '全国大学生创新创业大赛备赛要求',
            '学校计划组织团队参加全国大学生创新创业大赛。请在四周内完成选题论证、用户调研、商业计划书、路演材料和答辩演练。商业计划书需包含市场分析、解决方案、商业模式、财务预测与团队分工，路演控制在八分钟内。',
            '四周内完成创新创业大赛的选题验证、商业计划书、路演材料与答辩演练。',
            INTERVAL '28 days',
            '["选题论证报告", "用户调研摘要", "商业计划书", "路演演示文稿", "答辩问题清单"]'::jsonb,
            '["商业计划书覆盖市场、方案、模式、财务与团队", "路演时长不超过 8 分钟", "所有结论需有调研或数据依据"]'::jsonb,
            '["选题范围过大导致验证不足", "财务预测缺少可靠依据", "团队成员时间冲突影响演练"]'::jsonb,
            '[
                {"title":"完成选题与竞品初筛","description":"明确目标用户、核心痛点与差异化方向。","priority":"high","status":"done","deadline_days":3},
                {"title":"访谈 10 位目标用户","description":"记录使用场景、现有替代方案与付费意愿。","priority":"high","status":"doing","deadline_days":8},
                {"title":"撰写商业计划书初稿","description":"完成市场、方案、模式、财务和团队章节。","priority":"high","status":"todo","deadline_days":15},
                {"title":"制作八分钟路演稿","description":"将商业计划书压缩为清晰的路演叙事。","priority":"medium","status":"todo","deadline_days":21},
                {"title":"组织两轮模拟答辩","description":"记录问题并逐项补充证据和回答。","priority":"medium","status":"todo","deadline_days":26}
            ]'::jsonb,
            TRUE,
            '创新创业大赛备赛',
            '体验项目：展示进行中项目、不同任务状态、优先级与截止时间。',
            'active',
            INTERVAL '6 days'
        );
        INSERT INTO experience_seed_outcome VALUES (
            target.user_id, target.email, 'innovation-contest-v1', was_created
        );

        was_created := pg_temp.add_experience_dataset(
            target.user_id,
            'database-course-v1',
            '数据库课程设计验收说明',
            '课程设计要求提交需求分析、ER 图、数据库脚本、接口演示和总结报告。系统必须包含至少五张关联表，说明索引设计与事务边界，并在验收时现场演示核心业务流程。',
            '完成数据库课程设计的建模、实现、测试和验收材料，形成可复现的课程交付物。',
            INTERVAL '-10 days',
            '["需求分析文档", "ER 图", "建表与初始化脚本", "接口演示", "课程设计报告"]'::jsonb,
            '["至少包含 5 张关联表", "说明索引设计依据", "核心写操作使用事务", "提交步骤可由他人复现"]'::jsonb,
            '["测试数据覆盖不足", "演示环境与开发环境不一致", "文档与最终数据库结构不同步"]'::jsonb,
            '[
                {"title":"完成需求分析与数据字典","description":"确认实体、字段、约束和主要业务流程。","priority":"high","status":"done","deadline_days":-24},
                {"title":"绘制并评审 ER 图","description":"检查实体关系、基数和范式设计。","priority":"high","status":"done","deadline_days":-20},
                {"title":"实现迁移与测试数据脚本","description":"确保空数据库可以一次性完成初始化。","priority":"high","status":"done","deadline_days":-16},
                {"title":"录制核心流程演示","description":"覆盖创建、查询、修改和事务回滚场景。","priority":"medium","status":"done","deadline_days":-12},
                {"title":"整理课程设计总结报告","description":"记录方案、测试结果与后续改进。","priority":"medium","status":"done","deadline_days":-10}
            ]'::jsonb,
            TRUE,
            '数据库课程设计',
            '体验项目：展示已归档项目及已完成任务的历史记录。',
            'archived',
            INTERVAL '35 days'
        );
        INSERT INTO experience_seed_outcome VALUES (
            target.user_id, target.email, 'database-course-v1', was_created
        );

        was_created := pg_temp.add_experience_dataset(
            target.user_id,
            'internship-application-v1',
            '暑期实习申请材料准备',
            '计划在三周内投递后端开发实习岗位。需要更新中英文简历、整理两个代表项目、准备自我介绍和常见面试题，并建立投递记录表。确认解析结果前需要检查项目数据是否准确。',
            '三周内完成后端开发实习所需材料与面试准备，当前解析结果等待用户确认。',
            INTERVAL '21 days',
            '["中英文简历", "项目介绍材料", "自我介绍", "面试题复习清单", "岗位投递记录表"]'::jsonb,
            '["项目经历使用量化结果", "简历控制在两页以内", "每次投递记录岗位与反馈状态"]'::jsonb,
            '["准备范围过宽导致重点不突出", "简历项目数据可能不准确", "集中投递后面试时间冲突"]'::jsonb,
            '[
                {"title":"核对简历基础信息","description":"检查教育经历、时间线和联系方式。","priority":"high","status":"todo","deadline_days":3},
                {"title":"重写两个项目经历","description":"按背景、行动和量化结果组织内容。","priority":"high","status":"todo","deadline_days":7},
                {"title":"准备三分钟自我介绍","description":"分别准备中文和英文版本。","priority":"medium","status":"todo","deadline_days":10},
                {"title":"整理后端面试知识清单","description":"覆盖 Go、数据库、网络和常见系统设计题。","priority":"high","status":"todo","deadline_days":16}
            ]'::jsonb,
            FALSE,
            NULL,
            NULL,
            NULL,
            INTERVAL '1 day'
        );
        INSERT INTO experience_seed_outcome VALUES (
            target.user_id, target.email, 'internship-application-v1', was_created
        );
    END LOOP;
END
$seed$;

SELECT
    email,
    COUNT(*) FILTER (WHERE created) AS datasets_created,
    COUNT(*) FILTER (WHERE NOT created) AS datasets_skipped,
    COUNT(*) FILTER (WHERE created AND seed_key <> 'internship-application-v1') AS projects_created,
    COUNT(*) FILTER (WHERE created) AS parse_results_created
FROM experience_seed_outcome
GROUP BY user_id, email
ORDER BY email;

COMMIT;
