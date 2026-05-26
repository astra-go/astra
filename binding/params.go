package binding

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// BindHeader binds HTTP request headers into obj.
//
// Uses the `header:"Name"` struct tag. The tag value is canonicalized
// (e.g. "x-request-id" → "X-Request-Id") before lookup. Fields without
// an explicit "header" tag are skipped — there is no field-name fallback.
func BindHeader(h http.Header, obj any) error {
	return mapValues(obj, url.Values(h), "header")
}

// BindQuery binds URL query parameters to obj.
//
// Tag priority: `query:"name"` → `form:"name"` → lowercase field name.
func BindQuery(values url.Values, obj any) error {
	return mapValues(obj, values, "query")
}

// BindPath binds URL path parameters (from the router) to obj.
//
// Uses the `uri:"name"` struct tag. Falls back to the lowercase field name.
//
// Security: path parameters are opaque strings extracted by the router — they
// contain only the literal segment matched against the route pattern. No
// shell, SQL, or HTML interpretation is performed here.
func BindPath(params []Param, obj any) error {
	values := make(url.Values, len(params))
	for _, p := range params {
		values[p.Key] = []string{p.Value}
	}
	return mapValues(obj, values, "uri")
}

// ─── Struct mapper ────────────────────────────────────────────────────────────

// mapValues maps url.Values to a struct using the given primary tag name.
//
// Tag lookup order for tagName = "query":   query → form → lowercase field name
// Tag lookup order for tagName = "uri":     uri   → lowercase field name
// Tag lookup order for tagName = "form":    form  → lowercase field name
//
// Supports embedded structs (anonymous fields).
// Supports slices of strings and basic scalar types.
func mapValues(obj any, values url.Values, tagName string) error {
	rv := reflect.ValueOf(obj)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("binding: obj must be a non-nil pointer")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("binding: obj must point to a struct")
	}
	return mapStructFields(rv, values, tagName)
}

func mapStructFields(rv reflect.Value, values url.Values, tagName string) error {
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)

		if !fv.CanSet() {
			continue
		}

		// Recurse into embedded/anonymous structs.
		if field.Anonymous && fv.Kind() == reflect.Struct {
			if err := mapStructFields(fv, values, tagName); err != nil {
				return err
			}
			continue
		}

		key := lookupTagKey(field, tagName)
		if key == "-" || key == "" {
			continue
		}

		vals, ok := values[key]
		if !ok || len(vals) == 0 {
			continue
		}

		if err := setFieldValue(fv, field.Type, vals); err != nil {
			return fmt.Errorf("binding: field %q: %w", field.Name, err)
		}
	}
	return nil
}

// lookupTagKey returns the form key for a struct field.
// For tagName "query" it tries: query tag → form tag → lowercase field name.
// For tagName "uri"   it tries: uri tag → lowercase field name.
// For tagName "form"  it tries: form tag → lowercase field name.
// For tagName "header" it tries: header tag (canonicalized) → "" (no fallback).
func lookupTagKey(field reflect.StructField, tagName string) string {
	// Primary tag
	if name := strings.SplitN(field.Tag.Get(tagName), ",", 2)[0]; name != "" {
		if tagName == "header" {
			return http.CanonicalHeaderKey(name)
		}
		return name
	}
	// Fallback tags
	switch tagName {
	case "query":
		if name := strings.SplitN(field.Tag.Get("form"), ",", 2)[0]; name != "" {
			return name
		}
	case "header":
		return "" // no field-name fallback — header tag must be explicit
	}
	return strings.ToLower(field.Name)
}

// ─── Value setter ─────────────────────────────────────────────────────────────

// MaxSliceParams is the maximum number of values accepted for a single slice
// field during query / form binding.
//
// Without this limit an attacker can send GET /api?tags=x&tags=x&... with
// 100 000 repetitions, causing reflect.MakeSlice to allocate hundreds of
// megabytes per request.  100 concurrent requests would exhaust server memory.
//
// 1 000 elements covers all realistic use-cases (tag lists, multi-select
// filters, batch IDs) while bounding the per-request allocation to a few KB.
const MaxSliceParams = 1000

func setFieldValue(fv reflect.Value, ft reflect.Type, vals []string) error {
	// Handle pointer-to-scalar types.
	if ft.Kind() == reflect.Ptr {
		if vals[0] == "" {
			return nil // leave nil pointer as-is
		}
		newVal := reflect.New(ft.Elem())
		if err := setFieldValue(newVal.Elem(), ft.Elem(), vals); err != nil {
			return err
		}
		fv.Set(newVal)
		return nil
	}

	val := vals[0]

	switch ft.Kind() {
	case reflect.String:
		fv.SetString(val)

	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("invalid boolean %q", val)
		}
		fv.SetBool(b)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(val, 10, ft.Bits())
		if err != nil {
			return fmt.Errorf("invalid integer %q", val)
		}
		fv.SetInt(n)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(val, 10, ft.Bits())
		if err != nil {
			return fmt.Errorf("invalid unsigned integer %q", val)
		}
		fv.SetUint(n)

	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(val, ft.Bits())
		if err != nil {
			return fmt.Errorf("invalid float %q", val)
		}
		fv.SetFloat(n)

	case reflect.Slice:
		return setSliceField(fv, ft, vals)

	default:
		// Ignore unhandled types silently.
	}
	return nil
}

func setSliceField(fv reflect.Value, ft reflect.Type, vals []string) error {
	if len(vals) > MaxSliceParams {
		return fmt.Errorf("binding: slice exceeds maximum allowed length (%d)", MaxSliceParams)
	}
	elemKind := ft.Elem().Kind()
	slice := reflect.MakeSlice(ft, len(vals), len(vals))
	for i, v := range vals {
		elem := slice.Index(i)
		switch elemKind {
		case reflect.String:
			elem.SetString(v)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(v, 10, ft.Elem().Bits())
			if err != nil {
				return fmt.Errorf("invalid integer %q at index %d", v, i)
			}
			elem.SetInt(n)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n, err := strconv.ParseUint(v, 10, ft.Elem().Bits())
			if err != nil {
				return fmt.Errorf("invalid unsigned integer %q at index %d", v, i)
			}
			elem.SetUint(n)
		default:
			// Skip unsupported slice element types.
		}
	}
	fv.Set(slice)
	return nil
}
