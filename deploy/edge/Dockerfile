# Build frontend
FROM node:20-alpine AS frontend
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Build Go binary
FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache gcc musl-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/build ./internal/ui/dist
RUN CGO_ENABLED=1 go build -o /edge-gateway ./cmd/edge-gateway

# Runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata && \
    mkdir -p /data
COPY --from=builder /edge-gateway /usr/local/bin/
EXPOSE 8080
VOLUME /data
CMD ["edge-gateway"]
