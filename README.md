# xhandler-go

[![License](https://img.shields.io/github/license/project-openubl/xhandler-go?logo=apache)](https://www.apache.org/licenses/LICENSE-2.0)
[![CI](https://github.com/project-openubl/xhandler-go/actions/workflows/ci.yml/badge.svg)](https://github.com/project-openubl/xhandler-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/project-openubl/xhandler-go.svg)](https://pkg.go.dev/github.com/project-openubl/xhandler-go)

Crea, firma y envía los comprobantes electrónicos (XML UBL) de la SUNAT del Perú desde Go. Go puro,
sin cgo.

## Instalación

```shell
go get github.com/project-openubl/xhandler-go
```

Go 1.25 o superior, en cualquier sistema operativo y arquitectura que soporte Go.

## Uso

Con los paquetes `xbuilder`, `xsigner` y `xsender` de `github.com/project-openubl/xhandler-go`:

```go
ctx := context.Background()

// Crear
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
xml, err := xbuilder.Build(ctx, invoice, xbuilder.Defaults{}) // fecha de hoy en Perú y tasas vigentes
if err != nil {
	return err
}

// Firmar
pfx, err := os.ReadFile("certificado.pfx")
if err != nil {
	return err
}
signed, err := xsigner.Sign(ctx, []byte(xml), xsigner.FromPKCS12(pfx, "password"))
if err != nil {
	return err
}

// Enviar
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
case result.Ticket != nil: // resúmenes, bajas y guías: consulta el ticket después
	fmt.Println(sender.VerifyFileTicket(ctx, signed, *result.Ticket))
case result.Fault != nil:
	fmt.Println(result.Fault.Status, result.Fault.Code, result.Fault.Message)
}
```

- **Crear**: facturas, boletas, notas de crédito y débito, guías de remisión, percepciones,
  retenciones, resúmenes diarios, bajas y reversiones. `xbuilder.Enrich` devuelve el documento con
  los valores calculados (impuestos, totales, leyendas) en lugar del XML. `xbuilder.Defaults`
  cambia la fecha de emisión o las tasas.
- **Firmar**: `xsigner.FromPEM(llavePrivada, certificado)` para archivos PEM.
- **Enviar**: `SendPack` envía varias facturas en un zip. `xsender.SunatClient` también tiene los
  servicios de consulta y validación (`GetStatus`, `GetStatusCDR`, `ValidateCDP`, `ValidateFile`).
  Si envías por varias empresas, comparte un `SunatClient` (conexiones y tokens) entre sus
  `FileSender`.
- **Guías de remisión**: usan `xsender.ProductionURLs` (la SUNAT no tiene beta para ellas) y el
  `ClientID`/`ClientSecret` de la API.
- **Errores**: los del núcleo son `*xhandler.Error` con un `Kind`: `xhandler.KindValidation` (p. ej.
  un código de catálogo inválido), `KindInvalidKey`, `KindResponse`, `KindRateLimited`... Los rechazos
  de la SUNAT no son errores: llegan en `result.Fault`.

## Contribuir

Revisa [CONTRIBUTING.md](CONTRIBUTING.md). Preguntas en el
[chat de Zulip](https://projectopenubl.zulipchat.com/).

## Licencia

[Apache License, Version 2.0](https://www.apache.org/licenses/LICENSE-2.0)
