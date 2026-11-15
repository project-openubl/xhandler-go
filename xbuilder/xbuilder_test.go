package xbuilder_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/project-openubl/xhandler-go/v6"
	"github.com/project-openubl/xhandler-go/v6/internal/conformance"
	"github.com/project-openubl/xhandler-go/v6/xbuilder"
)

const corpus = "../testdata/conformance"

type caseInput struct {
	Document struct {
		Kind string          `json:"kind"`
		Spec json.RawMessage `json:"spec"`
	} `json:"document"`
	Defaults xbuilder.Defaults `json:"defaults"`
}

// decodeStrict fails on fields the Go types do not have, so a model change
// in Rust that was not synced breaks the build.
func decodeStrict(t *testing.T, data []byte, v any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		t.Fatal(err)
	}
}

func documentOf(t *testing.T, kind string, spec []byte) xbuilder.Document {
	t.Helper()
	document, ok := xbuilder.NewDocument(kind)
	if !ok {
		t.Fatalf("unknown kind %q", kind)
	}
	decodeStrict(t, spec, document)
	if document.Kind() != kind {
		t.Fatalf("%T reports kind %q", document, document.Kind())
	}
	return document
}

// The corpus inputs, decoded into the Go types and sent back to the core,
// render the same XML: the types carry every field faithfully.
func TestBuildCorpusThroughGoTypes(t *testing.T) {
	for _, c := range conformance.Cases(t, corpus, "build") {
		if c.Expected.Error != nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			var in caseInput
			decodeStrict(t, c.Input, &in)
			document := documentOf(t, in.Document.Kind, in.Document.Spec)

			xml, err := xbuilder.Build(context.Background(), document, in.Defaults)
			if err != nil {
				t.Fatal(err)
			}
			var expected string
			if err := json.Unmarshal(c.Expected.Ok, &expected); err != nil {
				t.Fatal(err)
			}
			if xml != expected {
				t.Fatalf("XML differs from the snapshot:\n%s", xml)
			}
		})
	}
}

func TestEnrichCorpusThroughGoTypes(t *testing.T) {
	for _, c := range conformance.Cases(t, corpus, "enrich") {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			var in caseInput
			decodeStrict(t, c.Input, &in)
			document := documentOf(t, in.Document.Kind, in.Document.Spec)

			expected := documentOf(t, in.Document.Kind, c.Expected.Ok)

			enriched, err := xbuilder.Enrich(context.Background(), document, in.Defaults)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := json.Marshal(enriched)
			want, _ := json.Marshal(expected)
			if !bytes.Equal(got, want) {
				t.Fatalf("enriched document differs:\n got %s\nwant %s", got, want)
			}
		})
	}
}

func invoice() *xbuilder.Invoice {
	return &xbuilder.Invoice{
		SerieNumero: "F001-1",
		Proveedor:   xbuilder.Proveedor{Ruc: "12345678912", RazonSocial: "Softgreen S.A.C."},
		Cliente: xbuilder.Cliente{
			TipoDocumentoIdentidad:   "6",
			NumeroDocumentoIdentidad: "12121212121",
			Nombre:                   "Carlos Feria",
		},
		Detalles: []xbuilder.Detalle{
			{Descripcion: "Item", Cantidad: xbuilder.DecimalFromInt(2), Precio: xbuilder.Ptr(xbuilder.DecimalFromInt(100))},
		},
	}
}

func TestEnrichComputesTotals(t *testing.T) {
	defaults := xbuilder.Defaults{FechaEmision: xbuilder.Ptr(xbuilder.NewDate(2019, 12, 24))}
	enriched, err := xbuilder.Enrich(context.Background(), invoice(), defaults)
	if err != nil {
		t.Fatal(err)
	}
	if got := enriched.TotalImporte.Importe; !got.Equal(xbuilder.MustDecimal("236").Decimal) {
		t.Errorf("importe = %s, want 236", got)
	}
	if enriched.Moneda == nil || *enriched.Moneda != "PEN" {
		t.Errorf("moneda = %v, want PEN", enriched.Moneda)
	}
	if enriched.FechaEmision == nil || *enriched.FechaEmision != *defaults.FechaEmision {
		t.Errorf("fecha_emision = %v", enriched.FechaEmision)
	}
}

func TestBuildReportsValidationErrors(t *testing.T) {
	document := invoice()
	document.Detalles[0].IgvTipo = xbuilder.Ptr("XX")
	_, err := xbuilder.Build(context.Background(), document, xbuilder.Defaults{})
	var xerr *xhandler.Error
	if !errors.As(err, &xerr) || xerr.Kind != xhandler.KindValidation {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// Without a date, the core uses today in Peru (UTC-5): it needs the real clock.
func TestEnrichDefaultsToTodayInPeru(t *testing.T) {
	before := xbuilder.DateOf(time.Now().In(time.FixedZone("PET", -5*60*60)))
	enriched, err := xbuilder.Enrich(context.Background(), invoice(), xbuilder.Defaults{})
	if err != nil {
		t.Fatal(err)
	}
	after := xbuilder.DateOf(time.Now().In(time.FixedZone("PET", -5*60*60)))
	if got := *enriched.FechaEmision; got != before && got != after {
		t.Errorf("fecha_emision = %s, want %s", got, before)
	}
}

func TestEnrichRejectsNilDocuments(t *testing.T) {
	if _, err := xbuilder.Enrich(context.Background(), (*xbuilder.Invoice)(nil), xbuilder.Defaults{}); err == nil {
		t.Fatal("expected an error")
	}
}
