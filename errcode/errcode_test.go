package errcode

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// ─── I18n key derivation ──────────────────────────────────────────────────────

func TestI18nKey_ThreeParts(t *testing.T) {
	got := I18nKey("USC-AUTH-1001")
	want := "error.usc.auth.1001"
	if got != want {
		t.Errorf("I18nKey(%q) = %q, want %q", "USC-AUTH-1001", got, want)
	}
}

func TestI18nKey_TwoParts(t *testing.T) {
	got := I18nKey("INT-5000")
	want := "error.int.5000"
	if got != want {
		t.Errorf("I18nKey(%q) = %q, want %q", "INT-5000", got, want)
	}
}

func TestI18nKey_UnknownFormat(t *testing.T) {
	got := I18nKey("invalid")
	if !strings.Contains(got, "error.internal_error") {
		t.Errorf("I18nKey(%q) = %q, want containing %q", "invalid", got, "error.internal_error")
	}
}

// ─── Category from code ───────────────────────────────────────────────────────

func TestCategoryFromCode_ThreeParts(t *testing.T) {
	if got := CategoryFromCode("USC-AUTH-1001"); got != "AUTH" {
		t.Errorf("CategoryFromCode = %q, want AUTH", got)
	}
}

func TestCategoryFromCode_TwoParts(t *testing.T) {
	if got := CategoryFromCode("INT-5000"); got != "INT" {
		t.Errorf("CategoryFromCode = %q, want INT", got)
	}
}

func TestCategoryFromCode_SinglePart(t *testing.T) {
	if got := CategoryFromCode("FOO"); got != "INT" {
		t.Errorf("CategoryFromCode = %q, want INT (fallback)", got)
	}
}

// ─── Code parts extraction ────────────────────────────────────────────────────

func TestServicePrefixFromCode(t *testing.T) {
	if got := ServicePrefixFromCode("USC-AUTH-1001"); got != "USC" {
		t.Errorf("ServicePrefixFromCode = %q, want USC", got)
	}
}

func TestNumberFromCode(t *testing.T) {
	if got := NumberFromCode("USC-AUTH-1001"); got != "1001" {
		t.Errorf("NumberFromCode = %q, want 1001", got)
	}
}

// ─── Define + registry ───────────────────────────────────────────────────────

func TestDefineAndRegistry(t *testing.T) {
	Reset()
	defer Reset()

	err := Define("USC-NOTF-2001", "usercenter-svc", "Account not found")
	if err == nil {
		t.Fatal("Define returned nil")
	}
	if err.Code != "USC-NOTF-2001" {
		t.Errorf("err.Code = %q, want USC-NOTF-2001", err.Code)
	}
	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("err.HTTPStatus = %d, want 404", err.HTTPStatus)
	}
	if err.Message != "Account not found" {
		t.Errorf("err.Message = %q, want Account not found", err.Message)
	}

	// Check registry
	desc := Lookup("USC-NOTF-2001")
	if desc == nil {
		t.Fatal("Lookup returned nil after Define")
	}
	if desc.Service != "usercenter-svc" {
		t.Errorf("desc.Service = %q, want usercenter-svc", desc.Service)
	}
	if desc.Category != "NOTF" {
		t.Errorf("desc.Category = %q, want NOTF", desc.Category)
	}
	if desc.I18nKey != "error.usc.notf.2001" {
		t.Errorf("desc.I18nKey = %q, want error.usc.notf.2001", desc.I18nKey)
	}
}

func TestDefineNoRegister(t *testing.T) {
	Reset()
	defer Reset()

	err := DefineNoRegister("USC-AUTH-1001", "Token expired")
	if err == nil {
		t.Fatal("DefineNoRegister returned nil")
	}
	if err.Code != "USC-AUTH-1001" {
		t.Errorf("err.Code = %q", err.Code)
	}
	if err.HTTPStatus != http.StatusUnauthorized {
		t.Errorf("err.HTTPStatus = %d, want 401", err.HTTPStatus)
	}

	// Should NOT be in registry
	if desc := Lookup("USC-AUTH-1001"); desc != nil {
		t.Error("DefineNoRegister should not register the error")
	}
}

func TestDefine_AutoHTTPStatus(t *testing.T) {
	tests := []struct {
		code   string
		status int
	}{
		{"USC-AUTH-1001", http.StatusUnauthorized},
		{"ORD-VAL-2001", http.StatusBadRequest},
		{"USC-NOTF-2001", http.StatusNotFound},
		{"USC-CONF-2003", http.StatusConflict},
		{"USC-PERM-4001", http.StatusForbidden},
		{"USC-RATE-5001", http.StatusTooManyRequests},
		{"USC-INT-9001", http.StatusInternalServerError},
		{"USC-EXT-4001", http.StatusBadGateway},
		{"USC-TIMEOUT-5001", http.StatusGatewayTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			err := DefineNoRegister(tt.code, "test")
			if err.HTTPStatus != tt.status {
				t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, tt.status)
			}
		})
	}
}

// ─── Registry operations ─────────────────────────────────────────────────────

func TestRegisterAndLookup(t *testing.T) {
	Reset()
	defer Reset()

	Register("USC-AUTH-1001", "usercenter-svc", "AUTH", "Token expired", 401)

	if desc := Lookup("USC-AUTH-1001"); desc == nil {
		t.Fatal("Lookup returned nil after Register")
	}
	if desc := Lookup("nonexistent"); desc != nil {
		t.Error("Lookup returned non-nil for nonexistent code")
	}
}

func TestMustRegister_DuplicatePanics(t *testing.T) {
	Reset()
	defer Reset()

	MustRegister("USC-AUTH-1001", "usercenter-svc", "AUTH", "Token expired", 401)

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRegister did not panic on duplicate")
		}
	}()
	MustRegister("USC-AUTH-1001", "usercenter-svc", "AUTH", "Token expired", 401)
}

func TestListAll(t *testing.T) {
	Reset()
	defer Reset()

	Register("ORD-NOTF-2001", "order-svc", "NOTF", "Order not found", 404)
	Register("USC-AUTH-1001", "usercenter-svc", "AUTH", "Token expired", 401)

	all := ListAll()
	if len(all) != 2 {
		t.Fatalf("ListAll returned %d items, want 2", len(all))
	}
	// Should be sorted: ORD < USC
	if all[0].Code != "ORD-NOTF-2001" {
		t.Errorf("First code = %q, want ORD-NOTF-2001", all[0].Code)
	}
}

func TestListByService(t *testing.T) {
	Reset()
	defer Reset()

	Register("USC-AUTH-1001", "usercenter-svc", "AUTH", "Token expired", 401)
	Register("USC-NOTF-2001", "usercenter-svc", "NOTF", "Account not found", 404)
	Register("ORD-NOTF-2001", "order-svc", "NOTF", "Order not found", 404)

	items := ListByService("usercenter-svc")
	if len(items) != 2 {
		t.Fatalf("ListByService returned %d, want 2", len(items))
	}
}

func TestListByCategory(t *testing.T) {
	Reset()
	defer Reset()

	Register("USC-AUTH-1001", "usercenter-svc", "AUTH", "Token expired", 401)
	Register("USC-NOTF-2001", "usercenter-svc", "NOTF", "Not found", 404)
	Register("ORD-NOTF-2001", "order-svc", "NOTF", "Not found 2", 404)

	items := ListByCategory("NOTF")
	if len(items) != 2 {
		t.Fatalf("ListByCategory returned %d, want 2", len(items))
	}
}

func TestCount(t *testing.T) {
	Reset()
	defer Reset()

	if c := Count(); c != 0 {
		t.Errorf("Count = %d, want 0", c)
	}
	Register("USC-AUTH-1001", "usercenter-svc", "AUTH", "", 401)
	if c := Count(); c != 1 {
		t.Errorf("Count = %d, want 1", c)
	}
}

func TestReset(t *testing.T) {
	Reset()
	Register("USC-AUTH-1001", "", "", "", 401)
	Reset()
	if c := Count(); c != 0 {
		t.Errorf("Count = %d after Reset, want 0", c)
	}
}

func TestMarkdownTable(t *testing.T) {
	Reset()
	defer Reset()

	Register("USC-AUTH-1001", "usercenter-svc", "AUTH", "Token expired", 401)
	table := MarkdownTable()
	if !strings.Contains(table, "USC-AUTH-1001") {
		t.Error("MarkdownTable missing USC-AUTH-1001")
	}
	if !strings.Contains(table, "usercenter-svc") {
		t.Error("MarkdownTable missing usercenter-svc")
	}
}

// ─── Common wrappers ──────────────────────────────────────────────────────────

func TestWrapDBError(t *testing.T) {
	original := fmt.Errorf("connection refused")
	err := WrapDBError(original, "FindUser")

	if err.Code != "INT-DB-0001" {
		t.Errorf("WrapDBError.Code = %q, want INT-DB-0001", err.Code)
	}
	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("WrapDBError.HTTPStatus = %d, want 500", err.HTTPStatus)
	}
	if err.Message != "database operation failed" {
		t.Errorf("WrapDBError.Message = %q", err.Message)
	}
	if err.Unwrap() != original {
		t.Error("WrapDBError.Unwrap() should return the original error")
	}
	if err.Details == nil {
		t.Fatal("WrapDBError.Details is nil")
	}
	if err.Details["operation"] != "FindUser" {
		t.Errorf("WrapDBError operation detail = %v", err.Details["operation"])
	}
}

func TestWrapCacheError(t *testing.T) {
	original := fmt.Errorf("timeout")
	err := WrapCacheError(original, "GetToken")

	if err.Code != "INT-CACHE-0001" {
		t.Errorf("WrapCacheError.Code = %q, want INT-CACHE-0001", err.Code)
	}
	if err.Unwrap() != original {
		t.Error("WrapCacheError.Unwrap() should return the original error")
	}
	if err.Details["operation"] != "GetToken" {
		t.Errorf("WrapCacheError operation detail = %v", err.Details["operation"])
	}
}

func TestWrapExternalError(t *testing.T) {
	original := fmt.Errorf("API returned 503")
	err := WrapExternalError("wechat", original)

	if err.Code != "EXT-0001" {
		t.Errorf("WrapExternalError.Code = %q, want EXT-0001", err.Code)
	}
	if err.HTTPStatus != http.StatusBadGateway {
		t.Errorf("WrapExternalError.HTTPStatus = %d, want 502", err.HTTPStatus)
	}
	if err.Unwrap() != original {
		t.Error("WrapExternalError.Unwrap() should return the original error")
	}
	if err.Details["platform"] != "wechat" {
		t.Errorf("WrapExternalError platform detail = %v", err.Details["platform"])
	}
}

// ─── Category constants ──────────────────────────────────────────────────────

func TestCategoryConstants(t *testing.T) {
	if CatAuth != "AUTH" {
		t.Error("CatAuth mismatch")
	}
	if CatValid != "VAL" {
		t.Error("CatValid mismatch")
	}
	if CatNotF != "NOTF" {
		t.Error("CatNotF mismatch")
	}
	if CatConf != "CONF" {
		t.Error("CatConf mismatch")
	}
	if CatPerm != "PERM" {
		t.Error("CatPerm mismatch")
	}
	if CatRate != "RATE" {
		t.Error("CatRate mismatch")
	}
	if CatInt != "INT" {
		t.Error("CatInt mismatch")
	}
	if CatExt != "EXT" {
		t.Error("CatExt mismatch")
	}
	if CatTimeout != "TIMEOUT" {
		t.Error("CatTimeout mismatch")
	}
}
