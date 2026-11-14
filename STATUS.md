# Status: xhandler bindings (Go, Python, Java over the Rust core)

All four repositories have their work on a `feat/wasm-bindings` branch, committed
locally. Nothing is pushed.

| Repository | Branch base | What changed |
|---|---|---|
| `xhandler-rust` (worktree `../xhandler-rust-wasm`) | `main` (rebased on `ad310c4`) | `xhandler-wasm` crate (WebAssembly core, JSON ABI, schema, `codegen.py`, conformance corpus); `xsender` made sans-IO |
| `xhandler-go` | `master` | new library: `xbuilder`, `xsigner`, `xsender` |
| `xhandler-python` | `main` | new library: `xhandler.xbuilder`, `xhandler.xsigner`, `xhandler.xsender` |
| `xhandler-java` | `main` | 6.0 migration: `xhandler-core` (Chicory); XBuilder/XSigner/XSender on the core |

## Architecture

- One Rust core compiled to `wasm32-wasip1` (`xhandler-wasm`, about 4.4 MB) with a single
  JSON entry point. The ops are `build`, `enrich`, `sign`, `prepare_send_file`,
  `prepare_send_pack`, `verify_ticket_target`, `request`, `parse` and `version` (see
  `xhandler-wasm/README.md`).
- The hosts load the module:
  - Go uses wazero (pure Go, `CGO_ENABLED=0` works).
  - Python uses wasmtime-py.
  - Java uses Chicory with its build-time compiler (wasm turned into JVM bytecode).
- The core does the XML building, signing, routing, zipping, building SOAP/REST requests,
  reading CDRs/faults, and the SUNAT status mapping. The hosts do the HTTP: timeouts, one
  retry on connection failures, `rate_limited` retries after 1s and 2s, and the OAuth
  token cache of the guías (one token request at a time; a rejected token is retried once).
- Models are generated from the JSON Schema of the Rust models by
  `xhandler-wasm/codegen.py`: Go structs, pydantic v2 models, and Java Lombok classes.
- `make -C xhandler-wasm dist` (Rust) writes the wasm, schema, models and corpus;
  `make sync` (each host) copies them. `RUST_REPO` defaults to `../xhandler-rust`. Until
  `feat/wasm-bindings` is merged there, run `make sync RUST_REPO=../xhandler-rust-wasm`.

## Verification (last run)

- Rust: `cargo test --workspace`, `cargo clippy --all-targets --all-features -D warnings`
  and `cargo fmt --check` all pass. `xsender` also builds with `--no-default-features`
  for `wasm32-wasip1`.
- Corpus: 577 cases. It contains:
  - the 258 xbuilder snapshots, built and enriched;
  - byte-exact signatures (PEM, a 3DES PKCS#12, ISO-8859-1 input);
  - the sender requests and answers;
  - error kinds.

  Every host passes all cases, both raw and through its typed API.
- Go: `go test -race ./...`, `golangci-lint` (0 issues) and `CGO_ENABLED=0 go build`
  all pass.
- Python: 1126 tests pass on 3.10 and 3.14; ruff and mypy are clean.
- Java: `mvn install -Pexamples` passes (1434 tests). Quarkus integration tests: xbuilder
  259/259, xsender 2/2. The existing SUNAT beta suite (`-Dxsender.sunat-beta=true`)
  passes 4/4.
- End to end (`XHANDLER_E2E=true`): create, sign and send an invoice to **SUNAT beta**,
  expecting CDR 0 and ACEPTADO. It passes in Go, Python and Java.

## Not verified / blocked

- **Quarkus/GraalVM native builds** (`-Pnative-image`): GraalVM `native-image` is not
  installed on this machine. The core ships `META-INF/native-image` resource/reflection
  config and the xbuilder extension registers the models; CI runs the native ITs.
- **SUNAT beta is flaky**: Rust's live tests (`xsender/tests/send_files.rs`,
  `cli/tests/send_command.rs`) intermittently got non-SOAP HTTP 401 pages. Java already
  retried those (rate limiting); the core now classifies them as `rate_limited` and every
  client, Rust's own `SunatClient` included, retries them.
- **CI workflows** were written for the Go and Python repos and adjusted for Rust/Java, but
  never run on GitHub.

## Behavior changes to review

- **Rust `xsender`:**
  - `SunatClientError::Response(u16)` instead of `StatusCode`, and the enum is
    `#[non_exhaustive]`.
  - New `RateLimited` variant.
  - `Credentials.token` (new field).
  - `SunatClient { security_api_url }`.
  - `Call::Token { url }`.
  - REST 5xx answers are errors, not faults.
  - REST path segments are percent-encoded.
  - Rate-limit retries; a rejected guía token is retried once.
- **Rust `xbuilder`:** tera and chrono without default features (the CLI enables chrono
  `clock`); `Defaults::new(fecha)`; an optional `schema` feature (schemars).
- **Java 6.0** (see `xhandler-java/xbuilder/MIGRATION.md`):
  - Models are regenerated (renamed fields, e.g. `serieNumero`, `formaPago`, `detalles`).
  - `ContentEnricher`/`TemplateProducer`/`XMLSigner` are removed.
  - The **XML → model JAXB mappers are removed** (Rust has no equivalent).
  - Guía REST error details move from `metadata.notes` into `metadata.description`.
  - The signed XML is UTF-8.

## Decisions taken on my own (worth a look)

- **Go 1.25** instead of 1.24: current wazero requires it.
- **One in-house generator (`codegen.py`)** instead of three third-party schema tools, for
  identical naming in the three languages.
- **Go `xbuilder.Decimal`** wraps shopspring/decimal to keep the scale (`118.00`): some
  templates render amounts as given.
- **Defaults are optional:** the core fills in today's date in Peru (UTC-5) and the
  current SUNAT rates. The hosts give it the real clock.
- **Java behaviors moved into the core for everyone:** rate-limit retries, token retry,
  configurable security API URL, pre-supplied token.
- **The Java fault code keeps 4 digits** (`"0111"`) in `ConsultResponse` for compatibility.
- **Python models mirror the schema defaults** (`list` fields can be appended to); Go and
  Java leave unset fields to Rust.
- **The name `xhandler` is free on PyPI** (checked).

## Open questions

- `Envio.fecha_traslado` (Rust) is a non-optional date that defaults to 1970-01-01 when
  missing. Should it become optional and be validated?
- REST faults: SUNAT's detailed code (`errors[].cod`, e.g. 1033) is folded into the
  message, and the HTTP code (422) becomes the fault code. Should the detailed code be
  used instead?
- Java `XSender`/`SunatResponse` kept their API, while Go and Python use `FileSender`
  (Rust's name). Should the names be unified? Java's `verifyTicket(xml, ticket)` is
  Go's `VerifyFileTicket`, not its `VerifyTicket(target, ticket)`.
- Java `io.github.project.openubl.xbuilder.signature.KeyPair` clashes by name with
  `java.security.KeyPair`. Should it be renamed (e.g. `SigningKey`)?
- Java's live beta suite uses `-Dxsender.sunat-beta=true`, while every e2e test uses
  `XHANDLER_E2E=true`. Should they be unified?
- Each milestone was reviewed by a subagent. The findings above are the ones left
  open; all the others were applied.
