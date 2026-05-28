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

// ─── XML binder ───────────────────────────────────────────────────────────────

type xmlUser struct {
	Name  string `xml:"name"`
	Email string `xml:"email"`
}

func TestXML_BindValidStruct(t *testing.T) {
	body := `<xmlUser><name>Alice</name><email>alice@example.com</email></xmlUser>`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/xml")

	var u xmlUser
	err := binding.XML.Bind(r, &u)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Alice", u.Name)
	testutil.AssertEqual(t, "alice@example.com", u.Email)
}

func TestXML_EmptyBody_ReturnsError(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	var u xmlUser
	err := binding.XML.Bind(r, &u)
	testutil.AssertError(t, err)
}

func TestXML_InvalidXML_ReturnsError(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not-xml`))
	var u xmlUser
	err := binding.XML.Bind(r, &u)
	testutil.AssertError(t, err)
}

// ─── Form binder ──────────────────────────────────────────────────────────────

type formUser struct {
	Name  string `form:"name"`
	Age   int    `form:"age"`
}

func TestForm_BindURLEncoded(t *testing.T) {
	body := "name=Bob&age=25"
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var u formUser
	err := binding.Form.Bind(r, &u)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Bob", u.Name)
	testutil.AssertEqual(t, 25, u.Age)
}

func TestForm_InvalidForm_ReturnsError(t *testing.T) {
	// ParseForm never errors on simple bodies, but invalid multipart should
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("--boundary\r\nnot-valid-multipart"))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	var u formUser
	err := binding.Form.Bind(r, &u)
	testutil.AssertError(t, err)
}

// ─── BindHeader ───────────────────────────────────────────────────────────────

type headerStruct struct {
	RequestID string `header:"X-Request-Id"`
	Auth      string `header:"Authorization"`
}

func TestBindHeader_PopulatesFields(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-Id", "req-123")
	h.Set("Authorization", "Bearer token")

	var s headerStruct
	err := binding.BindHeader(h, &s)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "req-123", s.RequestID)
	testutil.AssertEqual(t, "Bearer token", s.Auth)
}

func TestBindHeader_MissingField_LeftEmpty(t *testing.T) {
	h := http.Header{}
	var s headerStruct
	err := binding.BindHeader(h, &s)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "", s.RequestID)
}

// ─── DefaultBinder ────────────────────────────────────────────────────────────

func TestDefaultBinder_BindForm(t *testing.T) {
	body := "name=Carol&age=30"
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var u formUser
	err := binding.Default.BindForm(r, &u)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Carol", u.Name)
}

func TestDefaultBinder_BindQuery(t *testing.T) {
	q := url.Values{"name": {"Dave"}, "age": {"40"}}
	var u formUser
	err := binding.Default.BindQuery(q, &u)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Dave", u.Name)
}

func TestDefaultBinder_BindPath(t *testing.T) {
	type pathStruct struct {
		ID   int    `uri:"id"`
		Slug string `uri:"slug"`
	}
	params := []binding.Param{{Key: "id", Value: "99"}, {Key: "slug", Value: "hello"}}
	var p pathStruct
	err := binding.Default.BindPath(params, &p)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, 99, p.ID)
	testutil.AssertEqual(t, "hello", p.Slug)
}

func TestDefaultBinder_BindHeader(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-Id", "hdr-456")
	var s headerStruct
	err := binding.Default.BindHeader(h, &s)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "hdr-456", s.RequestID)
}

func TestDefaultBinder_Validate(t *testing.T) {
	type req struct {
		Name string `validate:"required"`
	}
	err := binding.Default.Validate(&req{Name: ""})
	testutil.AssertError(t, err)

	err = binding.Default.Validate(&req{Name: "ok"})
	testutil.AssertNoError(t, err)
}

// ─── SetMaxBodySize ───────────────────────────────────────────────────────────

func TestSetMaxBodySize_LimitsJSON(t *testing.T) {
	original := int64(1 << 20)
	binding.SetMaxBodySize(10) // 10 bytes
	defer binding.SetMaxBodySize(original)

	body := `{"name":"Alice","email":"alice@example.com","age":30}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	var u userRequest
	err := binding.JSON.Bind(r, &u)
	// Body exceeds 10 bytes but LimitedReader just truncates — JSON decode fails
	if err == nil {
		t.Error("expected error when body exceeds max size")
	}
}

func TestSetMaxBodySize_Reset(t *testing.T) {
	binding.SetMaxBodySize(0) // reset to default
	body := `{"name":"Alice","email":"alice@example.com","age":30}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var u userRequest
	err := binding.JSON.Bind(r, &u)
	testutil.AssertNoError(t, err)
}

// ─── SetDefaultValidator / GetDefaultValidator ────────────────────────────────

func TestSetGetDefaultValidator(t *testing.T) {
	orig := binding.GetDefaultValidator()
	if orig == nil {
		t.Fatal("GetDefaultValidator returned nil")
	}
	// Replace and restore
	binding.SetDefaultValidator(orig)
	if binding.GetDefaultValidator() != orig {
		t.Error("SetDefaultValidator did not persist")
	}
}

// ─── setFieldValue edge cases ─────────────────────────────────────────────────

type numericFields struct {
	Uint8Val  uint8   `form:"u8"`
	Uint16Val uint16  `form:"u16"`
	Uint32Val uint32  `form:"u32"`
	Uint64Val uint64  `form:"u64"`
	Float32   float32 `form:"f32"`
	Float64   float64 `form:"f64"`
}

func TestBindQuery_NumericTypes(t *testing.T) {
	q := url.Values{
		"u8":  {"255"},
		"u16": {"65535"},
		"u32": {"4294967295"},
		"u64": {"18446744073709551615"},
		"f32": {"3.14"},
		"f64": {"2.718281828"},
	}
	var f numericFields
	err := binding.BindQuery(q, &f)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, uint8(255), f.Uint8Val)
	testutil.AssertEqual(t, uint16(65535), f.Uint16Val)
	testutil.AssertEqual(t, float32(3.14), f.Float32)
}

func TestBindQuery_InvalidUint_ReturnsError(t *testing.T) {
	q := url.Values{"u8": {"-1"}}
	var f numericFields
	err := binding.BindQuery(q, &f)
	testutil.AssertError(t, err)
}

func TestBindQuery_InvalidFloat_ReturnsError(t *testing.T) {
	q := url.Values{"f64": {"not-a-float"}}
	var f numericFields
	err := binding.BindQuery(q, &f)
	testutil.AssertError(t, err)
}

type ptrFields struct {
	Name *string `form:"name"`
	Age  *int    `form:"age"`
}

func TestBindQuery_PointerFields(t *testing.T) {
	q := url.Values{"name": {"Eve"}, "age": {"28"}}
	var p ptrFields
	err := binding.BindQuery(q, &p)
	testutil.AssertNoError(t, err)
	if p.Name == nil || *p.Name != "Eve" {
		t.Errorf("expected Name=Eve, got %v", p.Name)
	}
	if p.Age == nil || *p.Age != 28 {
		t.Errorf("expected Age=28, got %v", p.Age)
	}
}

func TestBindQuery_PointerField_EmptyValue_LeftNil(t *testing.T) {
	q := url.Values{"name": {""}}
	var p ptrFields
	err := binding.BindQuery(q, &p)
	testutil.AssertNoError(t, err)
	if p.Name != nil {
		t.Errorf("expected nil pointer for empty value, got %v", p.Name)
	}
}

// ─── validationMessage branches ───────────────────────────────────────────────

func TestValidate_MessageBranches(t *testing.T) {
	type req struct {
		Email   string `validate:"email"`
		URL     string `validate:"url"`
		UUID    string `validate:"uuid"`
		Min     string `validate:"min=5"`
		Max     string `validate:"max=3"`
		GT      int    `validate:"gt=10"`
		GTE     int    `validate:"gte=10"`
		LT      int    `validate:"lt=0"`
		LTE     int    `validate:"lte=0"`
		OneOf   string `validate:"oneof=a b c"`
		Alpha   string `validate:"alpha"`
		AlphNum string `validate:"alphanum"`
		Numeric string `validate:"numeric"`
	}

	tests := []struct {
		name  string
		obj   any
		want  string
	}{
		{"email", &struct{ Email string `validate:"email"` }{Email: "bad"}, "valid email"},
		{"url", &struct{ URL string `validate:"url"` }{URL: "bad"}, "valid URL"},
		{"uuid", &struct{ UUID string `validate:"uuid"` }{UUID: "bad"}, "valid UUID"},
		{"min string", &struct{ S string `validate:"min=5"` }{S: "ab"}, "at least"},
		{"max string", &struct{ S string `validate:"max=3"` }{S: "toolong"}, "at most"},
		{"gt", &struct{ N int `validate:"gt=10"` }{N: 5}, "greater than"},
		{"gte", &struct{ N int `validate:"gte=10"` }{N: 5}, "greater than or equal"},
		{"lt", &struct{ N int `validate:"lt=0"` }{N: 5}, "less than"},
		{"lte", &struct{ N int `validate:"lte=0"` }{N: 5}, "less than or equal"},
		{"oneof", &struct{ S string `validate:"oneof=a b c"` }{S: "z"}, "one of"},
		{"alpha", &struct{ S string `validate:"alpha"` }{S: "123"}, "alphabetic"},
		{"alphanum", &struct{ S string `validate:"alphanum"` }{S: "!@#"}, "alphanumeric"},
		{"numeric", &struct{ S string `validate:"numeric"` }{S: "abc"}, "numeric"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := binding.Validate(tc.obj)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestBindQuery_NonPtrNonStruct_ReturnsError(t *testing.T) {
	q := url.Values{"x": {"1"}}
	var n int
	err := binding.BindQuery(q, &n)
	testutil.AssertError(t, err)
}

func TestBindQuery_NilPtr_ReturnsError(t *testing.T) {
	q := url.Values{"x": {"1"}}
	var p *formUser
	err := binding.BindQuery(q, p)
	testutil.AssertError(t, err)
}

func TestSetMaxStringLen_GetMaxStringLen(t *testing.T) {
	orig := binding.GetMaxStringLen()
	binding.SetMaxStringLen(100)
	if binding.GetMaxStringLen() != 100 {
		t.Error("SetMaxStringLen did not persist")
	}
	binding.SetMaxStringLen(orig)
}
