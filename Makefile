# Tareas habituales de desarrollo. `make help` lista los objetivos.

-include .env
export

.DEFAULT_GOAL := help
.PHONY: help up down migrate dev web test test-db vet build clean

help: ## Muestra esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-10s %s\n", $$1, $$2}'

up: ## Levanta PostgreSQL, Redis y MinIO
	docker compose up -d --wait postgres redis minio

down: ## Detiene los servicios (conserva los datos)
	docker compose down

migrate: ## Aplica migraciones y carga el catálogo de 93 controles
	go run ./cmd/migrate

dev: ## Arranca la API (usa .env)
	go run ./cmd/api

web: ## Arranca el frontend Next.js
	cd web && npm install && npm run dev

test: ## Pruebas unitarias con detector de carreras
	go test -race ./...

test-db: ## Pruebas de integración y aislamiento contra PostgreSQL
	TEST_DATABASE_URL=$${TEST_DATABASE_URL:-$(DATABASE_MIGRATE_URL)} go test -race -count=1 ./internal/database/... ./internal/directory/... ./internal/api/...

vet: ## Análisis estático
	go vet ./...

build: ## Compila los binarios en ./bin
	go build -o bin/api ./cmd/api
	go build -o bin/migrate ./cmd/migrate
	go build -o bin/iso-cli ./cmd/iso-cli

clean: ## Elimina binarios compilados
	rm -rf bin
