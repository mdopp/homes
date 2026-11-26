package tagaktion

import (
	"os"
	"path/filepath"
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
	for a, want := range map[string]bool{"": true, ScannerFragen: true, ScannerIch: true, ScannerHaushalt: true, "scannen-ich": true, "licht": false} {
		if Gibt(a) != want {
			t.Errorf("%q: %v", a, !want)
		}
	}
}

// §4's names are read as §8's (ADR 0004 amendment): a tag set up before
// keeps doing what it did.
func TestTheOldNamesAreReadAsTheNew(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tag-aktionen.json"), []byte(`{"a":"scannen-ich","b":"scannen-haushalt","c":"scanner-fragen"}`), 0o600)
	s, err := Open(dir)
	if err != nil || s.Von("a") != ScannerIch || s.Von("b") != ScannerHaushalt || s.Von("c") != ScannerFragen {
		t.Fatalf("%v %q %q %q", err, s.Von("a"), s.Von("b"), s.Von("c"))
	}
	s.Setze("d", "scannen-haushalt")
	if s.Von("d") != ScannerHaushalt {
		t.Fatalf("set under the old name: %q", s.Von("d"))
	}
}
