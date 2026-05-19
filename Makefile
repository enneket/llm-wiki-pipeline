.PHONY: build run test clean migrate

BINARY=llm-wiki
VERSION=$(shell git describe --always --dirty 2>/dev/null || echo "dev")
DATABASE_URL?=postgresql://postgres:postgres@localhost:5432/llm_wiki

build:
	go build -ldflags="-X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/$(BINARY)

run: build
	./bin/$(BINARY) start

test:
	go test -v -race -cover ./...

clean:
	rm -rf bin/
	go clean

migrate:
	psql "$(DATABASE_URL)" -f pkg/database/migrations/001_init.sql

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

# 开发依赖
dev-deps:
	go install github.com/golangci-lint/golangci-lint@latest