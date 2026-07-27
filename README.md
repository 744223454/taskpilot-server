# TaskPilot Server

基于 `Gin + Gorm + PostgreSQL` 的 TaskPilot 后端服务仓库。

当前阶段已经打通“文本文档 -> 异步 AI 解析 -> 结果编辑确认”的后端闭环，并可直接部署到单台云服务器上的 Docker Compose 环境。

## 当前状态

已实现：

- Gin 服务入口
- Gorm + PostgreSQL 连接初始化
- JWT 注册、登录、鉴权
- Redis Refresh 会话、HttpOnly Cookie、CSRF 防护和当前设备登出
- Redis Streams 解析队列与独立 Worker 进程
- SoruxGPT `/responses` 严格 JSON Schema 结构化解析
- 解析结果查询、乐观锁编辑与幂等确认
- 统一响应信封
- 本地数据库初始化脚本
- 生产部署基础资产：
  - `.gitignore`
  - `Dockerfile`
  - `docker-compose.prod.yml`
  - `scripts/deploy_prod.sh`
  - GitHub Actions 自动测试与部署工作流

待补充：

- PDF 上传、文件存储和文本提取
- 项目与任务管理接口
- Redis 状态缓存和登录限流

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

2. 如需通过环境变量覆盖配置，再复制：

```bash
cp .env.example .env
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
- `TASKPILOT_AI_API_KEY`
- `TASKPILOT_AI_BASE_URL`
- `TASKPILOT_AI_MODEL`
- `TASKPILOT_WORKER_CONCURRENCY`

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
```

文档、解析任务与解析结果接口兼容 Bearer Token，也支持 Access Cookie。使用 Cookie 鉴权的写请求必须携带 `X-CSRF-Token`。解析任务落库后会发布到 Redis Streams，由独立 Worker 推进 `pending -> processing -> success / failed`；发布失败由 Worker 的 PostgreSQL 对账循环补偿。

解析结果仅归创建用户所有，未确认时使用 `version` 做乐观锁编辑；确认后不可修改，重复确认幂等返回当前结果。

文本文档请求体上限为 `256 KiB`，正文最多 `50,000` 个 Unicode 字符。删除文档采用软删除，存在活跃解析任务时会返回冲突，已生成项目和任务不会被级联删除。

生产和开发部署脚本会在更新应用容器前自动执行幂等增量迁移。仅本地手动升级已有数据库时执行：

```bash
make migrate-documents-soft-delete-parse-jobs-unique
```

统一返回格式：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

## GitHub 上传建议

如果你选择让 `taskpilot-server` 作为仓库根目录，那么这个目录现在已经适合作为独立仓库：

- 本地缓存和产物已被忽略
- 本地配置与生产配置模板已拆分
- README 已按独立仓库口径整理
- 已补好部署与发布基础文件

后续只需要在 `taskpilot-server/` 下执行：

```bash
git init -b main
git add .
git commit -m "chore: bootstrap taskpilot-server repository"
```

## 云服务器部署

生产部署采用“单台服务器 + Docker Compose”模式：

- `app` 监听 `127.0.0.1:8888`
- `worker` 与 `app` 使用同一镜像、不同启动命令
- `postgres` 仅容器网络内可访问
- `redis` 仅容器网络内可访问
- 反向代理负责域名和 HTTPS

首次部署步骤见 [docs/deployment.md](docs/deployment.md)。
