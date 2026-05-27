//go:build mage

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MergeCover merges multiple Go coverprofile files into one.
// Set COVER_OUT to the output file path and COVER_IN to a space-separated list
// of input files (glob patterns are supported).
//
// Example:
//
//	COVER_OUT=coverage/merged.out COVER_IN="coverage/a.out coverage/b.out" mage mergeCover
func MergeCover() error {
	output := os.Getenv("COVER_OUT")
	if output == "" {
		return fmt.Errorf("COVER_OUT env var is required")
	}
	inStr := os.Getenv("COVER_IN")
	if inStr == "" {
		return fmt.Errorf("COVER_IN env var is required")
	}

	var inputs []string
	for _, pattern := range strings.Fields(inStr) {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			inputs = append(inputs, pattern)
		} else {
			inputs = append(inputs, matches...)
		}
	}
	if len(inputs) == 0 {
		return fmt.Errorf("no input files found from COVER_IN=%q", inStr)
	}
	return mergeCoverFiles(output, inputs)
}

func mergeCoverFiles(output string, inputs []string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	out, err := os.Create(output)
	if err != nil {
		return err
	}
	defer out.Close()

	totalBytes := int64(0)
	for i, input := range inputs {
		f, err := os.Open(input)
		if err != nil {
			return fmt.Errorf("open %s: %w", input, err)
		}
		if i == 0 {
			n, err := io.Copy(out, f)
			f.Close()
			if err != nil {
				return err
			}
			totalBytes += n
		} else {
			// skip the "mode: ..." header line
			oneByte := make([]byte, 1)
			for {
				_, err := f.Read(oneByte)
				if err != nil || oneByte[0] == '\n' {
					break
				}
			}
			n, err := io.Copy(out, f)
			f.Close()
			if err != nil {
				return err
			}
			totalBytes += n
		}
	}

	// rough line count estimate
	lines := int(totalBytes/60) + 1
	fmt.Printf("merged %d coverprofile(s) → %s  (%d coverage lines)\n", len(inputs), output, lines)
	return nil
}
