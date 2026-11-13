# RUST_REPO is a checkout of xhandler-rust; `make sync` copies what its
# `make -C xhandler-wasm dist` produced: the WebAssembly module, the generated
# models and the conformance corpus.
RUST_REPO ?= ../xhandler-rust-wasm
DIST := $(RUST_REPO)/dist

.PHONY: sync dist test lint

dist:
	$(MAKE) -C $(RUST_REPO)/xhandler-wasm dist

sync: dist
	cp $(DIST)/xhandler.wasm $(DIST)/VERSION internal/core/
	cp $(DIST)/models/go/models_gen.go xbuilder/models_gen.go
	gofmt -w xbuilder/models_gen.go
	rm -rf testdata/conformance && cp -r $(DIST)/conformance testdata/conformance

test:
	go test ./...

lint:
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run
	CGO_ENABLED=0 go build ./...
