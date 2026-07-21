# TaskPilot Deployment Guide

## Deployment mode

This repository is prepared for a single-cloud-server deployment with Docker Compose:

- `app`: Go API service
- `postgres`: PostgreSQL 16, persistent volume enabled
- `redis`: Redis 7, persistent volume enabled

The application listens on `127.0.0.1:8888` on the host. A reverse proxy such as Nginx or Caddy should forward public traffic to it.

## Server prerequisites

- Linux server with Docker Engine and Docker Compose v2
- Git installed
- A deployment directory, for example `/srv/taskpilot-server`
- Firewall open only for `22`, `80`, and `443`

## First-time setup

1. Clone the repository on the server.
2. Copy `.env.prod.example` to `.env.prod`.
3. Copy `etc/taskpilot-api.prod.example.yaml` to `etc/taskpilot-api.prod.yaml`.
4. Replace all placeholder secrets and passwords in `.env.prod`.
5. Run:

```bash
chmod +x scripts/deploy_prod.sh
./scripts/deploy_prod.sh
```

## Production config model

`etc/taskpilot-api.prod.yaml` is the committed structure template. Real secrets should come from `.env.prod` via environment variable overrides:

- `TASKPILOT_DATABASE_DSN`
- `TASKPILOT_REDIS_HOST`
- `TASKPILOT_REDIS_PASS`
- `TASKPILOT_AUTH_ACCESS_SECRET`
- `TASKPILOT_AUTH_ACCESS_EXPIRE`
- `POSTGRES_PASSWORD`

This keeps secrets out of Git while preserving a stable YAML file layout for the app.

## Ongoing release flow

For manual deployment on the server:

```bash
git pull --ff-only
./scripts/deploy_prod.sh
```

For automated deployment, configure the GitHub Actions workflow in `.github/workflows/ci-deploy.yml` and add these repository secrets:

- `DEPLOY_HOST`
- `DEPLOY_PORT`
- `DEPLOY_USER`
- `DEPLOY_SSH_KEY`
- `DEPLOY_PATH`

## Reverse proxy example

Your reverse proxy should forward the chosen subdomain to `http://127.0.0.1:8888`.

The API itself does not need to expose PostgreSQL or Redis to the public internet.
