package generate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/astra-go/astra/cmd/astra-cli/internal/fsutil"
	"github.com/astra-go/astra/cmd/astra-cli/internal/tpldata"
	"github.com/astra-go/astra/cmd/astra-cli/internal/templates"
)

// newGenerateCrudCmd creates the `astra-cli generate crud` command.
func newGenerateCrudCmd() *cobra.Command {
	var (
		interactive bool
		optName     string
		optDir      string
		optWithService bool
	)

	c := &cobra.Command{
		Use:   "crud [entity-name]",
		Short: "Generate CRUD handler + model + repository from a table schema",
		Long: `Generates a complete CRUD scaffold for a database entity:
  - model/<name>.go        — GORM model with tags
  - repository/<name>_repo.go  — data access layer
  - handler/<name>_handler.go  — HTTP handler with List/Create/Get/Update/Delete
  - service/<name>_svc.go   — (optional) service layer interface + stub

Table columns can be provided interactively or parsed from a schema file.
Each column asks for: name, Go type, GORM tag, and whether it's required.

Examples:
  astra-cli generate crud Article
  astra-cli generate crud Product -o ./internal
  astra-cli generate crud --interactive`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var entityName string
			var columns []colDef
			var err error

			if interactive || len(args) == 0 {
				entityName, columns, err = promptsCrud()
				if err != nil {
					return fmt.Errorf("interactive prompt cancelled: %w", err)
				}
			} else {
				entityName = args[0]
				if optName != "" {
					entityName = optName
				}
				// Default: empty columns (user fills in TODOs)
				columns = defaultColumns(entityName)
			}

			if entityName == "" {
				return errors.New("entity name is required")
			}
			if !isValidGoIdent(pascal(entityName)) {
				return fmt.Errorf("invalid entity name: %q", entityName)
			}

			data := tpldata.New(entityName, "")
			data.Columns = columns

			outDir := optDir
			if outDir == "" && globalOutDir != "" {
				outDir = globalOutDir
			}

			entityNameL := strings.ToLower(pascal(entityName))
			modelFile   := filepath.Join(outDir, "model", entityNameL+".go")
			repoFile    := filepath.Join(outDir, "repository", entityNameL+"_repo.go")
			handlerFile := filepath.Join(outDir, "handler", entityNameL+"_handler.go")

			// Create subdirs
			mkdirAll(filepath.Join(outDir, "model"))
			mkdirAll(filepath.Join(outDir, "repository"))
			mkdirAll(filepath.Join(outDir, "handler"))

			render := func(path, content string) error {
				if fileExists(path) && !globalForce {
					return fmt.Errorf("file already exists: %s (use --force to overwrite)", path)
				}
				return fsutil.WriteString(path, content)
			}

			if err := render(modelFile, templates.RenderCRUDModel(data)); err != nil {
				return err
			}
			if err := render(repoFile, templates.RenderCRUDRepo(data)); err != nil {
				return err
			}
			if err := render(handlerFile, templates.RenderCRUDHandler(data)); err != nil {
				return err
			}

			var created []string
			created = append(created, modelFile, repoFile, handlerFile)

			if optWithService {
				mkdirAll(filepath.Join(outDir, "service"))
				svcFile := filepath.Join(outDir, "service", entityNameL+"_svc.go")
				if err := render(svcFile, templates.RenderCRUDService(data)); err != nil {
					return err
				}
				created = append(created, svcFile)
			}

			fmt.Printf("\n✓ CRUD scaffold generated for entity: %s\n", entityName)
			fmt.Printf("  Layout: %s/\n", outDir)
			fmt.Println("  Files:")
			for _, f := range created {
				fmt.Printf("  - %s\n", f)
			}
			return nil
		},
	}

	c.Flags().BoolVar(&interactive, "interactive", false, "run in interactive mode")
	c.Flags().StringVarP(&optName, "name", "n", "", "entity name (same as first positional arg)")
	c.Flags().StringVarP(&optDir, "dir", "d", "", "base output directory")
	c.Flags().BoolVar(&optWithService, "with-service", false, "also generate a service layer file")

	return c
}

// ─── Column definition ─────────────────────────────────────────────────────────

type colDef struct {
	Name    string // Go field name (PascalCase)
	JSONTag string // json:"..."
	GoType  string // e.g. string, int64, *time.Time
	GORMCol string // gorm:"column:...;..."
	Comment string // field comment
}

var goTypeOptions = []string{
	"string",
	"int64",
	"int",
	"float64",
	"bool",
	"time.Time",
	"[]byte",
}

// defaultColumns returns placeholder columns for an entity with TODOs.
func defaultColumns(name string) []colDef {
	pName := pascal(name)
	return []colDef{
		{Name: "ID", JSONTag: `"id"`, GoType: "int64", GORMCol: `gorm:"primaryKey;autoIncrement"`, Comment: "// TODO: primary key"},
		{Name: "CreatedAt", JSONTag: `"created_at,omitempty"`, GoType: "time.Time", GORMCol: `gorm:"autoCreateTime"`, Comment: "// TODO: created timestamp"},
		{Name: "UpdatedAt", JSONTag: `"updated_at,omitempty"`, GoType: "time.Time", GORMCol: `gorm:"autoUpdateTime"`, Comment: "// TODO: updated timestamp"},
		{Name: pascal(name) + "Name", JSONTag: `"name,omitempty"`, GoType: "string", GORMCol: `gorm:"size:255"`, Comment: "// TODO: add your fields"},
	}
}

func isValidGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
		} else if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

// parseSchemaFile tries to parse columns from a SQL schema file.
func parseSchemaFile(path string) ([]colDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cols []colDef
	lines := strings.Split(string(data), "\n")

	colRe := regexp.MustCompile(`(?i)^\s*(\w+)\s+([\w()]+(?:\([\d,]+\))?)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comments, empty lines, and constraint lines
		if strings.HasPrefix(line, "--") || strings.HasPrefix(line, "//") || line == "" {
			continue
		}
		if strings.Contains(strings.ToLower(line), "primary key") ||
			strings.Contains(strings.ToLower(line), "foreign key") ||
			strings.Contains(strings.ToLower(line), "index ") ||
			strings.Contains(strings.ToLower(line), "unique ") ||
			strings.Contains(strings.ToLower(line), "constraint ") ||
			strings.HasPrefix(strings.ToLower(line), "create table") ||
			strings.HasPrefix(strings.ToLower(line), ")") {
			continue
		}

		m := colRe.FindStringSubmatch(line)
		if len(m) >= 3 {
			name := m[1]
			dbType := m[2]

			// Skip non-column keywords
			if strings.EqualFold(name, "PRIMARY") || strings.EqualFold(name, "KEY") ||
				strings.EqualFold(name, "INDEX") || strings.EqualFold(name, "UNIQUE") ||
				strings.EqualFold(name, "FOREIGN") || strings.EqualFold(name, "CONSTRAINT") {
				continue
			}

			goType := sqlTypeToGo(dbType)
			cols = append(cols, colDef{
				Name:    pascal(name),
				JSONTag: fmt.Sprintf(`"%s"`, strings.ToLower(name)),
				GoType:  goType,
				GORMCol: fmt.Sprintf(`gorm:"column:%s"`, name),
			})
		}
	}
	return cols, nil
}

func sqlTypeToGo(sqlType string) string {
	sqlType = strings.ToLower(sqlType)
	switch {
	case strings.Contains(sqlType, "int") && !strings.Contains(sqlType, "bigint"):
		return "int"
	case strings.Contains(sqlType, "bigint"), strings.Contains(sqlType, "serial"):
		return "int64"
	case strings.Contains(sqlType, "numeric"), strings.Contains(sqlType, "decimal"), strings.Contains(sqlType, "float"), strings.Contains(sqlType, "real"):
		return "float64"
	case strings.Contains(sqlType, "bool"):
		return "bool"
	case strings.Contains(sqlType, "text"), strings.Contains(sqlType, "char"), strings.Contains(sqlType, "varchar"):
		return "string"
	case strings.Contains(sqlType, "date"), strings.Contains(sqlType, "time"), strings.Contains(sqlType, "timestamp"):
		return "time.Time"
	case strings.Contains(sqlType, "json"), strings.Contains(sqlType, "blob"), strings.Contains(sqlType, "bytea"):
		return "[]byte"
	default:
		return "string"
	}
}

// promptsCrud runs interactive prompts for CRUD generation.
func promptsCrud() (entityName string, columns []colDef, err error) {
	fmt.Println("=== astra-cli generate crud — interactive mode ===")
	fmt.Println()

	entityName, err = promptString("Entity name", "Article")
	if err != nil {
		return
	}

	entityName = pascal(entityName)
	entityNameL := strings.ToLower(entityName)

	fmt.Println("\nColumn definitions (press Enter on a prompt to finish adding columns):")
	fmt.Println()

	var addMore bool
	for {
		colName, err := promptString("Column name (empty to finish)", "")
		if err != nil {
			return "", nil, err
		}
		if colName == "" {
			break
		}

		goType, err := promptSelect("Go type", []string{"string", "int64", "int", "float64", "bool", "time.Time", "[]byte"}, "string")
		if err != nil {
			return "", nil, err
		}

		nullable, _ := promptConfirm("Nullable", false)
		gormTag := fmt.Sprintf(`gorm:"column:%s"`, strings.ToLower(colName))
		if nullable {
			gormTag += ";default:null"
		}

		columns = append(columns, colDef{
			Name:    pascal(colName),
			JSONTag: fmt.Sprintf(`"%s"`, strings.ToLower(colName)),
			GoType:  goType,
			GORMCol: gormTag,
		})
		_ = addMore
		_ = entityNameL
	}

	// Ensure ID column always exists
	hasID := false
	for _, c := range columns {
		if c.Name == "ID" || c.Name == entityName+"ID" {
			hasID = true
			break
		}
	}
	if !hasID {
		// Prepend ID as first column
		columns = append([]colDef{
			{Name: "ID", JSONTag: `"id"`, GoType: "int64", GORMCol: `gorm:"primaryKey;autoIncrement"`, Comment: "// Primary key"},
		}, columns...)
	}

	return entityName, columns, nil
}
