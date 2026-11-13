package xbuilder_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/project-openubl/xhandler-go"
	"github.com/project-openubl/xhandler-go/internal/conformance"
	"github.com/project-openubl/xhandler-go/xbuilder"
)

const corpus = "../testdata/conformance"

var kinds = map[string]func() xbuilder.Document{
	"Invoice":          func() xbuilder.Document { return &xbuilder.Invoice{} },
	"CreditNote":       func() xbuilder.Document { return &xbuilder.CreditNote{} },
	"DebitNote":        func() xbuilder.Document { return &xbuilder.DebitNote{} },
	"DespatchAdvice":   func() xbuilder.Document { return &xbuilder.DespatchAdvice{} },
	"Perception":       func() xbuilder.Document { return &xbuilder.Perception{} },
	"Retention":        func() xbuilder.Document { return &xbuilder.Retention{} },
	"SummaryDocuments": func() xbuilder.Document { return &xbuilder.SummaryDocuments{} },
	"VoidedDocuments":  func() xbuilder.Document { return &xbuilder.VoidedDocuments{} },
	"Reversion":        func() xbuilder.Document { return &xbuilder.Reversion{} },
}

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
	newDocument, ok := kinds[kind]
	if !ok {
		t.Fatalf("unknown kind %q", kind)
	}
	document := newDocument()
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

			var out struct {
				Kind string          `json:"kind"`
				Spec json.RawMessage `json:"spec"`
			}
			decodeStrict(t, c.Expected.Ok, &out)
			expected := documentOf(t, out.Kind, out.Spec)

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
			{Descripcion: "Item", Cantidad: xbuilder.DecimalFromInt(2), Precio: ptr(xbuilder.DecimalFromInt(100))},
		},
	}
}

func ptr[T any](v T) *T { return &v }

func TestEnrichComputesTotals(t *testing.T) {
	defaults := xbuilder.Defaults{FechaEmision: xbuilder.NewDate(2019, 12, 24)}
	enriched, err := xbuilder.Enrich(context.Background(), invoice(), defaults)
	if err != nil {
		t.Fatal(err)
	}
	if got := enriched.TotalImporte.Importe; !got.Equal(xbuilder.MustDecimal("236").Decimal) {
		t.Errorf("importe = %s, want 236", got)
	}
	if enriched.Moneda != "PEN" {
		t.Errorf("moneda = %q, want PEN", enriched.Moneda)
	}
	if enriched.FechaEmision == nil || *enriched.FechaEmision != defaults.FechaEmision {
		t.Errorf("fecha_emision = %v", enriched.FechaEmision)
	}
}

func TestBuildReportsValidationErrors(t *testing.T) {
	document := invoice()
	document.Detalles[0].IgvTipo = "XX"
	_, err := xbuilder.Build(context.Background(), document, xbuilder.TodayDefaults())
	var xerr *xhandler.Error
	if !errors.As(err, &xerr) || xerr.Kind != xhandler.ErrValidation {
		t.Fatalf("expected a validation error, got %v", err)
	}
}
