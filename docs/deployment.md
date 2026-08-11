# TaskPilot 部署指南

## 当前唯一部署模式

后端只维护一个远程运行环境：原 `dev` 分支重命名为新的 `main`，原开发服务器继续作为唯一服务器。公网统一使用 `https://taskpilot.1kuansi.cn`，前端静态文件和后端 `/api/` 由同一个 Nginx 站点提供。

服务器仍按“单台云服务器 + Docker Compose”运行：

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

## 新服务器首次部署

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

## 配置模型

新建独立环境可以使用 `deploy_prod.sh` 初始化完整 Compose；当前从原开发服务器迁移而来的唯一环境继续使用服务器私有的 `.env.dev`、`.env.worker.dev`、`etc/taskpilot-api.dev.yaml` 与 `docker-compose.dev.yml`。真正的敏感值必须留在服务器，不提交 Git。

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

PDF 上传配置可保留在 YAML，也可通过环境变量覆盖。远程环境至少应确保 `TASKPILOT_UPLOAD_ROOT=/app/uploads`；默认限制为 10 MiB、50 页、50,000 字符、15 秒提取超时和 2 个并发提取进程。

Worker 专属敏感配置放在不提交的 `.env.worker.dev`：

- `TASKPILOT_AI_API_KEY`
- `TASKPILOT_AI_BASE_URL`，默认 `https://ai.soruxgpt.com/v1`
- `TASKPILOT_AI_MODEL`，默认 `gpt-5.4`
- `TASKPILOT_AI_REQUEST_TIMEOUT`，默认整次解析 `180` 秒
- `TASKPILOT_AI_MAX_OUTPUT_TOKENS`，默认 `8000`

Worker 的队列、租约、恢复、心跳和停止参数默认放在 YAML 中，也可通过 `TASKPILOT_WORKER_*` 环境变量覆盖；完整清单以 `internal/config/config.go` 和工作区根目录 `docs/development.md` 为准。环境变量数值无法解析时当前实现会回退 YAML 值，发布后应通过日志、`/healthz` 与 `/readyz` 核对运行状态。

应用镜像已安装 `poppler-utils`。PDF 原文件保存在共享卷中，不暴露下载接口；文档软删除后 API 尝试立即删除文件，失败时由 Worker 孤儿扫描最终清理。数据库引用查询失败时 Worker 不会删除任何文件。

这样做的好处是：既能让应用保持稳定的 YAML 配置结构，又能避免把生产密钥直接提交到 Git。

## 自动与手动发布

`.github/workflows/ci-deploy.yml` 只监听新的 `main`：测试、PostgreSQL 集成测试、构建和进程冒烟全部通过后，才会部署唯一服务器。不再维护第二套生产/开发分支部署。

GitHub Actions Secrets：

- `DEPLOY_HOST`
- `DEPLOY_PORT`
- `DEPLOY_USER`
- `DEPLOY_SSH_KEY`
- `DEV_DEPLOY_PATH`：唯一服务器上的后端仓库绝对路径，例如 `/srv/taskpilot-server`

服务器仓库必须检出新的 `main`，并保留不提交的 `.env.dev`、`.env.worker.dev`、`etc/taskpilot-api.dev.yaml` 与 `docker-compose.dev.yml`。工作流实际执行：

```bash
git switch main
git pull --ff-only origin main
sh ./scripts/deploy_dev.sh
```

`deploy_dev.sh` 使用服务器已有且不提交的 `docker-compose.dev.yml`，叠加仓库内的 `docker-compose.dev.worker.yml`，重新构建共享镜像并启动 `app`、`worker` 与 `redis`。脚本通过 `taskpilot-postgres` 容器访问 `taskpilot_dev` 数据库，执行所有增量迁移，再等待 `/readyz` 通过。虽然文件名保留 `dev` 以避免高风险重命名，但它现在代表唯一远程环境。

脚本默认使用以下值，必要时可在执行脚本前覆盖：

- `POSTGRES_CONTAINER=taskpilot-postgres`
- `POSTGRES_USER=taskpilot`
- `POSTGRES_DB=taskpilot_dev`

不要为了改名修改 `POSTGRES_DB`；保留 `taskpilot_dev` 可以避免无必要的数据搬迁。数据库名不会暴露给用户。

部署脚本仍将 Docker Compose 项目名固定为 `taskpilot-dev-server`。不要仅为改名重建 Compose 项目或命名卷，否则容易误建空库或丢失上传文件挂载；这些内部名称不影响公网品牌和域名。

如果 `docker-compose.dev.yml` 继续复用旧项目创建的命名卷，应将这些卷声明为 `external: true`，避免 Docker Compose 输出归属警告。

Worker 应用内部最多等待 180 秒完成在途解析；Compose 为 Worker 保留 190 秒停止宽限期。`GET /healthz` 始终用于存活检查并展示依赖状态；`GET /readyz` 只有在数据库、Redis 与 Worker 心跳全部正常时返回 `200`，Worker 停止后最多约 30 秒变为未就绪。

注册/登录限流依赖真实客户端 IP。远程环境必须通过 `TASKPILOT_HTTP_TRUSTED_PROXIES` 明确配置 Nginx/网关地址或网段，并由代理正确设置 `X-Forwarded-For`。不要配置不受控的公网网段。

## 从双服务器迁移到唯一服务器

目标是保留原开发服务器、原 `taskpilot_dev` 数据库及上传卷，将它们作为唯一线上数据；原生产服务器只在验证完成后下线。不要直接删除旧生产服务器。

1. 在 DNS 服务商将 `taskpilot.1kuansi.cn` 的 TTL 临时降到 `300` 秒；记录当前生产和开发服务器 IP。
2. 备份两台服务器。至少导出旧生产库、保留旧生产上传卷，并在保留服务器导出 `taskpilot_dev`：

   ```bash
   docker exec taskpilot-postgres pg_dump -U taskpilot -Fc taskpilot_dev > taskpilot_dev-before-cutover.dump
   docker run --rm -v taskpilot-dev-server_taskpilot_uploads:/source:ro -v "$PWD:/backup" alpine \
     tar -czf /backup/taskpilot-uploads-before-cutover.tar.gz -C /source .
   ```

   上传卷实际名称先用 `docker volume ls | grep taskpilot` 确认，不要猜测后执行删除命令。
3. 在保留服务器修改私有 `.env.dev`：设置 `TASKPILOT_AUTH_COOKIE_SECURE=true`、`TASKPILOT_CORS_ALLOWED_ORIGINS=https://taskpilot.1kuansi.cn`，核对 `TASKPILOT_DATABASE_DSN` 仍指向 `taskpilot_dev`，并保留原 JWT 密钥、数据库密码和 Redis 配置。
4. 在 `etc/taskpilot-api.dev.yaml` 中只保留线上 CORS Origin `https://taskpilot.1kuansi.cn`；环境变量非空时会覆盖 YAML。
5. 保持服务器私有 `docker-compose.dev.yml` 的容器、网络和卷映射不变，只确认 `app` 继续绑定 `127.0.0.1:8888:8888`，PostgreSQL/Redis 不暴露公网。
6. 为 `taskpilot.1kuansi.cn` 配置 Nginx：`/` 指向前端 `/srv/taskpilot-web/current`，`/api/` 代理到 `http://127.0.0.1:8888`。完整示例见前端仓库 `deploy/nginx.taskpilot.conf`。
7. 将 DNS 的 `taskpilot.1kuansi.cn` A/AAAA 记录切到保留服务器，签发或迁移该域名证书，然后执行 `nginx -t && systemctl reload nginx`。
8. 在保留服务器执行一次 `sh ./scripts/deploy_dev.sh`，验证 `https://taskpilot.1kuansi.cn/healthz`、`/readyz`、注册登录、PDF 上传和 AI 解析。
9. 先将原远端 `main` 重命名为 `archive/main-before-dev-cutover`，再将 `dev` 重命名为新的 `main`；将默认分支和分支保护规则指向新 `main`。Actions Secrets 保持不变，只删除不再使用的 `DEPLOY_PATH`。
10. 按下面的“服务器首次切换分支”执行一次分支迁移，随后推送或手动触发 Actions，确认新的 `main` 可以自动部署。
11. 至少观察 24–72 小时并确认备份可恢复，再停止和释放旧生产服务器；确认不再需要旧代码分支后，可以删除 `archive/main-before-dev-cutover`，但建议永久保留同名 Git Tag。

### 服务器首次切换分支

GitHub 完成分支重命名后，在当前绑定 `dev` 的服务器仓库执行：

```bash
cd /srv/taskpilot-server
git status --short
git fetch origin --prune
git switch dev
git branch -m dev main
git branch --set-upstream-to=origin/main main
git pull --ff-only origin main
sh ./scripts/deploy_dev.sh
```

如果 `git branch -m dev main` 提示服务器本地已经存在旧 `main`，先保留它再重试：

```bash
git branch -m main archive-main-before-dev-cutover-local
git branch -m dev main
git branch --set-upstream-to=origin/main main
```

执行前 `git status --short` 必须没有未知的已跟踪文件修改；服务器私有且被 Git 忽略的 `.env.dev`、`.env.worker.dev`、`etc/taskpilot-api.dev.yaml` 和 `docker-compose.dev.yml` 不受分支重命名影响。

旧 `https://dev.taskpilot.1kuansi.cn` 建议保留 7–14 天并做 `308` 跳转到新域名。由于认证 Cookie 是 host-only，用户切换域名后需要重新登录一次，这是预期行为。

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

### 为已有体验账号补充数据

已有账号需要保留密码、资料和原有业务数据时，使用增量脚本，不要执行上面的全量开发种子脚本。增量脚本只接受已存在且启用的用户：主账号不存在、附加账号任一不存在或已停用时，整个事务都会回滚；脚本不会创建、更新或删除用户，也不会删除或覆盖已有文档、解析结果、项目和任务。

```bash
COMPOSE_PROJECT_NAME=taskpilot-dev-server \
COMPOSE_FILE="$PWD/docker-compose.dev.yml" \
ENV_FILE="$PWD/.env.dev" \
POSTGRES_DB=taskpilot_dev \
PRIMARY_EMAIL=seed.dev01@taskpilot.1kuansi.cn \
ADDITIONAL_EMAILS=seed.dev02@taskpilot.1kuansi.cn,seed.dev03@taskpilot.1kuansi.cn \
./scripts/supplement_experience_data.sh --confirm-additive
```

`PRIMARY_EMAIL` 默认是 `seed.dev01@taskpilot.1kuansi.cn`；`ADDITIONAL_EMAILS` 可省略，多个邮箱使用英文逗号分隔。每个目标账号会补充一个进行中项目、一个已归档项目和一条待确认解析结果。脚本通过固定的 `ai_model` 种子键判断数据是否已经存在，重复执行会跳过对应数据集，也不会覆盖用户后续对体验数据的修改。

## 反向代理示例

反向代理需要把你选定的子域名转发到 `http://127.0.0.1:8888`。

当 `TASKPILOT_AUTH_COOKIE_SECURE=true` 时，业务接口会强制 HTTPS：反向代理必须传递真实协议，例如 Nginx 配置 `proxy_set_header X-Forwarded-Proto $scheme;`。HTTPS 请求会返回 HSTS 响应头，未标记为 HTTPS 的业务请求会收到 `308` 跳转；代理层也应在流量进入应用前完成 HTTP 到 HTTPS 的跳转。`/healthz` 与 `/readyz` 明确允许容器内部通过 HTTP 直接探测，不参与 HTTPS 跳转。

API 进程设置了请求头、读写、空闲连接和业务请求超时，并在收到 `SIGINT`/`SIGTERM` 后优雅停止 HTTP 服务，再关闭 PostgreSQL 与 Redis 客户端。所有 `/api/v1` 响应都会设置禁止缓存响应头。

PostgreSQL 和 Redis 不需要直接暴露到公网。
