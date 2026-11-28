package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// haus#26, laib ADR 0022 §4: nothing old is carried. No Go file — code or
// test — names the Solaris app, ServiceBay or the old token prefix; this
// rule's own test is the one place the words appear. Nor does production
// code name Haus' (laib#1021), but the start's one-time migration.
func TestNoOldNamesInGoCode(t *testing.T) {
	root := filepath.Join("..", "..")
	self := filepath.Join("internal", "app", "names_test.go")
	seen := 0
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == self {
			return nil
		}
		seen++
		b, _ := os.ReadFile(path)
		low := strings.ToLower(string(b))
		for _, name := range []string{"solaris", "servicebay", "sol_device"} {
			if strings.Contains(low, name) {
				t.Errorf("%s names %q (laib ADR 0022 §4: nothing old is carried)", filepath.ToSlash(rel), name)
			}
		}
		// Haus' names (laib#1021) are gone from the code (owner 2026-11-28); only the
		// start's migration of what was stored under them names them, and the tests
		if strings.HasSuffix(rel, "_test.go") || filepath.ToSlash(rel) == "cmd/homes/umbenennen.go" {
			return nil
		}
		for _, name := range []string{"haus_device_", "._haus-", "x-haus-inhalt", "haus haushalt", "haus takeout", "legacyaccountpath", "legacytokenprefix", "/napi/persona", "/napi/app\""} {
			if strings.Contains(low, name) {
				t.Errorf("%s names %q (owner 2026-11-28: nothing of Haus is carried)", filepath.ToSlash(rel), name)
			}
		}
		return nil
	})
	if seen < 20 {
		t.Fatalf("read %d Go files; the walk did not find the repo", seen)
	}
}
