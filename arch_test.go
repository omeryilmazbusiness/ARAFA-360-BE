package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

const mod = "github.com/arafat-360/arafa-360-be/"

var frameworks = []string{"gofiber/", "jackc/pgx", "redis/go-redis", "getsentry/", "fasthttp"}

// TestDependencyRule enforces Clean Architecture: dependencies point inward
// (delivery -> usecase -> domain <- repository). A violation fails the build.
func TestDependencyRule(t *testing.T) {
	rules := []struct {
		dir       string
		forbidden []string
	}{
		// Innermost: pure business rules, no framework, no other layer.
		{"domain", append([]string{mod + "usecase", mod + "repository", mod + "delivery", mod + "infrastructure"}, frameworks...)},
		// Use cases know only the domain.
		{"usecase", append([]string{mod + "repository", mod + "delivery", mod + "infrastructure"}, frameworks...)},
		// Repositories implement domain ports; they never reach outward or sideways.
		{"repository/postgres", []string{mod + "usecase", mod + "delivery", mod + "repository/redis", "gofiber/", "redis/go-redis"}},
		{"repository/redis", []string{mod + "usecase", mod + "delivery", mod + "repository/postgres", "gofiber/", "jackc/pgx"}},
		// Delivery talks to use cases, never to storage.
		{"delivery", []string{mod + "repository", mod + "infrastructure", "jackc/pgx", "redis/go-redis"}},
		{"delivery/websocket", []string{mod + "delivery/http"}},
	}
	for _, r := range rules {
		assertNoImports(t, r.dir, r.forbidden)
	}
}

func assertNoImports(t *testing.T, dir string, forbidden []string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range f.Imports {
			imp := strings.Trim(spec.Path.Value, `"`)
			for _, bad := range forbidden {
				if strings.Contains(imp, bad) {
					t.Errorf("layer violation: %s imports %s", path, imp)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
