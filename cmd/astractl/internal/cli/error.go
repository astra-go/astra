package cli

import (
	"errors"
	"fmt"
	"os"
)

// CLIError carries a user-facing error message with optional hint and example.
type CLIError struct {
	Msg     string
	Hint    string
	Example string
}

func (e *CLIError) Error() string { return e.Msg }

// Errorf wraps a formatted message as a CLIError with no hint.
func Errorf(format string, args ...any) *CLIError {
	return &CLIError{Msg: fmt.Sprintf(format, args...)}
}

// Render prints a CLIError to stderr in three-section format and returns 1.
// For non-CLIError values it falls back to a plain "error: ..." line.
// The caller is responsible for passing the return value to os.Exit.
func Render(err error) int {
	var ce *CLIError
	if errors.As(err, &ce) {
		fmt.Fprintf(os.Stderr, "[error] %s\n", ce.Msg)
		if ce.Hint != "" {
			fmt.Fprintf(os.Stderr, "  hint:    %s\n", ce.Hint)
		}
		if ce.Example != "" {
			fmt.Fprintf(os.Stderr, "  example: %s\n", ce.Example)
		}
	} else {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	return 1
}
