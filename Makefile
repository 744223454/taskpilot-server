.PHONY: run build test tidy fmt migrate migrate-users-email-normalized docker-build prod-up prod-down prod-deploy migrate-prod

APP := taskpilot
CONFIG ?= etc/taskpilot-api.yaml
PROD_COMPOSE ?= docker-compose.prod.yml
PROD_CONFIG ?= etc/taskpilot-api.prod.yaml

run:
	go run ./cmd/api -f $(CONFIG)

build:
	go build -o bin/$(APP) ./cmd/api

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

migrate:
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U taskpilot -d taskpilot < scripts/migrate.sql

migrate-users-email-normalized:
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U taskpilot -d taskpilot < scripts/migrate_users_email_normalized.sql

docker-build:
	docker build -t $(APP):local .

prod-up:
	docker compose -f $(PROD_COMPOSE) up -d --build

prod-down:
	docker compose -f $(PROD_COMPOSE) down

migrate-prod:
	docker compose -f $(PROD_COMPOSE) exec -T postgres psql -v ON_ERROR_STOP=1 -U $$POSTGRES_USER -d $$POSTGRES_DB < scripts/migrate.sql

prod-deploy:
	sh ./scripts/deploy_prod.sh
