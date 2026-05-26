package proto

import (
	"fmt"
	"regexp"
	"strings"
)

// FieldInfo holds parsed proto field information.
type FieldInfo struct {
	OrigName string
	GoName   string
	GoType   string
	JSONTag  string
}

// MsgInfo holds a parsed proto message.
type MsgInfo struct {
	Name   string
	Fields []FieldInfo
}

// EnumValInfo holds one enum value.
type EnumValInfo struct {
	GoName string
	Number string
}

// EnumInfo holds a parsed proto enum.
type EnumInfo struct {
	Name   string
	Values []EnumValInfo
}

// RPCInfo holds one RPC definition.
type RPCInfo struct {
	GoName   string
	Req      string
	Resp     string
	HTTPVerb string
	HTTPPath string
	UseBody  bool
}

// SvcInfo holds a parsed proto service.
type SvcInfo struct {
	Name string
	RPCs []RPCInfo
}

// ParseResult holds everything parsed from a .proto file.
type ParseResult struct {
	Enums    []EnumInfo
	Messages []MsgInfo
	Services []SvcInfo
}

// Parse parses a proto source string (with comments already stripped).
// skipHTTP suppresses HTTP verb/path inference (used for --grpc and --contract modes).
func Parse(src string, skipHTTP bool) ParseResult {
	enums := parseEnums(src)
	enumSet := make(map[string]bool, len(enums))
	for _, e := range enums {
		enumSet[e.Name] = true
	}

	msgs := parseMessages(src, enumSet)
	svcs := parseServices(src, skipHTTP)

	return ParseResult{Enums: enums, Messages: msgs, Services: svcs}
}

// StripComments removes // and /* */ comments from proto source.
func StripComments(src string) string {
	src = regexp.MustCompile(`//[^\n]*`).ReplaceAllString(src, "")
	src = regexp.MustCompile(`/\*[\s\S]*?\*/`).ReplaceAllString(src, "")
	return src
}

// ─── internal parsers ─────────────────────────────────────────────────────────

func parseEnums(src string) []EnumInfo {
	enumBodyRe := regexp.MustCompile(`(?s)enum\s+(\w+)\s*\{([^{}]*)\}`)
	enumValRe := regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]*)\s*=\s*(\d+)`)
	var enums []EnumInfo
	for _, m := range enumBodyRe.FindAllStringSubmatch(src, -1) {
		name, body := m[1], m[2]
		var vals []EnumValInfo
		for _, vm := range enumValRe.FindAllStringSubmatch(body, -1) {
			if strings.EqualFold(vm[1], "option") {
				continue
			}
			vals = append(vals, EnumValInfo{
				GoName: enumConstName(name, vm[1]),
				Number: vm[2],
			})
		}
		if len(vals) > 0 {
			enums = append(enums, EnumInfo{Name: name, Values: vals})
		}
	}
	return enums
}

func parseMessages(src string, enumSet map[string]bool) []MsgInfo {
	msgHeaderRe := regexp.MustCompile(`message\s+(\w+)\s*\{`)
	fieldRe := regexp.MustCompile(
		`(?m)^\s*(?:optional\s+)?(repeated\s+)?` +
			`(?:map\s*<\s*(\w+)\s*,\s*(\w+)\s*>\s*|(\w+)\s+)` +
			`(\w+)\s*=\s*\d+`)
	var msgs []MsgInfo
	for _, loc := range msgHeaderRe.FindAllStringSubmatchIndex(src, -1) {
		name := src[loc[2]:loc[3]]
		braceOpen := loc[1] - 1
		body, ok := extractBracedBlock(src, braceOpen)
		if !ok {
			continue
		}
		var fields []FieldInfo
		for _, fm := range fieldRe.FindAllStringSubmatch(body, -1) {
			repeated := strings.TrimSpace(fm[1]) == "repeated"
			mapKey, mapVal := fm[2], fm[3]
			fieldType, fieldName := fm[4], fm[5]
			switch fieldType {
			case "option", "reserved", "oneof", "extensions", "":
				continue
			}
			goType := GoType(fieldType, enumSet, repeated, mapKey, mapVal)
			fields = append(fields, FieldInfo{
				OrigName: fieldName,
				GoName:   SnakeToPascal(fieldName),
				GoType:   goType,
				JSONTag:  fieldName,
			})
		}
		msgs = append(msgs, MsgInfo{Name: name, Fields: fields})
	}
	return msgs
}

func parseServices(src string, skipHTTP bool) []SvcInfo {
	svcHeaderRe := regexp.MustCompile(`service\s+(\w+)\s*\{`)
	rpcRe := regexp.MustCompile(`rpc\s+(\w+)\s*\(\s*(\w+)\s*\)\s+returns\s+\(\s*(\w+)\s*\)([^;]*)`)
	var svcs []SvcInfo
	for _, loc := range svcHeaderRe.FindAllStringSubmatchIndex(src, -1) {
		svcName := src[loc[2]:loc[3]]
		braceOpen := loc[1] - 1
		svcBody, ok := extractBracedBlock(src, braceOpen)
		if !ok {
			continue
		}
		var rpcs []RPCInfo
		for _, rm := range rpcRe.FindAllStringSubmatch(svcBody, -1) {
			rpcName, reqType, respType, rpcTail := rm[1], rm[2], rm[3], rm[4]
			rpc := RPCInfo{GoName: rpcName, Req: reqType, Resp: respType}
			if !skipHTTP {
				verb, path := extractHTTPOption(rpcTail)
				if verb != "" {
					rpc.HTTPVerb = verb
					rpc.HTTPPath = path
					rpc.UseBody = verb == "POST" || verb == "PUT" || verb == "PATCH"
				} else {
					verb = InferHTTPVerb(rpcName)
					rpc.HTTPVerb = verb
					rpc.HTTPPath = "/" + CamelToKebab(rpcName)
					rpc.UseBody = verb == "POST" || verb == "PUT"
				}
			}
			rpcs = append(rpcs, rpc)
		}
		if len(rpcs) > 0 {
			svcs = append(svcs, SvcInfo{Name: svcName, RPCs: rpcs})
		}
	}
	return svcs
}

// extractBracedBlock returns the content between the balanced { } starting at
// src[start] (which must be '{'). Returns ("", false) if braces are unbalanced.
func extractBracedBlock(src string, start int) (string, bool) {
	if start >= len(src) || src[start] != '{' {
		return "", false
	}
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start+1 : i], true
			}
		}
	}
	return "", false
}

// extractHTTPOption parses a google.api.http option block from the text after
// the rpc signature (before the semicolon or closing brace).
// Returns ("", "") if no option is present.
func extractHTTPOption(s string) (verb, path string) {
	// Locate the opening brace of the option body.
	optHeaderRe := regexp.MustCompile(`(?s)option\s+\(google\.api\.http\)\s*=\s*\{`)
	loc := optHeaderRe.FindStringIndex(s)
	if loc == nil {
		return "", ""
	}
	// loc[1]-1 is the '{' that opens the option body.
	body, ok := extractBracedBlock(s, loc[1]-1)
	if !ok {
		return "", ""
	}
	for _, pair := range []struct{ method, goVerb string }{
		{"get", "GET"}, {"post", "POST"}, {"put", "PUT"},
		{"delete", "DELETE"}, {"patch", "PATCH"},
	} {
		re := regexp.MustCompile(`(?m)\b` + pair.method + `\s*:\s*"([^"]+)"`)
		if pm := re.FindStringSubmatch(body); pm != nil {
			return pair.goVerb, pm[1]
		}
	}
	return "", ""
}

func enumConstName(enumName, valueName string) string {
	parts := strings.Split(strings.ToLower(valueName), "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	pascal := strings.Join(parts, "")
	if strings.HasPrefix(pascal, enumName) {
		return pascal
	}
	return enumName + pascal
}

// GoType converts a proto field's type to a Go type string.
func GoType(fieldType string, enumSet map[string]bool, repeated bool, mapKey, mapVal string) string {
	if mapKey != "" {
		gk := scalarTypes[mapKey]
		if gk == "" {
			gk = mapKey
		}
		gv := scalarTypes[mapVal]
		if gv == "" {
			gv = "*" + mapVal
		}
		return fmt.Sprintf("map[%s]%s", gk, gv)
	}
	goBase, isScalar := scalarTypes[fieldType]
	isEnum := enumSet[fieldType]
	if !isScalar {
		goBase = fieldType
	}
	if repeated {
		if !isScalar && !isEnum {
			return "[]*" + goBase
		}
		return "[]" + goBase
	}
	if !isScalar && !isEnum {
		return "*" + goBase
	}
	return goBase
}

var scalarTypes = map[string]string{
	"double": "float64", "float": "float32",
	"int32": "int32", "int64": "int64",
	"uint32": "uint32", "uint64": "uint64",
	"sint32": "int32", "sint64": "int64",
	"fixed32": "uint32", "fixed64": "uint64",
	"sfixed32": "int32", "sfixed64": "int64",
	"bool": "bool", "string": "string", "bytes": "[]byte",
}

var goAbbrevs = map[string]string{
	"id": "ID", "url": "URL", "uri": "URI", "api": "API",
	"http": "HTTP", "json": "JSON", "xml": "XML", "sql": "SQL",
	"db": "DB", "ip": "IP", "cpu": "CPU", "io": "IO", "uid": "UID",
}

// SnakeToPascal converts snake_case to PascalCase with Go abbreviation conventions.
func SnakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		lower := strings.ToLower(p)
		if abbrev, ok := goAbbrevs[lower]; ok {
			parts[i] = abbrev
		} else if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, "")
}

// CamelToKebab converts PascalCase/camelCase to kebab-case.
func CamelToKebab(s string) string {
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && ch >= 'A' && ch <= 'Z' {
			b.WriteByte('-')
		}
		if ch >= 'A' && ch <= 'Z' {
			b.WriteByte(byte(ch + 32))
		} else {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// InferHTTPVerb infers the HTTP method from the RPC name prefix.
func InferHTTPVerb(name string) string {
	lower := strings.ToLower(name)
	for _, p := range []string{"get", "fetch", "find", "read", "list", "search", "query"} {
		if strings.HasPrefix(lower, p) {
			return "GET"
		}
	}
	for _, p := range []string{"delete", "remove", "purge"} {
		if strings.HasPrefix(lower, p) {
			return "DELETE"
		}
	}
	for _, p := range []string{"update", "modify", "edit", "patch", "set", "put"} {
		if strings.HasPrefix(lower, p) {
			return "PUT"
		}
	}
	return "POST"
}
