package pages

import (
	"net/http"
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

// Only the key's owner: Kontakte links the page, the page draws the
// middle (the owner, by her card's e-mail) and her ties as links that
// move the middle, and lists them with reasons and both buttons; anyone
// else is sent to Kontakte and sees no link.
func TestNetzNurEigentuemer(t *testing.T) {
	cfg := netzConfig(t)
	mustContain(t, as(Kontakte, cfg, KontaktePath, "anna", "anna@box.test").Body.String(), `<a href="/kontakte/netz">Wer steht wem nahe</a>`)
	rec := as(NetzSeite, cfg, NetzPath, "anna", "anna@box.test")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, body)
	}
	mustContain(t, body, "<a href=/kontakte>Kontakte</a> › <span>Wer steht wem nahe</span></nav>", `<svg class=netzbild viewBox="0 0 400 400" role=img aria-label="Wer Anna nahe steht">`,
		`<a href="/kontakte/netz?mitte=p-ben" aria-label="Ben in die Mitte">`, `<g class=mitte><a href="/kontakte/netz?mitte=p-anna"`,
		`<a href="/kontakte/netz?mitte=p-ben">Ben</a> <a class=ruf href="/kontakte/b">Kontakt</a> <span class=k>2 Tage auf deinen Fotos seit 2024</span>`,
		`<button class=leise name=art value="aus" data-laeuft="Blendet aus …">Ausblenden</button>`, `value="nicht" data-laeuft="Nimmt heraus …">Nicht berechnen</button>`)
	mustNotContain(t, body, "Dora", "Mehr zeigen") // one day only
	ben := as(NetzSeite, cfg, NetzPath+"?mitte=p-ben", "anna", "anna@box.test").Body.String()
	mustContain(t, ben, `aria-label="Wer Ben nahe steht"`, `<line class=zw`, ">Carla</a>") // Anna–Carla, a tie among the shown
	for _, who := range []string{"ben", ""} {
		if rec := as(NetzSeite, cfg, NetzPath, who, who+"@box.test"); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != KontaktePath {
			t.Fatalf("%q: %d", who, rec.Code)
		}
	}
	mustNotContain(t, as(Kontakte, cfg, KontaktePath, "ben", "ben@box.test").Body.String(), NetzPath)
	mustNotContain(t, as(Kontakt, cfg, "/kontakte/a", "ben", "ben@box.test").Body.String(), "Steht nahe")
}

// On a contact's page the key's owner sees „Steht nahe“: each tie with its
// reasons, to its contact or into the picture, and the picture around it.
func TestNetzAufDemKontakt(t *testing.T) {
	cfg := netzConfig(t)
	body := as(Kontakt, cfg, "/kontakte/b", "anna", "anna@box.test").Body.String()
	mustContain(t, body, `<h2 id=karte-steht-nahe>Steht nahe</h2>`, `<a href="/kontakte/a">Anna</a> <span class=k>2 Tage auf deinen Fotos seit 2024</span>`,
		`<a href="/kontakte/netz?mitte=p-carla">Carla</a>`, `<a href="/kontakte/netz?mitte=p-ben">Wer steht wem nahe</a>`)
}

// Ausblenden and Nicht berechnen from the page, said on it; „Ausgeblendet“
// lists both and Zurückholen restores; only from Homes' own page.
func TestNetzVerbergenAufDerSeite(t *testing.T) {
	cfg := netzConfig(t)
	rec := antworteBei(NetzSeite(cfg), NetzPath, "anna", url.Values{"id": {"p-ben"}, "art": {"aus"}, "mitte": {"p-anna"}})
	body := rec.Body.String()
	mustContain(t, body, "Ben ist ausgeblendet.", `<h2 id=netz-verborgen>Ausgeblendet</h2>`, `value="" data-laeuft="Holt zurück …">Zurückholen</button>`)
	if strings.Contains(body, `mitte=p-ben"`) {
		t.Fatal("ben still shown")
	}
	body = antworteBei(NetzSeite(cfg), NetzPath, "anna", url.Values{"id": {"p-carla"}, "art": {"nicht"}, "mitte": {"p-anna"}}).Body.String()
	mustContain(t, body, "Carla wird nicht mehr berechnet", "Carla <span class=k>nicht berechnet</span>")
	body = antworteBei(NetzSeite(cfg), NetzPath, "anna", url.Values{"id": {"p-ben"}, "art": {""}, "mitte": {"p-anna"}}).Body.String()
	mustContain(t, body, "Ben ist wieder da.", `mitte=p-ben"`)
	mustContain(t, antworteBei(NetzSeite(cfg), NetzPath, "anna", url.Values{"id": {"p-ben"}, "art": {"weg"}}).Body.String(), "Das hat nicht geklappt")
	if rec := antworteBei(NetzSeite(cfg), NetzPath, "ben", url.Values{"id": {"p-anna"}, "art": {"aus"}}); rec.Code != http.StatusSeeOther || len(cfg.Netz.Verborgene()) != 1 {
		t.Fatalf("ben hid: %d %v", rec.Code, cfg.Netz.Verborgene())
	}
}

// Before the first run: one plain line, no picture.
func TestNetzLeer(t *testing.T) {
	cfg, _ := dieselbeConfig(t)
	cfg.Netz = &kontaktnetz.Netz{G: cfg.Gesichter}
	cfg.Gesichter.Merke("anna", "anna@box.test")
	body := as(NetzSeite, cfg, NetzPath, "anna", "anna@box.test").Body.String()
	mustContain(t, body, "Noch keine Verbindungen.")
	mustNotContain(t, body, "<svg class=netzbild")
}

// The questions moved into the person's archive (archiv#27, ADR 0002): the
// net's page and a contact's ask none; the page links to the archive's
// Umfeld for whom has one.
func TestNetzFragenImArchiv(t *testing.T) {
	cfg := netzConfig(t)
	cfg.Archive, cfg.ArchivURL = map[string]bool{"anna": true}, "https://archiv.box.test"
	body := as(NetzSeite, cfg, NetzPath, "anna", "anna@box.test").Body.String()
	mustContain(t, body, `<a href="https://archiv.box.test/tresor/umfeld">Wie die Menschen zueinander stehen, klärst du jetzt in deinem Archiv</a>`)
	mustNotContain(t, body, "Beziehungen klären", "Wie steht")
	mustNotContain(t, as(Kontakt, cfg, "/kontakte/b", "anna", "anna@box.test").Body.String(), "Wie steht", "<span class=anzahl")
	cfg.Archive = nil
	mustNotContain(t, as(NetzSeite, cfg, NetzPath, "anna", "anna@box.test").Body.String(), "/tresor/umfeld")
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
