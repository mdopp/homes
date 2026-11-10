package netz

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// Eintragen writes the relation into both household cards, each the other
// by its UID with the kind seen from it; the next read of the phone books
// shows it once, the answer's.
func TestEintragen(t *testing.T) {
	n, _, jetzt := netzBox(t)
	carla := n.Von("haushalt/c")
	x, y, err := n.Aendern(carla, "p-anna", "kollege")
	if err == nil {
		err = n.Eintragen(t.Context(), x, y, "kollege")
	}
	if err != nil {
		t.Fatal(err)
	}
	if x, y, _ := n.Aendern("p-kim", "p-anna", "kind"); n.Eintragen(t.Context(), x, y, "kind") != nil { // kim is no household contact: anna's card does not name her
		t.Fatal("kim")
	}
	cards, _, _ := n.G.Kontakte.Cards(t.Context())
	text := map[string]string{}
	for _, c := range cards {
		text[c.UID] = c.Text()
	}
	if !strings.Contains(text["a"], "RELATED;TYPE=colleague:urn:uuid:c\r\n") || !strings.Contains(text["c"], "RELATED;TYPE=colleague:urn:uuid:a\r\n") ||
		strings.Contains(text["a"], "Kim Berg") {
		t.Fatalf("the cards:\n%s\n%s", text["a"], text["c"])
	}
	*jetzt = jetzt.Add(25 * time.Hour)
	n.Rechnen(t.Context())
	nahe := n.Nahe("p-anna", 10)
	v := nahe[slices.IndexFunc(nahe, func(v Verbindung) bool { return v.Mit.ID == carla })]
	if got := n.Gruende("p-anna", v.Kante); got != "gleiche Firma · Kollege/Kollegin (von dir angegeben)" {
		t.Fatalf("carla: %q", got)
	}
}

// Fremd (owner 2026-11-10: „die kennen sich nicht“) takes the tie out of
// the net: neither end shows the other; forgetting it brings the tie back.
func TestFremd(t *testing.T) {
	n, _, _ := netzBox(t)
	nahe := func() bool {
		return slices.ContainsFunc(n.Nahe("p-kim", 10), func(k Verbindung) bool { return k.Mit.ID == "p-ben" })
	}
	if !nahe() {
		t.Fatal("kim–ben: no tie to start with")
	}
	if _, _, err := n.Aendern("p-kim", "p-ben", Fremd); err != nil {
		t.Fatal(err)
	}
	if nahe() {
		t.Fatal("fremd: still a tie")
	}
	if _, _, err := n.Aendern("p-kim", "p-ben", ""); err != nil || !nahe() {
		t.Fatalf("forgotten: %v, tie back %v", err, nahe())
	}
}
