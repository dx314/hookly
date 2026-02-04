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
