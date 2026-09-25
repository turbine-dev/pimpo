VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: dmg build ui test test-go test-ui lint proof check e2e desktop release-snapshot

build: ui
	go build -ldflags "$(LDFLAGS)" -o bin/zodim ./cmd/zodim

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

# e2e runs the browser flows and accessibility checks against the demo.
e2e: build
	cd ui && npx playwright test

# desktop builds the Tauri app with this machine's server binary inside.
desktop: ui
	mkdir -p desktop/src-tauri/binaries
	go build -ldflags "$(LDFLAGS)" -o desktop/src-tauri/binaries/zodim-$$(rustc -vV | sed -n 's/host: //p') ./cmd/zodim
	cd desktop && npm ci --no-audit --no-fund && npx tauri build --bundles app

# dmg builds the macOS installer too; its Finder styling step sometimes
# fails when run without a desktop session, so it is kept apart.
dmg: desktop
	scripts/dmg-cleanup.sh
	cd desktop && npx tauri build --bundles dmg

release-snapshot:
	goreleaser release --snapshot --clean --skip=publish
