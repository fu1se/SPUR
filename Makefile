MODULE     := github.com/fu1se/localizator
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -s -w -X '$(MODULE)/internal/adapter/cli.version=$(VERSION)'
BIN_DIR    := bin
DIST_DIR   := dist
RELEASE_PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: help
help:
	@echo "targets:"
	@echo "  build    - build ./$(BIN_DIR)/app for the current platform"
	@echo "  test     - go test ./... -race"
	@echo "  vet      - go vet + gofmt -l (fails if any file is unformatted)"
	@echo "  fmt      - gofmt -w every .go file"
	@echo "  proto    - regenerate internal/adapter/controlproto from proto/control/v1/control.proto"
	@echo "  release  - cross-compile release binaries into ./$(DIST_DIR) for: $(RELEASE_PLATFORMS)"
	@echo "  clean    - remove $(BIN_DIR) and $(DIST_DIR)"

.PHONY: build
build:
	mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/app ./cmd/app

.PHONY: test
test:
	go test ./... -race

.PHONY: vet
vet:
	go vet ./...
	@fmtdiff="$$(gofmt -l .)"; \
	if [ -n "$$fmtdiff" ]; then \
		echo "gofmt needed on:"; echo "$$fmtdiff"; exit 1; \
	fi

.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: proto
proto:
	protoc --go_out=internal/adapter/controlproto --go_opt=paths=source_relative \
		--proto_path=proto/control/v1 proto/control/v1/control.proto

.PHONY: release
release: clean
	mkdir -p $(DIST_DIR)
	$(foreach platform,$(RELEASE_PLATFORMS), \
		$(eval GOOS := $(word 1,$(subst /, ,$(platform)))) \
		$(eval GOARCH := $(word 2,$(subst /, ,$(platform)))) \
		$(eval EXT := $(if $(filter windows,$(GOOS)),.exe,)) \
		echo "building $(platform)..."; \
		GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
			-o $(DIST_DIR)/app-$(GOOS)-$(GOARCH)$(EXT) ./cmd/app || exit 1; \
	)

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
