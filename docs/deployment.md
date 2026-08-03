# TaskPilot 部署指南

## 部署模式

当前仓库已经按“单台云服务器 + Docker Compose”方式准备好部署：

- `app`：Go API 服务
- `worker`：独立解析进程，消费 Redis Streams 并调用 AI
- `postgres`：PostgreSQL 16，已启用持久化卷
- `redis`：Redis 7，已启用持久化卷

API 与 Worker 共享 `taskpilot_uploads:/app/uploads` 持久化卷。API 在上传请求中同步调用 Poppler 提取 PDF 文字，Worker 在启动时及每 24 小时扫描临时文件和正式孤儿文件。

应用在宿主机监听 `127.0.0.1:8888`。公网流量建议通过 Nginx 或 Caddy 之类的反向代理转发到该地址。

## 服务器前置条件

- 一台已安装 Docker Engine 和 Docker Compose v2 的 Linux 服务器
- 已安装 Git
- 一个部署目录，例如 `/srv/taskpilot-server`
- 防火墙仅放行 `22`、`80`、`443`

## 首次部署

1. 在服务器上克隆仓库。
2. 将 `.env.prod.example` 复制为 `.env.prod`。
3. 将 `.env.worker.prod.example` 复制为 `.env.worker.prod`。
4. 将 `etc/taskpilot-api.prod.example.yaml` 复制为 `etc/taskpilot-api.prod.yaml`。
5. 替换 `.env.prod` 中的应用密钥与密码，并在 `.env.worker.prod` 配置 SoruxGPT API Key。
6. 执行：

```bash
chmod +x scripts/deploy_prod.sh
./scripts/deploy_prod.sh
```

部署脚本会先校验 Worker AI Key，在 PostgreSQL 就绪后自动执行基础建表（仅新库）、邮箱规范化、文档软删除/活跃解析任务唯一约束、项目来源结果唯一约束，以及项目/任务乐观锁版本字段与查询索引增量迁移，再构建共享镜像并更新 API 与 Worker。启动后脚本等待 `/readyz` 确认 PostgreSQL、Redis 和 Worker 心跳全部就绪；迁移或就绪失败会使部署失败。

`scripts/migrate_users_email_normalized.sql` 已自动执行。若旧库存在规范化后冲突的邮箱，迁移会停止并保留全部账号，不会自动合并或删除；人工清理后重新发布。新建数据库的 `scripts/migrate.sql` 已直接创建 `LOWER(email)` 唯一索引，重复执行增量迁移仍保持幂等。

`scripts/migrate_projects_parse_result_unique.sql` 会检查同一 `parse_result_id` 是否已有多个项目。检测到重复时迁移会停止并保留全部数据，不会自动删除或合并；处理重复数据后再重新发布。

`scripts/migrate_projects_tasks_version.sql` 幂等增加 `projects.version`、`tasks.version`、版本 CHECK 约束，以及项目列表和任务排序索引；开发和生产部署脚本会自动执行。

## 生产配置模型

`etc/taskpilot-api.prod.yaml` 负责保存“已提交到仓库的配置结构模板”；真正的敏感值通过 `.env.prod` 中的环境变量覆盖注入：

- `TASKPILOT_DATABASE_DSN`
- `TASKPILOT_REDIS_HOST`
- `TASKPILOT_REDIS_PASS`
- `TASKPILOT_AUTH_ACCESS_SECRET`
- `TASKPILOT_AUTH_ACCESS_EXPIRE`
- `TASKPILOT_AUTH_REFRESH_EXPIRE`
- `TASKPILOT_AUTH_COOKIE_SECURE`
- `TASKPILOT_HTTP_TRUSTED_PROXIES`
- `TASKPILOT_AUTH_LOGIN_RATE_LIMIT` / `TASKPILOT_AUTH_LOGIN_RATE_WINDOW`
- `TASKPILOT_AUTH_REGISTER_RATE_LIMIT` / `TASKPILOT_AUTH_REGISTER_RATE_WINDOW`
- `POSTGRES_PASSWORD`

PDF 上传配置可保留在 YAML，也可通过 `.env.prod` 的 `TASKPILOT_UPLOAD_*` 覆盖。生产环境至少应确保 `TASKPILOT_UPLOAD_ROOT=/app/uploads`；默认限制为 10 MiB、50 页、50,000 字符、15 秒提取超时和 2 个并发提取进程。

Worker 专属敏感配置放在不提交的 `.env.worker.prod`：

- `TASKPILOT_AI_API_KEY`
- `TASKPILOT_AI_BASE_URL`，默认 `https://ai.soruxgpt.com/v1`
- `TASKPILOT_AI_MODEL`，默认 `gpt-5.4`
- `TASKPILOT_AI_REQUEST_TIMEOUT`，默认整次解析 `180` 秒
- `TASKPILOT_AI_MAX_OUTPUT_TOKENS`，默认 `8000`

Worker 的队列、租约、恢复、心跳和停止参数默认放在 YAML 中，也可通过 `TASKPILOT_WORKER_*` 环境变量覆盖；完整清单以 `internal/config/config.go` 和工作区根目录 `docs/development.md` 为准。环境变量数值无法解析时当前实现会回退 YAML 值，发布后应通过日志、`/healthz` 与 `/readyz` 核对运行状态。

生产镜像已安装 `poppler-utils`。PDF 原文件保存在共享卷中，不暴露下载接口；文档软删除后 API 尝试立即删除文件，失败时由 Worker 孤儿扫描最终清理。数据库引用查询失败时 Worker 不会删除任何文件。

这样做的好处是：既能让应用保持稳定的 YAML 配置结构，又能避免把生产密钥直接提交到 Git。

## 日常发布流程

如果你是在服务器上手动发布，可以执行：

```bash
git pull --ff-only
./scripts/deploy_prod.sh
```

`.github/workflows/ci-deploy.yml` 支持按分支自动部署：

- `main` 更新并通过测试后，部署生产服务器。
- `dev` 更新并通过测试后，部署开发服务器。

首次启用开发环境自动部署时，需要先将工作流和 `scripts/deploy_dev.sh` 同步到 `dev` 分支。GitHub 只会执行当前被推送分支中已经存在的工作流。

在仓库 Secrets 中补齐以下变量：

- `DEPLOY_HOST`
- `DEPLOY_PORT`
- `DEPLOY_USER`
- `DEPLOY_SSH_KEY`
- `DEPLOY_PATH`：生产服务器仓库目录。
- `DEV_DEPLOY_PATH`：开发服务器仓库目录，例如 `/www/wwwroot/dev.taskpilot.1kuansi.cn/taskpilot-dev-server`。

如果两个环境位于同一台服务器，可以共用主机、端口、用户和 SSH 密钥，只分别配置两个部署目录。

开发服务器需在部署目录中准备不提交到 Git 的 `.env.dev`、`.env.worker.dev` 和 `etc/taskpilot-api.dev.yaml`。`.env.worker.dev` 可从 `.env.worker.dev.example` 复制后填入开发环境 AI Key。工作流会在服务器执行：

```bash
git switch dev
git pull --ff-only origin dev
sh ./scripts/deploy_dev.sh
```

开发部署脚本使用服务器已有且不提交的 `docker-compose.dev.yml`，叠加仓库内的 `docker-compose.dev.worker.yml`，重新构建共享镜像并启动开发 `app`、`worker` 与 `redis` 容器，不会在开发 Compose 中创建 PostgreSQL。开发应用通过外部 `taskpilot_prod_net` 复用生产环境的 `taskpilot-postgres` 容器，但必须连接独立的 `taskpilot_dev` 数据库；脚本会通过该容器自动执行邮箱规范化及其他增量迁移，然后等待 `/readyz` 通过。

脚本默认使用以下值，必要时可在执行脚本前覆盖：

- `POSTGRES_CONTAINER=taskpilot-postgres`
- `POSTGRES_USER=taskpilot`
- `POSTGRES_DB=taskpilot_dev`

禁止将开发环境的 `POSTGRES_DB` 设置为生产数据库 `taskpilot`。

部署脚本将开发环境的 Docker Compose 项目名固定为 `taskpilot-dev-server`，避免与同一 Docker 主机上的生产项目混用。首次迁移时，如果新项目尚未完整拥有 `app` 和 `redis` 服务，脚本会自动删除名称包含 `taskpilot-dev-` 的旧开发容器以及失败重建遗留的临时容器，再由新项目重新创建；新项目完整建立后，后续部署不会重复清理正常容器。命名卷不会被删除，开发数据会保留。

如果 `docker-compose.dev.yml` 继续复用旧项目创建的命名卷，应将这些卷声明为 `external: true`，避免 Docker Compose 输出归属警告。

Worker 应用内部最多等待 180 秒完成在途解析；Compose 为 Worker 保留 190 秒停止宽限期。`GET /healthz` 始终用于存活检查并展示依赖状态；`GET /readyz` 只有在数据库、Redis 与 Worker 心跳全部正常时返回 `200`，Worker 停止后最多约 30 秒变为未就绪。

注册/登录限流依赖真实客户端 IP。应用默认不信任代理头；生产必须通过 `TASKPILOT_HTTP_TRUSTED_PROXIES` 明确配置 Nginx/网关地址或网段，并由代理正确设置 `X-Forwarded-For`。不要配置不受控的公网网段。

## 开发服务器测试数据

仅在开发服务器上执行以下脚本。脚本会重建 8 个固定邮箱的体验账号及其测试数据，不会修改其他用户的数据；重复执行可将样例恢复到初始状态。

```bash
COMPOSE_PROJECT_NAME=taskpilot-dev-server \
COMPOSE_FILE="$PWD/docker-compose.dev.yml" \
ENV_FILE="$PWD/.env.dev" \
POSTGRES_DB=taskpilot_dev \
./scripts/seed_dev_data.sh --confirm-dev
```

脚本会在命令输出中显示 8 个测试账号和本次生成的随机共享密码。密码只显示一次，请勿将其提交到仓库。

主账号包含进行中的项目、已归档项目、待确认解析结果、处理中的解析任务和失败样例；其余账号各自包含独立的文档、解析结果、项目和任务，用于验证用户数据隔离。脚本中的 PDF 路径仅用于数据库界面测试，不包含真实上传文件。

## 反向代理示例

反向代理需要把你选定的子域名转发到 `http://127.0.0.1:8888`。

当 `TASKPILOT_AUTH_COOKIE_SECURE=true` 时，业务接口会强制 HTTPS：反向代理必须传递真实协议，例如 Nginx 配置 `proxy_set_header X-Forwarded-Proto $scheme;`。HTTPS 请求会返回 HSTS 响应头，未标记为 HTTPS 的业务请求会收到 `308` 跳转；代理层也应在流量进入应用前完成 HTTP 到 HTTPS 的跳转。`/healthz` 与 `/readyz` 明确允许容器内部通过 HTTP 直接探测，不参与 HTTPS 跳转。

API 进程设置了请求头、读写、空闲连接和业务请求超时，并在收到 `SIGINT`/`SIGTERM` 后优雅停止 HTTP 服务，再关闭 PostgreSQL 与 Redis 客户端。所有 `/api/v1` 响应都会设置禁止缓存响应头。

PostgreSQL 和 Redis 不需要直接暴露到公网。
