# xhandler-go

Create, sign and send the electronic documents (UBL XML) of SUNAT Peru from Go.

- **xbuilder**: invoices (boletas and facturas), credit and debit notes, guías de
  remisión, perceptions, retentions, summaries, voided documents and reversions. Only
  the essential data is required: defaults, taxes and totals are computed.
- **xsigner**: the enveloped RSA-SHA256 XML signature SUNAT requires (PEM or PKCS#12).
- **xsender**: SUNAT's SOAP services (billService, consult and validation) and the REST
  API of the guías de remisión, with the CDR, tickets and faults already read.

Pure Go, no cgo: the work is done by the [xhandler](https://github.com/project-openubl/xhandler-rust)
Rust core, embedded as a WebAssembly module and run with [wazero](https://wazero.io). The
Python and Java libraries use the same core, so the three produce the same XML,
signatures and requests.

## Install

```shell
go get github.com/project-openubl/xhandler-go
```

Go 1.25 or newer.

## Usage

```go
ctx := context.Background()

// Create
invoice := &xbuilder.Invoice{
	SerieNumero: "F001-1",
	Proveedor:   xbuilder.Proveedor{Ruc: "12345678912", RazonSocial: "Softgreen S.A.C."},
	Cliente: xbuilder.Cliente{
		TipoDocumentoIdentidad:   "6", // RUC (Catalog6)
		NumeroDocumentoIdentidad: "12121212121",
		Nombre:                   "Carlos Feria",
	},
	Detalles: []xbuilder.Detalle{
		{Descripcion: "Item", Cantidad: xbuilder.DecimalFromInt(2), Precio: xbuilder.Ptr(xbuilder.MustDecimal("100.00"))},
	},
}
xml, err := xbuilder.Build(ctx, invoice, xbuilder.Defaults{}) // today in Peru, current tax rates
if err != nil {
	return err
}

// Sign
pfx, err := os.ReadFile("certificate.pfx")
if err != nil {
	return err
}
signed, err := xsigner.Sign(ctx, []byte(xml), xsigner.FromPKCS12(pfx, "password"))
if err != nil {
	return err
}

// Send
sender := &xsender.FileSender{
	URLs:        xsender.BetaURLs,
	Credentials: xsender.Credentials{Username: "12345678959MODDATOS", Password: "MODDATOS"},
}
result, err := sender.SendFile(ctx, signed)
if err != nil {
	return err
}
switch {
case result.CDR != nil:
	fmt.Println(result.CDR.Status, result.CDR.Description) // ACEPTADO La Factura numero F001-1, ha sido aceptada
case result.Ticket != nil: // summaries, voided documents, guías: verify the ticket later
	fmt.Println(sender.VerifyFileTicket(ctx, signed, *result.Ticket))
case result.Fault != nil:
	fmt.Println(result.Fault.Status, result.Fault.Message)
}
```

- `xbuilder.Enrich` returns the document with the computed values (taxes, totals,
  legends) instead of the XML.
- Errors of the core are `*xhandler.Error` with a `Kind`: `xhandler.KindValidation`
  (e.g. an unknown catalog code), `KindInvalidKey`, `KindResponse`...
- Guías de remisión need `xsender.ProductionURLs` (SUNAT has no beta for them) and the
  `ClientID`/`ClientSecret` of the API client. Share one `xsender.SunatClient` between
  the `FileSender`s of several companies: it keeps the connections and the tokens.

## Development

The WebAssembly module, the models (`xbuilder/models_gen.go`) and the conformance corpus
(`testdata/conformance`) come from xhandler-rust:

```shell
make sync RUST_REPO=../xhandler-rust   # runs make -C xhandler-wasm dist there and copies the results
make test                               # every corpus case, raw and through the Go types
make lint
XHANDLER_E2E=true go test ./e2e/        # creates, signs and sends an invoice to SUNAT beta
```

## License

[Apache License, Version 2.0](https://www.apache.org/licenses/LICENSE-2.0)
