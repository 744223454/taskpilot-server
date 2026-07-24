# TaskPilot Server

基于 `Gin + Gorm + PostgreSQL` 的 TaskPilot 后端服务仓库。

当前阶段已经具备基础可运行框架和认证链路，适合作为独立 GitHub 仓库上传，并可直接部署到单台云服务器上的 Docker Compose 环境。

## 当前状态

已实现：

- Gin 服务入口
- Gorm + PostgreSQL 连接初始化
- JWT 注册、登录、鉴权
- 统一响应信封
- 本地数据库初始化脚本
- 生产部署基础资产：
  - `.gitignore`
  - `Dockerfile`
  - `docker-compose.prod.yml`
  - `scripts/deploy_prod.sh`
  - GitHub Actions 自动测试与部署工作流

待补充：

- `documents / parse_jobs / parse_results / projects / tasks` 业务接口
- Redis 真实接入
- 文件上传与 AI 解析能力

## 目录结构

```text
taskpilot-server/
├── .github/workflows/         # GitHub Actions
├── cmd/api/                   # Gin 服务入口
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
GET  /api/v1/users/me
POST /api/v1/documents/text
GET  /api/v1/documents
GET  /api/v1/documents/:documentId
DELETE /api/v1/documents/:documentId
POST /api/v1/parse-jobs
GET  /api/v1/parse-jobs/:jobId
POST /api/v1/parse-jobs/:jobId/retry
GET  /api/v1/documents/:documentId/latest-job
```

文档与解析任务接口需要 Bearer Token。解析任务当前只负责落库并进入 `pending`，Redis 消费和 AI 解析将在下一阶段接入。

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
- `postgres` 仅容器网络内可访问
- `redis` 仅容器网络内可访问
- 反向代理负责域名和 HTTPS

首次部署步骤见 [docs/deployment.md](docs/deployment.md)。
