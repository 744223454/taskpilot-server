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

COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-taskpilot-server}

compose() {
	docker compose --project-name "$COMPOSE_PROJECT_NAME" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

compose config --quiet
compose up -d --build
