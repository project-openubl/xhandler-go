// Package xhandler creates, signs and sends the electronic documents (UBL XML)
// of SUNAT Peru. The work is done by the xhandler Rust core, embedded as a
// WebAssembly module: see the xbuilder, xsigner and xsender packages.
package xhandler

import "fmt"

// ErrorKind classifies the errors returned by the xhandler core.
type ErrorKind string

const (
	// ErrInvalidRequest means the request did not reach an operation.
	ErrInvalidRequest ErrorKind = "invalid_request"
	// ErrInvalidInput means the input has missing fields or wrong types.
	ErrInvalidInput ErrorKind = "invalid_input"
	// ErrValidation means the document has invalid values, e.g. an unknown
	// catalog code.
	ErrValidation ErrorKind = "validation"
	// ErrRender means the XML could not be rendered.
	ErrRender ErrorKind = "render"
)

// Error is an error reported by the xhandler core.
type Error struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("xhandler %s: %s", e.Kind, e.Message)
}
