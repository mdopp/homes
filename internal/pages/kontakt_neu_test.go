package pages

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/laib"
)

// einladenLink makes a new person's link as anna (an admin) and returns its token.
func einladenLink(t *testing.T, cfg StartConfig) string {
	t.Helper()
	req := httptest.NewRequest("POST", KontaktEinladenPath, strings.NewReader(""))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Remote-User", "anna")
	req.Header.Set("Remote-Name", "Anna Beispiel")
	req.Header.Set("Remote-Groups", "family,admins")
	rec := httptest.NewRecorder()
	KontaktEinladen(cfg).ServeHTTP(rec, req)
	m := linkRE.FindStringSubmatch(rec.Body.String())
	if rec.Code != 200 || m == nil || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || !strings.Contains(rec.Body.String(), "7 Tage und nur einmal") {
		t.Fatalf("no link (%d):\n%s", rec.Code, rec.Body.String())
	}
	return m[1]
}

// owner 2026-11-28: Kontakte's button and its link, an admin's only; a GET goes back to Kontakte.
func TestKontaktEinladenOnlyForAnAdmin(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	mustContain(t, frag(Kontakte(cfg), "GET", KontaktePath, "anna", "family,admins", nil).Body.String(),
		`<form method=post action=/kontakte/einladen class=einladen data-teilen>`, "Jemanden seine Kontaktdaten eintragen lassen", `data-laeuft="Erstellt den Link …"`)
	mustNotContain(t, frag(Kontakte(cfg), "GET", KontaktePath, "ben", "family", nil).Body.String(), "/kontakte/einladen")
	einladenLink(t, cfg)
	if rec := frag(KontaktEinladen(cfg), "POST", KontaktEinladenPath, "ben", "family", url.Values{}); rec.Code != 403 || !strings.Contains(rec.Body.String(), "Admin") {
		t.Errorf("a family member: %d %s", rec.Code, rec.Body.String())
	}
	if rec := frag(KontaktEinladen(cfg), "POST", KontaktEinladenPath, "anna", "family,admins", nil); rec.Code != 403 {
		t.Errorf("from elsewhere: %d", rec.Code)
	}
	if rec := frag(KontaktEinladen(cfg), "GET", KontaktEinladenPath, "anna", "family,admins", nil); rec.Code != http.StatusSeeOther {
		t.Errorf("a GET: %d", rec.Code)
	}
	off := cfg
	off.KontaktURL = ""
	mustNotContain(t, frag(Kontakte(off), "GET", KontaktePath, "anna", "family,admins", nil).Body.String(), "/kontakte/einladen")
}

// The public page of such a link: the same form, empty, titled for a new person, without
// „Alles stimmt so“; a name is needed; then thanks, and the link is used up — and one over says so too.
func TestKontaktEinladenPublicForm(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	token := einladenLink(t, cfg)
	pub := KontaktServer(cfg)
	body := frag(pub, "GET", "/"+token, "", "", nil).Body.String()
	mustContain(t, body, "<h1>Trag deine Kontaktdaten ein</h1>", "Damit dich Anna im Adressbuch hat", `name=vorname value=""`, `name=tel value=""`,
		`name=mail value=""`, `name=adr-strasse value=""`, "geb-tag", "Angaben senden", "Der Link gilt bis ")
	mustNotContain(t, body, "Alles stimmt so", "Anna Beispiel", "+49 151 23456789", "class=papierkorb formnovalidate")

	ohne := url.Values{"vorname": {""}, "nachname": {" "}, "mail": {"erika@example.org"}, "aktion": {"senden"}}
	rec := frag(pub, "POST", "/"+token, "", "", ohne)
	mustContain(t, rec.Body.String(), "Trag deinen Vor- oder Nachnamen ein.", `value="erika@example.org"`)
	if rec.Code != 200 || len(cfg.Antworten.ZuPruefen("anna")) != 0 {
		t.Fatalf("an answer without a name kept (%d)", rec.Code)
	}
	answer := url.Values{"vorname": {"Erika"}, "nachname": {"Muster"}, "tel": {"0170 1234567", ""}, "tel-art": {"mobil", "mobil"}, "mail": {"erika@example.org", ""},
		"adr-strasse": {"Neuer Weg 2"}, "adr-plz": {"12345"}, "adr-ort": {"Stadt"}, "geb-tag": {"12"}, "geb-monat": {"4"}, "aktion": {"senden"}}
	rec = frag(pub, "POST", "/"+token, "", "", answer)
	mustContain(t, rec.Body.String(), "Danke – Anna sieht deine Angaben und übernimmt sie.")
	if len(dav.Objects()) != 2 {
		t.Errorf("an address book written before the review: %d objects", len(dav.Objects()))
	}
	open := cfg.Antworten.ZuPruefen("anna")
	if len(open) != 1 || !open[0].Neu || len(open[0].Hinweise) != 0 {
		t.Fatalf("to review: %+v", open)
	}
	if rec := frag(pub, "GET", "/"+token, "", "", nil); rec.Code != 404 || !strings.Contains(rec.Body.String(), "Link ungültig oder abgelaufen") {
		t.Errorf("a used link: %d", rec.Code)
	}
	alt := einladenLink(t, cfg)
	at := time.Now().Add(7*24*time.Hour + time.Minute)
	cfg.Antworten.Now = func() time.Time { return at }
	later := cfg
	if rec := frag(KontaktServer(later), "GET", "/"+alt, "", "", nil); rec.Code != 404 || !strings.Contains(rec.Body.String(), "Link ungültig oder abgelaufen") {
		t.Errorf("a link over: %d", rec.Code)
	}
	for _, secret := range []string{"Erika", "Muster", "erika@example.org", token} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("the log holds %q", secret)
		}
	}
}

// neuBeantwortet is a new person's answer of anna's waiting, its id.
func neuBeantwortet(t *testing.T, cfg StartConfig, answer url.Values) string {
	t.Helper()
	token := einladenLink(t, cfg)
	answer.Set("aktion", "senden")
	if rec := frag(KontaktServer(cfg), "POST", "/"+token, "", "", answer); !strings.Contains(rec.Body.String(), "Danke") {
		t.Fatalf("not answered:\n%s", rec.Body.String())
	}
	open := cfg.Antworten.ZuPruefen("anna")
	return open[len(open)-1].ID
}

// The review, per viewer: anna who asked sees it at its own address (the task leads there), before →
// after with nothing before; ben (another person) and petra (an admin, not anna) do not.
func TestKontaktNeuReviewPerViewer(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	id := neuBeantwortet(t, cfg, url.Values{"vorname": {"Erika"}, "nachname": {"Muster"}, "mail": {"erika@example.org"}, "mail-art": {"privat"}})
	if rec := frag(Antworten(cfg), "GET", AntwortenPath, "anna", "family,admins", nil); rec.Header().Get("Location") != AntwortenPath+"/"+id {
		t.Errorf("the task's address leads to %q", rec.Header().Get("Location"))
	}
	r := httptest.NewRequest("GET", "/aufgaben", nil)
	r.Header.Set("Remote-User", "anna")
	if as := einrichtung(r, cfg); len(as) == 0 || as[0].titel != "1 Kontakt-Antwort prüfen" {
		t.Errorf("Deine Aufgaben: %+v", as)
	}
	body := frag(Antworten(cfg), "GET", AntwortenPath+"/"+id, "anna", "family,admins", nil).Body.String()
	mustContain(t, body, "<h1>Neuer Kontakt: Erika Muster</h1>", "Nichts kommt ins Adressbuch, bevor du es übernimmst.",
		`<span class=nachher>erika@example.org (privat)</span> <span class=k>(neu)</span>`, `checked disabled title="Der Name kommt immer mit."`,
		`<button name=aktion value=anlegen data-laeuft="Legt an …">Ins Adressbuch übernehmen</button>`, `value=verwerfen`,
		"<h2>Ist das jemand, den du schon hast?</h2><p class=k>Im Adressbuch hat niemand dieselbe Nummer, dieselbe E-Mail-Adresse oder einen ähnlichen Namen.</p><h2>Neu ins Adressbuch</h2>")
	mustNotContain(t, body, "damit ergänzen", "Damit zusammenführen")
	for _, wer := range []struct{ user, groups string }{{"ben", "family"}, {"petra", "family,admins"}} {
		rec := frag(Antworten(cfg), "GET", AntwortenPath+"/"+id, wer.user, wer.groups, nil)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != KontaktePath {
			t.Errorf("%s: %d %q", wer.user, rec.Code, rec.Header().Get("Location"))
		}
		if rec := frag(Antworten(cfg), "POST", AntwortenPath+"/"+id, wer.user, wer.groups, url.Values{"id": {id}, "aktion": {"anlegen"}}); rec.Code != http.StatusSeeOther {
			t.Errorf("%s decides anna's: %d", wer.user, rec.Code)
		}
		mustNotContain(t, frag(Kontakte(cfg), "GET", KontaktePath, wer.user, wer.groups, nil).Body.String(), "Kontakt-Antwort")
	}
	if len(cfg.Antworten.ZuPruefen("anna")) != 1 {
		t.Error("decided by someone else")
	}
}

// „Als neuen Kontakt anlegen“: done, said where it was pressed, the card written with the ticked;
// the page then links to the contact and offers nothing more.
func TestKontaktNeuAnlegen(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	id := neuBeantwortet(t, cfg, url.Values{"vorname": {"Erika"}, "nachname": {"Muster"}, "tel": {"0170 1234567"}, "tel-art": {"mobil"}, "mail": {"erika@example.org"}})
	// vorname 0, nachname 1, telefon 2, mail 3: the mail not ticked
	rec := frag(Antworten(cfg), "POST", AntwortenPath+"/"+id, "anna", "family,admins", url.Values{"id": {id}, "aktion": {"anlegen"}, "nimm": {"2"}})
	body := rec.Body.String()
	mustContain(t, body, "Angelegt: Erika Muster ist jetzt im Adressbuch des Haushalts.", "Zum Kontakt ›", "übernommen", "nicht übernommen")
	mustNotContain(t, body, "value=anlegen", "<input type=checkbox")
	card := ""
	for p, v := range dav.Objects() {
		if strings.Contains(v, "FN:Erika Muster") {
			card = p + "\n" + v
		}
	}
	mustContain(t, card, "/homes/haushalt-kontakte/", "FN:Erika Muster", "TEL", "170")
	mustNotContain(t, card, "erika@example.org")
	if len(cfg.Antworten.ZuPruefen("anna")) != 0 {
		t.Error("still open")
	}
	// pressed again (a second tab): nothing happens twice
	n := len(dav.Objects())
	frag(Antworten(cfg), "POST", AntwortenPath+"/"+id, "anna", "family,admins", url.Values{"id": {id}, "aktion": {"anlegen"}})
	if len(dav.Objects()) != n {
		t.Error("a second card")
	}
}

// A household card with the same number (owner 2026-11-28): the review asks first whether it is
// that person, the card with its reason and „Damit zusammenführen“, the new card below; picked, the
// answer before → after against that card with „<Name> damit ergänzen“ and the way back; a card
// that is no candidate is never offered by its address. Written into that card; Verwerfen writes
// nothing; Radicale down is said in words, nothing kept.
func TestKontaktNeuErgaenzenVerwerfenFehler(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	id := neuBeantwortet(t, cfg, url.Values{"vorname": {"Anne"}, "nachname": {"Beispiel"}, "tel": {"0151 23456789"}, "tel-art": {"mobil"}, "geb-tag": {"3"}, "geb-monat": {"5"}, "geb-jahr": {"1990"}})
	action := AntwortenPath + "/" + id
	body := frag(Antworten(cfg), "GET", action, "anna", "family,admins", nil).Body.String()
	mustContain(t, body, "<h2>Ist das jemand, den du schon hast?</h2><ul class=kandidaten><li><span><b>Anna Beispiel</b><br><span class=k>gleiche Mobilnummer</span></span>"+
		`<a class=knopf href="`+action+`?ziel=h1">Damit zusammenführen</a></li></ul><h2>Keiner davon? Dann neu ins Adressbuch</h2>`, "Ins Adressbuch übernehmen", "03.05.1990")
	mustNotContain(t, body, "damit ergänzen", `name=ziel`)
	body = frag(Antworten(cfg), "GET", action+"?ziel=h1", "anna", "family,admins", nil).Body.String()
	mustContain(t, body, "<h2>Mit Anna Beispiel zusammenführen</h2>", `<input type=hidden name=ziel value="h1">`, "Anna Beispiel damit ergänzen", "03.05.1990",
		`value=0><span><span class=art>Vorname</span><br><span class=vorher>Anna</span> → <span class=nachher>Anne</span>`, // over the card's value: not ticked
		`<a href="`+action+`">‹ Doch nicht – zurück zur Auswahl</a>`)
	mustNotContain(t, body, "Ins Adressbuch übernehmen", "Damit zusammenführen")
	for _, z := range []string{"3789r1-2B4D", "nichts"} { // anna's Handy-Kontakt, and no card: the choice again
		mustContain(t, frag(Antworten(cfg), "GET", action+"?ziel="+z, "anna", "family,admins", nil).Body.String(), "Damit zusammenführen", "Ins Adressbuch übernehmen")
	}
	rec := frag(Antworten(cfg), "POST", AntwortenPath+"/"+id, "anna", "family,admins", url.Values{"id": {id}, "aktion": {"ergaenzen"}, "ziel": {"h1"}, "nimm": {"1"}})
	mustContain(t, rec.Body.String(), "Anna Beispiel ergänzt.", "Zum Kontakt ›")
	mustContain(t, dav.Objects()["/homes/haushalt-kontakte/h1.vcf"], "BDAY:19900503", "X-IBAN:DE00", "FN:Anna Beispiel")

	id = neuBeantwortet(t, cfg, url.Values{"vorname": {"Bert"}, "mail": {"bert@example.org"}})
	n := len(dav.Objects())
	if rec := frag(Antworten(cfg), "POST", AntwortenPath+"/"+id, "anna", "family,admins", url.Values{"id": {id}, "aktion": {"ergaenzen"}, "ziel": {"h1"}}); !strings.Contains(rec.Body.String(), "gibt es") && !strings.Contains(rec.Body.String(), "nicht mehr") {
		t.Errorf("completing a card that is no match:\n%s", rec.Body.String())
	}
	mustContain(t, frag(Antworten(cfg), "POST", AntwortenPath+"/"+id, "anna", "family,admins", url.Values{"id": {id}, "aktion": {"verwerfen"}}).Body.String(), "Verworfen: nichts wurde geändert.")
	if len(dav.Objects()) != n || len(cfg.Antworten.ZuPruefen("anna")) != 0 {
		t.Error("Verwerfen wrote or kept something")
	}

	id = neuBeantwortet(t, cfg, url.Values{"vorname": {"Carla"}})
	down := cfg
	down.Contacts = &caldav.Client{URL: "http://127.0.0.1:1", User: cfg.Contacts.User, Password: cfg.Contacts.Password}
	rec = frag(Antworten(down), "POST", AntwortenPath+"/"+id, "anna", "family,admins", url.Values{"id": {id}, "aktion": {"anlegen"}})
	mustContain(t, rec.Body.String(), "Der Kontakt ließ sich gerade nicht anlegen. Versuch es gleich noch einmal.", "value=anlegen")
	if len(cfg.Antworten.ZuPruefen("anna")) != 1 {
		t.Error("a failed write ended the review")
	}
}

// Several candidates (owner 2026-11-28): the strongest reason first, each with its words; a name
// alone is enough to be offered, never to be picked without the owner's tap.
func TestKontaktNeuKandidatenRanked(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	dav.Put("/homes/haushalt-kontakte/h2.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:h2\r\nFN:Muster Erika\r\nEND:VCARD\r\n")
	dav.Put("/homes/haushalt-kontakte/h3.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:h3\r\nFN:E. M.\r\nEMAIL:ERIKA@example.org\r\nTEL;TYPE=HOME:0711 445566\r\nEND:VCARD\r\n")
	id := neuBeantwortet(t, cfg, url.Values{"vorname": {"Erika"}, "nachname": {"Muster"}, "mail": {"erika@example.org"}, "tel": {"0711 445566"}, "tel-art": {"privat"}})
	body := frag(Antworten(cfg), "GET", AntwortenPath+"/"+id, "anna", "family,admins", nil).Body.String()
	mustContain(t, body, "<b>E. M.</b><br><span class=k>gleiche Telefonnummer · gleiche E-Mail</span>", "<b>Muster Erika</b><br><span class=k>ähnlicher Name</span>")
	before(t, body, "<b>E. M.</b>", "<b>Muster Erika</b>")
	before(t, body, "<b>Muster Erika</b>", "Keiner davon? Dann neu ins Adressbuch")
	mustNotContain(t, body, "<b>Anna Beispiel</b>")
	if len(cfg.Antworten.ZuPruefen("anna")) != 1 {
		t.Error("offering decided something")
	}
}

// The review message (owner 2026-11-28): an answer arrives and the link's owner is told over Laib
// with the review's address — a new person's own page, a card's answer its contact page; Laib
// failing or missing keeps the answer and its task all the same.
func TestKontaktAntwortMeldung(t *testing.T) {
	cfg, told := teaserCfg(t)
	cfg.TeaserHost = ""
	warte := func(n int) []string {
		t.Helper()
		for range 100 {
			if got := told(); len(got) >= n {
				return got
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("told %v, want %d", told(), n)
		return nil
	}
	id := neuBeantwortet(t, cfg, url.Values{"vorname": {"Erika"}, "nachname": {"Muster"}})
	if got := warte(1); got[0] != "anna: Erika hat seine Kontaktdaten eingetragen – prüfen: https://www.example.org/kontakte/antworten/"+id {
		t.Errorf("told %q", got[0])
	}
	token := startLink(t, cfg, "haushalt", "h1", "family,admins")
	if rec := frag(KontaktServer(cfg), "POST", "/"+token, "", "", url.Values{"vorname": {"Anna"}, "nachname": {"Beispiel"}, "firma": {"Café"}, "aktion": {"senden"}}); !strings.Contains(rec.Body.String(), "Danke") {
		t.Fatalf("not answered:\n%s", rec.Body.String())
	}
	if got := warte(2); got[1] != "anna: Anna hat seine Kontaktdaten eingetragen – prüfen: https://www.example.org/kontakte/h1" {
		t.Errorf("told %q", got[1])
	}

	down := cfg
	down.Laib = &laib.Client{URL: "http://127.0.0.1:1", Token: "t"}
	tok := einladenLink(t, down)
	if rec := frag(KontaktServer(down), "POST", "/"+tok, "", "", url.Values{"vorname": {"Bert"}, "aktion": {"senden"}}); !strings.Contains(rec.Body.String(), "Danke") {
		t.Errorf("Laib down: %s", rec.Body.String())
	}
	down.Laib = nil
	tok = einladenLink(t, down)
	if rec := frag(KontaktServer(down), "POST", "/"+tok, "", "", url.Values{"vorname": {"Carla"}, "aktion": {"senden"}}); !strings.Contains(rec.Body.String(), "Danke") {
		t.Errorf("no Laib: %s", rec.Body.String())
	}
	if n := len(cfg.Antworten.ZuPruefen("anna")); n != 4 {
		t.Errorf("%d answers on Deine Aufgaben, want 4", n)
	}
}
