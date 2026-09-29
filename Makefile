VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: install dmg build ui test test-go test-ui lint proof check e2e desktop release-snapshot

build: ui
	go build -ldflags "$(LDFLAGS)" -o bin/pimpo ./cmd/pimpo

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
	@! git grep -n -i -E 'be[e]vo|bs[o]lus' -- . ':!*.bundle' || (echo "remove the references above: the repository must not name them" && exit 1)
	go vet ./...
	cd ui && npx tsc -b && npx vitest run && npx vite build
	go test -race ./...

# e2e runs the browser flows and accessibility checks against the demo.
e2e: build
	cd ui && npx playwright test

# desktop builds the Tauri app with this machine's server binary inside.
desktop: ui
	mkdir -p desktop/src-tauri/binaries
	go build -ldflags "$(LDFLAGS)" -o desktop/src-tauri/binaries/pimpo-$$(rustc -vV | sed -n 's/host: //p') ./cmd/pimpo
	cd desktop && npm ci --no-audit --no-fund && npx tauri build --bundles app

# install puts the desktop app just built in /Applications, keeping the
# previous one as Pimpo.app.previous until the next install.
install: desktop
	rm -rf /Applications/Pimpo.app.previous
	test ! -d /Applications/Pimpo.app || mv /Applications/Pimpo.app /Applications/Pimpo.app.previous
	ditto desktop/src-tauri/target/release/bundle/macos/Pimpo.app /Applications/Pimpo.app

# dmg builds the macOS installer too; its Finder styling step sometimes
# fails when run without a desktop session, so it is kept apart.
dmg: desktop
	scripts/dmg-cleanup.sh
	cd desktop && npx tauri build --bundles dmg

release-snapshot:
	goreleaser release --snapshot --clean --skip=publish
