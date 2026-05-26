// astractl is the Astra framework CLI for scaffolding projects and generating code.
//
// Commands:
//
//	astractl new <project-name>  [--module MODULE] [--layout simple|ddd] [--template microservice]
//	astractl gen handler <name>  [--dir DIR] [--pkg PKG] [--service] [--force]
//	astractl gen model <name>    [--dir DIR] [--pkg PKG] [--force]
//	astractl gen middleware <name> [--dir DIR] [--pkg PKG] [--force]
//	astractl gen crud <name>     [--dir DIR] [--with-service] [--force]
//	astractl gen repo <name>     [--dir DIR] [--pkg PKG] [--force]
//	astractl gen service <name>  [--dir DIR] [--pkg PKG] [--force]
//	astractl gen service         --proto FILE [--out-dir DIR] [--module MODULE] [--force]
//	astractl gen wire            [--dir DIR] [--pkg PKG] [--scan] [--force]
//	astractl gen errors          [--dir DIR] [--pkg PKG] [--force]
//	astractl gen test <name>     [--dir DIR] [--pkg PKG] [--force]
//	astractl gen proto <file>    [--dir DIR] [--pkg PKG] [--grpc] [--contract] [--impl] [--force]
//	astractl gen openapi <file>  [--dir DIR] [--pkg PKG] [--force]
//	astractl gen container       [--dir DIR] [--force]
//	astractl generate            (alias for gen)
//	astractl migrate create <name>
//	astractl doctor              [--dir DIR]
//	astractl tidy
//	astractl version
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/doctor"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
	gencontainer "github.com/astra-go/astra/cmd/astractl/internal/gen/container"
	gencrud "github.com/astra-go/astra/cmd/astractl/internal/gen/crud"
	generrors "github.com/astra-go/astra/cmd/astractl/internal/gen/errors"
	genhandler "github.com/astra-go/astra/cmd/astractl/internal/gen/handler"
	genmiddleware "github.com/astra-go/astra/cmd/astractl/internal/gen/middleware"
	genmodel "github.com/astra-go/astra/cmd/astractl/internal/gen/model"
	genopenapi "github.com/astra-go/astra/cmd/astractl/internal/gen/openapi"
	genschema  "github.com/astra-go/astra/cmd/astractl/internal/gen/schema"
	genproto "github.com/astra-go/astra/cmd/astractl/internal/gen/proto"
	genrepo "github.com/astra-go/astra/cmd/astractl/internal/gen/repo"
	genservice "github.com/astra-go/astra/cmd/astractl/internal/gen/service"
	gentest "github.com/astra-go/astra/cmd/astractl/internal/gen/test"
	genwire "github.com/astra-go/astra/cmd/astractl/internal/gen/wire"
	"github.com/astra-go/astra/cmd/astractl/internal/tmpl"
	"github.com/astra-go/astra/cmd/astractl/internal/tpldata"
)

const version = "1.4.0"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printHelp()
		return
	}

	// Strip --template-dir <dir> from args before dispatching.
	args = extractTemplateDir(args)

	var err error
	switch args[0] {
	case "new":
		err = cmdNew(args[1:])
	case "gen", "generate":
		err = cmdGen(args[1:])
	case "migrate":
		err = cmdMigrate(args[1:])
	case "tidy":
		err = cmdTidy(args[1:])
	case "doctor":
		err = cmdDoctor(args[1:])
	case "version", "-v", "--version":
		fmt.Printf("astractl version %s\n", version)
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", args[0])
		printHelp()
		os.Exit(1)
	}

	if err != nil {
		os.Exit(cli.Render(err))
	}
}

// ─── gen dispatcher ───────────────────────────────────────────────────────────

func cmdGen(args []string) error {
	if len(args) == 0 {
		return &cli.CLIError{
			Msg:     "missing gen subcommand",
			Example: "astractl gen handler|model|middleware|crud|repo|service|wire|container|errors|test|proto|openapi|schema",
		}
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "handler":
		return genhandler.Run(rest)
	case "model":
		return genmodel.Run(rest)
	case "middleware":
		return genmiddleware.Run(rest)
	case "crud":
		return gencrud.Run(rest)
	case "repo":
		return genrepo.Run(rest)
	case "service":
		return genservice.Run(rest)
	case "wire":
		return genwire.Run(rest)
	case "container":
		return gencontainer.Run(rest)
	case "errors":
		return generrors.Run(rest)
	case "test":
		return gentest.Run(rest)
	case "proto":
		return genproto.Run(rest)
	case "openapi":
		return genopenapi.Run(rest)
	case "schema":
		return genschema.Run(rest)
	default:
		return &cli.CLIError{
			Msg:     fmt.Sprintf("unknown gen subcommand: %q", sub),
			Example: "astractl gen handler|model|middleware|crud|repo|service|wire|container|errors|test|proto|openapi|schema",
		}
	}
}

// ─── new ──────────────────────────────────────────────────────────────────────

func cmdNew(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return &cli.CLIError{
			Msg:     "missing required argument: <project-name>",
			Example: "astractl new my-api --module github.com/myorg/my-api --layout simple",
		}
	}
	name := args[0]
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.Contains(name, "..") {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("invalid project name: %q", name),
			Hint:    "project name must not contain path separators or be '.' / '..'",
			Example: "astractl new my-api",
		}
	}

	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	modulePath := fs.String("module", "", "Go module path (default: project-name)")
	layout     := fs.String("layout", "simple", "project layout: simple | ddd")
	template   := fs.String("template", "", "project template: microservice")
	if err := fs.Parse(args[1:]); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl new --help' to see all available flags",
		}
	}
	if *modulePath == "" {
		*modulePath = name
	}
	if *template != "" && *template != "microservice" {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("invalid --template value: %q", *template),
			Hint:    "--template must be 'microservice'",
			Example: "astractl new my-api --template=microservice",
		}
	}
	if *template != "" && *layout != "simple" {
		return &cli.CLIError{
			Msg:  "--template and --layout are mutually exclusive",
			Hint: "use --template=microservice OR --layout=ddd, not both",
		}
	}
	if *layout != "simple" && *layout != "ddd" {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("invalid --layout value: %q", *layout),
			Hint:    "--layout must be 'simple' or 'ddd'",
			Example: "astractl new my-api --layout ddd",
		}
	}

	data := tpldata.New(name, *modulePath, "")
	data.Layout = *layout

	commonDirs := []string{
		filepath.Join(name, "config"),
		filepath.Join(name, "migrations"),
	}
	for _, d := range commonDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", d, err)}
		}
	}

	// Shared files.
	sharedWrites := []func() error{
		func() error {
			return fsutil.WriteTemplate("", filepath.Join(name, "go.mod"), tmpl.GoMod(), data, false)
		},
		func() error {
			return fsutil.WriteTemplate("", filepath.Join(name, ".gitignore"), tmpl.Gitignore(), data, false)
		},
		func() error {
			return fsutil.WriteTemplate("", filepath.Join(name, "Makefile"), tmpl.Makefile(), data, false)
		},
		func() error {
			return fsutil.WriteTemplate("", filepath.Join(name, "Dockerfile"), tmpl.Dockerfile(), data, false)
		},
		func() error {
			return fsutil.WriteTemplate("", filepath.Join(name, "docker-compose.yml"), tmpl.DockerCompose(), data, false)
		},
		func() error {
			return fsutil.WriteTemplate("", filepath.Join(name, "config", "dev.yaml"), tmpl.ConfigDev(), data, false)
		},
		func() error {
			return fsutil.WriteTemplate("", filepath.Join(name, "config", "prod.yaml"), tmpl.ConfigProd(), data, false)
		},
	}
	for _, w := range sharedWrites {
		if err := w(); err != nil {
			return err
		}
	}

	if *template == "microservice" {
		msDirs := []string{
			filepath.Join(name, "cmd", "server"),
			filepath.Join(name, "internal", "handler"),
			filepath.Join(name, "internal", "service"),
			filepath.Join(name, "internal", "model"),
			filepath.Join(name, ".github", "workflows"),
		}
		for _, d := range msDirs {
			if err := os.MkdirAll(d, 0755); err != nil {
				return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", d, err)}
			}
		}
		if err := fsutil.WriteTemplate("", filepath.Join(name, "cmd", "server", "main.go"), tmpl.MainDDD(), data, false); err != nil {
			return err
		}
		if err := fsutil.WriteTemplate("", filepath.Join(name, ".github", "workflows", "ci.yml"), tmpl.CIWorkflow(), data, false); err != nil {
			return err
		}
		fmt.Printf("\nProject created: %s/  (template: microservice)\n", name)
		fmt.Printf("  Module:  %s\n", *modulePath)
		fmt.Println("\nNext steps:")
		fmt.Printf("  cd %s\n", name)
		fmt.Println("  go mod tidy")
		fmt.Println("  go run ./cmd/server/...")
		fmt.Println("  docker compose up  # optional: start Postgres + Redis")
	} else if *layout == "ddd" {
		dddDirs := []string{
			filepath.Join(name, "cmd", "server"),
			filepath.Join(name, "internal", "domain", "entity"),
			filepath.Join(name, "internal", "application", "usecase", "dto"),
			filepath.Join(name, "internal", "infrastructure", "persistence"),
			filepath.Join(name, "internal", "handler"),
			filepath.Join(name, "pkg", "errors"),
		}
		for _, d := range dddDirs {
			if err := os.MkdirAll(d, 0755); err != nil {
				return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", d, err)}
			}
		}
		if err := fsutil.WriteTemplate("", filepath.Join(name, "cmd", "server", "main.go"), tmpl.MainDDD(), data, false); err != nil {
			return err
		}
		if err := fsutil.WriteTemplate("", filepath.Join(name, "cmd", "server", "wire.go"), tmpl.WireProvider(), data, false); err != nil {
			return err
		}
		if err := fsutil.WriteTemplate("", filepath.Join(name, "pkg", "errors", "errors.go"), tmpl.ErrorCodes(), tpldata.New(name, *modulePath, "errors"), false); err != nil {
			return err
		}
		fmt.Printf("\nProject created: %s/  (layout: ddd)\n", name)
		fmt.Printf("  Module:  %s\n", *modulePath)
		fmt.Println("\nNext steps:")
		fmt.Printf("  cd %s\n", name)
		fmt.Println("  go mod tidy")
		fmt.Println("  go run ./cmd/server/...")
		fmt.Println("  docker compose up  # optional: start Postgres + Redis")
	} else {
		simpleDirs := []string{
			filepath.Join(name, "handler"),
			filepath.Join(name, "model"),
			filepath.Join(name, "repository"),
			filepath.Join(name, "service"),
		}
		for _, d := range simpleDirs {
			if err := os.MkdirAll(d, 0755); err != nil {
				return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", d, err)}
			}
		}
		if err := fsutil.WriteTemplate("", filepath.Join(name, "main.go"), tmpl.Main(), data, false); err != nil {
			return err
		}
		if err := fsutil.WriteTemplate("", filepath.Join(name, "routes.go"), tmpl.Routes(), data, false); err != nil {
			return err
		}
		fmt.Printf("\nProject created: %s/  (layout: simple)\n", name)
		fmt.Printf("  Module:  %s\n", *modulePath)
		fmt.Println("\nNext steps:")
		fmt.Printf("  cd %s\n", name)
		fmt.Println("  go mod tidy")
		fmt.Println("  go run .")
		fmt.Println("  docker compose up  # optional: start Postgres + Redis")
	}
	return nil
}

// ─── migrate ──────────────────────────────────────────────────────────────────

func cmdMigrate(args []string) error {
	if len(args) == 0 {
		return &cli.CLIError{
			Msg:     "missing migrate subcommand",
			Example: "astractl migrate create|up|down|status",
		}
	}
	switch args[0] {
	case "create":
		desc := ""
		if len(args) > 1 {
			desc = strings.Join(args[1:], " ")
		}
		return cmdMigrateCreate(desc)
	case "up":
		printMigrateNotice("up")
	case "down":
		printMigrateNotice("down")
	case "status":
		printMigrateNotice("status")
	default:
		return &cli.CLIError{
			Msg:     fmt.Sprintf("unknown migrate subcommand: %q", args[0]),
			Example: "astractl migrate create|up|down|status",
		}
	}
	return nil
}

func cmdMigrateCreate(name string) error {
	if name == "" {
		return &cli.CLIError{
			Msg:     "missing required argument: <description>",
			Example: "astractl migrate create \"add users table\"",
		}
	}
	ts := time.Now().Format("20060102150405")
	id := ts + "_" + strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	goID := strings.ReplaceAll(tpldata.Pascal(id), "_", "")
	// The template emits "Migration<goID>" — validate the full identifier.
	if !tpldata.IsValidGoIdent("Migration" + goID) {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("migration description %q produces invalid Go identifier %q", name, "Migration"+goID),
			Hint:    "use only letters, digits and spaces in the description",
			Example: `astractl migrate create "add users table"`,
		}
	}

	if err := os.MkdirAll("migrations", 0755); err != nil {
		return &cli.CLIError{Msg: fmt.Sprintf("mkdir migrations: %v", err)}
	}

	data := tpldata.Data{
		ID:          goID,
		IDStr:       id,
		Description: name,
	}
	file := filepath.Join("migrations", id+".go")
	if err := fsutil.WriteTemplate("", file, tmpl.Migration(), data, false); err != nil {
		return err
	}
	fmt.Printf("Migration created: %s\n", file)
	fmt.Println("\nRemember to register it in your migration list:")
	fmt.Printf("  m.Register(migrations.Migration%s)\n", goID)
	return nil
}

func printMigrateNotice(subcmd string) {
	fmt.Printf(`
The 'migrate %s' command must be run from your application code since
the CLI does not have access to your database configuration.

Add the following to your application startup (or a separate cmd/migrate/main.go):

    import (
        "context"
        "database/sql"
        "log"

        "github.com/astra-go/astra/migrate"
        migs "your-module/migrations"
    )

    func runMigrations(db *sql.DB) {
        m := migrate.New(db)
        m.Register(
            migs.Migration001CreateUsers,
            // ... more migrations
        )

        switch os.Args[1] {
        case "up":
            if err := m.Up(context.Background()); err != nil { log.Fatal(err) }
        case "down":
            if err := m.Down(context.Background()); err != nil { log.Fatal(err) }
        case "status":
            statuses, err := m.Status(context.Background())
            if err != nil { log.Fatal(err) }
            for _, s := range statuses {
                mark := " "
                if s.Applied { mark = "✓" }
                fmt.Printf("[%%s] %%s\n", mark, s.ID)
            }
        }
    }

`, subcmd)
}

// ─── doctor ───────────────────────────────────────────────────────────────────

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	dir := fs.String("dir", ".", "directory to inspect")
	if err := fs.Parse(args); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl doctor --help' to see all available flags",
		}
	}
	checks := doctor.Run(*dir)
	doctor.Print(checks)
	if doctor.HasFailures(checks) {
		os.Exit(1)
	}
	return nil
}

// ─── tidy ─────────────────────────────────────────────────────────────────────

// modDir pairs a human-readable module path with its absolute directory.
type modDir struct{ rel, abs string }

func cmdTidy(args []string) error {
	fs := flag.NewFlagSet("tidy", flag.ContinueOnError)
	check := fs.Bool("check", false, "verify modules are tidy without permanently modifying files (exits 1 if diff would occur)")
	if err := fs.Parse(args); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl tidy --help' to see all available flags",
		}
	}
	root, err := repoRoot()
	if err != nil {
		return &cli.CLIError{
			Msg:  "cannot determine repo root: " + err.Error(),
			Hint: "run astractl tidy from inside the repository",
		}
	}

	modules, err := modulesFromGoWork(root)
	if err != nil {
		return &cli.CLIError{
			Msg:  "cannot read go.work: " + err.Error(),
			Hint: "ensure go.work is valid or run from the workspace root",
		}
	}

	// Filter to directories that actually contain a go.mod.
	var dirs []modDir
	for _, mod := range modules {
		dir := root
		if mod != "." {
			dir = filepath.Join(root, mod)
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
			continue
		}
		dirs = append(dirs, modDir{rel: mod, abs: dir})
	}

	if *check {
		return checkTidyAll(dirs)
	}

	var failed []string
	for _, d := range dirs {
		fmt.Printf("▶  go mod tidy — %s\n", d.rel)
		goBin, err := resolvedGo()
		if err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("go binary not found: %v", err)}
		}
		cmd := exec.Command(goBin, "mod", "tidy")
		cmd.Dir = d.abs
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("go mod tidy failed in %s: %v", d.rel, err)}
		}
	}

	if len(failed) > 0 {
		return &cli.CLIError{Msg: "go mod tidy failed in: " + strings.Join(failed, ", ")}
	}
	fmt.Printf("✓ go mod tidy complete (%d module(s))\n", len(dirs))
	return nil
}

// checkTidyAll runs go mod tidy on each module in parallel, snapshots go.mod/go.sum
// before and restores them after, reporting which modules were not tidy.
func checkTidyAll(dirs []modDir) error {
	type result struct {
		rel string
		err error
	}
	results := make(chan result, len(dirs))
	var wg sync.WaitGroup
	for _, d := range dirs {
		wg.Add(1)
		go func(d modDir) {
			defer wg.Done()
			results <- result{rel: d.rel, err: checkTidyModule(d.abs)}
		}(d)
	}
	wg.Wait()
	close(results)

	var failed []string
	for r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", r.rel, r.err)
			failed = append(failed, r.rel)
		}
	}
	if len(failed) > 0 {
		return &cli.CLIError{Msg: "not tidy: " + strings.Join(failed, ", ")}
	}
	fmt.Printf("✓ All %d module(s) are tidy\n", len(dirs))
	return nil
}

// checkTidyModule snapshots go.mod/go.sum, runs go mod tidy, compares results,
// and restores the originals — so the working tree is unchanged on exit.
func checkTidyModule(dir string) error {
	read := func(name string) []byte {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		return b
	}
	beforeMod := read("go.mod")
	beforeSum := read("go.sum")

	goBin, err := resolvedGo()
	if err != nil {
		return fmt.Errorf("go binary not found: %w", err)
	}
	out, err := exec.Command(goBin, "mod", "tidy").CombinedOutput()
	if err != nil {
		return fmt.Errorf("go mod tidy error: %v\n%s", err, out)
	}

	afterMod := read("go.mod")
	afterSum := read("go.sum")

	// Always restore originals.
	if len(beforeMod) > 0 {
		_ = os.WriteFile(filepath.Join(dir, "go.mod"), beforeMod, 0644)
	}
	if len(beforeSum) > 0 {
		_ = os.WriteFile(filepath.Join(dir, "go.sum"), beforeSum, 0644)
	} else if len(afterSum) > 0 {
		_ = os.Remove(filepath.Join(dir, "go.sum"))
	}

	if !bytes.Equal(beforeMod, afterMod) || !bytes.Equal(beforeSum, afterSum) {
		return fmt.Errorf("go.mod or go.sum would change (run 'astractl tidy' to fix)")
	}
	return nil
}

// modulesFromGoWork reads use directives from go.work and returns relative paths.
// Falls back to ["."] when go.work is absent.
func modulesFromGoWork(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err != nil {
		return []string{"."}, nil
	}
	var mods []string
	inBlock := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if line == "use (" {
			inBlock = true
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			if idx := strings.Index(line, "//"); idx >= 0 {
				line = strings.TrimSpace(line[:idx])
			}
			if line != "" {
				mods = append(mods, filepath.Clean(line))
			}
			continue
		}
		if rest, ok := strings.CutPrefix(line, "use "); ok {
			rest = strings.TrimSpace(rest)
			if idx := strings.Index(rest, "//"); idx >= 0 {
				rest = strings.TrimSpace(rest[:idx])
			}
			if rest != "" && rest != "(" {
				mods = append(mods, filepath.Clean(rest))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(mods) == 0 {
		return []string{"."}, nil
	}
	return mods, nil
}

func repoRoot() (string, error) {
	gitBin, err := resolvedGit()
	if err != nil {
		// git not found — fall back to cwd
		wd, werr := os.Getwd()
		if werr != nil {
			return "", werr
		}
		fmt.Fprintf(os.Stderr, "warning: git not found, using current directory as root: %s\n", wd)
		return wd, nil
	}
	out, err := exec.Command(gitBin, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		wd, werr := os.Getwd()
		if werr != nil {
			return "", werr
		}
		fmt.Fprintf(os.Stderr, "warning: not inside a git repository, using current directory as root: %s\n", wd)
		return wd, nil
	}
	return strings.TrimSpace(string(out)), nil
}

// resolvedGo returns the absolute path to the Go toolchain binary, resolved
// once at first call via a PATH sanitised to absolute-only entries (see
// safeLookPath).  Using LookPath rather than runtime.GOROOT keeps the binary
// portable across machines.
var resolvedGo = sync.OnceValues(func() (string, error) {
	name := "go"
	if runtime.GOOS == "windows" {
		name = "go.exe"
	}
	return safeLookPath(name)
})

// resolvedGit returns the absolute path to the git binary, resolved once at
// first call using a PATH sanitised to absolute-only entries.
var resolvedGit = sync.OnceValues(func() (string, error) {
	return safeLookPath("git")
})

// safeLookPath resolves name by walking PATH entries that are absolute paths,
// skipping any relative entries (e.g. "." or "bin") which are the primary
// vector for PATH-hijacking attacks.
func safeLookPath(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}
		candidate := filepath.Join(dir, name)
		fi, err := os.Stat(candidate)
		if err != nil || fi.IsDir() {
			continue
		}
		if runtime.GOOS == "windows" || fi.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

// ─── help ─────────────────────────────────────────────────────────────────────

func printHelp() {
	fmt.Print(`
   ___        _
  / _ \  __ _| |_ _ __ __ _
 | | | |/ _  | __| '__/ _  |
 | |_| | (_| | |_| | | (_| |
  \___/ \__,_|\__|_|  \__,_|

  Astra CLI — code generator for the Astra Go framework  (v` + version + `)

Usage:
  astractl <command> [arguments] [flags]

Commands:
  new <project-name>              Scaffold a new Astra project
    --module MODULE               Go module path (default: project-name)
    --layout simple|ddd           Project layout (default: simple)
    --template microservice       Microservice layout with CI workflow

  gen handler <name>              Generate a handler file (CRUD + pagination)
    --dir DIR                     Output directory (default: current directory)
    --pkg PKG                     Package name (default: handler)
    --service                     Co-generate a service interface in the file
    --force                       Overwrite existing file

  gen model <name>                Generate a GORM model file
  gen middleware <name>           Generate a middleware scaffold
  gen repo <name>                 Generate a generic repository
  gen service <name>              Generate a service interface + implementation
    --proto FILE                  Generate full microservice skeleton from .proto
    --out-dir DIR                 Output directory for skeleton (default: <svc>-svc)
    --module MODULE               Go module path for skeleton
    (all support --dir, --pkg, --force)

  gen crud <name>                 Generate handler + model + repository (in handler/model/repository/ subdirs)
    --dir DIR                     Base output directory (subdirs created inside)
    --with-service                Also generate a service layer file (in service/ subdir)
    --force                       Overwrite existing files

  gen wire                        Generate DI wiring code
    --scan                        Scan di.Provide* calls and emit di_gen.go
    --dir DIR   --pkg PKG   --force

  gen errors                      Generate typed error codes (errors.go)
  gen container                   Generate Astra di/ container setup (container.go)
    (support --dir, --force)

  gen test <name>                 Generate httptest-based handler test skeleton
  gen proto <file.proto>          Generate Go types + service interface + HTTP adapter
    --contract                    Only types + service interface (no HTTP adapter)
    --grpc                        gRPC-first: types + interface + gRPC stub
    --impl                        Also generate a service implementation skeleton
    (support --dir, --pkg, --force)

  gen openapi <file.yaml>         Generate handler stubs from OpenAPI 3.x YAML
    --dir DIR   --pkg PKG   --force

  gen schema                      Generate OpenAPI 3.1 spec from Go types & annotations
    --dir DIR                     Directory to scan (default: current directory)
    --out FILE                    Output file: .json (default) or .yaml/.yml
    --title TITLE                 API title (default: API)
    --version VERSION             API version (default: 0.1.0)
    --force                       Overwrite existing output file

  generate                        Alias for 'gen'

  migrate create <description>    Create a new migration file
  migrate up|down|status          Show how to run migrations in your app

  tidy                            Run go mod tidy across all workspace modules
    --check                       Verify tidy without modifying (exits 1 if diff)

  doctor                          Diagnose project setup for gen command prerequisites
    --dir DIR                     Directory to inspect (default: current directory)

  version                         Print version information
  help                            Show this help message

Examples:
  astractl new my-api --module github.com/myorg/my-api
  astractl new my-svc --module github.com/myorg/my-svc --template=microservice
  astractl gen service --proto api/user.proto --module github.com/myorg/user-svc
  astractl gen handler User --dir ./internal/handler --service
  astractl gen crud Product --dir ./internal --with-service
  astractl gen wire --scan --dir ./cmd/server
  astractl gen proto api/service.proto --dir ./internal/handler --impl
  astractl gen openapi api/openapi.yaml --dir ./internal/handler
  astractl gen schema --dir ./internal/handler --out api/openapi.json --title "My API" --version 1.0.0
  astractl doctor
  astractl migrate create "add users table"

  Global flags (before the subcommand):
  --template-dir <dir>   load custom .tmpl files from <dir>, overriding embedded templates

`)
}

// extractTemplateDir strips --template-dir <dir> from args, calls tmpl.SetDir, and returns the remaining args.
func extractTemplateDir(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--template-dir" && i+1 < len(args):
			tmpl.SetDir(args[i+1])
			i++
		case len(args[i]) > 15 && args[i][:15] == "--template-dir=":
			tmpl.SetDir(args[i][15:])
		default:
			out = append(out, args[i])
		}
	}
	return out
}
