# Build Go binary
FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache gcc musl-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /hookly-mcp ./cmd/hookly-mcp

# Runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /hookly-mcp /usr/local/bin/
ENTRYPOINT ["hookly-mcp"]
