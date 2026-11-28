package kontaktantwort

import (
	"fmt"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/gesichter"
)

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
