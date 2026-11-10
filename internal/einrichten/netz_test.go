package einrichten

import "testing"

// owner 2026-11-09: the open relation questions („Wie steht X zu Y?“) are a
// task beside Immich's, counted as they are, leading to /kontakte/netz.
func TestNetzFragenAlsAufgabe(t *testing.T) {
	q := box(t, fotos)
	q.Immich = func(string) int { return 1 }
	q.Netz = func(p string) int { return map[string]int{"anna": 4}[p] }
	fr := q.Fragen("anna")
	if len(fr) != 2 || fr[1].Modul != NetzKlaeren || fr[1].Titel != "4 Beziehungen klären" || fr[1].Pfad != NetzPfad || fr[0].Pfad != ImmichPfad {
		t.Fatalf("%+v", fr)
	}
	a := q.Napi("anna")
	if last := a.Module[len(a.Module)-1]; a.Offen != 3 || last.Name != NetzKlaeren || last.Web != "https://homes.box.test/kontakte/netz" {
		t.Fatalf("%d %+v", a.Offen, last)
	}
	if fr := q.Fragen("ben"); len(fr) != 1 || fr[0].Modul != ImmichPruefen {
		t.Fatalf("ben: %+v", fr)
	}
}
