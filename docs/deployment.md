# TaskPilot 部署指南

## 部署模式

当前仓库已经按“单台云服务器 + Docker Compose”方式准备好部署：

- `app`：Go API 服务
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
3. 将 `etc/taskpilot-api.prod.example.yaml` 复制为 `etc/taskpilot-api.prod.yaml`。
4. 将 `.env.prod` 中的占位密钥、密码全部替换为真实值。
5. 执行：

```bash
chmod +x scripts/deploy_prod.sh
./scripts/deploy_prod.sh
```

## 生产配置模型

`etc/taskpilot-api.prod.yaml` 负责保存“已提交到仓库的配置结构模板”；真正的敏感值通过 `.env.prod` 中的环境变量覆盖注入：

- `TASKPILOT_DATABASE_DSN`
- `TASKPILOT_REDIS_HOST`
- `TASKPILOT_REDIS_PASS`
- `TASKPILOT_AUTH_ACCESS_SECRET`
- `TASKPILOT_AUTH_ACCESS_EXPIRE`
- `POSTGRES_PASSWORD`

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

开发服务器需在部署目录中准备不提交到 Git 的 `.env.dev` 和 `etc/taskpilot-api.dev.yaml`。工作流会在服务器执行：

```bash
git switch dev
git pull --ff-only origin dev
sh ./scripts/deploy_dev.sh
```

开发部署脚本使用 `docker-compose.dev.yml` 重新构建并启动开发容器，不会执行生产环境的数据库初始化流程。

部署脚本将开发环境的 Docker Compose 项目名固定为 `taskpilot-dev-server`，避免与同一 Docker 主机上的生产项目混用。部署时会自动删除旧 `taskpilot-server` 项目下名称包含 `taskpilot-dev-` 的开发容器以及失败重建遗留的临时容器，再由新项目重新创建；命名卷不会被删除，开发数据会保留。

如果 `docker-compose.dev.yml` 继续复用旧项目创建的命名卷，应将这些卷声明为 `external: true`，避免 Docker Compose 输出归属警告。

## 开发服务器测试数据

仅在开发服务器上执行以下脚本。脚本会重建 8 个固定邮箱的体验账号及其测试数据，不会修改其他用户的数据；重复执行可将样例恢复到初始状态。

```bash
chmod +x scripts/seed_dev_data.sh
./scripts/seed_dev_data.sh --confirm-dev
```

默认读取 `.env.prod` 和 `docker-compose.prod.yml`。如果开发服务器使用其他文件，可在执行时覆盖：

```bash
ENV_FILE=/srv/taskpilot-server/.env.dev \
COMPOSE_FILE=/srv/taskpilot-server/docker-compose.dev.yml \
./scripts/seed_dev_data.sh --confirm-dev
```

脚本会在命令输出中显示 8 个测试账号和本次生成的随机共享密码。密码只显示一次，请勿将其提交到仓库。

主账号包含进行中的项目、已归档项目、待确认解析结果、处理中的解析任务和失败样例；其余账号各自包含独立的文档、解析结果、项目和任务，用于验证用户数据隔离。脚本中的 PDF 路径仅用于数据库界面测试，不包含真实上传文件。

## 反向代理示例

反向代理需要把你选定的子域名转发到 `http://127.0.0.1:8888`。

PostgreSQL 和 Redis 不需要直接暴露到公网。
