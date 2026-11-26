package tagaktion

import "testing"

func TestTheActionSurvivesAReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil || s.Von("a") != "" {
		t.Fatalf("fresh: %v %q", err, s.Von("a"))
	}
	if err := s.Setze("a", ScannenIch); err != nil {
		t.Fatal(err)
	}
	s.Setze("b", ScannenHaushalt)
	s.Setze("b", "")
	again, err := Open(dir)
	if err != nil || again.Von("a") != ScannenIch || again.Von("b") != "" {
		t.Fatalf("reopened: %v %q %q", err, again.Von("a"), again.Von("b"))
	}
}

func TestGibt(t *testing.T) {
	for a, want := range map[string]bool{"": true, ScannenIch: true, ScannenHaushalt: true, "licht": false} {
		if Gibt(a) != want {
			t.Errorf("%q: %v", a, !want)
		}
	}
}
