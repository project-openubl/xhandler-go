# The core (internal/core/xhandler.wasm, xbuilder/models_gen.go and testdata/conformance)
# comes from xhandler-rust:
#
#   make sync CORE=<version>   the xhandler-rust v<version> release
#   make sync                  a checkout of xhandler-rust (RUST_REPO), for development
#   make verify-core           checks that the committed core is the one released as its VERSION
#   make verify-release TAG=v<version>
#                              what releasing <version> requires: the core of the xhandler-rust
#                              release of the same version
RUST_REPO ?= ../xhandler-rust
CORE ?=
DIST := $(if $(CORE),.dist,$(RUST_REPO)/dist)

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
	@test "v$$(cat internal/core/VERSION)" = "$(TAG)" || { echo "error: the core is $$(cat internal/core/VERSION), not $(TAG:v%=%): run make sync CORE=$(TAG:v%=%)" >&2; exit 1; }
	STRICT=true ./hack/core.sh verify internal/core/xhandler.wasm internal/core/VERSION

test:
	go test ./...

lint:
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2 run
	CGO_ENABLED=0 go build ./...
