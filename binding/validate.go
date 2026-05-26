package binding

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/astra-go/astra/contract"
	"github.com/go-playground/validator/v10"
)

// ValidationError and ValidationErrors are re-exported from contract so that
// callers who import only the binding package get the same canonical types as
// callers who import the astra package.  The stable public definition lives in
// contract; this file is the implementation detail.
type ValidationError = contract.ValidationError
type ValidationErrors = contract.ValidationErrors

// Param is a URL path parameter key-value pair.
// It is a type alias for contract.PathParam so that callers who import only
// the binding package receive the same canonical type.
type Param = contract.PathParam

// ─── Validator ────────────────────────────────────────────────────────────────

// defaultValidatorPtr holds the package-level validator instance.
// Access only through SetDefaultValidator / GetDefaultValidator / Validate.
var defaultValidatorPtr atomic.Pointer[validator.Validate]

func init() {
	defaultValidatorPtr.Store(newDefaultValidator())
}

func newDefaultValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())

	// Use the json tag name (or form/query/uri tag) in validation error messages
	// so field names match what the client sent, not the Go struct field name.
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		for _, tag := range []string{"json", "form", "query", "uri"} {
			if name := strings.SplitN(fld.Tag.Get(tag), ",", 2)[0]; name != "" && name != "-" {
				return name
			}
		}
		return fld.Name
	})

	return v
}

// SetDefaultValidator replaces the package-level validator atomically.
// Safe for concurrent use; use with t.Cleanup for parallel test isolation:
//
//	orig := binding.GetDefaultValidator()
//	t.Cleanup(func() { binding.SetDefaultValidator(orig) })
//	binding.SetDefaultValidator(myValidator)
func SetDefaultValidator(v *validator.Validate) {
	defaultValidatorPtr.Store(v)
}

// GetDefaultValidator returns the current package-level validator.
// Use it to register custom validators, translations, or tag aliases before
// the first request is processed.
func GetDefaultValidator() *validator.Validate {
	return defaultValidatorPtr.Load()
}

// Validate validates obj using its `validate:"..."` struct tags.
// Returns ValidationErrors (which implements error) when validation fails.
func Validate(obj any) error {
	if err := defaultValidatorPtr.Load().Struct(obj); err != nil {
		var verr validator.ValidationErrors
		if errors.As(err, &verr) {
			out := make(ValidationErrors, len(verr))
			for i, fe := range verr {
				out[i] = ValidationError{
					Field:   fe.Field(),
					Message: validationMessage(fe),
				}
			}
			return out
		}
		return err
	}
	return nil
}

// validationMessage converts a validator.FieldError to a human-readable message.
func validationMessage(fe validator.FieldError) string {
	p := fe.Param()
	switch fe.Tag() {
	case "required":
		return "this field is required"
	case "required_if", "required_with", "required_without":
		return "this field is required"
	case "email":
		return "must be a valid email address"
	case "url", "uri":
		return "must be a valid URL"
	case "uuid", "uuid3", "uuid4", "uuid5":
		return "must be a valid UUID"
	case "min":
		if fe.Kind() == reflect.String || fe.Kind() == reflect.Slice {
			return fmt.Sprintf("must be at least %s characters long", p)
		}
		return fmt.Sprintf("must be at least %s", p)
	case "max":
		if fe.Kind() == reflect.String || fe.Kind() == reflect.Slice {
			return fmt.Sprintf("must be at most %s characters long", p)
		}
		return fmt.Sprintf("must be at most %s", p)
	case "len":
		return fmt.Sprintf("must be exactly %s characters long", p)
	case "eq":
		return fmt.Sprintf("must equal %s", p)
	case "ne":
		return fmt.Sprintf("must not equal %s", p)
	case "gt":
		return fmt.Sprintf("must be greater than %s", p)
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", p)
	case "lt":
		return fmt.Sprintf("must be less than %s", p)
	case "lte":
		return fmt.Sprintf("must be less than or equal to %s", p)
	case "oneof":
		return fmt.Sprintf("must be one of: %s", p)
	case "alpha":
		return "must contain only alphabetic characters"
	case "alphanum":
		return "must contain only alphanumeric characters"
	case "numeric":
		return "must be a numeric value"
	case "e164":
		return "must be a valid E.164 phone number"
	case "ip", "ipv4", "ipv6":
		return "must be a valid IP address"
	default:
		return fmt.Sprintf("failed validation: %s", fe.Tag())
	}
}
