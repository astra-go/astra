package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
)

// WriteTemplate writes tpl rendered with data to dir/filename (or filename if dir is empty).
// Returns a CLIError if the file already exists and force is false, or if any I/O fails.
func WriteTemplate(dir, filename string, tpl *template.Template, data any, force bool) error {
	path, err := resolvePath(dir, filename)
	if err != nil {
		return err
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return &cli.CLIError{
				Msg:     fmt.Sprintf("file already exists: %s", path),
				Hint:    "use --force to overwrite",
				Example: os.Args[0] + " " + joinArgs(os.Args[1:]) + " --force",
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return &cli.CLIError{
			Msg:  fmt.Sprintf("create %s: %v", path, err),
			Hint: "check that the output directory exists and you have write permission",
		}
	}
	defer f.Close()
	if err := tpl.Execute(f, data); err != nil {
		return &cli.CLIError{Msg: fmt.Sprintf("render %s: %v", path, err)}
	}
	return nil
}

// WriteString writes raw src to dir/filename (or filename if dir is empty).
func WriteString(dir, filename, src string, force bool) error {
	path, err := resolvePath(dir, filename)
	if err != nil {
		return err
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return &cli.CLIError{
				Msg:     fmt.Sprintf("file already exists: %s", path),
				Hint:    "use --force to overwrite",
				Example: os.Args[0] + " " + joinArgs(os.Args[1:]) + " --force",
			}
		}
	}
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		return &cli.CLIError{
			Msg:  fmt.Sprintf("write %s: %v", path, err),
			Hint: "check that you have write permission to the output directory",
		}
	}
	return nil
}

func resolvePath(dir, filename string) (string, error) {
	if dir == "" {
		return filename, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", dir, err)}
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", &cli.CLIError{Msg: fmt.Sprintf("resolve dir %s: %v", dir, err)}
	}
	full := filepath.Join(absDir, filename)
	// Guard against filename containing ".." that escapes the target directory.
	if !strings.HasPrefix(full, absDir+string(filepath.Separator)) {
		return "", &cli.CLIError{
			Msg:  fmt.Sprintf("output path %q escapes target directory %q", full, absDir),
			Hint: "ensure --dir and the file name do not contain '..'",
		}
	}
	return full, nil
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}
