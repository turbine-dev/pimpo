VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build ui test test-go test-ui lint proof check

build: ui
	go build -ldflags "$(LDFLAGS)" -o bin/vigia ./cmd/vigia

ui:
	cd ui && npm ci --no-audit --no-fund && npm run build

test: test-go test-ui

test-go:
	go test -race ./...

test-ui:
	cd ui && npx vitest run

lint:
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
	cd ui && npx tsc -b

proof:
	go run ./cmd/proof -workers 4

# check stops at the first failure: run it before every commit.
check:
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
	cd ui && npx tsc -b && npx vitest run && npx vite build
	go test -race ./...
