package kontaktantwort

import (
	"fmt"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/gesichter"
)

// owner 2026-11-09: „mache es zum Teil des Kontaktformulars“ — the
// relations by place: a kind changed, one taken away (a removal, only by
// a tick), a new one with its name and kind; one shown keeps its name, a
// new one without a kind or of a name there already is none. None goes
// into a card by Korrekturen: a relation is the net's.
func TestVergleichBeziehungen(t *testing.T) {
	vorher := Felder{Vorname: "Clara", Beziehungen: []Beziehung{{Name: "Anna Berg", Art: "eltern", Mit: "p-anna"}, {Name: "Ben Berg", Art: "eltern", Mit: "p-ben"}, {Name: "Kim", Art: "geschwister"}}}
	nachher := Sauber(Felder{Vorname: "Clara", Beziehungen: []Beziehung{{Name: "Anna Umbenannt", Art: "freund"}, {}, {Name: "Kim", Art: "geschwister"},
		{Name: " Oma  Hilde ", Art: "grosseltern"}, {Name: "Opa", Art: ""}, {Name: "kim", Art: "freund"}}})
	as := Vergleich(vorher, nachher)
	want := []Aenderung{
		{Art: "beziehung", Vorher: "Anna Berg", Nachher: "Anna Berg", VorherArt: "eltern", NachherArt: "freund", Mit: "p-anna"},
		{Art: "beziehung", Vorher: "Ben Berg", VorherArt: "eltern", Mit: "p-ben"},
		{Art: "beziehung", Nachher: "Oma Hilde", NachherArt: "grosseltern"},
	}
	if fmt.Sprint(as) != fmt.Sprint(want) {
		t.Fatalf("got %+v", as)
	}
	if !as[1].NurAngekreuzt() || as[0].NurAngekreuzt() {
		t.Fatal("only a removal waits for its tick")
	}
	f := Frage{Aenderungen: as}
	for i := range f.Aenderungen {
		f.Aenderungen[i].Stand = Angenommen
	}
	if ks, _ := Korrekturen(f); len(ks) != 0 {
		t.Fatalf("written into the card: %+v", ks)
	}
}

// A relation for the owner's phone (owner 2026-11-09): by the other's
// name, "" kind takes it away; a second one to the same other replaces the
// first, one to another or a rename of the same contact stays.
func TestHandyBeziehung(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	s := store(t, &now)
	for _, a := range []gesichter.HandyAenderung{
		{UID: "k1", AltVorname: "Juli", Vorname: "Julia"},
		{UID: "k1", AltVorname: "Juli Berg", Mit: "Kim Berg", Art: "kind"},
		{UID: "k1", AltVorname: "Juli Berg", Mit: "Ben Berg", Art: "partner"},
		{UID: "k1", AltVorname: "Juli Berg", Mit: "Kim Berg"},
	} {
		if err := s.Handy("anna", a); err != nil {
			t.Fatal(err)
		}
	}
	fh := s.FuersHandy("anna")
	if len(fh) != 3 || fh[0].Aenderungen[0].Art != "vorname" || fh[1].Name() != "Juli Berg" ||
		fmt.Sprint(fh[1].Aenderungen) != fmt.Sprint([]Aenderung{{Art: "beziehung", Vorher: "Ben Berg", Nachher: "Ben Berg", NachherArt: "partner", Stand: Angenommen}}) ||
		fmt.Sprint(fh[2].Aenderungen) != fmt.Sprint([]Aenderung{{Art: "beziehung", Vorher: "Kim Berg", Stand: Angenommen}}) {
		t.Fatalf("for the phone: %+v", fh)
	}
}
