package wire

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Provider holds one scanned di.Provide* call.
type Provider struct {
	TypeArg   string
	FuncName  string
	Deps      []string
	Named     string
	Kind      string
	SetupFunc string
}

// ScanResult collects everything found by ScanPackage.
type ScanResult struct {
	PkgName    string
	Providers  map[string]*Provider
	SetupFuncs []string
}

// defaultProviderKinds are the di function names recognised when no custom list is given.
var defaultProviderKinds = map[string]bool{
	"Provide": true, "ProvideNamed": true, "ProvideConstructor": true, "ProvideValue": true,
}

// ScanPackage parses all non-test, non-generated .go files in dir and extracts
// di.Provide* call sites and their enclosing setup functions.
// customFuncs overrides the default set of recognised function names (pkg.Func format).
// If recursive is true, subdirectories are scanned as well and their results merged.
func ScanPackage(dir string, customFuncs []string, recursive bool) (*ScanResult, error) {
	result := &ScanResult{
		Providers: make(map[string]*Provider),
	}
	setupSeen := make(map[string]bool)

	dirs := []string{dir}
	if recursive {
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && path != dir && !strings.HasPrefix(d.Name(), ".") {
				dirs = append(dirs, path)
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("walk dir %s: %w", dir, err)
		}
	}

	kindSet := defaultProviderKinds
	pkgFilter := "di"
	if len(customFuncs) > 0 {
		kindSet = make(map[string]bool, len(customFuncs))
		for _, f := range customFuncs {
			parts := strings.SplitN(f, ".", 2)
			if len(parts) == 2 {
				pkgFilter = parts[0]
				kindSet[parts[1]] = true
			} else {
				kindSet[f] = true
			}
		}
	}

	for _, d := range dirs {
		if err := scanDir(d, pkgFilter, kindSet, result, setupSeen); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func scanDir(dir, pkgFilter string, kindSet map[string]bool, result *ScanResult, setupSeen map[string]bool) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		name := fi.Name()
		return !strings.HasSuffix(name, "_test.go") &&
			name != "di_gen.go" &&
			name != "wire.go"
	}, 0)
	if err != nil {
		return fmt.Errorf("parse Go files in %s: %w\n  hint: fix syntax errors before running gen wire --scan", dir, err)
	}

	for pname, pkg := range pkgs {
		if result.PkgName == "" {
			result.PkgName = pname
		}
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				fnName := fn.Name.Name

				var fnProviders []*Provider
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					p := extractDICall(call, pkgFilter, kindSet)
					if p == nil {
						return true
					}
					p.SetupFunc = fnName
					p.Deps = extractInvokeDeps(call)
					fnProviders = append(fnProviders, p)
					return true
				})

				if len(fnProviders) == 0 {
					continue
				}
				if !setupSeen[fnName] {
					setupSeen[fnName] = true
					result.SetupFuncs = append(result.SetupFuncs, fnName)
				}
				for _, p := range fnProviders {
					key := p.TypeArg
					if p.Named != "" {
						key = p.TypeArg + ":" + p.Named
					}
					result.Providers[key] = p
				}
			}
		}
	}
	return nil
}

func extractInvokeDeps(call *ast.CallExpr) []string {
	var deps []string
	seen := make(map[string]bool)

	for _, arg := range call.Args {
		lit, ok := arg.(*ast.FuncLit)
		if !ok {
			continue
		}
		ast.Inspect(lit.Body, func(n ast.Node) bool {
			invokeCall, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ie, ok := invokeCall.Fun.(*ast.IndexExpr)
			if !ok {
				return true
			}
			sel, ok := ie.X.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "di" {
				return true
			}
			if sel.Sel.Name != "Invoke" && sel.Sel.Name != "InvokeNamed" {
				return true
			}
			dep := exprToString(ie.Index)
			if dep != "" && !seen[dep] {
				seen[dep] = true
				deps = append(deps, dep)
			}
			return true
		})
	}
	return deps
}

func extractDICall(call *ast.CallExpr, pkgFilter string, kindSet map[string]bool) *Provider {
	indexExpr, ok := call.Fun.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	sel, ok := indexExpr.X.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != pkgFilter {
		return nil
	}

	kind := sel.Sel.Name
	if !kindSet[kind] {
		return nil
	}

	typeArg := exprToString(indexExpr.Index)
	p := &Provider{TypeArg: typeArg, Kind: kind}

	if kind == "ProvideNamed" && len(call.Args) >= 2 {
		if lit, ok := call.Args[1].(*ast.BasicLit); ok {
			p.Named = strings.Trim(lit.Value, `"`)
		}
	}

	if kind == "ProvideConstructor" {
		factoryIdx := 1
		if len(call.Args) > factoryIdx {
			if _, isLit := call.Args[factoryIdx].(*ast.FuncLit); !isLit {
				p.FuncName = exprToString(call.Args[factoryIdx])
			}
		}
	}

	return p
}

func exprToString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprToString(v.X)
	case *ast.SelectorExpr:
		return exprToString(v.X) + "." + v.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprToString(v.Elt)
	case *ast.BasicLit:
		return v.Value
	case *ast.IndexExpr:
		// Single type parameter: T[U]
		return exprToString(v.X) + "[" + exprToString(v.Index) + "]"
	case *ast.IndexListExpr:
		// Multiple type parameters: T[A, B, C]
		parts := make([]string, len(v.Indices))
		for i, idx := range v.Indices {
			parts[i] = exprToString(idx)
		}
		return exprToString(v.X) + "[" + strings.Join(parts, ", ") + "]"
	}
	return ""
}
