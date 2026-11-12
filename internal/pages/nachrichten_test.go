package pages

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/texte"

	"github.com/mdopp/homes/internal/napi"
)

// haus#207, homes#362 (owner 2026-11-12: one entry in Mehr): what „Daten
// sichern“ said is on Meine Daten, below the sources — the keys one button
// into the archive, what never goes and the phone change folded; its
// addresses lead there, Mehr has Meine Daten only.
func TestDatenSichernIsOnMeineDaten(t *testing.T) {
	inbox, data := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(data, "sicherung", "anna"), 0o700)
	os.WriteFile(filepath.Join(data, "sicherung", "anna", "signal.jsonl"), []byte(`{"pfad":"s.backup","groesse":2048,"zeit":"2026-11-04T02:00:00Z"}`+"\n"), 0o600)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	cfg := StartConfig{Sicherung: napi.NewSicherung(inbox, data), Module: modulListe(t, "sicherung"), Loc: berlin(t), Now: func() time.Time { return now }}
	for _, old := range []string{DatenPath, NachrichtenPath} {
		if code, _, hdr := familyPage(t, Nachrichten, cfg, "GET", old, "family"); code != 301 || hdr.Get("Location") != MeineDatenPath {
			t.Errorf("%s → %d %q", old, code, hdr.Get("Location"))
		}
	}
	_, body, _ := familyPage(t, MeineDaten, cfg, "GET", MeineDatenPath, "family")
	mustContain(t, body, "<h1>Meine Daten</h1>", "Bitwarden", "<summary>Was nie ins Archiv kommt</summary>", "<details class=wechsel><summary>Handy wechseln</summary>",
		texte.T(texte.KontaktKeinArchiv)) // no archive for anna here: no link into it
	mustNotContain(t, body, "/tresor")
	before(t, body, "li class=quelle", "Bitwarden")
	before(t, body, "Was nie ins Archiv kommt", "Handy wechseln")
	_, mehr, _ := familyPage(t, Mehr, cfg, "GET", mehrPath, "family")
	mustContain(t, mehr, `href="/meine-daten"`)
	mustNotContain(t, mehr, "href="+DatenPath, `href="`+DatenPath, ">Daten sichern<")
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
	_, body, _ := familyPage(t, MeineDaten, cfg, "GET", MeineDatenPath, "family")
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
	before(t, body, "<h1>Meine Daten</h1>", "Zum Zurückholen")
	_, ben, _ := familyPageAs(t, MeineDaten, cfg, "GET", MeineDatenPath, "ben", "family")
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
		req := httptest.NewRequest("GET", MeineDatenPath, nil)
		req.Header.Set("Remote-User", user)
		req.Header.Set("X-Forwarded-For", von)
		rec := httptest.NewRecorder()
		MeineDaten(cfg).ServeHTTP(rec, req)
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
	code, body, _ := familyPage(t, MeineDaten, StartConfig{}, "GET", MeineDatenPath, "family")
	if code != 200 || strings.Contains(body, "Handy wechseln") || strings.Contains(body, "Bitwarden") {
		t.Fatalf("%d %s", code, body)
	}
	if _, mehr, _ := familyPage(t, Mehr, StartConfig{}, "GET", mehrPath, "family"); strings.Contains(mehr, DatenPath) {
		t.Error("Mehr lists a page the box does not have")
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
