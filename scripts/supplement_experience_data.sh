#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-"$ROOT_DIR/docker-compose.prod.yml"}
ENV_FILE=${ENV_FILE:-"$ROOT_DIR/.env.prod"}
PRIMARY_EMAIL=${PRIMARY_EMAIL:-seed.dev01@taskpilot.1kuansi.cn}
ADDITIONAL_EMAILS=${ADDITIONAL_EMAILS:-}
POSTGRES_USER=${POSTGRES_USER:-taskpilot}
POSTGRES_DB=${POSTGRES_DB:-taskpilot}

if [ "${1:-}" != "--confirm-additive" ]; then
	echo "usage: PRIMARY_EMAIL=<email> [ADDITIONAL_EMAILS=<email,email>] $0 --confirm-additive" >&2
	echo "this script only supplements existing active users; it never creates or deletes users" >&2
	exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
	echo "missing compose file: $COMPOSE_FILE" >&2
	exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
	echo "missing environment file: $ENV_FILE" >&2
	exit 1
fi

echo "Supplementing experience data for primary account: $PRIMARY_EMAIL"
if [ -n "$ADDITIONAL_EMAILS" ]; then
	echo "Additional existing accounts: $ADDITIONAL_EMAILS"
fi

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" exec -T postgres \
	psql -X -v ON_ERROR_STOP=1 \
	-v primary_email="$PRIMARY_EMAIL" \
	-v additional_emails="$ADDITIONAL_EMAILS" \
	-U "$POSTGRES_USER" -d "$POSTGRES_DB" \
	< "$ROOT_DIR/scripts/supplement_experience_data.sql"

echo "Experience data supplement completed. Existing seeded datasets were left unchanged."
