package netz

import (
	"slices"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/gesichter"
)

func bezuegeVon(bs []Bezug) (out []string) {
	for _, b := range bs {
		out = append(out, b.Mit.Name+"="+b.Art+"|"+b.Wie+map[bool]string{true: "|gesagt"}[b.Gesagt])
	}
	return out
}

// owner 2026-11-09: „inkl. Änderbarkeit“ — a node's relations from both
// ends, the phone books' seen from the other side too; an answer changes
// one (the household cards' RELATED rewritten, line by line), Keine takes
// it away (the lines go), "" forgets the answer and the phone book is back.
func TestAendern(t *testing.T) {
	n, _, _ := netzBox(t)
	if got := bezuegeVon(n.Beziehungen("p-ben")); !slices.Equal(got, []string{"Anna Berg=partner|Partner/in"}) {
		t.Fatalf("ben: %v", got)
	}
	if got := bezuegeVon(n.Beziehungen("p-anna")); len(got) != 2 || got[0] != "Ben Berg=partner|Ehepartner/in" || !strings.HasPrefix(got[1], "Fritz Kurz=") {
		t.Fatalf("anna: %v", got)
	}
	karte := func(uid string) string {
		cards, _, _ := n.G.Kontakte.Cards(t.Context())
		return cards[slices.IndexFunc(cards, func(c caldav.Card) bool { return c.UID == uid })].Text()
	}
	for _, c := range []struct{ art, want, a string }{
		{"freund", "Ben Berg=freund|Freund/in|gesagt", "RELATED;TYPE=friend:urn:uuid:b\r\n"},
		{"partner", "Ben Berg=partner|Partner/in|gesagt", "RELATED;TYPE=spouse:urn:uuid:b\r\n"},
		{Keine, "Ben Berg=keine|Keine Beziehung|gesagt", ""},
	} {
		x, y, err := n.Aendern("p-ben", "p-anna", c.art)
		if err == nil {
			err = n.Eintragen(t.Context(), x, y, c.art)
		}
		if got := bezuegeVon(n.Beziehungen("p-anna")); err != nil || got[0] != c.want {
			t.Fatalf("%s: %v %v", c.art, got, err)
		}
		a := karte("a")
		if strings.Count(a, "\nRELATED") != map[bool]int{true: 1}[c.a != ""] || !strings.Contains(a, c.a) || strings.Count(karte("b"), "\nRELATED") != strings.Count(a, "\nRELATED") {
			t.Fatalf("%s: the cards:\n%s\n%s", c.art, a, karte("b"))
		}
	}
	if _, _, err := n.Aendern("p-ben", "p-anna", ""); err != nil || bezuegeVon(n.Beziehungen("p-ben"))[0] != "Anna Berg=partner|Partner/in" {
		t.Fatalf("forgotten: %v %v", err, n.Beziehungen("p-ben"))
	}
	if _, _, err := n.Aendern("p-ben", "p-nobody", "freund"); err != ErrKeineFrage {
		t.Fatalf("no such node: %v", err)
	}
}

// The key owner's own phone contact gets a relation through the phone
// (owner 2026-11-09), by name; Keine asks the phone to take it away.
func TestEintragenAufsHandy(t *testing.T) {
	n, _, _ := netzBox(t)
	var got []gesichter.HandyAenderung
	n.G.Handy = func(wer string, a gesichter.HandyAenderung) error {
		if wer == "anna" {
			got = append(got, a)
		}
		return nil
	}
	fritz, _ := n.Person("p-fritz")
	kim, _ := n.Person("p-kim")
	n.Eintragen(t.Context(), kim, fritz, "neffe")
	n.Eintragen(t.Context(), kim, fritz, Keine)
	want := []gesichter.HandyAenderung{{UID: "urn:uuid:k2", AltVorname: "Fritz Kurz", Mit: "Kim Berg", Art: "neffe"}, {UID: "urn:uuid:k2", AltVorname: "Fritz Kurz", Mit: "Kim Berg"}}
	same := func(a, b gesichter.HandyAenderung) bool {
		return a.UID == b.UID && a.AltVorname == b.AltVorname && a.Mit == b.Mit && a.Art == b.Art
	}
	if !strings.HasPrefix(fritz.Kontakt, "privat/") || !slices.EqualFunc(got, want, same) {
		t.Fatalf("%q: %+v", fritz.Kontakt, got)
	}
}

// A household contact and the key owner's phone contact: only the phone
// gets the relation; the household card, which the family reads, does not
// name the phone contact (owner 2026-11-10: „Das Leck bitte schließen“).
func TestEintragenPrivatBleibtPrivat(t *testing.T) {
	n, _, _ := netzBox(t)
	var got []gesichter.HandyAenderung
	n.G.Handy = func(_ string, a gesichter.HandyAenderung) error { got = append(got, a); return nil }
	anna, _ := n.Person("p-anna")
	fritz, _ := n.Person("p-fritz")
	if err := n.Eintragen(t.Context(), fritz, anna, "freund"); err != nil {
		t.Fatal(err)
	}
	cards, _, _ := n.G.Kontakte.Cards(t.Context())
	for _, c := range cards {
		if strings.Contains(c.Text(), "Fritz Kurz") {
			t.Fatalf("household card %s names the phone contact:\n%s", c.UID, c.Text())
		}
	}
	if len(got) != 1 || got[0].Mit != "Anna Beispiel" && got[0].Mit != anna.Name {
		t.Fatalf("phone: %+v", got)
	}
}

// Offen counts the questions for the key owner only.
func TestOffen(t *testing.T) {
	n, _, _ := netzBox(t)
	if n.Offen("anna") != len(n.Fragen()) || n.Offen("anna") == 0 || n.Offen("ben") != 0 {
		t.Fatalf("offen %d of %d, ben %d", n.Offen("anna"), len(n.Fragen()), n.Offen("ben"))
	}
}
