#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-"$ROOT_DIR/docker-compose.prod.yml"}
ENV_FILE=${ENV_FILE:-"$ROOT_DIR/.env.prod"}

if [ "${1:-}" != "--confirm-dev" ]; then
	echo "usage: $0 --confirm-dev" >&2
	echo "this script must only be run against a development database" >&2
	exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
	echo "missing compose file: $COMPOSE_FILE" >&2
	exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
	echo "missing environment file: $ENV_FILE" >&2
	exit 1
fi

POSTGRES_USER=${POSTGRES_USER:-taskpilot}
POSTGRES_DB=${POSTGRES_DB:-taskpilot}

echo "Seeding development data into PostgreSQL service 'postgres'..."

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" exec -T postgres \
	psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" <<'SQL'
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TEMP TABLE seed_credentials (
    password TEXT NOT NULL,
    password_hash TEXT NOT NULL
) ON COMMIT DROP;

WITH generated AS (
    SELECT 'TP-' || encode(gen_random_bytes(12), 'hex') AS password
)
INSERT INTO seed_credentials (password, password_hash)
SELECT password, crypt(password, gen_salt('bf', 10))
FROM generated;

DO $seed$
DECLARE
    seed_emails CONSTANT TEXT[] := ARRAY[
        'seed.dev01@taskpilot.1kuansi.cn',
        'seed.dev02@taskpilot.1kuansi.cn',
        'seed.dev03@taskpilot.1kuansi.cn',
        'seed.dev04@taskpilot.1kuansi.cn',
        'seed.dev05@taskpilot.1kuansi.cn',
        'seed.dev06@taskpilot.1kuansi.cn',
        'seed.dev07@taskpilot.1kuansi.cn',
        'seed.dev08@taskpilot.1kuansi.cn',
        'seed.dev@taskpilot.1kuansi.cn'
    ];
    seed_password_hash TEXT;
    seed_user_id BIGINT;
    launch_document_id BIGINT;
    launch_job_id BIGINT;
    launch_result_id BIGINT;
    launch_project_id BIGINT;
    archived_document_id BIGINT;
    archived_job_id BIGINT;
    archived_result_id BIGINT;
    archived_project_id BIGINT;
    review_document_id BIGINT;
    review_job_id BIGINT;
    pending_document_id BIGINT;
    failed_document_id BIGINT;
    account_no INTEGER;
    account_email TEXT;
    secondary_user_id BIGINT;
    secondary_document_id BIGINT;
    secondary_job_id BIGINT;
    secondary_result_id BIGINT;
    secondary_project_id BIGINT;
BEGIN
    IF to_regclass('public.users') IS NULL
        OR to_regclass('public.documents') IS NULL
        OR to_regclass('public.parse_jobs') IS NULL
        OR to_regclass('public.parse_results') IS NULL
        OR to_regclass('public.projects') IS NULL
        OR to_regclass('public.tasks') IS NULL THEN
        RAISE EXCEPTION 'TaskPilot schema is incomplete; run the database migration first';
    END IF;

    SELECT password_hash INTO seed_password_hash FROM seed_credentials;

    DELETE FROM tasks
    WHERE user_id IN (SELECT id FROM users WHERE LOWER(email) = ANY(seed_emails));
    DELETE FROM projects
    WHERE user_id IN (SELECT id FROM users WHERE LOWER(email) = ANY(seed_emails));
    DELETE FROM parse_results
    WHERE user_id IN (SELECT id FROM users WHERE LOWER(email) = ANY(seed_emails));
    DELETE FROM parse_jobs
    WHERE user_id IN (SELECT id FROM users WHERE LOWER(email) = ANY(seed_emails));
    DELETE FROM documents
    WHERE user_id IN (SELECT id FROM users WHERE LOWER(email) = ANY(seed_emails));
    DELETE FROM users WHERE LOWER(email) = ANY(seed_emails);

    INSERT INTO users (
        email, password_hash, nickname, avatar_url, status, created_at, updated_at
    ) VALUES (
        seed_emails[1],
        seed_password_hash,
        '开发环境主体验账号',
        'https://api.dicebear.com/9.x/initials/svg?seed=TP01',
        1,
        CURRENT_TIMESTAMP - INTERVAL '30 days',
        CURRENT_TIMESTAMP
    ) RETURNING id INTO seed_user_id;

    INSERT INTO documents (
        user_id, source_type, title, text_input, raw_text, status, created_at, updated_at
    ) VALUES (
        seed_user_id,
        'text',
        'TaskPilot v1.0 上线计划',
        '目标是在两周内完成 TaskPilot v1.0 上线。需要完成回归测试、部署检查、用户文档和发布公告，并确保核心接口可用。',
        '目标是在两周内完成 TaskPilot v1.0 上线。需要完成回归测试、部署检查、用户文档和发布公告，并确保核心接口可用。',
        'ready',
        CURRENT_TIMESTAMP - INTERVAL '10 days',
        CURRENT_TIMESTAMP - INTERVAL '10 days'
    ) RETURNING id INTO launch_document_id;

    INSERT INTO parse_jobs (
        user_id, document_id, job_type, status, retry_count,
        started_at, finished_at, created_at, updated_at
    ) VALUES (
        seed_user_id,
        launch_document_id,
        'ai_parse',
        'success',
        0,
        CURRENT_TIMESTAMP - INTERVAL '10 days',
        CURRENT_TIMESTAMP - INTERVAL '10 days' + INTERVAL '18 seconds',
        CURRENT_TIMESTAMP - INTERVAL '10 days',
        CURRENT_TIMESTAMP - INTERVAL '10 days' + INTERVAL '18 seconds'
    ) RETURNING id INTO launch_job_id;

    INSERT INTO parse_results (
        user_id, document_id, parse_job_id, title, summary, deadline,
        deliverables, key_requirements, risk_warnings, generated_tasks,
        ai_model, version, is_confirmed, created_at, updated_at
    ) VALUES (
        seed_user_id,
        launch_document_id,
        launch_job_id,
        'TaskPilot v1.0 上线计划',
        '在两周内完成产品回归、部署验证和发布物料，按计划发布 TaskPilot v1.0。',
        CURRENT_TIMESTAMP + INTERVAL '14 days',
        '["回归测试报告", "生产部署检查表", "用户使用文档", "版本发布公告"]'::jsonb,
        '["核心接口通过回归测试", "发布前完成数据库备份", "上线后持续观察服务指标"]'::jsonb,
        '["测试时间可能不足", "生产配置与开发环境存在差异", "上线高峰可能出现性能波动"]'::jsonb,
        '[
            {"title":"冻结 v1.0 功能范围","priority":"high"},
            {"title":"执行核心流程回归测试","priority":"high"},
            {"title":"核对生产环境配置","priority":"high"},
            {"title":"编写用户使用文档","priority":"medium"},
            {"title":"准备版本发布公告","priority":"medium"}
        ]'::jsonb,
        'gpt-5-mini',
        1,
        TRUE,
        CURRENT_TIMESTAMP - INTERVAL '10 days' + INTERVAL '20 seconds',
        CURRENT_TIMESTAMP - INTERVAL '9 days'
    ) RETURNING id INTO launch_result_id;

    INSERT INTO projects (
        user_id, source_document_id, parse_result_id, name, description,
        deadline, status, created_at, updated_at
    ) VALUES (
        seed_user_id,
        launch_document_id,
        launch_result_id,
        'TaskPilot v1.0 发布',
        '用于验证项目列表、任务看板、优先级和进度状态的主要测试项目。',
        CURRENT_TIMESTAMP + INTERVAL '14 days',
        'active',
        CURRENT_TIMESTAMP - INTERVAL '9 days',
        CURRENT_TIMESTAMP - INTERVAL '1 hour'
    ) RETURNING id INTO launch_project_id;

    INSERT INTO tasks (
        project_id, user_id, source_parse_result_id, title, description,
        status, priority, deadline, sort_order, source_type, created_at, updated_at
    ) VALUES
        (launch_project_id, seed_user_id, launch_result_id, '冻结 v1.0 功能范围', '确认本次发布包含的功能与延期项。', 'done', 'high', CURRENT_TIMESTAMP - INTERVAL '7 days', 0, 'ai', CURRENT_TIMESTAMP - INTERVAL '9 days', CURRENT_TIMESTAMP - INTERVAL '7 days'),
        (launch_project_id, seed_user_id, launch_result_id, '执行核心流程回归测试', '覆盖注册、登录、文档解析和项目任务流程。', 'doing', 'high', CURRENT_TIMESTAMP + INTERVAL '3 days', 1, 'ai', CURRENT_TIMESTAMP - INTERVAL '9 days', CURRENT_TIMESTAMP - INTERVAL '2 hours'),
        (launch_project_id, seed_user_id, launch_result_id, '核对生产环境配置', '检查数据库、Redis、JWT 和反向代理配置。', 'todo', 'high', CURRENT_TIMESTAMP + INTERVAL '6 days', 2, 'ai', CURRENT_TIMESTAMP - INTERVAL '9 days', CURRENT_TIMESTAMP - INTERVAL '9 days'),
        (launch_project_id, seed_user_id, launch_result_id, '编写用户使用文档', '补齐首次使用和常见问题说明。', 'doing', 'medium', CURRENT_TIMESTAMP + INTERVAL '9 days', 3, 'ai', CURRENT_TIMESTAMP - INTERVAL '9 days', CURRENT_TIMESTAMP - INTERVAL '1 day'),
        (launch_project_id, seed_user_id, launch_result_id, '准备版本发布公告', '整理新功能、已知问题和反馈入口。', 'todo', 'medium', CURRENT_TIMESTAMP + INTERVAL '12 days', 4, 'ai', CURRENT_TIMESTAMP - INTERVAL '9 days', CURRENT_TIMESTAMP - INTERVAL '9 days'),
        (launch_project_id, seed_user_id, NULL, '邀请 5 位种子用户体验', '收集上线前的最后一轮使用反馈。', 'todo', 'low', CURRENT_TIMESTAMP + INTERVAL '10 days', 5, 'manual', CURRENT_TIMESTAMP - INTERVAL '2 days', CURRENT_TIMESTAMP - INTERVAL '2 days');

    INSERT INTO documents (
        user_id, source_type, title, file_name, file_url, raw_text,
        page_count, file_size, status, created_at, updated_at
    ) VALUES (
        seed_user_id,
        'pdf',
        '第二季度客户访谈复盘',
        'q2-customer-interviews.pdf',
        '/uploads/dev-seed/q2-customer-interviews.pdf',
        '第二季度共访谈 18 位用户，主要反馈集中在任务排序、截止时间提醒和移动端体验。',
        12,
        428960,
        'ready',
        CURRENT_TIMESTAMP - INTERVAL '45 days',
        CURRENT_TIMESTAMP - INTERVAL '45 days'
    ) RETURNING id INTO archived_document_id;

    INSERT INTO parse_jobs (
        user_id, document_id, job_type, status, retry_count,
        started_at, finished_at, created_at, updated_at
    ) VALUES (
        seed_user_id,
        archived_document_id,
        'ai_parse',
        'success',
        0,
        CURRENT_TIMESTAMP - INTERVAL '45 days',
        CURRENT_TIMESTAMP - INTERVAL '45 days' + INTERVAL '25 seconds',
        CURRENT_TIMESTAMP - INTERVAL '45 days',
        CURRENT_TIMESTAMP - INTERVAL '45 days' + INTERVAL '25 seconds'
    ) RETURNING id INTO archived_job_id;

    INSERT INTO parse_results (
        user_id, document_id, parse_job_id, title, summary, deadline,
        deliverables, key_requirements, risk_warnings, generated_tasks,
        ai_model, version, is_confirmed, created_at, updated_at
    ) VALUES (
        seed_user_id,
        archived_document_id,
        archived_job_id,
        '第二季度客户访谈复盘',
        '汇总客户访谈反馈，形成产品改进优先级并完成内部评审。',
        CURRENT_TIMESTAMP - INTERVAL '20 days',
        '["访谈结论摘要", "需求优先级列表", "Q3 改进建议"]'::jsonb,
        '["反馈需按用户类型分类", "结论需要标注样本数量"]'::jsonb,
        '["样本数量有限，结论不代表全部用户"]'::jsonb,
        '[
            {"title":"整理访谈记录","priority":"medium"},
            {"title":"归纳高频反馈","priority":"high"},
            {"title":"完成产品评审","priority":"medium"}
        ]'::jsonb,
        'gpt-5-mini',
        2,
        TRUE,
        CURRENT_TIMESTAMP - INTERVAL '45 days' + INTERVAL '30 seconds',
        CURRENT_TIMESTAMP - INTERVAL '40 days'
    ) RETURNING id INTO archived_result_id;

    INSERT INTO projects (
        user_id, source_document_id, parse_result_id, name, description,
        deadline, status, created_at, updated_at
    ) VALUES (
        seed_user_id,
        archived_document_id,
        archived_result_id,
        'Q2 客户访谈复盘',
        '已完成并归档的项目样例。',
        CURRENT_TIMESTAMP - INTERVAL '20 days',
        'archived',
        CURRENT_TIMESTAMP - INTERVAL '40 days',
        CURRENT_TIMESTAMP - INTERVAL '18 days'
    ) RETURNING id INTO archived_project_id;

    INSERT INTO tasks (
        project_id, user_id, source_parse_result_id, title, description,
        status, priority, deadline, sort_order, source_type, created_at, updated_at
    ) VALUES
        (archived_project_id, seed_user_id, archived_result_id, '整理访谈记录', '清理并统一访谈记录格式。', 'done', 'medium', CURRENT_TIMESTAMP - INTERVAL '34 days', 0, 'ai', CURRENT_TIMESTAMP - INTERVAL '40 days', CURRENT_TIMESTAMP - INTERVAL '34 days'),
        (archived_project_id, seed_user_id, archived_result_id, '归纳高频反馈', '按出现频率和影响范围归类。', 'done', 'high', CURRENT_TIMESTAMP - INTERVAL '28 days', 1, 'ai', CURRENT_TIMESTAMP - INTERVAL '40 days', CURRENT_TIMESTAMP - INTERVAL '27 days'),
        (archived_project_id, seed_user_id, archived_result_id, '完成产品评审', '确认 Q3 进入排期的改进项。', 'done', 'medium', CURRENT_TIMESTAMP - INTERVAL '20 days', 2, 'ai', CURRENT_TIMESTAMP - INTERVAL '40 days', CURRENT_TIMESTAMP - INTERVAL '19 days');

    INSERT INTO documents (
        user_id, source_type, title, text_input, raw_text, status, created_at, updated_at
    ) VALUES (
        seed_user_id,
        'text',
        '团队季度团建方案',
        '为 20 人团队安排一次周末团建，预算不超过 15000 元，地点在上海周边，需要包含交通、住宿、餐饮和活动安排。',
        '为 20 人团队安排一次周末团建，预算不超过 15000 元，地点在上海周边，需要包含交通、住宿、餐饮和活动安排。',
        'ready',
        CURRENT_TIMESTAMP - INTERVAL '2 days',
        CURRENT_TIMESTAMP - INTERVAL '2 days'
    ) RETURNING id INTO review_document_id;

    INSERT INTO parse_jobs (
        user_id, document_id, job_type, status, retry_count,
        started_at, finished_at, created_at, updated_at
    ) VALUES (
        seed_user_id,
        review_document_id,
        'ai_parse',
        'success',
        0,
        CURRENT_TIMESTAMP - INTERVAL '2 days',
        CURRENT_TIMESTAMP - INTERVAL '2 days' + INTERVAL '12 seconds',
        CURRENT_TIMESTAMP - INTERVAL '2 days',
        CURRENT_TIMESTAMP - INTERVAL '2 days' + INTERVAL '12 seconds'
    ) RETURNING id INTO review_job_id;

    INSERT INTO parse_results (
        user_id, document_id, parse_job_id, title, summary, deadline,
        deliverables, key_requirements, risk_warnings, generated_tasks,
        ai_model, version, is_confirmed, created_at, updated_at
    ) VALUES (
        seed_user_id,
        review_document_id,
        review_job_id,
        '团队季度团建方案',
        '在预算内完成上海周边 20 人周末团建的行程和供应商方案。',
        CURRENT_TIMESTAMP + INTERVAL '30 days',
        '["候选地点清单", "费用预算表", "详细行程", "应急预案"]'::jsonb,
        '["总预算不超过 15000 元", "覆盖 20 人交通与住宿", "至少准备一个雨天方案"]'::jsonb,
        '["周末房源紧张", "天气可能影响户外活动", "部分成员可能临时退出"]'::jsonb,
        '[
            {"title":"收集团队时间偏好","priority":"high"},
            {"title":"筛选三个候选地点","priority":"medium"},
            {"title":"询价并制作预算表","priority":"high"},
            {"title":"发起方案投票","priority":"medium"}
        ]'::jsonb,
        'gpt-5-mini',
        1,
        FALSE,
        CURRENT_TIMESTAMP - INTERVAL '2 days' + INTERVAL '15 seconds',
        CURRENT_TIMESTAMP - INTERVAL '2 days' + INTERVAL '15 seconds'
    );

    INSERT INTO documents (
        user_id, source_type, title, text_input, status, created_at, updated_at
    ) VALUES (
        seed_user_id,
        'text',
        '移动端适配需求',
        '适配手机端项目列表和任务详情页面，优先保证常用操作可用。',
        'ready',
        CURRENT_TIMESTAMP - INTERVAL '5 minutes',
        CURRENT_TIMESTAMP - INTERVAL '5 minutes'
    ) RETURNING id INTO pending_document_id;

    INSERT INTO parse_jobs (
        user_id, document_id, job_type, status, retry_count,
        started_at, created_at, updated_at
    ) VALUES (
        seed_user_id,
        pending_document_id,
        'ai_parse',
        'processing',
        0,
        CURRENT_TIMESTAMP - INTERVAL '4 minutes',
        CURRENT_TIMESTAMP - INTERVAL '5 minutes',
        CURRENT_TIMESTAMP - INTERVAL '4 minutes'
    );

    INSERT INTO documents (
        user_id, source_type, title, file_name, file_url,
        page_count, file_size, status, created_at, updated_at
    ) VALUES (
        seed_user_id,
        'pdf',
        '损坏文件解析样例',
        'broken-market-report.pdf',
        '/uploads/dev-seed/broken-market-report.pdf',
        0,
        1024,
        'failed',
        CURRENT_TIMESTAMP - INTERVAL '1 day',
        CURRENT_TIMESTAMP - INTERVAL '23 hours'
    ) RETURNING id INTO failed_document_id;

    INSERT INTO parse_jobs (
        user_id, document_id, job_type, status, retry_count, error_message,
        started_at, finished_at, created_at, updated_at
    ) VALUES (
        seed_user_id,
        failed_document_id,
        'ai_parse',
        'failed',
        2,
        '无法提取 PDF 文本：文件内容为空或已损坏（开发环境样例）',
        CURRENT_TIMESTAMP - INTERVAL '1 day',
        CURRENT_TIMESTAMP - INTERVAL '23 hours',
        CURRENT_TIMESTAMP - INTERVAL '1 day',
        CURRENT_TIMESTAMP - INTERVAL '23 hours'
    );

    FOR account_no IN 2..8 LOOP
        account_email := seed_emails[account_no];

        INSERT INTO users (
            email, password_hash, nickname, avatar_url, status, created_at, updated_at
        ) VALUES (
            account_email,
            seed_password_hash,
            format('开发环境测试用户 %s', lpad(account_no::TEXT, 2, '0')),
            format('https://api.dicebear.com/9.x/initials/svg?seed=TP%s', lpad(account_no::TEXT, 2, '0')),
            1,
            CURRENT_TIMESTAMP - make_interval(days => account_no),
            CURRENT_TIMESTAMP
        ) RETURNING id INTO secondary_user_id;

        INSERT INTO documents (
            user_id, source_type, title, text_input, raw_text,
            status, created_at, updated_at
        ) VALUES (
            secondary_user_id,
            'text',
            format('测试用户 %s 的本周工作计划', lpad(account_no::TEXT, 2, '0')),
            '整理本周重点工作，完成需求评审、功能开发和上线复盘，并及时同步风险。',
            '整理本周重点工作，完成需求评审、功能开发和上线复盘，并及时同步风险。',
            'ready',
            CURRENT_TIMESTAMP - make_interval(days => account_no),
            CURRENT_TIMESTAMP - make_interval(days => account_no)
        ) RETURNING id INTO secondary_document_id;

        INSERT INTO parse_jobs (
            user_id, document_id, job_type, status, retry_count,
            started_at, finished_at, created_at, updated_at
        ) VALUES (
            secondary_user_id,
            secondary_document_id,
            'ai_parse',
            'success',
            0,
            CURRENT_TIMESTAMP - make_interval(days => account_no),
            CURRENT_TIMESTAMP - make_interval(days => account_no) + INTERVAL '10 seconds',
            CURRENT_TIMESTAMP - make_interval(days => account_no),
            CURRENT_TIMESTAMP - make_interval(days => account_no) + INTERVAL '10 seconds'
        ) RETURNING id INTO secondary_job_id;

        INSERT INTO parse_results (
            user_id, document_id, parse_job_id, title, summary, deadline,
            deliverables, key_requirements, risk_warnings, generated_tasks,
            ai_model, version, is_confirmed, created_at, updated_at
        ) VALUES (
            secondary_user_id,
            secondary_document_id,
            secondary_job_id,
            format('测试用户 %s 的本周工作计划', lpad(account_no::TEXT, 2, '0')),
            '完成需求评审、功能实现和上线复盘，形成可跟踪的本周任务清单。',
            CURRENT_TIMESTAMP + make_interval(days => account_no + 3),
            '["需求评审记录", "功能交付物", "上线复盘记录"]'::jsonb,
            '["每天同步任务进度", "阻塞问题需及时升级"]'::jsonb,
            '["需求变更可能影响交付时间"]'::jsonb,
            '[
                {"title":"完成需求评审","priority":"high"},
                {"title":"实现并验证核心功能","priority":"high"},
                {"title":"完成上线复盘","priority":"medium"}
            ]'::jsonb,
            'gpt-5-mini',
            1,
            TRUE,
            CURRENT_TIMESTAMP - make_interval(days => account_no) + INTERVAL '12 seconds',
            CURRENT_TIMESTAMP - make_interval(days => account_no) + INTERVAL '12 seconds'
        ) RETURNING id INTO secondary_result_id;

        INSERT INTO projects (
            user_id, source_document_id, parse_result_id, name, description,
            deadline, status, created_at, updated_at
        ) VALUES (
            secondary_user_id,
            secondary_document_id,
            secondary_result_id,
            format('测试用户 %s 的迭代项目', lpad(account_no::TEXT, 2, '0')),
            '用于验证不同用户之间的数据隔离、列表分页和项目状态筛选。',
            CURRENT_TIMESTAMP + make_interval(days => account_no + 3),
            CASE WHEN account_no IN (4, 7) THEN 'archived' ELSE 'active' END,
            CURRENT_TIMESTAMP - make_interval(days => account_no),
            CURRENT_TIMESTAMP - INTERVAL '1 hour'
        ) RETURNING id INTO secondary_project_id;

        INSERT INTO tasks (
            project_id, user_id, source_parse_result_id, title, description,
            status, priority, deadline, sort_order, source_type, created_at, updated_at
        ) VALUES
            (
                secondary_project_id, secondary_user_id, secondary_result_id,
                '完成需求评审', '确认范围、验收标准和风险。', 'done', 'high',
                CURRENT_TIMESTAMP - INTERVAL '1 day', 0, 'ai',
                CURRENT_TIMESTAMP - make_interval(days => account_no),
                CURRENT_TIMESTAMP - INTERVAL '1 day'
            ),
            (
                secondary_project_id, secondary_user_id, secondary_result_id,
                '实现并验证核心功能', '完成开发、自测和必要的联调。',
                CASE WHEN account_no IN (4, 7) THEN 'done' ELSE 'doing' END,
                'high', CURRENT_TIMESTAMP + INTERVAL '2 days', 1, 'ai',
                CURRENT_TIMESTAMP - make_interval(days => account_no),
                CURRENT_TIMESTAMP - INTERVAL '2 hours'
            ),
            (
                secondary_project_id, secondary_user_id, secondary_result_id,
                '完成上线复盘', '整理结果、问题和后续改进项。',
                CASE WHEN account_no IN (4, 7) THEN 'done' ELSE 'todo' END,
                'medium', CURRENT_TIMESTAMP + make_interval(days => account_no + 3),
                2, 'ai', CURRENT_TIMESTAMP - make_interval(days => account_no),
                CURRENT_TIMESTAMP - make_interval(days => account_no)
            );
    END LOOP;

    RAISE NOTICE 'Development seed data created for 8 users; primary user id is %', seed_user_id;
END
$seed$;

SELECT
    u.id AS user_id,
    u.email,
    COUNT(DISTINCT d.id) AS documents,
    COUNT(DISTINCT pj.id) AS parse_jobs,
    COUNT(DISTINCT pr.id) AS parse_results,
    COUNT(DISTINCT p.id) AS projects,
    COUNT(DISTINCT t.id) AS tasks
FROM users u
LEFT JOIN documents d ON d.user_id = u.id
LEFT JOIN parse_jobs pj ON pj.user_id = u.id
LEFT JOIN parse_results pr ON pr.user_id = u.id
LEFT JOIN projects p ON p.user_id = u.id
LEFT JOIN tasks t ON t.user_id = u.id
WHERE LOWER(u.email) ~ '^seed\.dev0[1-8]@taskpilot\.1kuansi\.cn$'
GROUP BY u.id, u.email
ORDER BY u.email;

SELECT
    'seed.dev01@taskpilot.1kuansi.cn ... seed.dev08@taskpilot.1kuansi.cn' AS login_accounts,
    password AS shared_password
FROM seed_credentials;

COMMIT;
SQL

echo "Development seed data is ready."
echo "Use one of the 8 accounts and the shared password printed above."
