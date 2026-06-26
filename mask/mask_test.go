package mask

import "testing"

func TestSensitive_FullMask(t *testing.T) {
	got := Sensitive("hello", -1)
	if want := "*****"; got != want {
		t.Fatalf("Sensitive(hello, -1) = %q, want %q", got, want)
	}
}

func TestSensitive_ShowLast(t *testing.T) {
	got := Sensitive("13812345678", 4)
	if want := "*******5678"; got != want {
		t.Fatalf("Sensitive(13812345678, 4) = %q, want %q", got, want)
	}
}

func TestSensitive_Empty(t *testing.T) {
	got := Sensitive("", 4)
	if want := "[redacted]"; got != want {
		t.Fatalf("Sensitive(\"\", 4) = %q, want %q", got, want)
	}
}

func TestSensitive_FullMaskMax8(t *testing.T) {
	// length=12 but capped at 8 when fully masked
	got := Sensitive("toolongstring", -1)
	if want := "********"; got != want {
		t.Fatalf("Sensitive(toolongstring, -1) = %q, want %q", got, want)
	}
}

func TestPhone(t *testing.T) {
	tests := []struct{ input, want string }{
		{"13812345678", "*******5678"},
		{"", "[redacted]"},
		{"1234", "****"}, // showLast>=len → full mask
	}
	for _, tt := range tests {
		got := Phone(tt.input)
		if got != tt.want {
			t.Errorf("Phone(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestToken(t *testing.T) {
	tests := []struct{ input, want string }{
		{"abc123xyz789", "********z789"},
		{"tok", "***"}, // showLast>=len → full mask
		{"", "[redacted]"},
	}
	for _, tt := range tests {
		got := Token(tt.input)
		if got != tt.want {
			t.Errorf("Token(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEmail(t *testing.T) {
	tests := []struct{ input, want string }{
		{"user@example.com", "u***@example.com"},
		{"ab@example.com", "a*@example.com"},
		{"@example.com", "[redacted]@example.com"},
		{"notanemail", "********il"},
		{"", "[redacted]"},
	}
	for _, tt := range tests {
		got := Email(tt.input)
		if got != tt.want {
			t.Errorf("Email(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHash(t *testing.T) {
	h1 := Hash("secret123")
	h2 := Hash("secret123")
	if h1 != h2 {
		t.Fatalf("Hash not deterministic: %q vs %q", h1, h2)
	}
	if len(h1) != 16 {
		t.Fatalf("Hash length = %d, want 16", len(h1))
	}

	empty := Hash("")
	if empty != "[empty]" {
		t.Fatalf("Hash(\"\") = %q, want \"[empty]\"", empty)
	}
}

func TestByFieldName(t *testing.T) {
	tests := []struct{ field, value, want string }{
		{"phone", "13812345678", "*******5678"},
		{"mobile", "13998765432", "*******5432"},
		{"email", "hello@abc.com", "h****@abc.com"},
		{"token", "abc123def456", "********f456"},
		{"password", "myP@ss1", "***@ss1"},
		{"code", "123456", "**3456"},
		{"name", "张三", "张三"},
	}
	for _, tt := range tests {
		got := ByFieldName(tt.field, tt.value)
		if got != tt.want {
			t.Errorf("ByFieldName(%q, %q) = %q, want %q", tt.field, tt.value, got, tt.want)
		}
	}
}
