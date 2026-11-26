package tagaktion

import (
	"context"
	"net/http"
	"testing"

	"github.com/mdopp/homes/internal/laib"
)

// ADR 0004 §8: at the scanner a tag asks for whom (Homes' Scannen), or
// presses the scanner's button in HA (§6) for whoever touched it or for
// the household; while HA has no such button, Laib's scan order.

const scanner = "homes-5ca11e000001"

func TestScannerFragenLeadsToScannen(t *testing.T) {
	r := newRig(t)
	r.d.Store.Setze(scanner, ScannerFragen)
	e, err := r.d.Ausloesen(context.Background(), scanner, "anna")
	if err != nil || e.Aktion != ScannerFragen || e.Oeffnen != "https://www.example.org/dokumente#scannen" || e.Text == "" {
		t.Fatalf("%+v %v", e, err)
	}
	if len(r.ha.Calls()) != 0 || len(r.laib.Scanauftraege()) != 0 || len(r.ha.Events()) != 0 {
		t.Fatal("asking for whom started something")
	}
}

// With §6's buttons in HA: the toucher's own button, or the household's;
// never another person's, nothing to Laib.
func TestTheScannersButtonInHAIsPressed(t *testing.T) {
	r := newRig(t)
	r.ha.SetRegistries(nil, nil, []map[string]any{
		{"entity_id": "button.scanner_haushalt", "unique_id": KnopfHaushalt},
		{"entity_id": "button.scanner_anna", "unique_id": KnopfPerson + "anna"},
		{"entity_id": "button.scanner_ben", "unique_id": KnopfPerson + "ben"},
		{"entity_id": "button.scanner_alt", "unique_id": KnopfPerson + "michael", "disabled_by": "user"},
	})
	r.d.Store.Setze(scanner, ScannerIch)
	for _, who := range []string{"anna", "ben"} {
		e, err := r.d.Ausloesen(context.Background(), scanner, who)
		if err != nil || e.Text != "Der Scanner startet – nur für dich." {
			t.Fatalf("%s: %+v %v", who, e, err)
		}
	}
	r.d.Store.Setze(scanner, ScannerHaushalt)
	r.d.Ausloesen(context.Background(), scanner, "anna")
	calls := r.ha.Calls()
	if len(calls) != 3 {
		t.Fatalf("calls: %+v", calls)
	}
	for i, e := range []string{"button.scanner_anna", "button.scanner_ben", "button.scanner_haushalt"} {
		if c := calls[i]; c.Domain != "button" || c.Service != "press" || c.Data["entity_id"] != e {
			t.Errorf("call %d: %+v", i, c)
		}
	}
	if len(r.laib.Scanauftraege()) != 0 {
		t.Fatalf("laib too: %v", r.laib.Scanauftraege())
	}
	// a disabled button is none: michael's goes to Laib
	r.d.Store.Setze(scanner, ScannerIch)
	r.d.Ausloesen(context.Background(), scanner, "michael")
	if o := r.laib.Scanauftraege(); len(o) != 1 || o[0]["person"] != "michael" || o[0][laib.ScanFuerFeld] != "person" {
		t.Fatalf("fallback: %v", o)
	}
}

// Without §6 on the box: Laib's order, as before; its absence in words.
func TestWithoutTheButtonLaibsScanOrder(t *testing.T) {
	r := newRig(t)
	r.d.Store.Setze(scanner, ScannerHaushalt)
	if e, err := r.d.Ausloesen(context.Background(), scanner, "anna"); err != nil || e.Text != "Der Scanner startet – für den Haushalt." {
		t.Fatalf("%+v %v", e, err)
	}
	if o := r.laib.Scanauftraege(); len(o) != 1 || o[0]["action"] != "start" || o[0][laib.ScanFuerFeld] != nil {
		t.Fatalf("%v", o)
	}
	r.laib.OhneScan = true
	_, err := r.d.Ausloesen(context.Background(), scanner, "anna")
	neinIst(t, err, http.StatusServiceUnavailable, "kein Scanner")
	r.d.Store.Setze(scanner, ScannerIch)
	_, err = r.d.Ausloesen(context.Background(), scanner, "Kein Login!")
	neinIst(t, err, http.StatusForbidden, "Login")
}

func TestATagWithoutActionReportsToHA(t *testing.T) {
	r := newRig(t)
	e, err := r.d.Ausloesen(context.Background(), buero, "anna")
	if err != nil || e.Name != "Bürotür" || e.Aktion != "" {
		t.Fatalf("%+v %v", e, err)
	}
	if ev := r.ha.Events(); len(ev) != 1 || ev[0].Type != "tag_scanned" || ev[0].Data["person"] != "anna" {
		t.Fatalf("%+v", ev)
	}
	if r.d.URI(buero) != "https://www.example.org/tag/"+buero || (&Dienst{}).URI("x") != "cloud.dopp.homes://tag/x" {
		t.Fatal("uri")
	}
}
