package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/mustafa-oezdemir/banking_go/"

var allowedProjectImports = map[string]map[string]bool{
	"account":      {},
	"identity":     {},
	"notification": {},
	"ledger": allowed(
		"internal/account",
		"internal/ledger/domain",
	),
	"payment": allowed(
		"internal/account",
		"internal/ledger",
		"internal/ledger/domain",
		"internal/notification",
		"internal/payment/domain",
		"internal/platform/database",
		"postgres/sqlc",
	),
	"platform/bootstrap": allowed(
		"internal/account",
		"internal/identity",
		"internal/ledger",
		"internal/payment",
		"internal/platform/database",
		"postgres/sqlc",
	),
	"platform/database": allowed(
		"internal/account",
		"internal/identity",
		"internal/ledger",
		"internal/ledger/domain",
		"postgres/sqlc",
	),
	"platform/email": allowed(
		"internal/account",
		"internal/notification",
		"internal/platform/database",
	),
	"platform/httpapi": allowed(
		"internal/account",
		"internal/identity",
		"internal/ledger",
		"internal/notification",
		"internal/payment",
		"internal/platform/database",
		"postgres/sqlc",
	),
}

var forbiddenBusinessInfrastructure = []string{
	"github.com/go-chi/",
	"github.com/go-chi/chi/",
	"github.com/lib/pq",
	"github.com/resend/",
	"net/http",
	"net/smtp",
}

func TestModuleDependencyRules(t *testing.T) {
	internalRoot := filepath.Clean("..")
	err := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		relative, err := filepath.Rel(internalRoot, path)
		if err != nil {
			return err
		}
		owner := ownerForPath(filepath.ToSlash(relative))
		if owner == "" {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			checkImport(t, owner, filepath.ToSlash(relative), importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect module imports: %v", err)
	}
}

func TestInternalPackageRootsAreIntentional(t *testing.T) {
	assertOnlyDirectories(t, "..", map[string]bool{
		"account": true, "architecture": true, "identity": true,
		"ledger": true, "notification": true, "payment": true, "platform": true,
	})
	assertOnlyDirectories(t, filepath.Join("..", "platform"), map[string]bool{
		"bootstrap": true, "database": true, "email": true, "httpapi": true,
	})
}

func TestCoreDomainPackagesAreInfrastructureFree(t *testing.T) {
	domainRules := map[string]map[string]bool{
		"account":        {},
		"identity":       {},
		"ledger/domain":  allowed("internal/account"),
		"payment/domain": {},
	}
	for domainPath, allowedImports := range domainRules {
		root := filepath.Join("..", filepath.FromSlash(domainPath))
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read domain package %s: %v", domainPath, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(root, entry.Name())
			parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseErr != nil {
				t.Fatalf("parse domain file %s: %v", path, parseErr)
			}
			for _, imported := range parsed.Imports {
				importPath, unquoteErr := strconv.Unquote(imported.Path.Value)
				if unquoteErr != nil {
					t.Fatalf("read import in %s: %v", path, unquoteErr)
				}
				if strings.HasPrefix(importPath, modulePath) {
					target := strings.TrimPrefix(importPath, modulePath)
					if !allowedImports[target] {
						t.Errorf("domain %s imports forbidden project package %q", domainPath, target)
					}
				}
				for _, forbidden := range []string{"database/sql", "net/http", "os", "github.com/go-chi/", "github.com/lib/pq", "github.com/resend/"} {
					if importPath == forbidden || strings.HasPrefix(importPath, forbidden) {
						t.Errorf("domain %s imports infrastructure %q", domainPath, importPath)
					}
				}
			}
		}
	}
}

func assertOnlyDirectories(t *testing.T, root string, allowedDirectories map[string]bool) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read architecture directory %s: %v", root, err)
	}
	for _, entry := range entries {
		if entry.IsDir() && directoryContainsGoFiles(t, filepath.Join(root, entry.Name())) && !allowedDirectories[entry.Name()] {
			t.Errorf("unexpected package directory %s; assign a domain owner and update the architecture decision", filepath.Join(root, entry.Name()))
		}
	}
}

func directoryContainsGoFiles(t *testing.T, directory string) bool {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read package directory %s: %v", directory, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			return true
		}
	}
	return false
}

func checkImport(t *testing.T, owner, file, importPath string) {
	t.Helper()
	if isBusinessModule(owner) {
		for _, forbidden := range forbiddenBusinessInfrastructure {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden) {
				t.Errorf("%s module imports forbidden infrastructure %q in %s", owner, importPath, file)
			}
		}
	}

	if !strings.HasPrefix(importPath, modulePath) {
		return
	}
	target := strings.TrimPrefix(importPath, modulePath)
	if !allowedProjectImports[owner][target] {
		t.Errorf("%s module imports forbidden project package %q in %s", owner, target, file)
	}
}

func ownerForPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	if parts[0] == "platform" && len(parts) > 1 {
		return strings.Join(parts[:2], "/")
	}
	if _, known := allowedProjectImports[parts[0]]; known {
		return parts[0]
	}
	return ""
}

func isBusinessModule(owner string) bool {
	switch owner {
	case "account", "identity", "ledger", "notification", "payment":
		return true
	default:
		return false
	}
}

func allowed(imports ...string) map[string]bool {
	result := make(map[string]bool, len(imports))
	for _, importPath := range imports {
		result[importPath] = true
	}
	return result
}
