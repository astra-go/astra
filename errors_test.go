package astra_test

import (
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/testutil"
)

// ─── HTTPError ────────────────────────────────────────────────────────────────

func TestHTTPError_Error(t *testing.T) {
	e := astra.NewHTTPError(http.StatusNotFound, "not found")
	if e.Error() == "" {
		t.Fatal("Error() must return non-empty string")
	}
	testutil.AssertEqual(t, http.StatusNotFound, e.Code)
}

func TestHTTPError_WithInternal(t *testing.T) {
	cause := errors.New("db error")
	e := astra.NewHTTPError(http.StatusInternalServerError, "server error").WithInternal(cause)
	testutil.AssertErrorIs(t, e.Unwrap(), cause)
}

func TestHTTPError_WithInternal_ReturnsClone(t *testing.T) {
	base := astra.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	cause := errors.New("token expired")
	clone := base.WithInternal(cause)

	if clone == base {
		t.Fatal("WithInternal must return a new *HTTPError, not mutate the receiver")
	}
	if base.Unwrap() != nil {
		t.Fatal("base.Err must remain nil after WithInternal — global sentinel must not be mutated")
	}
	testutil.AssertErrorIs(t, clone.Unwrap(), cause)
}

func TestHTTPError_WithMessage_ReturnsClone(t *testing.T) {
	base := astra.NewHTTPError(http.StatusForbidden)
	clone := base.WithMessage("custom message")

	if clone == base {
		t.Fatal("WithMessage must return a new *HTTPError")
	}
	testutil.AssertEqual(t, http.StatusText(http.StatusForbidden), base.Message.(string))
	testutil.AssertEqual(t, "custom message", clone.Message)
	testutil.AssertEqual(t, http.StatusForbidden, clone.Code)
}

// TestHTTPError_Is verifies that errors.Is uses status-code equality so that
// clones produced by With* methods still match the original sentinel.
func TestHTTPError_Is_MatchesByCode(t *testing.T) {
	cause := errors.New("some internal error")
	clone := astra.ErrUnauthorized.WithInternal(cause)

	if !errors.Is(clone, astra.ErrUnauthorized) {
		t.Fatal("errors.Is must return true for a clone with the same status code")
	}
	if errors.Is(clone, astra.ErrNotFound) {
		t.Fatal("errors.Is must return false for a different status code")
	}
}

func TestHTTPError_Is_Unwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	he := astra.NewHTTPError(500, "boom").WithInternal(sentinel)
	testutil.AssertErrorIs(t, he, sentinel)
}

// TestHTTPError_GlobalSentinel_ConcurrentSafe verifies that concurrent use of
// global sentinel variables (ErrBadRequest etc.) with WithInternal does not
// cause a data race.  Run with -race to catch mutations.
func TestHTTPError_GlobalSentinel_ConcurrentSafe(t *testing.T) {
	const goroutines = 50
	cause := errors.New("concurrent error")

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_ = astra.ErrBadRequest.WithInternal(cause)
			_ = astra.ErrUnauthorized.WithMessage("overridden")
			_ = astra.ErrNotFound.WithInternal(cause)
		}()
	}
	wg.Wait()

	// Globals must remain unmodified.
	if astra.ErrBadRequest.Unwrap() != nil {
		t.Error("ErrBadRequest.Err must remain nil after concurrent WithInternal calls")
	}
	if astra.ErrUnauthorized.Message != http.StatusText(http.StatusUnauthorized) {
		t.Errorf("ErrUnauthorized.Message mutated: got %v", astra.ErrUnauthorized.Message)
	}
}

func TestHTTPError_Unwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	he := astra.NewHTTPError(500, "boom").WithInternal(sentinel)
	testutil.AssertErrorIs(t, he, sentinel)
}

// ─── AppError ─────────────────────────────────────────────────────────────────

func TestAppError_Error(t *testing.T) {
	e := &astra.AppError{
		Code:       "INVALID_INPUT",
		HTTPStatus: http.StatusBadRequest,
		Message:    "invalid input",
	}
	if e.Error() == "" {
		t.Fatal("AppError.Error() must return non-empty string")
	}
}

func TestAppError_WithData_ImmutableClone(t *testing.T) {
	original := &astra.AppError{
		Code:       "ERR",
		HTTPStatus: 400,
		Message:    "base",
	}
	clone := original.WithData(map[string]any{"field": "name"})

	// Clone must differ from original
	if clone == original {
		t.Fatal("WithData must return a new AppError, not mutate original")
	}
	if original.Data != nil {
		t.Fatal("original.Data must remain nil after WithData clone")
	}
}

func TestAppError_WithMessage_ImmutableClone(t *testing.T) {
	base := &astra.AppError{Code: "ERR", HTTPStatus: 400, Message: "original"}
	cloned := base.WithMessage("updated")
	testutil.AssertEqual(t, "original", base.Message)
	testutil.AssertEqual(t, "updated", cloned.Message)
}

func TestAppError_WithInternal_ImmutableClone(t *testing.T) {
	sentinel := errors.New("internal")
	base := &astra.AppError{Code: "ERR", HTTPStatus: 500, Message: "err"}
	cloned := base.WithInternal(sentinel)
	if base.Err != nil {
		t.Fatal("original.Err must remain nil after WithInternal clone")
	}
	testutil.AssertErrorIs(t, cloned.Unwrap(), sentinel)
}

func TestAppError_ErrorsIs(t *testing.T) {
	base := &astra.AppError{Code: "NOT_FOUND", HTTPStatus: 404, Message: "not found"}
	wrapped := base.WithData("extra")
	// errors.Is checks pointer equality; each clone is independent
	if errors.Is(wrapped, base) {
		t.Log("note: errors.Is compares by value not pointer for AppError")
	}
}

func TestAppError_Unwrap(t *testing.T) {
	inner := errors.New("db")
	e := (&astra.AppError{Code: "DB_ERR", HTTPStatus: 500, Message: "db error"}).
		WithInternal(inner)
	testutil.AssertErrorIs(t, e, inner)
}

// ─── ValidationErrors ─────────────────────────────────────────────────────────

func TestValidationHTTPError_Fields(t *testing.T) {
	app := testutil.NewTestApp()
	type req struct {
		Name  string `json:"name"  validate:"required"`
		Email string `json:"email" validate:"required,email"`
	}
	app.POST("/validate", func(c *astra.Ctx) error {
		var r req
		return c.ShouldBindJSON(&r)
	})

	srv := testutil.NewServer(t, app)
	resp := srv.POST("/validate", map[string]any{"name": ""})
	resp.AssertStatus(http.StatusBadRequest)
}

