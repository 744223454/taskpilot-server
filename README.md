# TaskPilot Server

基于 `Gin + Gorm + PostgreSQL` 的 TaskPilot 后端服务仓库。

当前阶段已经打通“文本文档 -> 异步 AI 解析 -> 结果编辑确认 -> 保存项目 -> 项目与任务管理 -> 历史查询”的后端链路，并可部署到单台云服务器上的 Docker Compose 环境。产品级 MVP 仍需完成前端主业务联调与 PDF 输入。

## 当前状态

已实现：

- Gin 服务入口
- Gorm + PostgreSQL 连接初始化
- JWT 注册、登录、鉴权
- Redis Refresh 会话、HttpOnly Cookie、CSRF 防护和当前设备登出
- Redis Streams 解析队列与独立 Worker 进程
- SoruxGPT `/responses` 严格 JSON Schema 结构化解析
- 解析结果查询、乐观锁编辑与幂等确认
- 已确认解析结果幂等保存为项目，并在同一事务内生成初始任务
- 项目查询、乐观锁编辑、归档/恢复/逻辑删除状态机
- 任务查询、新增、乐观锁编辑、状态更新、物理删除和完整集合排序
- 解析结果历史、全状态项目历史与历史任务只读查询
- 统一响应信封
- 本地数据库初始化脚本
- 生产部署基础资产：
  - `.gitignore`
  - `Dockerfile`
  - `docker-compose.prod.yml`
  - `scripts/deploy_prod.sh`
  - GitHub Actions 自动测试与部署工作流

待补充：

- P0：同步 OpenAPI 和前端 API，完成文本主链路端到端回归
- P1：PDF 上传、文件存储和文字型 PDF 文本提取
- P1/P2：当前用户资料修改、登录限流；解析状态缓存按性能数据决定

项目创建采用一个解析结果只生成一个项目的幂等契约，并由 `uq_projects_parse_result_id` 唯一索引兜底。首次创建返回 `201`；重复或并发请求返回首次项目与任务，状态码为 `200`。

## 目录结构

```text
taskpilot-server/
├── .github/workflows/         # GitHub Actions
├── cmd/api/                   # Gin API 服务入口
├── cmd/worker/                # 独立解析 Worker 入口
├── compose.yaml               # 本地 PostgreSQL / Redis
├── docker-compose.prod.yml    # 云服务器生产部署
├── docs/deployment.md         # 部署说明
├── etc/                       # 配置模板
├── internal/                  # 应用内部代码
├── model/                     # Gorm 模型
├── pkg/                       # 通用组件
├── scripts/                   # SQL 与部署脚本
└── uploads/                   # 本地上传目录
```

## 环境要求

- Go `1.26.5`
- Docker / Docker Compose v2
- PostgreSQL 16
- Redis 7

## 本地开发

1. 复制本地开发配置：

```bash
cp etc/taskpilot-api.example.yaml etc/taskpilot-api.yaml
```

2. 如需通过环境变量覆盖配置，可复制 `.env.example`，并在当前 Shell 显式加载；Go 进程和 Makefile 不会自动读取 `.env`：

```bash
cp .env.example .env
set -a
. ./.env
set +a
```

3. 启动本地依赖：

```bash
docker compose up -d
```

4. 初始化数据库：

```bash
make migrate
```

5. 启动服务：

```bash
make run
```

6. 另开终端配置 `TASKPILOT_AI_API_KEY` 并启动 Worker：

```bash
export TASKPILOT_AI_API_KEY=<replace-with-soruxgpt-api-key>
make run-worker
```

常用命令：

```bash
make test
make fmt
make build
make tidy
```

## 配置说明

应用仍以 YAML 为主配置源，但现在已经支持环境变量覆盖，适合云服务器部署。

默认本地配置文件：

```text
etc/taskpilot-api.yaml
```

常用环境变量：

- `TASKPILOT_HOST`
- `TASKPILOT_PORT`
- `TASKPILOT_DATABASE_DSN`
- `TASKPILOT_REDIS_HOST`
- `TASKPILOT_REDIS_PASS`
- `TASKPILOT_AUTH_ACCESS_SECRET`
- `TASKPILOT_AUTH_ACCESS_EXPIRE`
- `TASKPILOT_AUTH_REFRESH_EXPIRE`
- `TASKPILOT_AUTH_COOKIE_SECURE`
- `TASKPILOT_CORS_ALLOWED_ORIGINS`
- `TASKPILOT_AI_API_KEY`
- `TASKPILOT_AI_BASE_URL`
- `TASKPILOT_AI_MODEL`
- `TASKPILOT_AI_REQUEST_TIMEOUT`
- `TASKPILOT_AI_MAX_OUTPUT_TOKENS`
- `TASKPILOT_WORKER_CONCURRENCY`
- `TASKPILOT_WORKER_LEASE_TIMEOUT`
- `TASKPILOT_WORKER_MAX_RECOVERIES`

推荐做法：

- Git 中只提交 `etc/taskpilot-api.example.yaml` 和 `etc/taskpilot-api.prod.example.yaml`
- 本地真实配置 `etc/taskpilot-api.yaml` 不提交
- 生产密钥放在 `.env.prod`，不提交

## 已实现接口

```http
GET  /healthz
GET  /from/:name
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
GET  /api/v1/users/me
POST /api/v1/documents/text
GET  /api/v1/documents
GET  /api/v1/documents/:documentId
DELETE /api/v1/documents/:documentId
POST /api/v1/parse-jobs
GET  /api/v1/parse-jobs/:jobId
POST /api/v1/parse-jobs/:jobId/retry
GET  /api/v1/documents/:documentId/latest-job
GET  /api/v1/parse-jobs/:jobId/result
GET  /api/v1/parse-results/:resultId
PUT  /api/v1/parse-results/:resultId
POST /api/v1/parse-results/:resultId/confirm
POST /api/v1/projects
GET  /api/v1/projects
GET  /api/v1/projects/:projectId
PUT  /api/v1/projects/:projectId
POST /api/v1/projects/:projectId/archive
POST /api/v1/projects/:projectId/unarchive
DELETE /api/v1/projects/:projectId
GET  /api/v1/projects/:projectId/tasks
POST /api/v1/projects/:projectId/tasks
PUT  /api/v1/tasks/:taskId
PATCH /api/v1/tasks/:taskId/status
DELETE /api/v1/tasks/:taskId
POST /api/v1/tasks/reorder
GET  /api/v1/history/projects
GET  /api/v1/history/projects/:projectId
GET  /api/v1/history/projects/:projectId/tasks
GET  /api/v1/history/parse-results
```

文档、解析任务与解析结果接口兼容 Bearer Token，也支持 Access Cookie。使用 Cookie 鉴权的写请求必须携带 `X-CSRF-Token`。解析任务落库后会发布到 Redis Streams，由独立 Worker 推进 `pending -> processing -> success / failed`；发布失败由 Worker 的 PostgreSQL 对账循环补偿。

解析结果仅归创建用户所有，未确认时使用 `version` 做乐观锁编辑；确认后不可修改，重复确认幂等返回当前结果。

文本文档请求体上限为 `256 KiB`，正文最多 `50,000` 个 Unicode 字符。删除文档采用软删除，存在活跃解析任务时会返回冲突，已生成项目和任务不会被级联删除。

生产和开发部署脚本当前会在更新应用容器前自动执行文档软删除/活跃解析任务唯一约束、项目来源结果唯一约束，以及项目/任务版本字段和查询索引迁移。仅本地手动升级已有数据库时执行：

```bash
make migrate-documents-soft-delete-parse-jobs-unique
make migrate-projects-parse-result-unique
make migrate-projects-tasks-version
```

邮箱规范化迁移 `scripts/migrate_users_email_normalized.sql` 当前未纳入部署脚本。新库的基础迁移已经包含 `LOWER(email)` 唯一索引；旧库升级时需要在发布前显式执行 `make migrate-users-email-normalized`（或在目标 Compose 中执行同一 SQL）。

统一返回格式：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

## 仓库与质量门槛

`taskpilot-server/` 已是独立 Git 仓库。日常修改在该目录内查看状态与历史；上层工作区根目录不是 Git 仓库。

提交后端变更前执行：

```bash
make fmt
make test
make tidy
```

真实 PostgreSQL 集成测试只有在设置 `TASKPILOT_TEST_DATABASE_DSN` 时运行；普通 `make test` 未设置时会跳过这些场景。

## 云服务器部署

生产部署采用“单台服务器 + Docker Compose”模式：

- `app` 监听 `127.0.0.1:8888`
- `worker` 与 `app` 使用同一镜像、不同启动命令
- `postgres` 仅容器网络内可访问
- `redis` 仅容器网络内可访问
- 反向代理负责域名和 HTTPS

首次部署步骤见 [docs/deployment.md](docs/deployment.md)。
