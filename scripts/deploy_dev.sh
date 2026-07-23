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

compose config --quiet
compose build
remove_legacy_containers
compose up -d
