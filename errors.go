// Package xhandler creates, signs and sends the electronic documents (UBL XML)
// of SUNAT Peru. The work is done by the xhandler Rust core, embedded as a
// WebAssembly module: see the xbuilder, xsigner and xsender packages.
package xhandler

import "github.com/project-openubl/xhandler-go/internal/core"

// Error is an error reported by the xhandler core; check its Kind with
// errors.As.
type Error = core.Error

// ErrorKind classifies the errors reported by the xhandler core.
type ErrorKind = core.ErrorKind

const (
	// KindInvalidRequest means the request did not reach an operation.
	KindInvalidRequest ErrorKind = "invalid_request"
	// KindInvalidInput means the input has missing fields or wrong types.
	KindInvalidInput ErrorKind = "invalid_input"
	// KindValidation means the document has invalid values, e.g. an unknown
	// catalog code.
	KindValidation ErrorKind = "validation"
	// KindRender means the XML could not be rendered.
	KindRender ErrorKind = "render"
)

// CoreVersion is the version of the embedded xhandler core.
var CoreVersion = core.Version
