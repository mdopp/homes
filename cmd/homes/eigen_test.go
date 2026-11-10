package main

import (
	"slices"
	"testing"

	"github.com/mdopp/homes/internal/modul"
	"github.com/mdopp/homes/internal/napi"
)

// The app's Dokumente-Adresse (owner 2026-11-10): <login>+doc+@<domain>, the
// domain the household's documents address's, to every person whose login
// fits an address; none without that address. „archiv“ to a person with a
// message archive only.
func TestEigenOf(t *testing.T) {
	s := site{docsMail: "dokumente@example.org", archive: map[string]napi.ArchivIntern{"anna": {}}}
	s.napi.Sicherung = napi.NewSicherung(t.TempDir(), t.TempDir())
	for uid, want := range map[string]struct {
		mehr    []string
		adresse string
	}{
		"anna":  {[]string{"archiv", "dokumente-adresse"}, "anna+doc+@example.org"},
		"Ben":   {[]string{"dokumente-adresse"}, "ben+doc+@example.org"},
		"an na": {nil, ""},
	} {
		k := modul.Kann{}
		eigenOf(s)(uid, &k)
		if !slices.Equal(k.App.Mehr, want.mehr) || k.DokumenteAdresse != want.adresse {
			t.Errorf("%q: %v %q, want %v %q", uid, k.App.Mehr, k.DokumenteAdresse, want.mehr, want.adresse)
		}
	}
	s.docsMail = "" // no household address (or the module dokumente off): no own address either
	k := modul.Kann{}
	eigenOf(s)("anna", &k)
	if !slices.Equal(k.App.Mehr, []string{"archiv"}) || k.DokumenteAdresse != "" {
		t.Errorf("without the household's address: %v %q", k.App.Mehr, k.DokumenteAdresse)
	}
}
