# TaskPilot 部署指南

## 部署模式

当前仓库已经按“单台云服务器 + Docker Compose”方式准备好部署：

- `app`：Go API 服务
- `worker`：独立解析进程，消费 Redis Streams 并调用 AI
- `postgres`：PostgreSQL 16，已启用持久化卷
- `redis`：Redis 7，已启用持久化卷

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

部署脚本会先校验 Worker AI Key，在 PostgreSQL 就绪后自动执行幂等增量迁移，再构建一个同时包含 API 与 Worker 二进制的共享镜像，并更新两个容器。迁移失败时部署会立即停止，旧容器继续运行。

## 生产配置模型

`etc/taskpilot-api.prod.yaml` 负责保存“已提交到仓库的配置结构模板”；真正的敏感值通过 `.env.prod` 中的环境变量覆盖注入：

- `TASKPILOT_DATABASE_DSN`
- `TASKPILOT_REDIS_HOST`
- `TASKPILOT_REDIS_PASS`
- `TASKPILOT_AUTH_ACCESS_SECRET`
- `TASKPILOT_AUTH_ACCESS_EXPIRE`
- `TASKPILOT_AUTH_REFRESH_EXPIRE`
- `TASKPILOT_AUTH_COOKIE_SECURE`
- `POSTGRES_PASSWORD`

Worker 专属敏感配置放在不提交的 `.env.worker.prod`：

- `TASKPILOT_AI_API_KEY`
- `TASKPILOT_AI_BASE_URL`，默认 `https://ai.soruxgpt.com/v1`
- `TASKPILOT_AI_MODEL`，默认 `gpt-5.4`
- `TASKPILOT_AI_REQUEST_TIMEOUT`，默认整次解析 `180` 秒
- `TASKPILOT_AI_MAX_OUTPUT_TOKENS`，默认 `8000`

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

开发部署脚本使用服务器已有且不提交的 `docker-compose.dev.yml`，叠加仓库内的 `docker-compose.dev.worker.yml`，重新构建共享镜像并启动开发 `app`、`worker` 与 `redis` 容器，不会在开发 Compose 中创建 PostgreSQL。开发应用通过外部 `taskpilot_prod_net` 复用生产环境的 `taskpilot-postgres` 容器，但必须连接独立的 `taskpilot_dev` 数据库；脚本会通过该容器对 `taskpilot_dev` 执行幂等增量迁移，然后更新两个应用进程。

脚本默认使用以下值，必要时可在执行脚本前覆盖：

- `POSTGRES_CONTAINER=taskpilot-postgres`
- `POSTGRES_USER=taskpilot`
- `POSTGRES_DB=taskpilot_dev`

禁止将开发环境的 `POSTGRES_DB` 设置为生产数据库 `taskpilot`。

部署脚本将开发环境的 Docker Compose 项目名固定为 `taskpilot-dev-server`，避免与同一 Docker 主机上的生产项目混用。首次迁移时，如果新项目尚未完整拥有 `app` 和 `redis` 服务，脚本会自动删除名称包含 `taskpilot-dev-` 的旧开发容器以及失败重建遗留的临时容器，再由新项目重新创建；新项目完整建立后，后续部署不会重复清理正常容器。命名卷不会被删除，开发数据会保留。

如果 `docker-compose.dev.yml` 继续复用旧项目创建的命名卷，应将这些卷声明为 `external: true`，避免 Docker Compose 输出归属警告。

Worker 应用内部最多等待 180 秒完成在途解析；Compose 为 Worker 保留 190 秒停止宽限期。`GET /healthz` 的 `worker` 字段由 Redis 心跳判断，Worker 停止后最多约 30 秒变为 `false`。

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

PostgreSQL 和 Redis 不需要直接暴露到公网。
