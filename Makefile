.PHONY: all clean build frontend backend test proto sqlc

# Default target
all: build

# Build everything
build: frontend backend

# Build frontend and copy to embed location
frontend:
	cd frontend && npm install && npm run build
	rm -rf internal/ui/dist
	cp -r frontend/build internal/ui/dist

# Build Go binaries
backend:
	go build -o bin/edge-gateway ./cmd/edge-gateway
	go build -o bin/hookly ./hookly
	go build -o bin/hookly-mcp ./cmd/hookly-mcp

# Run tests
test:
	go test ./...

# Generate protobuf code
proto:
	buf generate

# Generate sqlc queries
sqlc:
	sqlc generate

# Migration commands (uses DATABASE_PATH from .env or defaults to ./hookly.db)
migrate-status:
	@go run ./cmd/migrate status

migrate-up:
	@go run ./cmd/migrate up

migrate-down:
	@go run ./cmd/migrate down

migrate-baseline:
	@go run ./cmd/migrate baseline

migrate-create:
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=add_foo"; exit 1; fi
	@goose -dir internal/db/migrations create $(NAME) sql

# Dump schema from database (uses DATABASE_PATH from .env or override with DB=path)
dump-schema:
	@DB_PATH=$${DATABASE_PATH:-./hookly.db}; \
	if [ -n "$(DB)" ]; then DB_PATH="$(DB)"; fi; \
	if [ ! -f "$$DB_PATH" ]; then echo "Database not found: $$DB_PATH"; exit 1; fi; \
	echo "-- Hookly Database Schema (auto-generated from $$DB_PATH)" > sql/schema.sql; \
	echo "-- Run 'make dump-schema' to regenerate" >> sql/schema.sql; \
	echo "" >> sql/schema.sql; \
	sqlite3 "$$DB_PATH" ".schema --indent" | grep -v "^CREATE TABLE goose_db_version" >> sql/schema.sql; \
	echo "Schema dumped to sql/schema.sql"

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf internal/ui/dist
	rm -rf frontend/build
	rm -rf frontend/node_modules

# Development: run edge-gateway with hot reload UI
dev:
	DEV=true go run ./cmd/edge-gateway

# Development: run frontend dev server (separate terminal)
dev-frontend:
	cd frontend && npm run dev

# Docker images
docker-edge:
	docker build -f deploy/edge/Dockerfile -t hookly-edge .


docker-mcp:
	docker build -f deploy/mcp/Dockerfile -t hookly-mcp .

docker-all: docker-edge docker-mcp
