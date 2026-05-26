package validate_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	govalidator "github.com/go-playground/validator/v10"

	"github.com/astra-go/astra/validate"
)

// ─── Test models ─────────────────────────────────────────────────────────────

type RegisterReq struct {
	Username string `json:"username" validate:"required,username"`
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,password"`
	Mobile   string `json:"mobile"   validate:"omitempty,mobile"`
	Age      int    `json:"age"      validate:"required,gte=18,lte=120"`
	Role     string `json:"role"     validate:"required,oneof=admin user guest"`
}

type ProfileReq struct {
	Name    string `json:"name"    validate:"required,min=2,max=50"`
	Bio     string `json:"bio"     validate:"omitempty,no_html,max=500"`
	Website string `json:"website" validate:"omitempty,url"`
}

type Address struct {
	Street string `json:"street" validate:"required"`
	City   string `json:"city"   validate:"required"`
}

type OrderReq struct {
	Amount   float64 `json:"amount"   validate:"required,gt=0"`
	Quantity int     `json:"quantity" validate:"required,gte=1,lte=100"`
	Address  Address `json:"address"  validate:"required"`
}

// ─── Struct validation ────────────────────────────────────────────────────────

func TestStruct_ValidRequest(t *testing.T) {
	req := &RegisterReq{
		Username: "alice_99",
		Email:    "alice@example.com",
		Password: "Passw0rd!",
		Age:      25,
		Role:     "user",
	}
	if err := validate.Struct(req); err != nil {
		t.Errorf("Struct(valid): expected nil error, got %v", err)
	}
}

func TestStruct_MissingRequired(t *testing.T) {
	req := &RegisterReq{} // all zero
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("Struct(empty): expected errors")
	}
	var errs validate.Errors
	if !errors.As(err, &errs) {
		t.Fatalf("expected validate.Errors, got %T", err)
	}
	if len(errs) == 0 {
		t.Error("expected at least one error")
	}
}

func TestStruct_EmailInvalid(t *testing.T) {
	req := &RegisterReq{
		Username: "alice_99",
		Email:    "not-an-email",
		Password: "Passw0rd!",
		Age:      25,
		Role:     "user",
	}
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected email error")
	}
	var errs validate.Errors
	errors.As(err, &errs)
	m := errs.Map()
	if _, ok := m["email"]; !ok {
		t.Errorf("email error not in Map: got %v", m)
	}
}

func TestStruct_RoleOneof(t *testing.T) {
	req := &RegisterReq{
		Username: "bob_01",
		Email:    "bob@example.com",
		Password: "Secret1@",
		Age:      30,
		Role:     "superuser", // invalid
	}
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected role oneof error")
	}
	var errs validate.Errors
	errors.As(err, &errs)
	m := errs.Map()
	if _, ok := m["role"]; !ok {
		t.Errorf("role error not in Map: got %v", m)
	}
}

func TestStruct_AgeRange(t *testing.T) {
	for _, age := range []int{17, 121} {
		req := &RegisterReq{
			Username: "carol_x",
			Email:    "carol@example.com",
			Password: "Str0ng!1",
			Age:      age,
			Role:     "user",
		}
		err := validate.Struct(req)
		if err == nil {
			t.Errorf("age=%d: expected range error", age)
		}
	}
}

func TestStruct_NestedStruct(t *testing.T) {
	// With WithRequiredStructEnabled(), a required struct field that is the zero
	// value reports a single error for the field itself — it doesn't recurse.
	// To get nested-field errors, pass an initialised struct with empty strings.
	req := &OrderReq{
		Amount:   100,
		Quantity: 2,
		Address:  Address{Street: "", City: ""}, // missing required inner fields
	}
	err := validate.Struct(req)
	if err == nil {
		t.Fatal("expected nested struct errors")
	}
	var errs validate.Errors
	errors.As(err, &errs)
	if len(errs) < 1 {
		t.Errorf("expected ≥1 nested errors, got 0")
	}
}

// ─── Errors API ───────────────────────────────────────────────────────────────

func TestErrors_Map_FieldsPresent(t *testing.T) {
	req := &RegisterReq{
		Email:    "bad",
		Password: "weak",
		Role:     "unknown",
	}
	err := validate.Struct(req)
	var errs validate.Errors
	errors.As(err, &errs)

	m := errs.Map()
	if len(m) == 0 {
		t.Error("Map(): expected non-empty map")
	}
	// Each value should be a non-empty string message
	for field, msg := range m {
		if msg == "" {
			t.Errorf("Map()[%s]: empty message", field)
		}
	}
}

func TestErrors_First(t *testing.T) {
	req := &RegisterReq{}
	err := validate.Struct(req)
	var errs validate.Errors
	errors.As(err, &errs)
	if errs.First() == nil {
		t.Error("First(): expected non-nil for non-empty Errors")
	}
	if validate.Errors(nil).First() != nil {
		t.Error("First(): expected nil for empty Errors")
	}
}

func TestErrors_Error_String(t *testing.T) {
	req := &RegisterReq{}
	err := validate.Struct(req)
	if !strings.Contains(err.Error(), ":") {
		t.Errorf("Error(): unexpected format: %s", err.Error())
	}
}

// ─── Var validation ───────────────────────────────────────────────────────────

func TestVar_Email_Valid(t *testing.T) {
	if err := validate.Var("alice@example.com", "required,email"); err != nil {
		t.Errorf("Var(valid email): %v", err)
	}
}

func TestVar_Email_Invalid(t *testing.T) {
	if err := validate.Var("notanemail", "email"); err == nil {
		t.Error("Var(bad email): expected error")
	}
}

func TestVar_Min_Fail(t *testing.T) {
	if err := validate.Var("hi", "min=5"); err == nil {
		t.Error("Var(min=5 on 'hi'): expected error")
	}
}

func TestVar_Min_Pass(t *testing.T) {
	if err := validate.Var("hello world", "min=5"); err != nil {
		t.Errorf("Var(min=5 pass): %v", err)
	}
}

// ─── Built-in custom validators ───────────────────────────────────────────────

func TestValidator_Mobile_Valid(t *testing.T) {
	for _, n := range []string{"13812345678", "19912345678", "18812345678"} {
		if err := validate.Var(n, "mobile"); err != nil {
			t.Errorf("mobile(%s): expected valid, got %v", n, err)
		}
	}
}

func TestValidator_Mobile_Invalid(t *testing.T) {
	for _, n := range []string{"12345678901", "2381234567", "138abc45678", "1381234567"} {
		if err := validate.Var(n, "mobile"); err == nil {
			t.Errorf("mobile(%s): expected invalid", n)
		}
	}
}

func TestValidator_Password_Valid(t *testing.T) {
	for _, pw := range []string{"Passw0rd!", "Str0ng#Pass", "A1b2C3d$"} {
		if err := validate.Var(pw, "password"); err != nil {
			t.Errorf("password(%s): expected valid, got %v", pw, err)
		}
	}
}

func TestValidator_Password_Weak(t *testing.T) {
	// Each entry below should fail at least one strength criterion.
	for _, pw := range []string{
		"short1!",        // < 8 chars (7)
		"alllower1!",     // no uppercase
		"ALLUPPER1!",     // no lowercase
		"NoSpecial1a",    // no special character
		"NoDigit!Abc",    // no digit
	} {
		if err := validate.Var(pw, "password"); err == nil {
			t.Errorf("password(%q): expected invalid (weak)", pw)
		}
	}
}

func TestValidator_Username_Valid(t *testing.T) {
	for _, u := range []string{"alice", "alice_99", "A123", "abc"} {
		if err := validate.Var(u, "username"); err != nil {
			t.Errorf("username(%s): expected valid, got %v", u, err)
		}
	}
}

func TestValidator_Username_Invalid(t *testing.T) {
	for _, u := range []string{"ab", "alice-99", "alice@", strings.Repeat("a", 33)} {
		if err := validate.Var(u, "username"); err == nil {
			t.Errorf("username(%s): expected invalid", u)
		}
	}
}

func TestValidator_NoHTML_Valid(t *testing.T) {
	if err := validate.Var("Hello, World!", "no_html"); err != nil {
		t.Errorf("no_html(plain text): %v", err)
	}
}

func TestValidator_NoHTML_Invalid(t *testing.T) {
	for _, s := range []string{"<script>alert(1)</script>", "<b>bold</b>", "<img src=x>"} {
		if err := validate.Var(s, "no_html"); err == nil {
			t.Errorf("no_html(%q): expected invalid", s)
		}
	}
}

func TestValidator_NotBlank_Valid(t *testing.T) {
	if err := validate.Var("hello", "not_blank"); err != nil {
		t.Errorf("not_blank('hello'): %v", err)
	}
}

func TestValidator_NotBlank_Invalid(t *testing.T) {
	for _, s := range []string{"", "   ", "\t\n"} {
		if err := validate.Var(s, "not_blank"); err == nil {
			t.Errorf("not_blank(%q): expected invalid", s)
		}
	}
}

// ─── Validator instance & options ────────────────────────────────────────────

func TestNew_WithCustomValidator(t *testing.T) {
	v := validate.New(validate.WithCustom("even", func(fl govalidator.FieldLevel) bool {
		return fl.Field().Int()%2 == 0
	}))

	type Req struct {
		Count int `validate:"even"`
	}
	if err := v.Struct(&Req{Count: 4}); err != nil {
		t.Errorf("custom even(4): %v", err)
	}
	if err := v.Struct(&Req{Count: 3}); err == nil {
		t.Error("custom even(3): expected error")
	}
}

func TestNew_WithAlias(t *testing.T) {
	v := validate.New(validate.WithAlias("shortname", "min=2,max=10"))

	type Req struct {
		Name string `validate:"required,shortname"`
	}
	if err := v.Struct(&Req{Name: "alice"}); err != nil {
		t.Errorf("alias shortname('alice'): %v", err)
	}
	if err := v.Struct(&Req{Name: "a"}); err == nil {
		t.Error("alias shortname('a'): expected error")
	}
}

func TestNew_WithTagName(t *testing.T) {
	v := validate.New(validate.WithTagName(func(f reflect.StructField) string {
		if name := f.Tag.Get("label"); name != "" {
			return name
		}
		return f.Name
	}))
	_ = v // just ensure it compiles without panic
}

func TestRegisterValidation_OnDefault(t *testing.T) {
	if err := validate.RegisterValidation("test_upper", func(fl govalidator.FieldLevel) bool {
		s := fl.Field().String()
		return s == strings.ToUpper(s)
	}); err != nil {
		t.Fatalf("RegisterValidation: %v", err)
	}
	if err := validate.Var("HELLO", "test_upper"); err != nil {
		t.Errorf("test_upper(HELLO): %v", err)
	}
	if err := validate.Var("hello", "test_upper"); err == nil {
		t.Error("test_upper(hello): expected error")
	}
}

func TestRegisterAlias_OnDefault(t *testing.T) {
	validate.RegisterAlias("tinymsg", "min=1,max=160")
	if err := validate.Var("Hello!", "tinymsg"); err != nil {
		t.Errorf("tinymsg alias: %v", err)
	}
}

// ─── Error messages ───────────────────────────────────────────────────────────

func TestErrorMessage_Required(t *testing.T) {
	type S struct {
		Name string `json:"name" validate:"required"`
	}
	err := validate.Struct(&S{})
	var errs validate.Errors
	errors.As(err, &errs)
	fe := errs.First()
	if fe == nil || fe.Field != "name" {
		t.Fatalf("expected field=name, got %v", fe)
	}
	if !strings.Contains(fe.Message, "必填") {
		t.Errorf("required message: want 必填, got %q", fe.Message)
	}
}

func TestErrorMessage_Email(t *testing.T) {
	type S struct {
		Email string `json:"email" validate:"required,email"`
	}
	err := validate.Struct(&S{Email: "bad"})
	var errs validate.Errors
	errors.As(err, &errs)
	m := errs.Map()
	if !strings.Contains(m["email"], "邮箱") {
		t.Errorf("email message: want 邮箱, got %q", m["email"])
	}
}

func TestErrorMessage_OneOf(t *testing.T) {
	type S struct {
		Role string `json:"role" validate:"required,oneof=admin user"`
	}
	err := validate.Struct(&S{Role: "root"})
	var errs validate.Errors
	errors.As(err, &errs)
	msg := errs.Map()["role"]
	if !strings.Contains(msg, "admin") {
		t.Errorf("oneof message should list valid values, got %q", msg)
	}
}

// ─── Inner escape hatch ───────────────────────────────────────────────────────

func TestValidator_Inner(t *testing.T) {
	v := validate.New()
	if v.Inner() == nil {
		t.Error("Inner(): expected non-nil *validator.Validate")
	}
}
