package astra

import "net/http"

// HTTP method constants mirror net/http for convenience.
// Using these names keeps code self-documenting and avoids raw string literals.
const (
	MethodGET     = http.MethodGet
	MethodPOST    = http.MethodPost
	MethodPUT     = http.MethodPut
	MethodDELETE  = http.MethodDelete
	MethodPATCH   = http.MethodPatch
	MethodHEAD    = http.MethodHead
	MethodOPTIONS = http.MethodOptions
)
