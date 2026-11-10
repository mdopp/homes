package kontaktantwort

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/gesichter"
)

// owner 2026-11-10 (ADR 0002): the form's „Beziehungen“ is cut. An
// answer kept while it was there still loads: its relation changes (and
// the old fields) are dropped, its other changes wait for their review;
// a relation for the phone (no link, no Hash) stays.
func TestAltesFormularMitBeziehung(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	s := store(t, &now)
	alt := `[{"id":"a1","hash":"h","wer":"anna","buch":"haushalt","uid":"c1","vorher":{"vorname":"Clara","netz":true,"beziehungen":[{"name":"Anna Berg","art":"eltern","mit":"p-anna"}]},` +
		`"am":"2026-11-09T10:00:00Z","bis":"2026-11-23T10:00:00Z","beantwortet":"2026-11-09T11:00:00Z",` +
		`"aenderungen":[{"art":"vorname","vorher":"Clara","nachher":"Klara"},{"art":"beziehung","vorher":"Anna Berg","nachher":"Anna Berg","vorher_art":"eltern","nachher_art":"freund","mit":"p-anna"}]},` +
		`{"id":"h1","hash":"","wer":"anna","buch":"privat","uid":"k1","vorher":{"vorname":"Juli"},"am":"2026-11-09T10:00:00Z","bis":"2026-11-09T10:00:00Z",` +
		`"beantwortet":"2026-11-09T10:00:00Z","entschieden":"2026-11-09T10:00:00Z","aenderungen":[{"art":"beziehung","vorher":"Kim Berg","nachher":"Kim Berg","nachher_art":"kind","stand":"angenommen"}]}]`
	if err := os.WriteFile(s.Path, []byte(alt), 0o600); err != nil {
		t.Fatal(err)
	}
	offen := s.ZuPruefen("anna")
	if len(offen) != 1 || fmt.Sprint(offen[0].Aenderungen) != fmt.Sprint([]Aenderung{{Art: "vorname", Vorher: "Clara", Nachher: "Klara"}}) {
		t.Fatalf("the pending answer: %+v", offen)
	}
	if _, err := s.Entscheiden("a1", "anna", []string{Angenommen}); err != nil {
		t.Fatalf("its review: %v", err)
	}
	if fh := s.FuersHandy("anna"); len(fh) != 1 || fh[0].Aenderungen[0].Art != "beziehung" {
		t.Fatalf("the phone's relation: %+v", fh)
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
