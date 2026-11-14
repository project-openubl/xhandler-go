# Contribuir a xhandler-go

xhandler-go es una capa delgada en Go sobre el
[núcleo xhandler](https://github.com/project-openubl/xhandler-rust). Crear, firmar, enrutar,
comprimir, armar las peticiones a la SUNAT y leer sus respuestas ocurre en Rust; Go solo envía las
peticiones HTTP. Los cambios a esa lógica van en xhandler-rust.

## Entorno

Go 1.25 o superior. `make sync` necesita además un checkout de xhandler-rust junto a este (revisa
[su CONTRIBUTING.md](https://github.com/project-openubl/xhandler-rust/blob/main/CONTRIBUTING.md)).

```shell
make test                               # go test ./...
make lint                               # go vet, golangci-lint, build con CGO_ENABLED=0
go test -race ./...
XHANDLER_E2E=true go test ./e2e/        # crea, firma y envía una factura a la SUNAT beta
```

## Estructura

| Ruta | |
|---|---|
| `internal/core/` | Ejecuta el `xhandler.wasm` incrustado con [wazero](https://wazero.io): un pool de instancias y las llamadas JSON (`Invoke`) |
| `errors.go` | `xhandler.Error` y sus `Kind` |
| `xbuilder/` | `Build`, `Enrich`; `models_gen.go` es generado; `types.go` tiene `Decimal`, `Date`, `Time` |
| `xsigner/` | `Sign` y `SigningKey` |
| `xsender/` | `FileSender` y `SunatClient`: HTTP, timeouts, reintentos, caché de tokens OAuth de las guías |
| `internal/conformance/` | Lee el corpus de conformidad |
| `testdata/conformance/` | El corpus, copiado desde xhandler-rust |
| `e2e/` | Prueba contra la SUNAT beta (opcional) |

## Cómo funciona

El núcleo expone un solo punto de entrada JSON: la petición es `{"op", "input"}` y la respuesta
`{"ok": ...}` o `{"error": {"kind", "message"}}`. `internal/core` compila el módulo una vez y
mantiene instancias libres; una instancia atiende una llamada a la vez y se descarta si falla (un
panic en el núcleo).

`xsender` pide al núcleo la petición HTTP de cada llamada (op `request`), la envía con `net/http` y
le entrega la respuesta (op `parse`). Se encarga de lo que necesita IO: timeouts (30s/120s), un
reintento si la conexión falla, reintentos tras respuestas `rate_limited` (1s y 2s) y los tokens de
las guías (reutilizados hasta un minuto antes de expirar, un pedido a la vez, reintento con un token
nuevo ante HTTP 401).

## Archivos generados

Vienen de xhandler-rust y no se editan a mano:

- `internal/core/xhandler.wasm` e `internal/core/VERSION`
- `xbuilder/models_gen.go`
- `testdata/conformance/`

```shell
make sync RUST_REPO=../xhandler-rust    # corre make -C xhandler-wasm dist allá y los copia
```

Un cambio de modelos o de comportamiento empieza en xhandler-rust; luego `make sync` aquí, corrige
lo que señalen el compilador o los tests y abre un PR con los archivos copiados.

## Tests

- `internal/core`: cada caso del corpus por el punto de entrada JSON.
- `xbuilder`, `xsigner`: los casos del corpus a través de los tipos de Go. Las respuestas del núcleo
  se decodifican rechazando campos desconocidos, así que un modelo desactualizado falla aquí.
- `xsender`: una SUNAT falsa (`http.RoundTripper`) responde con las respuestas del corpus: CDR,
  faults, rate limiting, tokens de guías, fallos de conexión.

## Convenciones

- Commits y títulos de PR con [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `docs:`, `ci:`, `chore:`...); los cambios incompatibles llevan `!`.
- Checks requeridos: `lint` y `test` en Linux, macOS y Windows. Los PRs de Dependabot se fusionan
  solos cuando pasan.
- Los nombres siguen a las otras librerías (`FileSender`, `SunatClient`, `SigningKey`...):
  mantenlos alineados con Python y Java.

## Licencia

Al contribuir aceptas que tus contribuciones se distribuyan bajo la
[Apache License, Version 2.0](https://www.apache.org/licenses/LICENSE-2.0).
