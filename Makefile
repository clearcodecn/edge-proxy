# edge-proxy build & release
#
# Most recipes shell out to ~/go124/bin/go on local dev machines that have multi-version
# toolchains; override with `GO=go make ...` if your default `go` is fine.

GO       ?= ~/go124/bin/go
PROJECT  := edge-proxy
DIST     := dist
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

.PHONY: all build test test-race lint vet fmt clean release smoke help

all: build

help:
	@echo "Available targets:"
	@echo "  build      - native build → ./$(PROJECT)"
	@echo "  test       - go test ./..."
	@echo "  test-race  - go test -race ./..."
	@echo "  vet        - go vet ./..."
	@echo "  fmt        - gofmt -w on all .go files"
	@echo "  lint       - vet + gofmt check (no diff allowed)"
	@echo "  release    - cross-compile linux/amd64 + linux/arm64 into $(DIST)/"
	@echo "  smoke      - build, run briefly with tmp config, probe /login"
	@echo "  clean      - remove $(DIST)/ and built binary"

build:
	$(GO) build -o $(PROJECT) ./cmd/edge-proxy

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

lint: vet
	@diff=$$(gofmt -l .); \
	if [ -n "$$diff" ]; then \
		echo "gofmt diff in:"; echo "$$diff"; exit 1; \
	fi

release: clean
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build \
		-trimpath -ldflags="-s -w" \
		-o $(DIST)/$(PROJECT)-linux-amd64 ./cmd/edge-proxy
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build \
		-trimpath -ldflags="-s -w" \
		-o $(DIST)/$(PROJECT)-linux-arm64 ./cmd/edge-proxy
	cd $(DIST) && shasum -a 256 $(PROJECT)-* > SHA256SUMS
	@echo "Release artifacts in $(DIST)/:"
	@ls -lh $(DIST)/

clean:
	rm -rf $(DIST) $(PROJECT)

smoke: build
	@TMP=$$(mktemp -d); \
	HASH=$$(./$(PROJECT) gen-passwd smoke); \
	mkdir -p $$TMP/conf $$TMP/data; \
	printf 'admin:\n  bind: "127.0.0.1:18080"\n  username: admin\n  password_hash: "%s"\nacme:\n  email: smoke@example.com\npaths:\n  data_dir: %s/data\n  nginx_conf_dir: %s/conf\n  nginx_reload_cmd: /bin/true\n' "$$HASH" "$$TMP" "$$TMP" > $$TMP/config.yaml; \
	./$(PROJECT) run --config $$TMP/config.yaml > $$TMP/run.log 2>&1 & \
	PID=$$!; sleep 1; \
	echo "=== /login ==="; curl -sS -i http://127.0.0.1:18080/login | head -8; \
	echo "=== / (302) ==="; curl -sS -i http://127.0.0.1:18080/ | head -4; \
	kill $$PID 2>/dev/null; wait $$PID 2>/dev/null; \
	rm -rf $$TMP; \
	echo "smoke OK"
