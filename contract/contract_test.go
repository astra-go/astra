package contract_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/astra-go/astra/contract"
)

// ─── RouteKey ─────────────────────────────────────────────────────────────────

func TestRouteKey_Value(t *testing.T) {
	if contract.RouteKey != "astra.route" {
		t.Errorf("RouteKey = %q, want %q", contract.RouteKey, "astra.route")
	}
}

// ─── NewHTTPError ─────────────────────────────────────────────────────────────

func TestNewHTTPError_WithMessage(t *testing.T) {
	he := contract.NewHTTPError(http.StatusBadRequest, "bad input")
	if he.Code != http.StatusBadRequest {
		t.Errorf("Code = %d, want %d", he.Code, http.StatusBadRequest)
	}
	if he.Message != "bad input" {
		t.Errorf("Message = %v, want %q", he.Message, "bad input")
	}
}

func TestNewHTTPError_NoMessage_UsesStatusText(t *testing.T) {
	he := contract.NewHTTPError(http.StatusNotFound)
	want := http.StatusText(http.StatusNotFound)
	if he.Message != want {
		t.Errorf("Message = %v, want %q", he.Message, want)
	}
}

// ─── HTTPError.Error ──────────────────────────────────────────────────────────

func TestHTTPError_Error_WithoutInternalErr(t *testing.T) {
	he := contract.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	got := he.Error()
	if got != "code=401, message=unauthorized" {
		t.Errorf("Error() = %q", got)
	}
}

func TestHTTPError_Error_WithInternalErr(t *testing.T) {
	inner := errors.New("db timeout")
	he := contract.NewHTTPError(http.StatusInternalServerError).WithInternal(inner)
	got := he.Error()
	if got == "" {
		t.Error("Error() should not be empty")
	}
	// Should include the internal error text.
	if !containsStr(got, "db timeout") {
		t.Errorf("Error() = %q, expected to contain 'db timeout'", got)
	}
}

// ─── HTTPError.Unwrap ─────────────────────────────────────────────────────────

func TestHTTPError_Unwrap_ReturnsInternalErr(t *testing.T) {
	inner := errors.New("inner")
	he := contract.NewHTTPError(500).WithInternal(inner)
	if he.Unwrap() != inner {
		t.Error("Unwrap should return the internal error")
	}
}

func TestHTTPError_Unwrap_NilWhenNoInternal(t *testing.T) {
	he := contract.NewHTTPError(500)
	if he.Unwrap() != nil {
		t.Error("Unwrap should return nil when no internal error")
	}
}

// ─── HTTPError.WithInternal ───────────────────────────────────────────────────

func TestHTTPError_WithInternal_ReturnsClone(t *testing.T) {
	original := contract.NewHTTPError(http.StatusForbidden, "forbidden")
	inner := errors.New("reason")
	clone := original.WithInternal(inner)

	if clone == original {
		t.Error("WithInternal should return a clone, not the same pointer")
	}
	if original.Err != nil {
		t.Error("WithInternal should not mutate the original")
	}
	if clone.Err != inner {
		t.Error("clone.Err should be the provided internal error")
	}
}

// ─── HTTPError.WithMessage ────────────────────────────────────────────────────

func TestHTTPError_WithMessage_ReturnsClone(t *testing.T) {
	original := contract.NewHTTPError(http.StatusBadRequest, "original msg")
	clone := original.WithMessage("new msg")

	if clone == original {
		t.Error("WithMessage should return a clone")
	}
	if original.Message != "original msg" {
		t.Error("WithMessage should not mutate the original")
	}
	if clone.Message != "new msg" {
		t.Errorf("clone.Message = %v, want %q", clone.Message, "new msg")
	}
}

// ─── HTTPError.Is ─────────────────────────────────────────────────────────────

func TestHTTPError_Is_SameCode_ReturnsTrue(t *testing.T) {
	sentinel := contract.NewHTTPError(http.StatusUnauthorized)
	clone := sentinel.WithInternal(errors.New("reason"))
	if !errors.Is(clone, sentinel) {
		t.Error("errors.Is should return true for same status code")
	}
}

func TestHTTPError_Is_DifferentCode_ReturnsFalse(t *testing.T) {
	a := contract.NewHTTPError(http.StatusUnauthorized)
	b := contract.NewHTTPError(http.StatusForbidden)
	if errors.Is(a, b) {
		t.Error("errors.Is should return false for different status codes")
	}
}

func TestHTTPError_Is_NonHTTPError_ReturnsFalse(t *testing.T) {
	he := contract.NewHTTPError(http.StatusBadRequest)
	if errors.Is(he, errors.New("plain error")) {
		t.Error("errors.Is should return false for non-HTTPError target")
	}
}

// ─── ValidationError ─────────────────────────────────────────────────────────

func TestValidationError_Error(t *testing.T) {
	ve := contract.ValidationError{Field: "email", Message: "invalid format"}
	got := ve.Error()
	if got != "field=email: invalid format" {
		t.Errorf("ValidationError.Error() = %q", got)
	}
}

// ─── ValidationErrors ────────────────────────────────────────────────────────

func TestValidationErrors_Error_MultipleErrors(t *testing.T) {
	ves := contract.ValidationErrors{
		{Field: "name", Message: "required"},
		{Field: "age", Message: "must be positive"},
	}
	got := ves.Error()
	if got != "field=name: required; field=age: must be positive" {
		t.Errorf("ValidationErrors.Error() = %q", got)
	}
}

func TestValidationErrors_Error_SingleError(t *testing.T) {
	ves := contract.ValidationErrors{{Field: "x", Message: "bad"}}
	got := ves.Error()
	if got != "field=x: bad" {
		t.Errorf("ValidationErrors.Error() = %q", got)
	}
}

func TestValidationErrors_Error_Empty(t *testing.T) {
	ves := contract.ValidationErrors{}
	got := ves.Error()
	if got != "" {
		t.Errorf("empty ValidationErrors.Error() = %q, want empty", got)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
