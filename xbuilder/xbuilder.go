// Package xbuilder creates the UBL XML of SUNAT documents: invoices (boletas
// and facturas), credit and debit notes, guias de remision, perceptions,
// retentions, summaries, voided documents and reversions.
//
// Only the essential data is required: the xhandler core fills the defaults
// (currency, tax rates, document types...) and computes taxes and totals
// before rendering the XML.
package xbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/project-openubl/xhandler-go/internal/core"
)

// Document is any document xbuilder can enrich and render: *Invoice,
// *CreditNote, *DebitNote, *DespatchAdvice, *Perception, *Retention,
// *SummaryDocuments, *VoidedDocuments or *Reversion (values work too).
type Document interface {
	// Kind is the type of document, e.g. "Invoice".
	Kind() string
}

// TodayDefaults returns the Defaults for documents issued today, with the
// current SUNAT tax rates.
func TodayDefaults() Defaults {
	return Defaults{FechaEmision: Today()}
}

type taggedDocument struct {
	Kind string          `json:"kind"`
	Spec json.RawMessage `json:"spec"`
}

type input struct {
	Document taggedDocument `json:"document"`
	Defaults Defaults       `json:"defaults"`
}

func newInput(document Document, defaults Defaults) (input, error) {
	spec, err := json.Marshal(document)
	if err != nil {
		return input{}, fmt.Errorf("xbuilder: %w", err)
	}
	return input{taggedDocument{document.Kind(), spec}, defaults}, nil
}

// Build enriches the document and renders its XML. The document itself is
// not modified; use Enrich to get the computed values.
func Build(ctx context.Context, document Document, defaults Defaults) (string, error) {
	in, err := newInput(document, defaults)
	if err != nil {
		return "", err
	}
	var xml string
	err = core.Invoke(ctx, "build", in, &xml)
	return xml, err
}

// Enrich returns a copy of the document with its defaults filled in and its
// taxes and totals computed, as Build renders it. The copy has the same
// type as the document, e.g. *Invoice for an *Invoice.
func Enrich[T Document](ctx context.Context, document T, defaults Defaults) (T, error) {
	var zero T
	in, err := newInput(document, defaults)
	if err != nil {
		return zero, err
	}
	var out taggedDocument
	if err := core.Invoke(ctx, "enrich", in, &out); err != nil {
		return zero, err
	}
	// The dynamic type, so that T can also be the Document interface.
	enriched := reflect.New(reflect.TypeOf(document))
	if err := json.Unmarshal(out.Spec, enriched.Interface()); err != nil {
		return zero, fmt.Errorf("xbuilder: %w", err)
	}
	return enriched.Elem().Interface().(T), nil
}
