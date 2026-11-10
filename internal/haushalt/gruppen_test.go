package haushalt

import (
	"slices"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/texte"
)

// owner 2026-11-10 („Gruppen“): a card's groups each once, Homes' „Firma“ none; all of them by name;
// a name to add or take away, refused when empty, too long or „Firma“.
func TestGruppen(t *testing.T) {
	anna := caldav.Contact{Categories: []string{"Stammtisch", "firma", "stammtisch", "Kickermannschaft Renegades"}}
	ben := caldav.Contact{Categories: []string{"Chor", "Stammtisch"}}
	if got := Gruppen(anna); !slices.Equal(got, []string{"Stammtisch", "Kickermannschaft Renegades"}) {
		t.Errorf("Gruppen %q", got)
	}
	if !InGruppe(anna, "STAMMTISCH") || InGruppe(anna, "Firma") || InGruppe(ben, "Kickermannschaft Renegades") {
		t.Error("InGruppe")
	}
	if got := AlleGruppen([]caldav.Contact{anna, ben, {}}); !slices.Equal(got, []string{"Chor", "Kickermannschaft Renegades", "Stammtisch"}) {
		t.Errorf("AlleGruppen %q", got)
	}
	for _, c := range []struct {
		dazu, weg string
		want      caldav.Korrektur
		nein      texte.ID
	}{
		{"Stammtisch", "", caldav.Korrektur{Prop: "CATEGORIES", Neu: "Stammtisch"}, ""},
		{"", "Chor", caldav.Korrektur{Prop: "CATEGORIES", Alt: "Chor"}, ""},
		{"", "", caldav.Korrektur{}, texte.GruppeOhneName},
		{strings.Repeat("ä", GruppeMax+1), "", caldav.Korrektur{}, texte.GruppeZuLang},
		{"FIRMA", "", caldav.Korrektur{}, texte.GruppeFirma},
	} {
		if k, nein := GruppeKorrektur(c.dazu, c.weg); k.Prop != c.want.Prop || k.Alt != c.want.Alt || k.Neu != c.want.Neu || nein != c.nein {
			t.Errorf("%q/%q: %+v %q", c.dazu, c.weg, k, nein)
		}
	}
}
