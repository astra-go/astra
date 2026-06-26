// Package validate provides reusable input-validation utilities for Chinese and
// international user data: phone numbers, email addresses, passwords, ID
// (identity) card numbers, and real names.
//
// Quick start:
//
//	import "github.com/astra-go/astra/validate"
//
//	if !validate.IsPhone("13812345678") {
//	    return errors.New("invalid phone")
//	}
//	if !validate.IsStrongPassword("Abc12345") {
//	    return errors.New("weak password")
//	}
package validate

import (
	"regexp"
	"strings"
)

// compile once, use everywhere.
var (
	// China mobile: starts with 1, second digit 3-9, 11 digits total.
	phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

	// Minimal email: non-empty local part + @ + non-empty domain.
	emailRe = regexp.MustCompile(`^[^@]+@[^@]+\.[^@]+$`)

	// China ID card: 18 digits, last char may be X/x.
	idCardRe = regexp.MustCompile(`^\d{17}[\dXx]$`)

	// Base password: 6–32 printable ASCII characters.
	passwordLenRe = regexp.MustCompile(`^.{6,32}$`)

	// Real name characters: Chinese, Latin letters, middle dot.
	nameRe = regexp.MustCompile(`^[\p{Han}a-zA-Z·]{2,20}$`)
)

// IsPhone reports whether s is a valid China mainland mobile number
// (11 digits starting with 1[3-9]).
func IsPhone(s string) bool {
	return phoneRe.MatchString(s)
}

// IsEmail reports whether s has a basic email format (x@y.z).
func IsEmail(s string) bool {
	return emailRe.MatchString(s)
}

// IsIDCard reports whether s is a valid 18-digit China identity card number.
func IsIDCard(s string) bool {
	return idCardRe.MatchString(s)
}

// IsStrongPassword reports whether s is 6–32 chars and contains at least one
// letter and one digit.
func IsStrongPassword(s string) bool {
	if !passwordLenRe.MatchString(s) {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

// IsRealName reports whether s is 2–20 chars containing only Chinese
// characters, Latin letters, and the middle dot (·).
func IsRealName(s string) bool {
	return nameRe.MatchString(s)
}

// IsInRange reports whether len(s) is within [minLen, maxLen] inclusive.
func IsInRange(s string, minLen, maxLen int) bool {
	l := len(s)
	return l >= minLen && l <= maxLen
}

// ClassifyContact returns whether input is a valid phone and/or email.
//
//	ClassifyContact("13812345678") → (true, true, false)  // is valid, is phone
//	ClassifyContact("a@b.com")     → (true, false, true)   // is valid, is email
//	ClassifyContact("invalid")     → (false, false, false)
func ClassifyContact(input string) (valid, phone, email bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return false, false, false
	}
	if IsPhone(input) {
		return true, true, false
	}
	if IsEmail(input) {
		return true, false, true
	}
	return false, false, false
}
