package tagaktion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheActionSurvivesAReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil || s.Von("a") != "" {
		t.Fatalf("fresh: %v %q", err, s.Von("a"))
	}
	if err := s.Setze("a", ScannerIch); err != nil {
		t.Fatal(err)
	}
	s.Setze("b", ScannerHaushalt)
	s.Setze("b", "")
	again, err := Open(dir)
	if err != nil || again.Von("a") != ScannerIch || again.Von("b") != "" {
		t.Fatalf("reopened: %v %q %q", err, again.Von("a"), again.Von("b"))
	}
}

func TestGibt(t *testing.T) {
	for a, want := range map[string]bool{"": true, ScannerFragen: true, ScannerIch: true, ScannerHaushalt: true, "scannen-ich": false, "licht": false} {
		if Gibt(a) != want {
			t.Errorf("%q: %v", a, !want)
		}
	}
}

// §4's names are rewritten to §8's on start (ADR 0004 amendment, owner
// 2026-11-28): a tag set up before keeps doing what it did, the file holds
// the new name, and a second start changes nothing.
func TestTheOldNamesAreMigratedOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tag-aktionen.json")
	os.WriteFile(path, []byte(`{"a":"scannen-ich","b":"scannen-haushalt","c":"scanner-fragen"}`), 0o600)
	s, err := Open(dir)
	if err != nil || s.Von("a") != ScannerIch || s.Von("b") != ScannerHaushalt || s.Von("c") != ScannerFragen {
		t.Fatalf("%v %q %q %q", err, s.Von("a"), s.Von("b"), s.Von("c"))
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "scannen-") || !strings.Contains(string(b), `"scanner-haushalt"`) {
		t.Fatalf("the file still holds an old name: %s", b)
	}
	fi, _ := os.Stat(path)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if fj, _ := os.Stat(path); !fj.ModTime().Equal(fi.ModTime()) {
		t.Fatal("a second start wrote the file again")
	}
	if Gibt("scannen-ich") {
		t.Fatal("the old name is still an action")
	}
}
