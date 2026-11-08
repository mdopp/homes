package pages

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/kontaktantwort"
)

// mehrteilig sends the form as the browser does with a file field: multipart.
func mehrteilig(h http.Handler, target string, felder [][2]string, foto []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, f := range felder {
		mw.WriteField(f[0], f[1])
	}
	fw, _ := mw.CreateFormFile("foto", "bild.png")
	fw.Write(foto)
	mw.Close()
	req := httptest.NewRequest("POST", target, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func testPNG(w, h int) []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h)))
	return b.Bytes()
}

// owner 2026-11-08: the form's fields with their HTML form and autofill;
// a wrong one said beside it and nothing kept; kinds, the main number, a
// removal, the photo and the message reach the review; a removal and the
// message only when ticked; on accept the card gets them.
func TestKontaktFormularV2(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	token := startLink(t, cfg, "haushalt", "h1", "family,admins")
	pub := KontaktServer(cfg)
	body := frag(pub, "GET", "/"+token, "", "", nil).Body.String()
	mustContain(t, body, `enctype=multipart/form-data`, `autocomplete=given-name`, `autocomplete=family-name`, `autocomplete="mobile tel"`, `autocomplete="home email"`,
		`autocomplete="section-adr0 address-line1"`, `autocomplete="section-adr0 postal-code"`, `autocomplete="section-adr0 address-level2"`,
		`autocomplete="section-adr0 country-name"`, `autocomplete=bday-day`, `autocomplete=bday-month`, `autocomplete=bday-year`, `autocomplete=organization`,
		`autocomplete=organization-title`, `type=tel inputmode=tel`, `type=email inputmode=email`, `pattern="[0-9]{5}"`, `inputmode=numeric`,
		`<option value=3>März</option>`, `name=tel-haupt value=0>`, `name=loeschen value=tel.0 class=papierkorb formnovalidate aria-label="Nummer löschen"`,
		`name=loeschen value=mail.0 class=papierkorb formnovalidate aria-label="E-Mail löschen"`, `name=zurueck value=tel.0 class=zurueck formnovalidate hidden>Rückgängig`, `Haupt-Nummer`, `Haupt-Adresse`,
		`<option value=andere>andere</option>`, `type=file accept="image/jpeg,image/png,image/heic,image/heif,image/webp"`, `Nachricht an`, `maxlength=1000`,
		`data-fehler="Ein Name hat nur Buchstaben`)
	mustNotContain(t, body, "autocomplete=off", "TT.MM.JJJJ")
	if csp := frag(pub, "GET", "/"+token, "", "", nil).Header().Get("Content-Security-Policy"); !strings.Contains(csp, "img-src blob:;") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP %q", csp)
	}
	// every pattern of the page compiles in Go too (the browser's is the same syntax for these)
	for _, m := range regexp.MustCompile(`pattern="([^"]*)"`).FindAllStringSubmatch(body, -1) {
		if _, err := regexp.Compile(strings.ReplaceAll(m[1], "&#39;", "'")); err != nil {
			t.Errorf("pattern %q: %v", m[1], err)
		}
	}

	base := [][2]string{{"vorname", "Anna"}, {"nachname", "Beispiel"}, {"tel", "+49 151 23456789"}, {"tel-art", "mobil"}, {"tel", "0711 1234567"}, {"tel-art", "geschaeftlich"},
		{"tel-haupt", "1"}, {"mail", "anna@example.org"}, {"mail-art", "geschaeftlich"}, {"mail", ""}, {"mail-art", "privat"}, {"weg", "mail.0"},
		{"adr-strasse", "Lindenweg"}, {"adr-nr", "3"}, {"adr-plz", "70173"}, {"adr-ort", "München"}, {"adr-land", ""},
		{"geb-tag", "8"}, {"geb-monat", "3"}, {"geb-jahr", ""}, {"firma", "Bäckerei"}, {"position", ""}, {"nachricht", "Grüße <b>aus</b> Stuttgart"}, {"aktion", "senden"}}
	falsch := append([][2]string{{"tel", "0171 12"}, {"tel-art", "mobil"}}, base...)
	falsch[2][1] = "Anna2"
	rec := mehrteilig(pub, "/"+token, falsch, testPNG(40, 40))
	mustContain(t, rec.Body.String(), "Ein paar Angaben passen so noch nicht", `aria-invalid=true aria-describedby=f-tel.0><span class=fehler id=f-tel.0>`,
		"Ein Name hat nur Buchstaben", "Wähle das Foto bitte noch einmal", `<input type=hidden name=weg value=mail.0>`, `<div class="eintrag geloescht">`, `>Grüße &lt;b&gt;aus&lt;/b&gt; Stuttgart</textarea>`)
	if _, err := cfg.Antworten.Zum(token); err != nil {
		t.Fatalf("a refused form used the link up: %v", err)
	}
	if rec := mehrteilig(pub, "/"+token, base, []byte("GIF89a not a photo")); !strings.Contains(rec.Body.String(), "Dieses Bild lässt sich nicht öffnen") {
		t.Fatalf("a GIF taken:\n%s", rec.Body.String())
	}
	if rec := mehrteilig(pub, "/"+token, base, bytes.Repeat([]byte{1}, kontaktantwort.FotoBytes+10)); rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "höchstens 5 MB") {
		t.Fatalf("a photo over 5 MB: %d", rec.Code)
	}
	if rec := mehrteilig(pub, "/"+token, base, testPNG(1600, 1200)); !strings.Contains(rec.Body.String(), "Danke!") {
		t.Fatalf("answer %d:\n%s", rec.Code, rec.Body.String())
	}

	f := cfg.Antworten.ZuPruefen("anna")[0]
	rec = frag(Antworten(cfg), "GET", AntwortenPath+"/"+f.ID, "anna", "family", nil)
	page := rec.Body.String()
	mustContain(t, page, `0711 1234567 (geschäftlich, Haupt-Nummer)</span> <span class=k>(neu)`,
		`anna@example.org (andere)</span> <span class=weg>entfernen</span>`, `Lindenweg 3, 70173 München`, "gehört zu Stuttgart, nicht zu München",
		`<img class=foto src="data:image/jpeg;base64,`, `Nachricht von Anna Beispiel`, `Grüße &lt;b&gt;aus&lt;/b&gt; Stuttgart`, "An die Notiz des Kontakts anhängen")
	for i, a := range f.Aenderungen {
		if a.NurAngekreuzt() && strings.Contains(page, `name=nimm value=`+strconv.Itoa(i)+` checked`) {
			t.Errorf("%s ticked from the start", a.Art)
		}
	}
	// Alle übernehmen: not the removal, not the message
	frag(Antworten(cfg), "POST", AntwortenPath+"/"+f.ID, "anna", "family", url.Values{"aktion": {"alle"}})
	card := dav.Objects()["/homes/haushalt-kontakte/h1.vcf"]
	mustContain(t, card, "TEL;TYPE=WORK,PREF:0711 1234567", "TEL;TYPE=CELL:+49 151 23456789", "EMAIL:anna@example.org", "ADR:;;Lindenweg 3;München;;70173;", "PHOTO;ENCODING=b;TYPE=JPEG:/9j/", "BDAY:--0308")
	mustNotContain(t, card, "NOTE", "Grüße")
	if m := regexp.MustCompile(`PHOTO;ENCODING=b;TYPE=JPEG:([A-Za-z0-9+/=\r\n ]+)`).FindStringSubmatch(card); m == nil {
		t.Error("no photo")
	}
}

// The message reaches the note, a removal the card, when ticked.
func TestKontaktFormularTickedOnly(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	token := startLink(t, cfg, "haushalt", "h1", "family,admins")
	pub := KontaktServer(cfg)
	felder := [][2]string{{"vorname", "Anna"}, {"nachname", "Beispiel"}, {"tel", "+49 151 23456789"}, {"tel-art", "mobil"}, {"weg", "tel.0"},
		{"mail", "anna@example.org"}, {"mail-art", "andere"}, {"firma", "Bäckerei"}, {"nachricht", "Bin umgezogen."}, {"aktion", "senden"}}
	if rec := mehrteilig(pub, "/"+token, felder, nil); !strings.Contains(rec.Body.String(), "Danke!") {
		t.Fatalf("answer:\n%s", rec.Body.String())
	}
	f := cfg.Antworten.ZuPruefen("anna")[0]
	if len(f.Aenderungen) != 2 || !f.Aenderungen[0].Entfernen() || f.Aenderungen[1].Art != "notiz" {
		t.Fatalf("changes %+v", f.Aenderungen)
	}
	frag(Antworten(cfg), "POST", AntwortenPath+"/"+f.ID, "anna", "family", url.Values{"aktion": {"auswahl"}, "nimm": {"0", "1"}})
	card := dav.Objects()["/homes/haushalt-kontakte/h1.vcf"]
	mustContain(t, card, "NOTE:Bin umgezogen.")
	mustNotContain(t, card, "23456789")
}

// owner 2026-11-08: the trash can works without the script too: the form
// comes back with the row greyed and marked, the link unused; Rückgängig
// takes the mark back. A card's own value that has not the form gets no
// pattern, so the browser does not stop the contact over it.
func TestKontaktPapierkorbOhneSkript(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	token := startLink(t, cfg, "haushalt", "h1", "family,admins")
	pub := KontaktServer(cfg)
	felder := [][2]string{{"vorname", "Anna"}, {"nachname", "Beispiel"}, {"tel", "+49 151 23456789"}, {"tel-art", "mobil"}, {"mail", "anna@example.org"}, {"loeschen", "tel.0"}}
	body := mehrteilig(pub, "/"+token, felder, nil).Body.String()
	mustContain(t, body, `<div class="eintrag geloescht">`, `<input type=hidden name=weg value=tel.0>`, `name=tel value="+49 151 23456789" readonly `,
		`name=zurueck value=tel.0 class=zurueck formnovalidate>Rückgängig`)
	mustNotContain(t, body, "Danke!", "passen so noch nicht")
	if _, err := cfg.Antworten.Zum(token); err != nil {
		t.Fatalf("a trash can used the link up: %v", err)
	}
	body = mehrteilig(pub, "/"+token, append(felder[:5], [2]string{"weg", "tel.0"}, [2]string{"zurueck", "tel.0"}), nil).Body.String()
	mustNotContain(t, body, `class="eintrag geloescht"`, "name=weg value")
	form := frageForm(token, kontaktantwort.Frage{Vorher: kontaktantwort.Felder{Telefone: []kontaktantwort.Telefon{{Nummer: "123"}}}},
		frageStand{f: kontaktantwort.Felder{Telefone: []kontaktantwort.Telefon{{Nummer: "123"}}}}, "")
	mustContain(t, form, `name=tel value="123" type=tel`)
	if strings.Contains(form, `value="123" type=tel inputmode=tel maxlength=40 autocomplete="mobile tel" pattern`) {
		t.Error("the card's own number without its form has a pattern")
	}
}
