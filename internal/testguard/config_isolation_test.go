package testguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestConfigIsolation prevents tests from silently depending on a developer's
// default configuration. Test fixtures must always use a fresh explicit
// directory, normally config.WithConfigDir(t.TempDir()).
func TestConfigIsolation(t *testing.T) {
	root := repositoryRoot(t)
	fileset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "libs":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".py") && isUnderTests(root, path) {
			return checkPythonConfigIsolation(t, root, path)
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fileset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CallExpr:
				checkConfigCall(t, root, fileset, value)
			case *ast.FuncDecl:
				name := strings.ToLower(value.Name.Name)
				if strings.Contains(name, "config") && strings.Contains(name, "copy") {
					t.Errorf("%s: config-copy test helper %q is forbidden; construct an isolated fixture instead",
						position(root, fileset, value.Pos()), value.Name.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkPythonConfigIsolation(t *testing.T, root, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	for _, forbidden := range []string{
		"config_path or Path.home()",
		"CONFIG_PATH = os.path.expanduser",
		`Path.cwd() / "config.json"`,
	} {
		if strings.Contains(string(content), forbidden) {
			relative, _ := filepath.Rel(root, path)
			t.Errorf("%s: Python tests must not auto-discover the developer config (%q)",
				relative, forbidden)
		}
	}
	return nil
}

func isUnderTests(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	return len(parts) > 1 && parts[0] == "tests"
}

func checkConfigCall(t *testing.T, root string, fileset *token.FileSet, call *ast.CallExpr) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	switch selector.Sel.Name {
	case "GetTinglyConfDir":
		t.Errorf("%s: tests must not discover the developer config directory",
			position(root, fileset, call.Pos()))
	case "NewAppConfig", "NewConfig":
		if len(call.Args) == 0 {
			t.Errorf("%s: %s requires an explicit temporary config directory in tests",
				position(root, fileset, call.Pos()), selector.Sel.Name)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate config isolation guard")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func position(root string, fileset *token.FileSet, pos token.Pos) string {
	location := fileset.Position(pos)
	relative, err := filepath.Rel(root, location.Filename)
	if err != nil {
		relative = location.Filename
	}
	return fmt.Sprintf("%s:%d", relative, location.Line)
}
