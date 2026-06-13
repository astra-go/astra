package wire

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
	"github.com/astra-go/astra/cmd/astractl/internal/tmpl"
	"github.com/astra-go/astra/cmd/astractl/internal/tpldata"
)

// Run implements the gen wire subcommand.
func Run(args []string) error {
	fs := flag.NewFlagSet("gen wire", flag.ContinueOnError)
	dir           := fs.String("dir",           ".", "directory to scan and write output into")
	pkg           := fs.String("pkg",           "",  "Go package name (default: inferred from scanned files)")
	scan          := fs.Bool("scan",           false, "scan di.Provide* calls and generate di_gen.go")
	providerFuncs := fs.String("provider-funcs", "", "comma-separated list of provider function names to recognise (default: di.Provide,di.ProvideNamed,di.ProvideConstructor,di.ProvideValue)")
	recursive     := fs.Bool("recursive",      false, "scan subdirectories recursively")
	force         := fs.Bool("force",          false, "overwrite existing file")
	exportFunc    := fs.String("export-func",  "",    "when set, generate an exported func <name>(c *di.Container) instead of initDI (use for subpackages)")
	aggregate     := fs.Bool("aggregate",      false, "scan each immediate subdirectory independently and generate a root aggregator initDI")
	if err := fs.Parse(args); err != nil {
		return &cli.CLIError{
			Msg:     "invalid flags: " + err.Error(),
			Example: "astractl gen wire --scan --dir ./cmd/server",
		}
	}

	if !*scan {
		if err := fsutil.WriteTemplate(*dir, "wire.go", tmpl.WireProvider(), tpldata.Data{}, *force); err != nil {
			return err
		}
		fmt.Printf("Wire provider generated: %s/wire.go\n", *dir)
		fmt.Println("  Run 'wire gen .' to generate wire_gen.go")
		fmt.Println("  Requires: go install github.com/google/wire/cmd/wire@latest")
		return nil
	}

	var customFuncs []string
	if *providerFuncs != "" {
		for _, f := range strings.Split(*providerFuncs, ",") {
			if f = strings.TrimSpace(f); f != "" {
				customFuncs = append(customFuncs, f)
			}
		}
	}

	if *aggregate {
		return runAggregate(*dir, *pkg, customFuncs, *force)
	}
	return runSingle(*dir, *pkg, customFuncs, *recursive, *exportFunc, *force)
}

// runSingle handles the standard single-directory scan mode.
func runSingle(dir, pkg string, customFuncs []string, recursive bool, exportFunc string, force bool) error {
	result, err := ScanPackage(dir, customFuncs, recursive)
	if err != nil {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("scan %s: %v", dir, err),
			Hint:    "ensure the directory exists and contains valid Go source files with di.Provide* calls",
			Example: "astractl gen wire --scan --dir ./cmd/server",
		}
	}

	pkgName := result.PkgName
	if pkg != "" {
		pkgName = pkg
	}
	if pkgName == "" {
		pkgName = "main"
	}

	if len(result.Providers) == 0 {
		fmt.Printf("No di.Provide* calls found in %s — generating empty di_gen.go scaffold.\n", dir)
	}

	if cycle := DetectCycle(result.Providers); cycle != nil {
		return &cli.CLIError{
			Msg:  "dependency cycle detected: " + strings.Join(cycle, " → "),
			Hint: "remove one of the cyclic dependencies; circular DI graphs cannot be resolved at runtime",
		}
	}

	ordered := TopoSort(result.Providers)
	out := BuildDIGen(pkgName, ordered, result.Providers, result.SetupFuncs, exportFunc)
	if err := fsutil.WriteString(dir, "di_gen.go", out, force); err != nil {
		return err
	}

	printPath := filepath.Join(dir, "di_gen.go")
	fmt.Printf("DI wiring generated: %s\n", printPath)
	fmt.Printf("  package %s\n", pkgName)
	fmt.Printf("  %d provider(s) across %d setup function(s)\n", len(result.Providers), len(result.SetupFuncs))
	if exportFunc != "" {
		fmt.Printf("  Call %s.%s(c) from your root aggregator\n", pkgName, exportFunc)
	} else {
		fmt.Println("  Call initDI(app) in main() before app.Run()")
	}
	return nil
}

// runAggregate scans each immediate non-hidden subdirectory of dir, generates a
// per-subpackage di_gen.go with an exported RegisterDI function, then writes a
// root di_gen.go in dir that delegates to all of them.
func runAggregate(dir, pkg string, customFuncs []string, force bool) error {
	modPath, modRoot, err := readModulePath(dir)
	if err != nil {
		return &cli.CLIError{
			Msg:  fmt.Sprintf("read module path from %s: %v", dir, err),
			Hint: "ensure a go.mod file exists in or above the target directory",
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return &cli.CLIError{Msg: fmt.Sprintf("read dir %s: %v", dir, err)}
	}

	const regFunc = "RegisterDI"
	var subPkgs []SubPkg

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		subDir := filepath.Join(dir, e.Name())

		result, err := ScanPackage(subDir, customFuncs, false)
		if err != nil {
			return &cli.CLIError{
				Msg:  fmt.Sprintf("scan %s: %v", subDir, err),
				Hint: "fix syntax errors in the subdirectory before running --aggregate",
			}
		}
		if result.PkgName == "" {
			continue // no Go files
		}

		if cycle := DetectCycle(result.Providers); cycle != nil {
			return &cli.CLIError{
				Msg:  fmt.Sprintf("dependency cycle in %s: %s", subDir, strings.Join(cycle, " → ")),
				Hint: "remove the cycle before running --aggregate",
			}
		}

		ordered := TopoSort(result.Providers)
		out := BuildDIGen(result.PkgName, ordered, result.Providers, result.SetupFuncs, regFunc)
		if err := fsutil.WriteString(subDir, "di_gen.go", out, force); err != nil {
			return err
		}
		fmt.Printf("  subpackage: %s/di_gen.go (%d provider(s))\n", subDir, len(result.Providers))

		rel, _ := filepath.Rel(modRoot, subDir)
		subPkgs = append(subPkgs, SubPkg{
			Alias:      result.PkgName,
			ImportPath: modPath + "/" + filepath.ToSlash(rel),
			FuncName:   regFunc,
		})
	}

	if len(subPkgs) == 0 {
		fmt.Printf("No Go subpackages found under %s — nothing to aggregate.\n", dir)
		return nil
	}

	rootPkg := pkg
	if rootPkg == "" {
		rootPkg = "main"
	}
	out := BuildRootDIGen(rootPkg, subPkgs)
	if err := fsutil.WriteString(dir, "di_gen.go", out, force); err != nil {
		return err
	}

	fmt.Printf("DI aggregator generated: %s/di_gen.go\n", dir)
	fmt.Printf("  package %s — aggregates %d subpackage(s)\n", rootPkg, len(subPkgs))
	fmt.Println("  Call initDI(app) in main() before app.Run()")
	return nil
}

// readModulePath returns the module path declared in the nearest go.mod file
// and the directory where that go.mod lives, searching dir and its parents.
func readModulePath(dir string) (modPath, modRoot string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	for {
		candidate := filepath.Join(abs, "go.mod")
		f, err := os.Open(candidate) // 
		if err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "module ") {
					return strings.TrimSpace(strings.TrimPrefix(line, "module")), abs, nil
				}
			}
			return "", "", fmt.Errorf("no module directive found in %s", candidate)
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", "", fmt.Errorf("go.mod not found in %s or any parent directory", dir)
		}
		abs = parent
	}
}
