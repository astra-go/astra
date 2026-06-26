package validate

import "testing"

func TestIsPhone(t *testing.T) {
	tests := []struct{ input string; want bool }{
		{"13812345678", true},
		{"15912345678", true},
		{"10012345678", false}, // second digit 0
		{"12345678901", false}, // second digit 2
		{"1381234567", false},  // 10 digits
		{"138123456789", false},// 12 digits
		{"abc", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsPhone(tt.input)
		if got != tt.want {
			t.Errorf("IsPhone(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsEmail(t *testing.T) {
	tests := []struct{ input string; want bool }{
		{"user@example.com", true},
		{"a@b.co", true},
		{"@example.com", false},
		{"user@", false},
		{"user", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsEmail(tt.input)
		if got != tt.want {
			t.Errorf("IsEmail(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsIDCard(t *testing.T) {
	tests := []struct{ input string; want bool }{
		{"110101199001011234", true},
		{"11010119900101123x", true},
		{"11010119900101123X", true},
		{"1234567890123456789", false}, // 19 digits
		{"12345678901234567", false},   // 17 digits
		{"abcdefghijklmnopqr", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsIDCard(tt.input)
		if got != tt.want {
			t.Errorf("IsIDCard(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsStrongPassword(t *testing.T) {
	tests := []struct{ input string; want bool }{
		{"Abc12345", true},
		{"abcdef123", true},
		{"123456a", true},
		{"abcdef", false},     // no digit
		{"123456", false},     // no letter
		{"ab1", false},        // too short
		{"", false},
	}
	for _, tt := range tests {
		got := IsStrongPassword(tt.input)
		if got != tt.want {
			t.Errorf("IsStrongPassword(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsRealName(t *testing.T) {
	tests := []struct{ input string; want bool }{
		{"张三", true},
		{"李四光", true},
		{"John", true},
		{"Jean·Luc", true},
		{"A", false},      // too short
		{"", false},
		{"abc123", false}, // contains digits
	}
	for _, tt := range tests {
		got := IsRealName(tt.input)
		if got != tt.want {
			t.Errorf("IsRealName(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsInRange(t *testing.T) {
	if !IsInRange("hello", 2, 10) {
		t.Error("expected hello to be within [2,10]")
	}
	if IsInRange("hi", 5, 10) {
		t.Error("expected hi to be outside [5,10]")
	}
}

func TestClassifyContact(t *testing.T) {
	valid, phone, email := ClassifyContact("13812345678")
	if !(valid && phone && !email) {
		t.Errorf("phone: got (%v,%v,%v)", valid, phone, email)
	}
	valid, phone, email = ClassifyContact("a@b.com")
	if !(valid && !phone && email) {
		t.Errorf("email: got (%v,%v,%v)", valid, phone, email)
	}
	valid, phone, email = ClassifyContact("invalid")
	if valid || phone || email {
		t.Errorf("invalid: got (%v,%v,%v)", valid, phone, email)
	}
	valid, phone, email = ClassifyContact("")
	if valid || phone || email {
		t.Errorf("empty: got (%v,%v,%v)", valid, phone, email)
	}
}
