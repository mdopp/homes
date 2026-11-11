package netz

import (
	"slices"
	"strings"
	"testing"
)

// Eintragen writes the relation into both household cards, each the other
// by its UID with the kind seen from it; the next read of the phone books
// shows it once, the answer's.
func TestEintragen(t *testing.T) {
	n, _, _ := netzBox(t)
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
	if got := bezuegeVon(n.Beziehungen(carla)); !slices.Equal(got, []string{"Anna Berg=kollege|Kollege/Kollegin|gesagt"}) {
		t.Fatalf("carla: %v", got)
	}
}

// Fremd (owner 2026-11-10: „die kennen sich nicht“) is kept as the
// answer about the pair and shown from both ends; forgetting it drops it.
func TestFremd(t *testing.T) {
	n, _, _ := netzBox(t)
	if _, _, err := n.Aendern("p-kim", "p-ben", Fremd); err != nil {
		t.Fatal(err)
	}
	if got := bezuegeVon(n.Beziehungen("p-kim")); !slices.Equal(got, []string{"Ben Berg=fremd|Kennen sich nicht|gesagt"}) {
		t.Fatalf("fremd: %v", got)
	}
	if _, _, err := n.Aendern("p-kim", "p-ben", ""); err != nil || len(n.Beziehungen("p-kim")) != 0 {
		t.Fatalf("forgotten: %v %v", err, n.Beziehungen("p-kim"))
	}
}
