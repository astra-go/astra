package middleware

import "testing"

func TestMaskSensitive(t *testing.T) {
	tests := []struct {
		input    string
		showLast int
		want     string
	}{
		{"13812345678", 4, "*******5678"},   // 11 chars, show last 4: 7 asterisks
		{"", 4, "***"},
		{"abc", -1, "***"},
		{"abc", 5, "***"},   // showLast >= length → fixed mask
		{"a", 0, "*"},
		{"ab", 0, "**"},
		{"abcdefghij", 0, "**********"},
	}
	for _, tt := range tests {
		got := MaskSensitive(tt.input, tt.showLast)
		if got != tt.want {
			t.Errorf("MaskSensitive(%q, %d) = %q, want %q", tt.input, tt.showLast, got, tt.want)
		}
	}
}

func TestMaskPhone(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"13812345678", "*******5678"}, // 11 digits, show last 4
		{"", "***"},
		{"1234", "****"},
	}
	for _, tt := range tests {
		got := MaskPhone(tt.input)
		if got != tt.want {
			t.Errorf("MaskPhone(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMaskEmail(t *testing.T) {
	// Single @ variant tests
	if got, want := MaskEmail("user@example.com"), "u***@example.com"; got != want {
		t.Errorf("MaskEmail(user@example.com) = %q, want %q", got, want)
	}
	if got, want := MaskEmail("a@b.com"), "a*@b.com"; got != want {
		t.Errorf("MaskEmail(a@b.com) = %q, want %q", got, want)
	}
	if got, want := MaskEmail("@domain.com"), "***@domain.com"; got != want {
		t.Errorf("MaskEmail(@domain.com) = %q, want %q", got, want)
	}
	if got, want := MaskEmail(""), "***"; got != want {
		t.Errorf("MaskEmail(empty) = %q, want %q", got, want)
	}
}

func TestMaskEmailNoAtSign(t *testing.T) {
	got := MaskEmail("noatsign")
	// len("noatsign") = 8 → showLast=2 → 6 asterisks + "gn" = "******gn"
	want := "******gn"
	if got != want {
		t.Errorf("MaskEmail(%q) = %q, want %q", "noatsign", got, want)
	}
}

func TestMaskToken(t *testing.T) {
	if got, want := MaskToken("abc123xyz789"), "********z789"; got != want {
		t.Errorf("MaskToken(abc123xyz789) = %q, want %q", got, want)
	}
	if got, want := MaskToken(""), "***"; got != want {
		t.Errorf("MaskToken(empty) = %q, want %q", got, want)
	}
	if got, want := MaskToken("tok"), "***"; got != want {
		t.Errorf("MaskToken(tok) = %q, want %q", got, want)
	}
	if got, want := MaskToken("token1"), "**ken1"; got != want {
		t.Errorf("MaskToken(token1) = %q, want %q", got, want)
	}
}

func TestMaskTokenSmall(t *testing.T) {
	// "abc" showLast=4, 4 >= 3 → fixed mask of 3 asterisks "***"
	if got, want := MaskToken("abc"), "***"; got != want {
		t.Errorf("MaskToken(%q) = %q, want %q", "abc", got, want)
	}
}

func TestHashSensitive(t *testing.T) {
	h1 := HashSensitive("user@example.com")
	h2 := HashSensitive("user@example.com")
	if h1 != h2 {
		t.Errorf("HashSensitive not deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 16 {
		t.Errorf("HashSensitive length = %d, want 16", len(h1))
	}

	h3 := HashSensitive("other@example.com")
	if h1 == h3 {
		t.Errorf("HashSensitive collision on different inputs")
	}

	if h := HashSensitive(""); h != "empty" {
		t.Errorf("HashSensitive(%q) = %q, want 'empty'", "", h)
	}
}

func TestSanitizeLogField(t *testing.T) {
	if got, want := SanitizeLogField("phone", "13812345678"), "*******5678"; got != want {
		t.Errorf("phone = %q, want %q", got, want)
	}
	if got, want := SanitizeLogField("mobile", "13912345678"), "*******5678"; got != want {
		t.Errorf("mobile = %q, want %q", got, want)
	}
	if got, want := SanitizeLogField("email", "user@example.com"), "u***@example.com"; got != want {
		t.Errorf("email = %q, want %q", got, want)
	}
	// token 5 chars, show last 4: 1 asterisk + last 4 = "*c12"
	// token 5 chars, show last 4: 1 asterisk + last 4 "bc12" = "*bc12"
	if got, want := SanitizeLogField("token", "abc12"), "*bc12"; got != want {
		t.Errorf("token = %q, want %q", got, want)
	}
	// password 6 chars, show last 4: 2 asterisks + last 4 "cret" = "**cret"
	if got, want := SanitizeLogField("password", "secret"), "**cret"; got != want {
		t.Errorf("password = %q, want %q", got, want)
	}
	if got, want := SanitizeLogField("verification_code", "123456"), "**3456"; got != want {
		t.Errorf("verification_code = %q, want %q", got, want)
	}
	if got, want := SanitizeLogField("name", "John Doe"), "John Doe"; got != want {
		t.Errorf("name = %q, want %q", got, want)
	}
	if got, want := SanitizeLogField("", "value"), "value"; got != want {
		t.Errorf("empty field = %q, want %q", got, want)
	}
}

func TestDefaultSensitiveParams(t *testing.T) {
	if len(DefaultSensitiveParams) == 0 {
		t.Error("DefaultSensitiveParams is empty")
	}
}
