package binding_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/astra-go/astra/binding"
	"github.com/astra-go/astra/testutil"
)

// ─── JSON binder ──────────────────────────────────────────────────────────────

type userRequest struct {
	Name  string `json:"name"  validate:"required,min=2"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age"   validate:"gte=0,lte=150"`
}

func TestJSON_BindValidStruct(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "all fields",
			body:    `{"name":"Alice","email":"alice@example.com","age":30}`,
			wantErr: false,
		},
		{
			name:    "zero age",
			body:    `{"name":"Bob","email":"bob@b.com","age":0}`,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")

			var u userRequest
			err := binding.JSON.Bind(r, &u)
			if (err != nil) != tc.wantErr {
				t.Errorf("Bind() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestJSON_BindInvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not-json`))
	r.Header.Set("Content-Type", "application/json")

	var u userRequest
	err := binding.JSON.Bind(r, &u)
	testutil.AssertError(t, err)
}

func TestJSON_EmptyBody_ReturnsError(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Content-Type", "application/json")

	var u userRequest
	err := binding.JSON.Bind(r, &u)
	testutil.AssertError(t, err)
}

func TestJSON_Bind_DecodesFields(t *testing.T) {
	body := `{"name":"Carol","email":"carol@c.io","age":25}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	var u userRequest
	testutil.AssertNoError(t, binding.JSON.Bind(r, &u))
	testutil.AssertEqual(t, "Carol", u.Name)
	testutil.AssertEqual(t, "carol@c.io", u.Email)
	testutil.AssertEqual(t, 25, u.Age)
}

// ─── BindQuery ────────────────────────────────────────────────────────────────

type pageQuery struct {
	Page    int    `form:"page"`
	Limit   int    `form:"limit"`
	Keyword string `form:"keyword"`
}

func TestBindQuery_StringAndInt(t *testing.T) {
	tests := []struct {
		name    string
		values  url.Values
		want    pageQuery
		wantErr bool
	}{
		{
			name:   "all fields",
			values: url.Values{"page": {"2"}, "limit": {"20"}, "keyword": {"go"}},
			want:   pageQuery{Page: 2, Limit: 20, Keyword: "go"},
		},
		{
			name:   "partial fields",
			values: url.Values{"page": {"1"}},
			want:   pageQuery{Page: 1},
		},
		{
			name:    "invalid int",
			values:  url.Values{"page": {"abc"}},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var q pageQuery
			err := binding.BindQuery(tc.values, &q)
			if (err != nil) != tc.wantErr {
				t.Fatalf("BindQuery() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				testutil.AssertEqual(t, tc.want.Page, q.Page)
				testutil.AssertEqual(t, tc.want.Limit, q.Limit)
				testutil.AssertEqual(t, tc.want.Keyword, q.Keyword)
			}
		})
	}
}

func TestBindQuery_BoolField(t *testing.T) {
	type req struct {
		Active bool `form:"active"`
	}
	tests := []struct {
		raw  string
		want bool
	}{
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
	}
	for _, tc := range tests {
		var r req
		testutil.AssertNoError(t, binding.BindQuery(url.Values{"active": {tc.raw}}, &r))
		testutil.AssertEqual(t, tc.want, r.Active)
	}
}

func TestBindQuery_SliceOfStrings(t *testing.T) {
	type req struct {
		Tags []string `form:"tags"`
	}
	var r req
	testutil.AssertNoError(t, binding.BindQuery(url.Values{"tags": {"go", "web", "api"}}, &r))
	testutil.AssertEqual(t, 3, len(r.Tags))
	testutil.AssertEqual(t, "go", r.Tags[0])
}

// TestBindQuery_SliceAtLimit verifies that exactly MaxSliceParams values are
// accepted without error (boundary should be inclusive).
func TestBindQuery_SliceAtLimit(t *testing.T) {
	type req struct {
		IDs []string `form:"id"`
	}
	vals := make([]string, binding.MaxSliceParams)
	for i := range vals {
		vals[i] = "x"
	}
	var r req
	testutil.AssertNoError(t, binding.BindQuery(url.Values{"id": vals}, &r))
	testutil.AssertEqual(t, binding.MaxSliceParams, len(r.IDs))
}

// TestBindQuery_SliceExceedsLimit is the DoS regression test.
// Sending MaxSliceParams+1 values must be rejected before any allocation.
func TestBindQuery_SliceExceedsLimit(t *testing.T) {
	type req struct {
		Tags []string `form:"tags"`
	}
	vals := make([]string, binding.MaxSliceParams+1)
	for i := range vals {
		vals[i] = "a"
	}
	var r req
	err := binding.BindQuery(url.Values{"tags": vals}, &r)
	if err == nil {
		t.Fatal("expected error for slice exceeding MaxSliceParams, got nil")
	}
}

// TestBindQuery_StringExceedsMaxLen verifies that oversized string values are
// rejected to prevent database pressure (CVE-007).
func TestBindQuery_StringExceedsMaxLen(t *testing.T) {
	orig := binding.GetMaxStringLen()
	binding.SetMaxStringLen(10)
	t.Cleanup(func() { binding.SetMaxStringLen(orig) })

	type req struct {
		Name string `form:"name"`
	}
	var r req
	err := binding.BindQuery(url.Values{"name": {strings.Repeat("a", 11)}}, &r)
	if err == nil {
		t.Fatal("expected error for string exceeding MaxStringLen, got nil")
	}
}

// TestBindQuery_StringAtMaxLen verifies that a string exactly at the limit is accepted.
func TestBindQuery_StringAtMaxLen(t *testing.T) {
	orig := binding.GetMaxStringLen()
	binding.SetMaxStringLen(10)
	t.Cleanup(func() { binding.SetMaxStringLen(orig) })

	type req struct {
		Name string `form:"name"`
	}
	var r req
	testutil.AssertNoError(t, binding.BindQuery(url.Values{"name": {strings.Repeat("a", 10)}}, &r))
}

// TestBindQuery_StringLimitDisabled verifies that SetMaxStringLen(0) disables the check.
func TestBindQuery_StringLimitDisabled(t *testing.T) {
	orig := binding.GetMaxStringLen()
	binding.SetMaxStringLen(0)
	t.Cleanup(func() { binding.SetMaxStringLen(orig) })

	type req struct {
		Name string `form:"name"`
	}
	var r req
	testutil.AssertNoError(t, binding.BindQuery(url.Values{"name": {strings.Repeat("a", 100_000)}}, &r))
}

// TestBindQuery_SliceStringExceedsMaxLen verifies that oversized strings inside
// slice fields are also rejected.
func TestBindQuery_SliceStringExceedsMaxLen(t *testing.T) {
	orig := binding.GetMaxStringLen()
	binding.SetMaxStringLen(5)
	t.Cleanup(func() { binding.SetMaxStringLen(orig) })

	type req struct {
		Tags []string `form:"tags"`
	}
	var r req
	err := binding.BindQuery(url.Values{"tags": {"ok", strings.Repeat("x", 6)}}, &r)
	if err == nil {
		t.Fatal("expected error for slice element exceeding MaxStringLen, got nil")
	}
}

type pathParams struct {
	ID   int64  `uri:"id"`
	Slug string `uri:"slug"`
}

func TestBindPath(t *testing.T) {
	tests := []struct {
		name    string
		params  []binding.Param
		want    pathParams
		wantErr bool
	}{
		{
			name:   "int and string",
			params: []binding.Param{{Key: "id", Value: "42"}, {Key: "slug", Value: "hello-world"}},
			want:   pathParams{ID: 42, Slug: "hello-world"},
		},
		{
			name:   "id only",
			params: []binding.Param{{Key: "id", Value: "99"}},
			want:   pathParams{ID: 99},
		},
		{
			name:    "invalid int",
			params:  []binding.Param{{Key: "id", Value: "not-a-number"}},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var p pathParams
			err := binding.BindPath(tc.params, &p)
			if (err != nil) != tc.wantErr {
				t.Fatalf("BindPath() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				testutil.AssertEqual(t, tc.want.ID, p.ID)
				testutil.AssertEqual(t, tc.want.Slug, p.Slug)
			}
		})
	}
}

// ─── Validate ─────────────────────────────────────────────────────────────────

func TestValidate_ValidStruct_NoError(t *testing.T) {
	u := userRequest{Name: "Alice", Email: "alice@example.com", Age: 30}
	testutil.AssertNoError(t, binding.Validate(&u))
}

func TestValidate_InvalidFields_ReturnsValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		obj       any
		wantField string
	}{
		{
			name:      "missing required name",
			obj:       &userRequest{Email: "a@b.com", Age: 20},
			wantField: "name",
		},
		{
			name:      "invalid email",
			obj:       &userRequest{Name: "Alice", Email: "not-email", Age: 20},
			wantField: "email",
		},
		{
			name:      "name too short",
			obj:       &userRequest{Name: "A", Email: "a@b.com", Age: 20},
			wantField: "name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := binding.Validate(tc.obj)
			testutil.AssertError(t, err)

			var verrs binding.ValidationErrors
			var ok bool
			verrs, ok = err.(binding.ValidationErrors)
			if !ok {
				t.Fatalf("expected ValidationErrors, got %T", err)
			}
			found := false
			for _, ve := range verrs {
				if ve.Field == tc.wantField {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected validation error for field %q, got: %v", tc.wantField, verrs)
			}
		})
	}
}

func TestValidationErrors_ErrorString(t *testing.T) {
	ve := binding.ValidationErrors{
		{Field: "name", Message: "this field is required"},
		{Field: "email", Message: "must be a valid email address"},
	}
	msg := ve.Error()
	if !strings.Contains(msg, "name") || !strings.Contains(msg, "email") {
		t.Errorf("ValidationErrors.Error() missing field names: %s", msg)
	}
}

func TestValidationError_ErrorString(t *testing.T) {
	ve := binding.ValidationError{Field: "username", Message: "this field is required"}
	if !strings.Contains(ve.Error(), "username") {
		t.Errorf("ValidationError.Error() = %q, should contain field name", ve.Error())
	}
}
