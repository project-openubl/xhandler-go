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
	"errors"
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

type input struct {
	Document struct {
		Kind string   `json:"kind"`
		Spec Document `json:"spec"`
	} `json:"document"`
	Defaults Defaults `json:"defaults"`
}

func newInput(document Document, defaults Defaults) (input, error) {
	if document == nil || reflect.ValueOf(document).Kind() == reflect.Pointer && reflect.ValueOf(document).IsNil() {
		return input{}, errors.New("xbuilder: nil document")
	}
	var in input
	in.Document.Kind = document.Kind()
	in.Document.Spec = document
	in.Defaults = defaults
	return in, nil
}

// Build enriches the document and renders its XML. The zero Defaults use
// today's date in Peru and the current SUNAT tax rates. The document itself
// is not modified; use Enrich to get the computed values.
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
	var spec json.RawMessage
	if err := core.Invoke(ctx, "enrich", in, &spec); err != nil {
		return zero, err
	}
	// The dynamic type, so that T can also be the Document interface.
	enriched := reflect.New(reflect.TypeOf(document))
	if err := json.Unmarshal(spec, enriched.Interface()); err != nil {
		return zero, fmt.Errorf("xbuilder: %w", err)
	}
	return enriched.Elem().Interface().(T), nil
}
