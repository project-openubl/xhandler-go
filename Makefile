# The core (internal/core/xhandler.wasm, xbuilder/models_gen.go and testdata/conformance)
# comes from xhandler-rust:
#
#   make sync CORE=<version>   the xhandler-rust v<version> release
#   make sync                  a checkout of xhandler-rust (RUST_REPO), for development
#   make verify-core           checks that the committed core is the one released as its VERSION
#   make verify-release TAG=v<version>
#                              what releasing <version> requires: syncing the xhandler-rust
#                              release of the same version changes nothing
RUST_REPO ?= ../xhandler-rust
CORE ?=
DIST := $(if $(CORE),.dist,$(RUST_REPO)/dist)

SYNCED := internal/core/xhandler.wasm internal/core/VERSION xbuilder/models_gen.go testdata/conformance

.PHONY: sync dist verify-core verify-release test lint

sync: dist
	cp $(DIST)/xhandler.wasm $(DIST)/VERSION internal/core/
	cp $(DIST)/models/go/models_gen.go xbuilder/models_gen.go
	gofmt -w xbuilder/models_gen.go
	rm -rf testdata/conformance && cp -r $(DIST)/conformance testdata/conformance
	$(if $(CORE),rm -rf $(DIST))

dist:
ifeq ($(CORE),)
	$(MAKE) -C $(RUST_REPO)/xhandler-wasm dist
else
	./hack/core.sh download $(CORE) $(DIST)
endif

verify-core:
	./hack/core.sh verify internal/core/xhandler.wasm internal/core/VERSION

verify-release:
	./hack/core.sh verify-release $(TAG) internal/core/xhandler.wasm internal/core/VERSION
	$(MAKE) --no-print-directory sync CORE=$(TAG:v%=%)
	@if [ -n "$$(git status --porcelain -- $(SYNCED))" ]; then \
		echo "error: the core differs from the xhandler-rust $(TAG) release (now synced here):" >&2; \
		git status --short -- $(SYNCED) >&2; \
		exit 1; \
	fi

test:
	go test ./...

lint:
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2 run
	CGO_ENABLED=0 go build ./...
