package pages

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/caldav/davfake"
	"github.com/mdopp/homes/internal/napi"
	"github.com/mdopp/homes/internal/paperless"
	"github.com/mdopp/homes/internal/texte"
)

// meineDatenCfg is a box with every source: Anna's ledger (WhatsApp four
// days old with a file waiting for the archive, Signal from last night),
// two of her phone's contacts, her archive's status through Laib (archiv),
// Reitti, and the scan's status (scan). Now is 4 November, 12:00 UTC.
func meineDatenCfg(t *testing.T, archiv, scan string) StartConfig {
	t.Helper()
	inbox, data := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(data, "sicherung", "anna"), 0o700)
	os.WriteFile(filepath.Join(data, "sicherung", "anna", "whatsapp.jsonl"), []byte(
		`{"pfad":"Databases/msgstore.db.crypt15","groesse":3435973837,"zeit":"2026-10-31T02:10:00Z"}`+"\n"+
			`{"pfad":"Media/WhatsApp Images/IMG-1.jpg","groesse":1000,"zeit":"2026-10-31T02:11:00Z"}`+"\n"), 0o600)
	os.WriteFile(filepath.Join(data, "sicherung", "anna", "signal.jsonl"), []byte(`{"pfad":"s.backup","groesse":2048,"zeit":"2026-11-04T02:00:00Z"}`+"\n"), 0o600)
	os.MkdirAll(filepath.Join(inbox, "anna", "whatsapp", "Databases"), 0o755)
	os.WriteFile(filepath.Join(inbox, "anna", "whatsapp", "Databases", "msgstore.db.crypt15"), []byte("x"), 0o644)
	dav := davfake.New("homes", "pw")
	t.Cleanup(dav.Close)
	dav.AddPrincipal("anna")
	dav.AddAddressBook("/anna/handy-kontakte/", "Handy-Kontakte")
	for i, n := range []string{"Oma", "Opa"} {
		dav.Put("/anna/handy-kontakte/k"+strconv.Itoa(i)+".vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:k"+strconv.Itoa(i)+"\r\nFN:"+n+"\r\nEND:VCARD\r\n")
	}
	cfg, _ := standortCfg(t)
	var rows []map[string]any
	if archiv != "" {
		rows = append(rows, map[string]any{"name": "archiv-anna", "eigenes": jsonRaw(archiv)})
	}
	more := map[string]string{"laib_scan": scan}
	if scan == "" {
		more["laib_scan"] = "Der Dienst scan ist nicht eingerichtet."
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	cfg.Laib, cfg.Docs = fakeLaibWith(t, nil, rows, more), &paperless.Client{URL: "http://127.0.0.1:1"}
	cfg.Sicherung, cfg.Now, cfg.Loc = napi.NewSicherung(inbox, data), func() time.Time { return now }, berlin(t)
	cfg.Contacts = &caldav.Client{URL: dav.URL, User: "homes", Password: "pw", Loc: berlin(t)}
	cfg.Archive, cfg.ArchivURL, cfg.DocumentsMail = map[string]bool{"anna": true}, "https://archiv.box.test", "dokumente@box.test"
	return cfg
}

// jsonRaw is a JSON text as a value fakeLaibWith marshals as it is.
type jsonRaw string

func (j jsonRaw) MarshalJSON() ([]byte, error) { return []byte(j), nil }

// homes#342: every source of the person on one page, each in its state —
// never set up (below, with its way in), running, current, quiet since
// (past its threshold, written under it), failed with the reason and the
// next step — and the person's own only.
func TestMeineDatenSaysEachSourceInItsState(t *testing.T) {
	archiv := `{"zustand":"verschlossen","letztes_paket":{"mail":{"am":"2026-11-04T11:40:00+01:00","sicherung":"anna@example.org"}},` +
		`"letzter_import":{"signal":{"am":"2026-11-03T03:00:00Z","datei":"x"},"mail":{"am":"2026-11-04T09:00:00Z"}},` +
		`"letzter_fehler":{"am":"2026-11-04T03:00:00Z","quelle":"signal","datei":"s.backup","satz":"Die Signal-Passphrase passt nicht: trag sie im Archiv neu ein."}}`
	scan := `{"zustand":"bereit","scanner":true,"eingang":"scan-eingang","letzte":[` +
		`{"stapel":"a","ende":"fehler","quelle":"scanner","person":"anna","at":"2026-11-03T08:00:00Z","meldung":"Papierstau im Einzug: Blatt herausnehmen und den Stapel neu starten.\nmehr"},` +
		`{"stapel":"b","ende":"fertig","quelle":"scanner","person":"ben","at":"2026-11-04T08:00:00Z","dokumente":[{"titel":"x"}]}],` +
		`"warten":[{"stapel":"c","person":"anna","quelle":"ordner"}]}`
	cfg := meineDatenCfg(t, archiv, scan)
	code, body, hdr := familyPage(t, MeineDaten, cfg, "GET", MeineDatenPath, "family")
	if code != 200 || hdr.Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %q", code, hdr.Get("Cache-Control"))
	}
	mustContain(t, body, "<h1>Meine Daten</h1>", "<h2>Was ankommt</h2>", "<h2>So kommt mehr dazu</h2>",
		// quiet past its threshold, with the next step; and a file waiting for the archive
		`<span class="punkt gelb"></span><div>WhatsApp<span class=k>still seit Sa 31.10. 03:11 · die Homes-App öffnen: läuft die Sicherung noch? · 1 Datei wartet: öffne dein Archiv, dann liest es sie</span><small class=schwelle>still nach 2 Tagen</small>`,
		// failed: the archive's own sentence, after the last import
		`<span class="punkt rot"></span><div>Signal<span class=k>hängt: Die Signal-Passphrase passt nicht: trag sie im Archiv neu ein.</span><small class=schwelle>still nach 2 Tagen</small>`,
		// current: mail by the archive's last fetch, Berlin time
		`<span class="punkt "></span><div>E-Mail<span class=k>zuletzt heute 11:40</span><small class=schwelle>still nach 1 Tag</small>`,
		`<span class="punkt "></span><div>Kontakte<span class=k>2 Kontakte im Adressbuch „Handy-Kontakte“</span><small class=schwelle>die Box zählt nur, sie sieht keine Uhrzeit</small>`,
		// failed with the scan's first line; running
		`<span class="punkt rot"></span><div>Scanner<span class=k>hängt: Papierstau im Einzug: Blatt herausnehmen und den Stapel neu starten.</span><small class=schwelle>kommt, wenn du etwas schickst</small>`,
		`<span class="punkt lauf"></span><div>Eingangsordner<span class=k>läuft gerade: Dokumente trennt und legt ab</span>`,
		// never set up: below, with the way in
		`<div>SMS und Anrufe<span class=k>in der Homes-App einschalten</span></div>`,
		`<div>Facebook und LinkedIn<span class=k>Export auf dem Handy teilen → „Ins Archiv“</span></div>`,
		`<div><a href=/standort>Standort</a><span class=k>noch nicht verbunden</span></div>`,
		"Briefe und Rechnungen per Mail an anna+doc@box.test schicken", " frei.")
	before(t, body, "Was ankommt", "So kommt mehr dazu")
	before(t, body, "So kommt mehr dazu", "SMS und Anrufe")
	mustNotContain(t, body, "anna@example.org", "mehr</span>", "<form", "<button") // no archive content, the scan's first line only, nothing to change
	if strings.Count(body, "<li class=quelle>") != 11 {                            // no Share → Homes on this box (no Ablage)
		t.Errorf("%d sources, want 11 (the app's four, contacts, mail, places, the scanner and the folder, mail to Dokumente, Takeout)", strings.Count(body, "<li class=quelle>"))
	}
	// Ben: none of Anna's — no archive of his own, nothing backed up, his own scan
	_, ben, _ := familyPageAs(t, MeineDaten, cfg, "GET", MeineDatenPath, "ben", "family")
	mustContain(t, ben, `<div>E-Mail<span class=k>`+texte.T(texte.KontaktKeinArchiv), `<div>Scanner<span class=k>zuletzt heute 09:00 · 1 Dokument`,
		`<div>WhatsApp<span class=k>in der Homes-App einschalten`, "ben+doc@box.test")
	mustNotContain(t, ben, "3,2 GB", "11:40", "Passphrase passt", "Papierstau", "läuft gerade", "Oma")
	_, admin, _ := familyPageAs(t, MeineDaten, cfg, "GET", MeineDatenPath, "ben", "family,admins") // an admin: their own only too
	mustNotContain(t, admin, "3,2 GB", "11:40", "Passphrase passt", "Papierstau", "läuft gerade", "Oma", "anna+doc")
	if code, _, _ := familyPageAs(t, MeineDaten, cfg, "GET", MeineDatenPath, "", ""); code != 403 {
		t.Errorf("without a person → %d", code)
	}
	if code, _, _ := familyPage(t, MeineDaten, cfg, "POST", MeineDatenPath, "family"); code != 405 {
		t.Errorf("POST → %d", code)
	}
	if code, _, _ := familyPage(t, MeineDaten, cfg, "GET", MeineDatenPath+"/x", "family"); code != 404 {
		t.Errorf("below the page → %d", code)
	}
}

// The other states: current with its count, quiet places, a running scan,
// an archive whose status Laib does not give (one plain line, never „done“),
// and a box without the scan or the archive's inbox (no rows of theirs).
func TestMeineDatenCurrentQuietRunningAndUnread(t *testing.T) {
	scan := `{"scanner":true,"eingang":"scan-eingang","trennt":{"stapel":"d","person":"anna"},"letzte":[` +
		`{"stapel":"a","ende":"fertig","quelle":"scanner","person":"anna","at":"2026-11-01T08:00:00Z","dokumente":[{"titel":"x"},{"titel":"y"}]},` +
		`{"stapel":"e","ende":"fertig","quelle":"ordner","person":"anna","at":"2026-11-04T10:30:00Z","dokumente":[{"titel":"z"}]}]}`
	cfg := meineDatenCfg(t, "", scan)
	cfg.Reitti.Verbinden(context.Background(), "anna", "gut-0123")
	_, body, _ := familyPage(t, MeineDaten, cfg, "GET", MeineDatenPath, "family")
	mustContain(t, body, `<span class="punkt "></span><div>Signal<span class=k>zuletzt heute 03:00 · 1 Datei, 2,0 KB</span>`,
		`<span class="punkt grau"></span><div>E-Mail<span class=k>`+texte.T(texte.MeineDatenStumm),
		`<span class="punkt gelb"></span><div><a href=/standort>Standort</a><span class=k>verbunden, noch kein Punkt angekommen`,
		`<span class="punkt lauf"></span><div>Scanner<span class=k>läuft gerade`,
		`<span class="punkt "></span><div>Eingangsordner<span class=k>zuletzt heute 11:30 · 1 Dokument</span>`)
	// the app's Standortverlauf row says what Meine Daten says
	if _, stand := Zeile(cfg, "anna", StandortPath); stand != "Verbunden, noch kein Punkt angekommen" {
		t.Errorf("the app's row: %q", stand)
	}
	// no scan, no inbox, no Reitti, no archive: those rows are not there; an unreadable ledger is one line
	cfg = StartConfig{Sicherung: napi.NewSicherung(t.TempDir(), "/dev/null/kein"), Laib: fakeLaibWith(t, nil, nil, map[string]string{"laib_scan": "Der Dienst scan ist nicht eingerichtet."}), Docs: &paperless.Client{}}
	_, body, _ = familyPage(t, MeineDaten, cfg, "GET", MeineDatenPath, "family")
	mustContain(t, body, `<div>Handy<span class=k>`+saysStandFehlt)
	mustNotContain(t, body, "Scanner", "E-Mail", "Standort", "So kommt mehr dazu")
}

// Quiet is measured against each source's own threshold: the places after
// a day, the phone's backups after two; a scan is never quiet.
func TestMeineDatenQuietByItsThreshold(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	loc := berlin(t)
	for _, c := range []struct {
		q     string
		vor   time.Duration
		still bool
	}{{"standort", 23 * time.Hour, false}, {"standort", 25 * time.Hour, true}, {"signal", 47 * time.Hour, false}, {"signal", 49 * time.Hour, true},
		{"mail", 25 * time.Hour, true}, {"scanner", 400 * time.Hour, false}} {
		z := angekommen(datenZeile{q: c.q}, now.Add(-c.vor), "", now, loc)
		if (z.dot == "gelb") != c.still || strings.HasPrefix(z.says, "still seit") != c.still {
			t.Errorf("%s %v ago: %q %q", c.q, c.vor, z.dot, z.says)
		}
	}
}

// Mehr leads to Meine Daten in one entry and says no source's state of its
// own (homes#342: nothing twice in two words); Daten sichern's address
// leads there, and is no entry of its own (homes#362).
func TestMehrLeadsToMeineDaten(t *testing.T) {
	cfg := meineDatenCfg(t, "", "")
	cfg.Module = modulListe(t, "sicherung", "standort")
	_, mehr, _ := familyPage(t, Mehr, cfg, "GET", mehrPath, "family")
	mustContain(t, mehr, `<a href="/meine-daten">Meine Daten</a> <span class=k>Was von dir auf die Box kommt, und wann zuletzt</span>`)
	mustNotContain(t, mehr, "Zuletzt angekommen", "Noch nicht verbunden", "Noch nichts angekommen", "Daten sichern")
	if code, _, hdr := familyPage(t, Nachrichten, cfg, "GET", DatenPath, "family"); code != 301 || hdr.Get("Location") != MeineDatenPath {
		t.Errorf("Daten sichern → %d %q", code, hdr.Get("Location"))
	}
	if _, anon, _ := familyPageAs(t, Mehr, cfg, "GET", mehrPath, "", ""); strings.Contains(anon, MeineDatenPath) {
		t.Error("Meine Daten is a person's: not without one")
	}
}
