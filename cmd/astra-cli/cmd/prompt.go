package cmd

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// PromptString asks for a text value from stdin.
func PromptString(label, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal, nil
	}
	return line, nil
}

// PromptSelect presents numbered options and returns the chosen string.
func PromptSelect(label string, options []string, defaultVal string) (string, error) {
	fmt.Printf("%s:\n", label)
	for i, opt := range options {
		mark := "  "
		if opt == defaultVal {
			mark = "→ "
		}
		fmt.Printf("  %d. %s%s\n", i+1, mark, opt)
	}

	for {
		fmt.Printf("Select (1-%d) [default %s]: ", len(options), defaultVal)
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return defaultVal, nil
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return defaultVal, nil
		}
		// Accept number or string
		if num, err := parseInt(line); err == nil && num >= 1 && num <= len(options) {
			return options[num-1], nil
		}
		// Accept direct option string
		for _, opt := range options {
			if strings.EqualFold(opt, line) {
				return opt, nil
			}
		}
		fmt.Printf("  Invalid selection. Please enter a number (1-%d) or a valid option.\n", len(options))
	}
}

// PromptConfirm asks for a yes/no answer.
func PromptConfirm(label string, defaultVal bool) (bool, error) {
	defaultStr := "no"
	if defaultVal {
		defaultStr = "yes"
	}
	for {
		fmt.Printf("%s [%s/%s]: ", label, strings.ToUpper(defaultStr[:1])+"es", strings.ToUpper(defaultStr[:1])+"o")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return defaultVal, nil
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			return defaultVal, nil
		}
		switch line {
		case "y", "yes", "true", "1":
			return true, nil
		case "n", "no", "false", "0":
			return false, nil
		default:
			fmt.Printf("  Please answer yes or no.\n")
		}
	}
}

var intRe = regexp.MustCompile(`^\d+$`)

func parseInt(s string) (int, error) {
	if !intRe.MatchString(s) {
		return 0, fmt.Errorf("not a number")
	}
	var n int
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n, nil
}
