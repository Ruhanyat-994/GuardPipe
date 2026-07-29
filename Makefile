.PHONY: setup gen-secrets up down logs migrate migrate-down migration seed \
        test test-unit lint fmt sqlc build selfscan demo-prep clean

GOOSE := go run github.com/pressly/goose/v3/cmd/goose@v3

setup:
	@if [ ! -f .env ]; then cp .env.example .env; echo "created .env from .env.example — edit it, especially GUARDPIPE_JWT_SECRET and GUARDPIPE_ENCRYPTION_KEY"; fi
	@$(MAKE) gen-secrets
	go mod download
	cd frontend && npm install
	docker compose pull

gen-secrets:
	@echo "JWT secret suggestion:        $$(openssl rand -base64 48)"
	@echo "Encryption key suggestion:     $$(openssl rand -base64 32)"
	@echo "Paste these into .env as GUARDPIPE_JWT_SECRET / GUARDPIPE_ENCRYPTION_KEY if not already set."

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f guardpipe

migrate:
	set -a && . ./.env && set +a && $(GOOSE) -dir internal/store/migrations postgres "$$GUARDPIPE_DATABASE_URL" up

migrate-down:
	set -a && . ./.env && set +a && $(GOOSE) -dir internal/store/migrations postgres "$$GUARDPIPE_DATABASE_URL" down

migration:
	@if [ -z "$(name)" ]; then echo "usage: make migration name=add_x"; exit 1; fi
	$(GOOSE) -dir internal/store/migrations create $(name) sql

seed:
	@if [ -d cmd/seed ]; then go run ./cmd/seed; else echo "no seed program yet — added when the store module lands (see BUILD_GUIDE.md)"; fi

test:
	go test ./... -race

test-unit:
	go test ./... -short

lint:
	gofmt -l . | tee /tmp/gofmt-out; test ! -s /tmp/gofmt-out
	go vet ./...
	golangci-lint run ./...
	cd frontend && npm run lint && npm run typecheck && npm run format:check

fmt:
	gofmt -w .
	cd frontend && npm run format

sqlc:
	@if [ -f sqlc.yaml ]; then go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest generate; else echo "no sqlc.yaml yet"; fi

build:
	docker compose build

selfscan:
	@echo "not yet available — added once the orchestrator and engines exist (see BUILD_GUIDE.md)"

demo-prep:
	@echo "not yet available — added once seed data and AI caching exist (see BUILD_GUIDE.md)"

clean:
	docker compose down -v --remove-orphans
	rm -rf frontend/dist frontend/node_modules
