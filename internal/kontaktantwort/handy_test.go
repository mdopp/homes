package kontaktantwort

import (
	"fmt"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/gesichter"
)

// Homes' own changes for the phone (owner 2026-11-09, „App erweitern“):
// decided already, served to the owner's phone only, never on a review
// page; a rename of the same contact replaces the one still waiting; a
// new contact (UID "") carries its name and face; the phone's report
// ends each, as a correction's does.
func TestHandyAenderungen(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	s := store(t, &now)
	if err := s.Handy("anna", gesichter.HandyAenderung{UID: "k1", AltVorname: "Juli", Vorname: "Juliane", Nachname: "Huesmann"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Handy("anna", gesichter.HandyAenderung{UID: "k1", AltVorname: "Juli", Vorname: "Julia", Nachname: "Huesmann"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Handy("anna", gesichter.HandyAenderung{Vorname: "Yvonne", Nachname: "Huber", Bild: []byte{0xff, 0xd8}}); err != nil {
		t.Fatal(err)
	}
	fh := s.FuersHandy("anna")
	if len(fh) != 2 || len(s.FuersHandy("ben")) != 0 || len(s.ZuPruefen("anna")) != 0 {
		t.Fatalf("for the phone: %+v", fh)
	}
	um, neu := fh[0], fh[1]
	if um.UID != "k1" || um.Name() != "Juli" || fmt.Sprint(um.Aenderungen[0]) != fmt.Sprint(Aenderung{Art: "vorname", Vorher: "Juli", Nachher: "Julia", Stand: Angenommen}) || um.Aenderungen[1].Nachher != "Huesmann" {
		t.Fatalf("rename: %+v", um)
	}
	if neu.UID != "" || neu.Name() != "Yvonne Huber" || len(neu.Aenderungen) != 3 || neu.Aenderungen[2].Art != "foto" || len(neu.Aenderungen[2].Bild) != 2 {
		t.Fatalf("new: %+v", neu)
	}
	if _, err := s.VomHandy(neu.ID, "anna", []int{0, 1, 2}); err != nil {
		t.Fatal(err)
	}
	if fh := s.FuersHandy("anna"); len(fh) != 1 || fh[0].UID != "k1" {
		t.Fatalf("after the report: %+v", fh)
	}
}
