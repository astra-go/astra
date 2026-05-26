package security

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSecretString_Redaction(t *testing.T) {
	s := NewSecretString("super-secret")

	if got := s.String(); got != "[REDACTED]" {
		t.Errorf("String() = %q, want [REDACTED]", got)
	}
	if got := fmt.Sprintf("%v", s); got != "[REDACTED]" {
		t.Errorf("%%v = %q, want [REDACTED]", got)
	}
	if got := fmt.Sprintf("%s", s); got != "[REDACTED]" {
		t.Errorf("%%s = %q, want [REDACTED]", got)
	}
}

func TestSecretString_MarshalJSON(t *testing.T) {
	s := NewSecretString("super-secret")
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"[REDACTED]"` {
		t.Errorf("json.Marshal = %s, want \"[REDACTED]\"", b)
	}
}

func TestSecretString_MarshalText(t *testing.T) {
	s := NewSecretString("super-secret")
	b, err := s.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[REDACTED]" {
		t.Errorf("MarshalText = %s, want [REDACTED]", b)
	}
}

func TestSecretString_MarshalBinary(t *testing.T) {
	s := NewSecretString("super-secret")
	b, err := s.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte("[REDACTED]")) {
		t.Errorf("MarshalBinary = %s, want [REDACTED]", b)
	}
}

func TestSecretString_UnmarshalBinary_Forbidden(t *testing.T) {
	var s SecretString
	if err := s.UnmarshalBinary([]byte("anything")); err == nil {
		t.Error("UnmarshalBinary should return an error, got nil")
	}
}

// TestSecretString_GobEncode verifies that gob encoding does not expose the secret.
func TestSecretString_GobEncode(t *testing.T) {
	type wrapper struct {
		S SecretString
	}
	gob.Register(SecretString{})

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(wrapper{S: NewSecretString("super-secret")}); err != nil {
		t.Fatal(err)
	}
	// The encoded bytes must not contain the plaintext secret.
	if bytes.Contains(buf.Bytes(), []byte("super-secret")) {
		t.Error("gob-encoded bytes contain the plaintext secret")
	}
}

func TestSecretString_Plain(t *testing.T) {
	s := NewSecretString("super-secret")
	if got := s.Plain(); got != "super-secret" {
		t.Errorf("Plain() = %q, want super-secret", got)
	}
}

func TestSecretString_IsZero(t *testing.T) {
	if !NewSecretString("").IsZero() {
		t.Error("empty SecretString should be zero")
	}
	if NewSecretString("x").IsZero() {
		t.Error("non-empty SecretString should not be zero")
	}
}
