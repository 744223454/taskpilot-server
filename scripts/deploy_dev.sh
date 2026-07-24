#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-"$ROOT_DIR/docker-compose.dev.yml"}
ENV_FILE=${ENV_FILE:-"$ROOT_DIR/.env.dev"}
CONFIG_FILE=${CONFIG_FILE:-"$ROOT_DIR/etc/taskpilot-api.dev.yaml"}

if [ ! -f "$ENV_FILE" ]; then
	echo "missing $ENV_FILE"
	exit 1
fi

if [ ! -f "$CONFIG_FILE" ]; then
	echo "missing $CONFIG_FILE"
	exit 1
fi

COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-taskpilot-dev-server}
POSTGRES_CONTAINER=${POSTGRES_CONTAINER:-taskpilot-postgres}
POSTGRES_USER=${POSTGRES_USER:-taskpilot}
POSTGRES_DB=${POSTGRES_DB:-taskpilot_dev}

compose() {
	docker compose --project-name "$COMPOSE_PROJECT_NAME" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

remove_legacy_containers() {
	if [ -n "$(compose ps -aq app)" ] && [ -n "$(compose ps -aq redis)" ]; then
		return
	fi

	docker ps -aq --filter "name=taskpilot-dev-" |
	while IFS= read -r container_id; do
		if [ -n "$container_id" ]; then
			echo "removing conflicting development container $container_id"
			docker rm -f "$container_id"
		fi
	done
}

wait_for_postgres() {
	attempts=0
	until docker exec "$POSTGRES_CONTAINER" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; do
		attempts=$((attempts + 1))
		if [ "$attempts" -ge 30 ]; then
			echo "postgres container $POSTGRES_CONTAINER did not become ready in time"
			exit 1
		fi
		sleep 2
	done
}

apply_incremental_migrations() {
	echo "applying incremental database migrations to $POSTGRES_DB via $POSTGRES_CONTAINER"
	docker exec -i "$POSTGRES_CONTAINER" psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
		< "$ROOT_DIR/scripts/migrate_documents_soft_delete_parse_jobs_unique.sql"
}

compose config --quiet
compose build app
remove_legacy_containers
compose up -d redis
wait_for_postgres
apply_incremental_migrations
compose up -d app
