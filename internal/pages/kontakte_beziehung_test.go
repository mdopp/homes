package pages

import (
	"net/url"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/immich/immichfake"
	kontaktnetz "github.com/mdopp/homes/internal/netz"
)

// netzConfig: dieselbeConfig's Immich with photos: Anna and Ben on two
// days, Ben and Carla on two, Anna and Carla on two, Anna and Dora on one;
// the household has Anna and Ben; the net computed.
func netzConfig(t *testing.T) StartConfig {
	t.Helper()
	cfg, fake := dieselbeConfig(t, "a|Anna||anna@box.test", "b|Ben||")
	foto := func(d string, wer ...string) immichfake.Foto {
		return immichfake.Foto{ID: d + wer[1], Wann: d + "T12:00:00.000Z", Stadt: "Ulm", Wer: wer}
	}
	fake.Bibliothek = []immichfake.Foto{foto("2024-01-01", "p-anna", "p-ben"), foto("2025-01-01", "p-anna", "p-ben"),
		foto("2024-02-01", "p-ben", "p-carla"), foto("2025-02-01", "p-ben", "p-carla"), foto("2025-03-01", "p-anna", "p-dora"),
		foto("2023-04-01", "p-anna", "p-carla"), foto("2025-04-01", "p-anna", "p-carla")}
	cfg.Gesichter.Merke("anna", "anna@box.test")
	if err := cfg.Gesichter.Abgleichen(t.Context()); err != nil {
		t.Fatal(err)
	}
	cfg.Netz = &kontaktnetz.Netz{G: cfg.Gesichter}
	if err := cfg.Netz.Rechnen(t.Context()); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// The net lives in the person's archive (ADR 0002, owner 2026-11-11): Kontakte
// and a contact's page carry one link „Dein Netzwerk im Archiv ›“ to the
// archive's „Steht nahe“, for the viewer who has an archive, and nothing
// of the net itself; another person, an admin without an archive and a
// box without the archive's address see no link.
func TestNetzImArchiv(t *testing.T) {
	cfg := netzConfig(t)
	cfg.Archive, cfg.ArchivURL = map[string]bool{"anna": true}, "https://archiv.box.test"
	link := `<a href="https://archiv.box.test/tresor?tab=nahe">Dein Netzwerk im Archiv ›</a>`
	for _, body := range []string{as(Kontakte, cfg, KontaktePath, "anna", "anna@box.test").Body.String(), as(Kontakt, cfg, "/kontakte/b", "anna", "anna@box.test").Body.String()} {
		mustContain(t, body, link)
		mustNotContain(t, body, "/kontakte/netz", "Steht nahe", "Wer steht wem nahe", "Wie steht")
		if strings.Count(body, "Dein Netzwerk im Archiv") != 1 {
			t.Errorf("not one link:\n%s", body)
		}
	}
	for _, who := range []string{"ben", ""} {
		for _, body := range []string{as(Kontakte, cfg, KontaktePath, who, who+"@box.test").Body.String(), as(Kontakt, cfg, "/kontakte/b", who, who+"@box.test").Body.String()} {
			mustNotContain(t, body, "Dein Netzwerk im Archiv", "/tresor")
		}
	}
	mustNotContain(t, asAdmin(Kontakte, cfg, KontaktePath).Body.String(), "Dein Netzwerk im Archiv")
	cfg.ArchivURL = ""
	mustNotContain(t, as(Kontakte, cfg, KontaktePath, "anna", "anna@box.test").Body.String(), "Dein Netzwerk im Archiv")
}

// A contact's „Beziehungen“ (owner 2026-11-10: „Was ist los mit dem ux?“):
// one line per relation, the kind chosen, saved on choosing; Keine
// Beziehung is chosen when that was the answer and offers Vergessen;
// choosing Keine Beziehung takes a kind out of the cards, Vergessen drops
// the answer.
func TestNetzBeziehungZeile(t *testing.T) {
	cfg := netzConfig(t)
	post := func(v url.Values) string { return antworteBei(Kontakt(cfg), "/kontakte/b", "anna", v).Body.String() }
	post(url.Values{"x": {"p-ben"}, "y": {"p-anna"}, "antwort": {"aendern"}, "art": {"partner"}})
	post(url.Values{"x": {"p-ben"}, "y": {"p-carla"}, "antwort": {"aendern"}, "art": {"keine"}})
	body := as(Kontakt, cfg, "/kontakte/b", "anna", "anna@box.test").Body.String()
	mustContain(t, body, `<input type=hidden name=antwort value=aendern><label><span>Anna</span><select name=art aria-label="Anna ist …" onchange="this.form.requestSubmit()"><option value="partner" selected>Partner/in</option>`,
		`<option value="keine" selected>Keine Beziehung</option><option value="vergessen">Vergessen</option></select>`)
	mustNotContain(t, body, "<span>Anna</span><small", "Bitte wählen</option><option value=\"partner\" selected")
	mustContain(t, post(url.Values{"x": {"p-carla"}, "y": {"p-ben"}, "antwort": {"aendern"}, "art": {"vergessen"}}), "Vergessen")
	body = post(url.Values{"x": {"p-anna"}, "y": {"p-ben"}, "antwort": {"aendern"}, "art": {"keine"}})
	cards, _, _ := cfg.Contacts.Cards(t.Context())
	for _, c := range cards {
		if strings.Contains(c.Text(), "RELATED;TYPE=spouse") {
			t.Errorf("Keine Beziehung left the relation in card %s:\n%s\n%s", c.UID, c.Text(), body)
		}
	}
}
