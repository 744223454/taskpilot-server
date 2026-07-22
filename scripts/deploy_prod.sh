#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-"$ROOT_DIR/docker-compose.prod.yml"}
ENV_FILE=${ENV_FILE:-"$ROOT_DIR/.env.prod"}
CONFIG_FILE=${CONFIG_FILE:-"$ROOT_DIR/etc/taskpilot-api.prod.yaml"}
COMPOSE="docker compose --env-file $ENV_FILE -f $COMPOSE_FILE"

if [ ! -f "$ENV_FILE" ]; then
	echo "missing $ENV_FILE"
	exit 1
fi

if [ ! -f "$CONFIG_FILE" ]; then
	echo "missing $CONFIG_FILE"
	exit 1
fi

set -a
. "$ENV_FILE"
set +a

$COMPOSE up -d postgres redis

attempts=0
until $COMPOSE exec -T postgres pg_isready -U taskpilot -d taskpilot >/dev/null 2>&1; do
	attempts=$((attempts + 1))
	if [ "$attempts" -ge 30 ]; then
		echo "postgres did not become ready in time"
		exit 1
	fi
	sleep 2
done

if $COMPOSE exec -T postgres psql -tA -U taskpilot -d taskpilot -c "SELECT to_regclass('public.users');" | grep -qx 'users'; then
	echo "database schema already initialized, skipping base migration"
else
	$COMPOSE exec -T postgres psql -v ON_ERROR_STOP=1 -U taskpilot -d taskpilot < "$ROOT_DIR/scripts/migrate.sql"
fi

$COMPOSE up -d --build app
