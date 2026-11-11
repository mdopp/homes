package pages

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/texte"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/caldav/davfake"
	"github.com/mdopp/homes/internal/napi"
)

// haus#207, owner 2026-11-03: „Daten sichern“ shows the person's state
// from Homes' ledger and the inbox. Owner 2026-11-08: the page is the one
// overview — a line per source, a phone source with nothing yet says where
// to switch it on, E-Mail its way, the keys one button into the archive,
// what never goes and the phone change folded; no intro, no guide.
func TestDatenSichernShowsTheStandAndTheGuide(t *testing.T) {
	inbox, data := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(data, "sicherung", "anna"), 0o700)
	os.WriteFile(filepath.Join(data, "sicherung", "anna", "whatsapp.jsonl"), []byte(
		`{"pfad":"Databases/msgstore.db.crypt15","groesse":3435973837,"zeit":"2026-10-31T02:10:00Z"}`+"\n"+
			`{"pfad":"Media/WhatsApp Images/IMG-1.jpg","groesse":1000,"zeit":"2026-10-31T02:11:00Z"}`+"\n"), 0o600)
	os.WriteFile(filepath.Join(data, "sicherung", "anna", "signal.jsonl"), []byte(`{"pfad":"s.backup","groesse":2048,"zeit":"2026-11-04T02:00:00Z"}`+"\n"), 0o600)
	os.MkdirAll(filepath.Join(inbox, "anna", "whatsapp", "Databases"), 0o755)
	os.WriteFile(filepath.Join(inbox, "anna", "whatsapp", "Databases", "msgstore.db.crypt15"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(inbox, "anna", ".teil"), 0o755)
	os.WriteFile(filepath.Join(inbox, "anna", ".teil", "halb"), []byte("x"), 0o644) // a part does not wait
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	cfg := StartConfig{Sicherung: napi.NewSicherung(inbox, data), Module: modulListe(t, "sicherung"), Loc: berlin(t), Now: func() time.Time { return now }}

	code, body, hdr := familyPage(t, Nachrichten, cfg, "GET", DatenPath, "family")
	if code != 200 || hdr.Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %q", code, hdr.Get("Cache-Control"))
	}
	for _, want := range []string{"<h1>Daten sichern</h1>", "Dein Stand",
		`<span class="punkt gelb"></span><div>WhatsApp<span class=k>zuletzt Sa 31.10. 03:11 · 2 Dateien, 3,2 GB · 1 Datei wartet auf dein Archiv`, // Berlin time
		`<span class="punkt "></span><div>Signal<span class=k>zuletzt heute 03:00 · 1 Datei, 2,0 KB`,
		`<span class="punkt grau"></span><div>SMS und Anrufe<span class=k>noch nichts angekommen · in der Homes-App einschalten`, " frei.",
		`<span class="punkt grau"></span><div>Facebook und LinkedIn<span class=k>noch nichts angekommen · Export auf dem Handy teilen → „Ins Archiv“`,
		`<div>E-Mail<span class=k>Trag dein Postfach einmal im Archiv ein; es holt dann jede Stunde neue Mails.`, "Bitwarden",
		"<summary>Was nie ins Archiv kommt</summary>", "<details class=wechsel><summary>Handy wechseln</summary>"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	for _, gone := range []string{"Discord", "Nachrichten sichern", "Chat-Backups", "64-stelligen Schlüssel verwenden", "„Kontakte übertragen“", "je nach App-Version", "<h2>Signal</h2>",
		"class=lead", "Das Sichern macht die Homes-App", "Ohne Handy", "/tresor"} { // no archive for anna here: no link into it
		if strings.Contains(body, gone) {
			t.Errorf("the page still says %q", gone)
		}
	}
	mustContain(t, body, texte.T(texte.KontaktKeinArchiv))
	before(t, body, "Dein Stand", "Bitwarden")
	before(t, body, "Was nie ins Archiv kommt", "Handy wechseln")
	if strings.Contains(body, "<form") || strings.Contains(body, "<button") {
		t.Error("nothing on the page changes anything yet: no form, no button")
	}
	// another person's page shows only theirs
	if _, ben, _ := familyPageAs(t, Nachrichten, cfg, "GET", DatenPath, "ben", "family"); strings.Contains(ben, "3,2 GB") || strings.Contains(ben, "wartet") {
		t.Errorf("Ben sees Anna's backups:\n%s", ben)
	}
	if code, _, _ := familyPageAs(t, Nachrichten, cfg, "GET", DatenPath, "", ""); code != 403 {
		t.Errorf("without a person → %d", code)
	}
	if code, _, _ := familyPage(t, Nachrichten, cfg, "POST", DatenPath, "family"); code != 405 {
		t.Errorf("POST → %d", code)
	}
	// Mehr lists it for a signed-in person on a box with the inbox
	if _, mehr, _ := familyPage(t, Mehr, cfg, "GET", mehrPath, "family"); !strings.Contains(mehr, "href="+DatenPath) || !strings.Contains(mehr, "Daten sichern") {
		t.Error("Mehr does not lead to Daten sichern")
	}
	// the old address still lands
	if code, _, hdr := familyPage(t, Nachrichten, cfg, "GET", NachrichtenPath, "family"); code != 301 || hdr.Get("Location") != DatenPath {
		t.Errorf("the old address → %d %q", code, hdr.Get("Location"))
	}
}

// haus#209: the page shows what lies ready for a new phone (date, size)
// and the phone change in three steps; only the person's own.
func TestDatenSichernShowsWhatANewPhoneGetsBack(t *testing.T) {
	inbox, data := t.TempDir(), t.TempDir()
	lege := func(rel string, b []byte, at time.Time) {
		p := filepath.Join(inbox, "anna", "letzte", filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, b, 0o644)
		os.Chtimes(p, at, at)
	}
	nacht := time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)
	lege("signal/signal-2026-11-02-03-00-00.backup", make([]byte, 3<<20), nacht)
	lege("whatsapp/Databases/msgstore.db.crypt15", make([]byte, 2<<20), nacht.Add(time.Hour))
	// two media, age-shaped: header, nonce, one chunk with its tag
	for _, m := range []string{"WhatsApp Images/IMG-1.jpg", "WhatsApp Voice Notes/PTT-1.opus"} {
		age := []byte("age-encryption.org/v1\n-> X25519 abc\nxyz\n--- mac\n")
		age = append(age, make([]byte, 16+1024+16)...)
		lege("whatsapp/Media/"+m+".age", age, nacht)
	}
	cfg := StartConfig{Sicherung: napi.NewSicherung(inbox, data)}
	_, body, _ := familyPage(t, Nachrichten, cfg, "GET", DatenPath, "family")
	for _, want := range []string{"Zum Zurückholen",
		"Sicherung vom 2. November 2026, 03:00 · 3,0 MB", // Berlin time
		"Chats vom 2. November 2026, 04:00 · 2,0 MB · 2 Fotos und Sprachnachrichten, 2,0 KB",
		"<summary>Handy wechseln</summary>", "„Sicherung zurückholen“", "noch nicht öffnen", "Android/media/com.whatsapp/WhatsApp",
		"„Aus Sicherung wiederherstellen“", "30-stellige Passphrase aus Bitwarden", "„Wiederherstellen“ wählen",
		"während dein Archiv offen ist"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Count(body, "<summary>Handy wechseln</summary>") != 1 || strings.Count(body[strings.Index(body, "Handy wechseln"):], "<li>") < 3 {
		t.Error("not three steps")
	}
	before(t, body, "Dein Stand", "Zum Zurückholen")
	_, ben, _ := familyPageAs(t, Nachrichten, cfg, "GET", DatenPath, "ben", "family")
	if strings.Contains(ben, "3,0 MB") || !strings.Contains(ben, "noch keine Sicherung zum Zurückholen") || !strings.Contains(ben, "noch nichts zum Zurückholen") {
		t.Errorf("Ben's page:\n%s", ben)
	}
}

// Owner 2026-11-08: the keys are entered in the person's archive, never in
// the app; the page is the way there — its /tresor at home, the plain reason
// away from home or without an archive.
func TestDatenSichernLeadsToTheKeys(t *testing.T) {
	cfg := StartConfig{Sicherung: napi.NewSicherung(t.TempDir(), t.TempDir()), Archive: map[string]bool{"anna": true}, ArchivURL: "https://archiv.box.test"}
	seite := func(user, von string) string {
		req := httptest.NewRequest("GET", DatenPath, nil)
		req.Header.Set("Remote-User", user)
		req.Header.Set("X-Forwarded-For", von)
		rec := httptest.NewRecorder()
		Nachrichten(cfg).ServeHTTP(rec, req)
		return rec.Body.String()
	}
	mustContain(t, seite("anna", "192.168.1.23"), `<a class=knopf href="https://archiv.box.test/tresor">Schlüssel im Archiv eintragen</a>`,
		"Signal-Passphrase, WhatsApp-Schlüssel und Postfach")
	unterwegs := seite("anna", "203.0.113.7")
	mustContain(t, unterwegs, texte.T(texte.KontaktArchivDaheim))
	mustNotContain(t, unterwegs, "/tresor")
	ben := seite("ben", "192.168.1.23")
	mustContain(t, ben, texte.T(texte.KontaktKeinArchiv))
	mustNotContain(t, ben, "/tresor")
}

func TestDatenSichernWithoutTheArchiv(t *testing.T) {
	code, body, _ := familyPage(t, Nachrichten, StartConfig{}, "GET", DatenPath, "family")
	if code != 200 || !strings.Contains(body, "Gibt es hier noch nicht") || strings.Contains(body, "Dein Stand") {
		t.Fatalf("%d %s", code, body)
	}
	if _, admin, _ := familyPage(t, Nachrichten, StartConfig{}, "GET", DatenPath, "admins"); strings.Contains(admin, "HOMES_ARCHIV_DIR") {
		t.Error("the trace names no setting, an admin's way is the module's page")
	}
	if _, mehr, _ := familyPage(t, Mehr, StartConfig{}, "GET", mehrPath, "family"); strings.Contains(mehr, DatenPath) {
		t.Error("Mehr lists a page the box does not have")
	}
	// a stand that cannot be read is one plain line
	cfg := StartConfig{Sicherung: napi.NewSicherung(t.TempDir(), "/dev/null/kein")}
	if _, body, _ := familyPage(t, Nachrichten, cfg, "GET", DatenPath, "family"); !strings.Contains(body, saysStandFehlt) {
		t.Errorf("an unreadable ledger:\n%s", body)
	}
}

func TestGroesseWorte(t *testing.T) {
	for n, want := range map[int64]string{0: "0 Bytes", 999: "999 Bytes", 1536: "1,5 KB", 450 << 20: "450 MB", 3435973837: "3,2 GB", 12 << 40: "12 TB", 1500 << 30: "1,5 TB"} {
		if got := texte.Groesse(n); got != want {
			t.Errorf("texte.Groesse(%d) = %q, want %q", n, got, want)
		}
	}
	if got := texte.Anzahl(12345, "Datei", "Dateien"); got != "12.345 Dateien" {
		t.Errorf("anzahl = %q", got)
	}
}

// Owner 2026-11-04: „Standort und Kalender und Adressbuch sind auch Dinge,
// die gesichert werden.“ Dein Stand says each from what Homes can read:
// Reitti's last point, the cards in Handy-Kontakte (a count; Homes keeps no
// time of their arrival), the calendars the archive takes over (a count).
func TestDatenSichernStandortKontakteKalender(t *testing.T) {
	dav := davfake.New("homes", "pw")
	t.Cleanup(dav.Close)
	dav.AddPrincipal("anna")
	dav.AddCalendar("/homes/haushalt/", "Haushalt")
	dav.AddAddressBook("/anna/handy-kontakte/", "Handy-Kontakte")
	for i, n := range []string{"Oma", "Opa"} {
		dav.Put("/anna/handy-kontakte/k"+strconv.Itoa(i)+".vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:k"+strconv.Itoa(i)+"\r\nFN:"+n+"\r\nEND:VCARD\r\n")
	}
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	cfg, _ := standortCfg(t)
	cfg.Sicherung, cfg.Now, cfg.Loc = napi.NewSicherung(t.TempDir(), t.TempDir()), func() time.Time { return now }, berlin(t)
	dav2 := &caldav.Client{URL: dav.URL, User: "homes", Password: "pw", Loc: berlin(t)}
	cfg.Contacts, cfg.Calendar = dav2, dav2
	_, body, _ := familyPage(t, Nachrichten, cfg, "GET", DatenPath, "family")
	mustContain(t, body, `<span class="punkt grau"></span><div><a href=/standort>Standort</a><span class=k>noch nichts aufgezeichnet`,
		`<span class="punkt "></span><div>Kontakte<span class=k>2 Kontakte im Adressbuch „Handy-Kontakte“`,
		`<span class="punkt grau"></span><div>Kalender<span class=k>1 Kalender; das Archiv übernimmt sie, während es offen ist`)
	_, ben, _ := familyPageAs(t, Nachrichten, cfg, "GET", DatenPath, "ben", "family") // no contacts of his own
	mustContain(t, ben, `<div>Kontakte<span class=k>noch nichts angekommen · in der Homes-App einschalten`)
	mustNotContain(t, body, "Deine Kalender in Homes übernimmt")
	cfg.Reitti.Verbinden(context.Background(), "anna", "gut-0123")
	_, body, _ = familyPage(t, Nachrichten, cfg, "GET", DatenPath, "family")
	mustContain(t, body, `<span class="punkt gelb"></span><div><a href=/standort>Standort</a><span class=k>noch nichts aufgezeichnet`)
}
