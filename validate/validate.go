// Package validate provides struct and value validation using go-playground/validator/v10.
//
// The API is intentionally thin: decorate your structs with validate:"..." tags,
// call validate.Struct (or validate.Var for a single value), and inspect the
// returned *Errors.
//
// # Built-in custom validators
//
// In addition to all standard validator/v10 rules, the following custom tags
// are registered on the default Validator:
//
//	mobile    – matches \d{11} starting with 1[3-9] (Chinese mobile)
//	password  – ≥8 chars with uppercase, lowercase, digit, and special character
//	username  – [a-zA-Z0-9_]{3,32}
//	no_html   – rejects strings containing HTML tags
//	not_blank – rejects strings that are empty or all whitespace
//
// # Quick start
//
//	type RegisterReq struct {
//	    Username string `json:"username" validate:"required,username"`
//	    Email    string `json:"email"    validate:"required,email"`
//	    Password string `json:"password" validate:"required,password"`
//	    Mobile   string `json:"mobile"   validate:"omitempty,mobile"`
//	    Age      int    `json:"age"      validate:"required,gte=18,lte=120"`
//	    Role     string `json:"role"     validate:"required,oneof=admin user guest"`
//	}
//
//	if err := validate.Struct(&req); err != nil {
//	    var verrs validate.Errors
//	    if errors.As(err, &verrs) {
//	        c.JSON(400, gin.H{"errors": verrs.Map()})
//	    }
//	}
//
// # Instance-based usage
//
//	v := validate.New(
//	    validate.WithAlias("strongpw", "required,min=10,max=64,password"),
//	    validate.WithCustom("zipcode", func(fl validator.FieldLevel) bool {
//	        return zipRegex.MatchString(fl.Field().String())
//	    }),
//	)
//	if err := v.Struct(&req); err != nil { ... }
package validate

import (
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

// ─── Validator ────────────────────────────────────────────────────────────────

// Validator wraps *validator.Validate with a clean API and built-in custom validators.
type Validator struct {
	v *validator.Validate
}

// Inner returns the underlying *validator.Validate for advanced operations
// not covered by this package's API (e.g. cross-field validation, struct-level rules).
func (val *Validator) Inner() *validator.Validate { return val.v }

// ─── Options ─────────────────────────────────────────────────────────────────

// Option configures a Validator.
type Option func(*Validator)

// WithTagName sets a custom function to derive the field display name from struct
// tags. By default, the tag priority is: json > form > query > uri > Go field name.
//
//	validate.New(validate.WithTagName(func(f reflect.StructField) string {
//	    return f.Tag.Get("label") // use label:"..." as the field name in errors
//	}))
func WithTagName(fn func(reflect.StructField) string) Option {
	return func(val *Validator) {
		val.v.RegisterTagNameFunc(fn)
	}
}

// WithCustom registers a custom validation function for the given tag name.
//
//	validate.New(validate.WithCustom("zipcode", func(fl validator.FieldLevel) bool {
//	    return zipRegex.MatchString(fl.Field().String())
//	}))
func WithCustom(tag string, fn validator.Func, callValidationEvenIfNull ...bool) Option {
	return func(val *Validator) {
		callNull := len(callValidationEvenIfNull) > 0 && callValidationEvenIfNull[0]
		_ = val.v.RegisterValidation(tag, fn, callNull)
	}
}

// WithAlias registers a short alias tag that expands to a full validation chain.
//
//	validate.New(validate.WithAlias("strongpw", "min=10,max=64,password"))
//	// Now: validate:"required,strongpw"
func WithAlias(alias, tags string) Option {
	return func(val *Validator) {
		val.v.RegisterAlias(alias, tags)
	}
}

// ─── Constructor ──────────────────────────────────────────────────────────────

// New creates a Validator with built-in custom validators and the default tag
// name resolution (json > form > query > uri > field name).
func New(opts ...Option) *Validator {
	val := &Validator{
		v: validator.New(validator.WithRequiredStructEnabled()),
	}
	// Default: use json/form/query/uri tag as field name in error messages.
	val.v.RegisterTagNameFunc(defaultTagNameFunc)
	// Register built-in custom tags.
	registerBuiltins(val.v)
	// Apply caller options.
	for _, o := range opts {
		o(val)
	}
	return val
}

func defaultTagNameFunc(fld reflect.StructField) string {
	for _, tag := range []string{"json", "form", "query", "uri"} {
		if name := strings.SplitN(fld.Tag.Get(tag), ",", 2)[0]; name != "" && name != "-" {
			return name
		}
	}
	return fld.Name
}

// ─── Struct validation ────────────────────────────────────────────────────────

// Struct validates all exported fields of s using their validate:"..." struct tags.
// Returns validate.Errors (which implements error) on failure, nil on success.
//
//	if err := v.Struct(&req); err != nil {
//	    var errs validate.Errors
//	    errors.As(err, &errs)   // always true when err != nil
//	    return errs.Map()       // {"email":"请输入有效的电子邮箱地址"}
//	}
func (val *Validator) Struct(s any) error {
	if err := val.v.Struct(s); err != nil {
		return toErrors(err)
	}
	return nil
}

// Var validates a single value against the given tag string.
//
//	if err := v.Var(email, "required,email"); err != nil { ... }
func (val *Validator) Var(field any, tag string) error {
	if err := val.v.Var(field, tag); err != nil {
		return toErrors(err)
	}
	return nil
}

// RegisterValidation adds a custom validation function.
// Returns an error if the tag name conflicts with an existing built-in tag.
func (val *Validator) RegisterValidation(tag string, fn validator.Func, callValidationEvenIfNull ...bool) error {
	callNull := len(callValidationEvenIfNull) > 0 && callValidationEvenIfNull[0]
	return val.v.RegisterValidation(tag, fn, callNull)
}

// RegisterAlias registers a tag alias.
func (val *Validator) RegisterAlias(alias, tags string) {
	val.v.RegisterAlias(alias, tags)
}

// ─── Default package-level validator ─────────────────────────────────────────

var (
	defaultOnce      sync.Once
	defaultValidator *Validator
)

func getDefault() *Validator {
	defaultOnce.Do(func() { defaultValidator = New() })
	return defaultValidator
}

// Struct validates s using the default package-level Validator.
func Struct(s any) error { return getDefault().Struct(s) }

// Var validates a single value using the default Validator.
func Var(field any, tag string) error { return getDefault().Var(field, tag) }

// RegisterValidation adds a custom validator to the default Validator.
func RegisterValidation(tag string, fn validator.Func) error {
	return getDefault().RegisterValidation(tag, fn)
}

// RegisterAlias adds a tag alias to the default Validator.
func RegisterAlias(alias, tags string) { getDefault().RegisterAlias(alias, tags) }

// ─── Built-in custom validators ───────────────────────────────────────────────

var (
	// mobileRe matches Chinese mobile numbers: 1[3-9]XXXXXXXXX
	mobileRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

	// usernameRe: alphanumeric + underscore, 3–32 chars
	usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

	// htmlTagRe detects any HTML tag
	htmlTagRe = regexp.MustCompile(`<[^>]+>`)

	// password strength helpers
	pwUpper   = regexp.MustCompile(`[A-Z]`)
	pwLower   = regexp.MustCompile(`[a-z]`)
	pwDigit   = regexp.MustCompile(`[0-9]`)
	pwSpecial = regexp.MustCompile(`[!@#$%^&*()\-_=+\[\]{}|;':",.<>?/\\` + "`~]")
)

func registerBuiltins(v *validator.Validate) {
	_ = v.RegisterValidation("mobile", validateMobile)
	_ = v.RegisterValidation("password", validatePassword)
	_ = v.RegisterValidation("username", validateUsername)
	_ = v.RegisterValidation("no_html", validateNoHTML)
	_ = v.RegisterValidation("not_blank", validateNotBlank)
}

// mobile: Chinese mobile number (1[3-9]XXXXXXXXX, 11 digits).
func validateMobile(fl validator.FieldLevel) bool {
	return mobileRe.MatchString(fl.Field().String())
}

// password: ≥8 chars, at least one uppercase, lowercase, digit, special char.
func validatePassword(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	return len(s) >= 8 &&
		pwUpper.MatchString(s) &&
		pwLower.MatchString(s) &&
		pwDigit.MatchString(s) &&
		pwSpecial.MatchString(s)
}

// username: [a-zA-Z0-9_]{3,32}
func validateUsername(fl validator.FieldLevel) bool {
	return usernameRe.MatchString(fl.Field().String())
}

// no_html: rejects strings containing HTML tags.
func validateNoHTML(fl validator.FieldLevel) bool {
	return !htmlTagRe.MatchString(fl.Field().String())
}

// not_blank: rejects strings that are empty or all whitespace.
func validateNotBlank(fl validator.FieldLevel) bool {
	return strings.TrimSpace(fl.Field().String()) != ""
}
