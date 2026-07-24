CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(128) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    nickname VARCHAR(64) NOT NULL,
    avatar_url VARCHAR(255),
    status SMALLINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uq_users_email_lower ON users ((LOWER(email)));

CREATE TABLE documents (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    source_type VARCHAR(20) NOT NULL,
    title VARCHAR(255),
    file_name VARCHAR(255),
    file_url VARCHAR(500),
    raw_text TEXT,
    text_input TEXT,
    page_count INT,
    file_size BIGINT,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_documents_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chk_documents_source_type
        CHECK (source_type IN ('pdf', 'text')),
    CONSTRAINT chk_documents_status
        CHECK (status IN ('uploaded', 'ready', 'failed')),
    CONSTRAINT chk_documents_source_payload
        CHECK (
            (source_type = 'pdf' AND file_name IS NOT NULL AND file_url IS NOT NULL)
            OR
            (source_type = 'text' AND text_input IS NOT NULL)
        )
);

CREATE INDEX idx_documents_user_id ON documents(user_id);
CREATE INDEX idx_documents_deleted_at ON documents(deleted_at);

CREATE TABLE parse_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    document_id BIGINT NOT NULL,
    job_type VARCHAR(30) NOT NULL,
    status VARCHAR(20) NOT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_parse_jobs_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_parse_jobs_document
        FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT chk_parse_jobs_job_type
        CHECK (job_type IN ('ai_parse')),
    CONSTRAINT chk_parse_jobs_status
        CHECK (status IN ('pending', 'processing', 'success', 'failed')),
    CONSTRAINT chk_parse_jobs_retry_count
        CHECK (retry_count >= 0)
);

CREATE INDEX idx_parse_jobs_user_id ON parse_jobs(user_id);
CREATE INDEX idx_parse_jobs_document_id ON parse_jobs(document_id);
CREATE UNIQUE INDEX uq_parse_jobs_active_document
    ON parse_jobs(document_id)
    WHERE status IN ('pending', 'processing');

CREATE TABLE parse_results (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    document_id BIGINT NOT NULL,
    parse_job_id BIGINT NOT NULL,
    title VARCHAR(255) NOT NULL,
    summary TEXT NOT NULL,
    deadline TIMESTAMPTZ,
    deliverables JSONB NOT NULL DEFAULT '[]'::jsonb,
    key_requirements JSONB NOT NULL DEFAULT '[]'::jsonb,
    risk_warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    generated_tasks JSONB NOT NULL DEFAULT '[]'::jsonb,
    ai_model VARCHAR(64),
    version INT NOT NULL DEFAULT 1,
    is_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_parse_results_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_parse_results_document
        FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT fk_parse_results_parse_job
        FOREIGN KEY (parse_job_id) REFERENCES parse_jobs(id) ON DELETE CASCADE,
    CONSTRAINT uq_parse_results_parse_job_id
        UNIQUE (parse_job_id),
    CONSTRAINT chk_parse_results_version
        CHECK (version >= 1)
);

CREATE INDEX idx_parse_results_user_id ON parse_results(user_id);
CREATE INDEX idx_parse_results_document_id ON parse_results(document_id);

CREATE TABLE projects (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    source_document_id BIGINT NOT NULL,
    parse_result_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    deadline TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_projects_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_projects_source_document
        FOREIGN KEY (source_document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT fk_projects_parse_result
        FOREIGN KEY (parse_result_id) REFERENCES parse_results(id) ON DELETE RESTRICT,
    CONSTRAINT chk_projects_status
        CHECK (status IN ('active', 'archived', 'deleted'))
);

CREATE INDEX idx_projects_user_id ON projects(user_id);
CREATE INDEX idx_projects_source_document_id ON projects(source_document_id);
CREATE INDEX idx_projects_parse_result_id ON projects(parse_result_id);

CREATE TABLE tasks (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    source_parse_result_id BIGINT,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL,
    priority VARCHAR(20) NOT NULL,
    deadline TIMESTAMPTZ,
    sort_order INT NOT NULL DEFAULT 0,
    source_type VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_tasks_project
        FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT fk_tasks_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_tasks_source_parse_result
        FOREIGN KEY (source_parse_result_id) REFERENCES parse_results(id) ON DELETE SET NULL,
    CONSTRAINT chk_tasks_status
        CHECK (status IN ('todo', 'doing', 'done')),
    CONSTRAINT chk_tasks_priority
        CHECK (priority IN ('low', 'medium', 'high')),
    CONSTRAINT chk_tasks_source_type
        CHECK (source_type IN ('ai', 'manual')),
    CONSTRAINT chk_tasks_sort_order
        CHECK (sort_order >= 0)
);

CREATE INDEX idx_tasks_project_id ON tasks(project_id);
CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_source_parse_result_id ON tasks(source_parse_result_id);
