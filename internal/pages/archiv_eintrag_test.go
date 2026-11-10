package pages

import (
	"strings"
	"testing"
)

// „Dein Archiv“ (owner 2026-11-10): under Mehr, and in the wide window's
// list, for the person who has an archive — nobody else, and nobody on a
// box without one.
func TestDeinArchivOnlyForItsPerson(t *testing.T) {
	const link = `href="https://archiv.box.test/"`
	for _, c := range []struct {
		name string
		cfg  StartConfig
		sees bool
	}{
		{"anna's archive", StartConfig{Archive: map[string]bool{"anna": true}, ArchivURL: "https://archiv.box.test"}, true},
		{"ben's archive only", StartConfig{Archive: map[string]bool{"ben": true}, ArchivURL: "https://archiv.box.test"}, false},
		{"no archive address", StartConfig{Archive: map[string]bool{"anna": true}}, false},
	} {
		_, body, _ := familyPage(t, Mehr, c.cfg, "GET", "/mehr", "family")
		if got := strings.Contains(body, link) && strings.Contains(body, "Dein Archiv"); got != c.sees {
			t.Errorf("%s: Mehr shows Dein Archiv %v, want %v", c.name, got, c.sees)
		}
		_, body, _ = familyPage(t, Start, c.cfg, "GET", "/start", "family")
		if got := strings.Contains(body, "href=https://archiv.box.test/>"); got != c.sees {
			t.Errorf("%s: the list beside Start shows it %v, want %v", c.name, got, c.sees)
		}
	}
}
