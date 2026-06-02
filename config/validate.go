package config

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// ─── Validation errors ────────────────────────────────────────────────────────

// ValidationError describes a single field validation failure.
type ValidationError struct {
	Field   string // dot-separated field path (uses yaml/json/toml tag name)
	Rule    string // rule name, e.g. "required", "min", "pattern"
	Message string // human-readable description
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("config: field %q failed %q: %s", e.Field, e.Rule, e.Message)
}

// ValidationErrors is a slice of ValidationError returned when one or more
// validate rules fail. It implements the error interface.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	msgs := make([]string, len(e))
	for i, ve := range e {
		msgs[i] = ve.Error()
	}
	return strings.Join(msgs, "; ")
}

// ─── Validate ─────────────────────────────────────────────────────────────────

// Validate checks obj against its `validate` struct tags.
// obj must be a non-nil pointer to a struct.
//
// Supported rules (comma-separated in the tag value):
//
//	required          — field must be non-zero
//	min=N             — numeric value ≥ N
//	max=N             — numeric value ≤ N
//	minlen=N          — string length ≥ N (UTF-8 bytes)
//	maxlen=N          — string length ≤ N (UTF-8 bytes)
//	oneof=a|b|c       — fmt.Sprintf("%v") of the value must equal one option
//	pattern=<regex>   — string must match the compiled regular expression
//
// Example:
//
//	type ServerCfg struct {
//	    Host    string `yaml:"host"    validate:"required"`
//	    Port    int    `yaml:"port"    validate:"required,min=1,max=65535"`
//	    Mode    string `yaml:"mode"    validate:"oneof=debug|release|test"`
//	    APIKey  string `yaml:"api_key" validate:"minlen=32"`
//	}
func Validate(obj any) error {
	if obj == nil {
		return nil
	}
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("config: Validate requires a struct pointer, got %T", obj)
	}

	var errs ValidationErrors
	validateStruct(rv, "", &errs)
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// patternCache caches compiled regular expressions used by the "pattern" rule.
var patternCache sync.Map

func cachedRegexp(pattern string) (*regexp.Regexp, error) {
	if v, ok := patternCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	patternCache.Store(pattern, re)
	return re, nil
}

func validateStruct(rv reflect.Value, prefix string, errs *ValidationErrors) {
	rt := rv.Type()
	for i := range rt.NumField() {
		field := rt.Field(i)
		fv := rv.Field(i)

		if !field.IsExported() {
			continue
		}

		name := tagFieldName(field)
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}

		// Recurse into nested structs (both value and pointer).
		switch fv.Kind() {
		case reflect.Struct:
			validateStruct(fv, path, errs)
		case reflect.Pointer:
			if !fv.IsNil() && fv.Elem().Kind() == reflect.Struct {
				validateStruct(fv.Elem(), path, errs)
			}
		}

		tag := field.Tag.Get("validate")
		if tag == "" || tag == "-" {
			continue
		}

		for rule := range strings.SplitSeq(tag, ",") {
			rule = strings.TrimSpace(rule)
			if rule == "" {
				continue
			}
			if ve := applyRule(fv, path, rule); ve != nil {
				*errs = append(*errs, *ve)
			}
		}
	}
}

// tagFieldName returns the field name from yaml/json/toml tags, falling back
// to the lowercased Go field name.
func tagFieldName(f reflect.StructField) string {
	for _, tag := range []string{"yaml", "json", "toml"} {
		if v := f.Tag.Get(tag); v != "" && v != "-" {
			name, _, _ := strings.Cut(v, ",")
			if name != "" && name != "-" {
				return name
			}
		}
	}
	return strings.ToLower(f.Name)
}

func applyRule(fv reflect.Value, path, rule string) *ValidationError {
	name, arg, _ := strings.Cut(rule, "=")
	switch name {
	case "required":
		if fv.IsZero() {
			return &ValidationError{Field: path, Rule: "required", Message: "value is required"}
		}

	case "min":
		n, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return &ValidationError{Field: path, Rule: "min", Message: fmt.Sprintf("invalid min argument %q", arg)}
		}
		if v := reflectFloat(fv); v < n {
			return &ValidationError{Field: path, Rule: "min", Message: fmt.Sprintf("value %v must be >= %v", v, n)}
		}

	case "max":
		n, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			return &ValidationError{Field: path, Rule: "max", Message: fmt.Sprintf("invalid max argument %q", arg)}
		}
		if v := reflectFloat(fv); v > n {
			return &ValidationError{Field: path, Rule: "max", Message: fmt.Sprintf("value %v must be <= %v", v, n)}
		}

	case "minlen":
		n, err := strconv.Atoi(arg)
		if err != nil {
			return &ValidationError{Field: path, Rule: "minlen", Message: fmt.Sprintf("invalid minlen argument %q", arg)}
		}
		if fv.Kind() == reflect.String && len(fv.String()) < n {
			return &ValidationError{Field: path, Rule: "minlen",
				Message: fmt.Sprintf("length %d must be >= %d", len(fv.String()), n)}
		}

	case "maxlen":
		n, err := strconv.Atoi(arg)
		if err != nil {
			return &ValidationError{Field: path, Rule: "maxlen", Message: fmt.Sprintf("invalid maxlen argument %q", arg)}
		}
		if fv.Kind() == reflect.String && len(fv.String()) > n {
			return &ValidationError{Field: path, Rule: "maxlen",
				Message: fmt.Sprintf("length %d must be <= %d", len(fv.String()), n)}
		}

	case "oneof":
		options := strings.Split(arg, "|")
		val := fmt.Sprintf("%v", fv.Interface())
		if !slices.Contains(options, val) {
			return &ValidationError{Field: path, Rule: "oneof",
				Message: fmt.Sprintf("value %q must be one of [%s]", val, strings.Join(options, ", "))}
		}

	case "pattern":
		if fv.Kind() != reflect.String {
			return nil
		}
		re, err := cachedRegexp(arg)
		if err != nil {
			return &ValidationError{Field: path, Rule: "pattern",
				Message: fmt.Sprintf("invalid pattern %q: %v", arg, err)}
		}
		if !re.MatchString(fv.String()) {
			return &ValidationError{Field: path, Rule: "pattern",
				Message: fmt.Sprintf("value %q does not match pattern %q", fv.String(), arg)}
		}
	}
	return nil
}

func reflectFloat(fv reflect.Value) float64 {
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(fv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(fv.Uint())
	case reflect.Float32, reflect.Float64:
		return fv.Float()
	}
	return 0
}
