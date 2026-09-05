.DEFAULT_GOAL := help
SHELL := /bin/sh

BACKEND := backend
FRONTEND := frontend
COMPOSE := docker compose

.PHONY: help
help: ## Показать список команд
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# --- Инфраструктура ---------------------------------------------------------

.PHONY: infra-up
infra-up: ## Поднять только инфраструктуру (postgres, redis, mailpit)
	$(COMPOSE) --profile infra up -d

.PHONY: infra-down
infra-down: ## Остановить инфраструктуру
	$(COMPOSE) --profile infra down

.PHONY: infra-logs
infra-logs: ## Логи инфраструктуры
	$(COMPOSE) --profile infra logs -f --tail=100

.PHONY: up
up: ## Поднять всё окружение целиком
	$(COMPOSE) --profile full up -d --build

.PHONY: down
down: ## Остановить всё окружение
	$(COMPOSE) --profile full down

.PHONY: reset
reset: ## Полный сброс: удалить контейнеры и данные (необратимо)
	$(COMPOSE) --profile full down -v

# --- База данных ------------------------------------------------------------

.PHONY: migrate
migrate: ## Применить миграции
	cd $(BACKEND) && go run ./cmd/migrate up

.PHONY: migrate-down
migrate-down: ## Откатить последнюю миграцию
	cd $(BACKEND) && go run ./cmd/migrate down 1

.PHONY: migrate-status
migrate-status: ## Состояние миграций
	cd $(BACKEND) && go run ./cmd/migrate status

.PHONY: seed
seed: ## Загрузить демонстрационные данные
	cd $(BACKEND) && go run ./cmd/seed

.PHONY: psql
psql: ## Консоль psql внутри контейнера
	$(COMPOSE) exec postgres psql -U autoimport_app -d autoimport

# --- Бэкенд -----------------------------------------------------------------

.PHONY: api
api: ## Запустить API локально
	cd $(BACKEND) && go run ./cmd/api

.PHONY: worker
worker: ## Запустить обработчик фоновых задач
	cd $(BACKEND) && go run ./cmd/worker

.PHONY: build
build: ## Собрать бинарники
	cd $(BACKEND) && go build -trimpath -ldflags "-s -w" -o bin/api ./cmd/api
	cd $(BACKEND) && go build -trimpath -ldflags "-s -w" -o bin/worker ./cmd/worker
	cd $(BACKEND) && go build -trimpath -ldflags "-s -w" -o bin/migrate ./cmd/migrate

.PHONY: test
test: ## Тесты бэкенда
	cd $(BACKEND) && go test ./... -race -count=1

.PHONY: cover
cover: ## Тесты с отчётом о покрытии
	cd $(BACKEND) && go test ./... -coverprofile=coverage.out -covermode=atomic
	cd $(BACKEND) && go tool cover -func=coverage.out | tail -n 30

.PHONY: lint
lint: ## Проверки бэкенда
	cd $(BACKEND) && go vet ./...
	cd $(BACKEND) && gofmt -l .

.PHONY: audit
audit: ## Поиск известных уязвимостей в зависимостях
	cd $(BACKEND) && go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# --- Фронтенд ---------------------------------------------------------------

.PHONY: fe-install
fe-install: ## Установить зависимости фронтенда
	cd $(FRONTEND) && npm ci

.PHONY: fe-dev
fe-dev: ## Запустить фронтенд в режиме разработки
	cd $(FRONTEND) && npm run dev

.PHONY: fe-build
fe-build: ## Собрать фронтенд
	cd $(FRONTEND) && npm run build

.PHONY: fe-test
fe-test: ## Тесты фронтенда
	cd $(FRONTEND) && npm run test

.PHONY: fe-lint
fe-lint: ## Проверки фронтенда
	cd $(FRONTEND) && npm run lint && npm run typecheck

.PHONY: db-backup
db-backup: ## Дамп Postgres из Ubuntu WSL (pg_dump | gzip)
	scripts/backup-wsl.cmd

.PHONY: check
check: lint test audit
	cd $(FRONTEND) && npm.cmd audit --omit=dev
	cd $(FRONTEND) && npm.cmd run test

.PHONY: e2e
e2e: ## Playwright smoke против уже поднятых 5173 и 8080
	cd $(FRONTEND) && npm.cmd run e2e
