package core

import "fmt"

// ErrorKind classifies the errors returned by the xhandler core.
type ErrorKind string

// Error is an error reported by the xhandler core.
type Error struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("xhandler %s: %s", e.Kind, e.Message)
}
