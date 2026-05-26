package contract

import (
	"fmt"
	"strings"
)

// ValidationError represents a single field validation failure.
//
// It is the stable public type for field-level errors. The binding package
// produces these; the astra package and middleware consume them.
// Keeping the type here ensures that swapping the underlying validator library
// (go-playground/validator, ozzo, etc.) does not change the public API.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (v ValidationError) Error() string {
	return fmt.Sprintf("field=%s: %s", v.Field, v.Message)
}

// ValidationErrors is an ordered list of field-level validation failures.
// It implements the error interface so it can be returned directly from
// binding/validation functions and inspected with errors.As.
type ValidationErrors []ValidationError

// Error returns a semicolon-separated summary of all validation failures.
func (ve ValidationErrors) Error() string {
	msgs := make([]string, len(ve))
	for i, e := range ve {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}
