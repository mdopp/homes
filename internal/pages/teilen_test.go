package pages

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/teilen"
)

// teilenCfg is the public listener with the teaser on example.org, and Michael's session on a clock the test moves.
func teilenCfg(t *testing.T) (StartConfig, *teilen.Store, *time.Time, string, string) {
	t.Helper()
	jetzt := time.Now()
	st := &teilen.Store{Now: func() time.Time { return jetzt }}
	cfg := StartConfig{Teilen: st, Antworten: &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), kontaktantwort.Datei)},
		KontaktURL: "https://kontakt.example.org", TeaserHost: "example.org", Now: func() time.Time { return jetzt }}
	id, token, _, err := st.Neu("michael", "Michael")
	if err != nil {
		t.Fatal(err)
	}
	return cfg, st, &jetzt, id, token
}

func teilenGet(cfg StartConfig, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "https://kontakt.example.org"+path, nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec := httptest.NewRecorder()
	KontaktServer(cfg).ServeHTTP(rec, req)
	return rec
}

// Before anything is released: the sharer's first name and a quiet waiting line; the headers that
// keep the token to this page; the card about Homes linking to the box's teaser.
func TestTeilenWaiting(t *testing.T) {
	cfg, _, _, _, token := teilenCfg(t)
	rec := teilenGet(cfg, "/t/"+token)
	if rec.Code != 200 || rec.Header().Get("Referrer-Policy") != "no-referrer" || rec.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(rec.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	mustContain(t, body, "<h1 id=t-titel>Michael möchte etwas mit dir teilen …</h1>", "<p class=wartet>", "Michael wählt gerade aus, was geteilt wird.",
		`data-live="/t/`+token+`/live"`, "Geteilt mit Homes – das Zuhause der ganzen Familie, auf eurer eigenen Box statt in fremden Clouds.",
		`<a href="https://example.org/" rel="noopener noreferrer">Mehr erfahren →</a>`, `<section id=t-karte class=teil hidden></section>`)
	mustNotContain(t, body, "<form", "Gast-WLAN")
	cfg.TeaserHost = ""
	mustContain(t, teilenGet(cfg, "/t/"+token).Body.String(), `<a href="https://github.com/mdopp/homes"`)
	for _, p := range []string{"/t/" + token[:42] + "A", "/t/" + token + "/karte.vcf", "/t/" + token + "/sonst", "/t/" + token + "?x=1"} {
		if rec := teilenGet(cfg, p); rec.Code != 404 || !strings.Contains(rec.Body.String(), "Link ungültig oder abgelaufen") {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}

// A card's subset: only the released fields, on the page and in the .vcf.
func TestTeilenCardSubset(t *testing.T) {
	cfg, st, _, id, token := teilenCfg(t)
	st.Setzen(id, "michael", "", teilen.Freigabe{Karte: &teilen.Karte{Name: "Michael Dopp", Mobil: "+49 170 1234567", Geburtstag: "1980-04-01"}}, teilen.Einladung{})
	body := teilenGet(cfg, "/t/"+token).Body.String()
	mustContain(t, body, "<h2>Michaels Kontakt speichern</h2>", "<dt>Name</dt><dd>Michael Dopp</dd>", "<dt>Mobil</dt><dd>+49 170 1234567</dd>",
		"<dt>Geburtstag</dt><dd>1. April 1980</dd>", `href="/t/`+token+`/karte.vcf" download>Kontakt speichern (.vcf)</a>`)
	mustNotContain(t, body, "<dt>E-Mail</dt>", "<dt>Adresse</dt>", "<dt>Telefon</dt>", "<p class=wartet>")
	rec := teilenGet(cfg, "/t/"+token+"/karte.vcf")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/vcard; charset=utf-8" || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment;") ||
		rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	vcf := rec.Body.String()
	mustContain(t, vcf, "FN:Michael Dopp\r\n", "TEL;TYPE=CELL:+49 170 1234567\r\n", "BDAY:1980-04-01\r\n")
	mustNotContain(t, vcf, "EMAIL", "ADR", "ORG", "TYPE=VOICE")
	if genitiv("Hans") != "Hans’" || genitiv("Anna") != "Annas" {
		t.Errorf("genitiv: %s %s", genitiv("Hans"), genitiv("Anna"))
	}
}

// The guest Wi-Fi: its name and password, each with „Kopieren“; an open one says so.
func TestTeilenWLAN(t *testing.T) {
	cfg, st, _, id, token := teilenCfg(t)
	st.Setzen(id, "michael", "", teilen.Freigabe{WLAN: &teilen.WLAN{SSID: "Gast <Dopp>", Kennwort: "Sup3r\"Geheim"}}, teilen.Einladung{})
	body := teilenGet(cfg, "/t/"+token).Body.String()
	mustContain(t, body, "<h2>Gast-WLAN</h2>", "<code>Gast &lt;Dopp&gt;</code>", `data-kopie="Sup3r&#34;Geheim"`, ">Kopieren</button>", `data-kopiert="Kopiert ✓"`)
	mustNotContain(t, body, "<h2>Michaels Kontakt speichern</h2>", "WIFI:")
	st.Setzen(id, "michael", "", teilen.Freigabe{WLAN: &teilen.WLAN{SSID: "Gast", Offen: true}}, teilen.Einladung{})
	body = teilenGet(cfg, "/t/"+token).Body.String()
	mustContain(t, body, "Offenes Netz – kein Kennwort nötig.")
	mustNotContain(t, body, "Kennwort</span>")
}

// „Und du?“: the form of Kontaktdaten eintragen lassen, posting to its own link, with the Contact
// Picker and the .vcf file; once answered, the thanks.
func TestTeilenIntake(t *testing.T) {
	cfg, st, _, id, token := teilenCfg(t)
	link, f, _ := cfg.Antworten.Einladen("michael", "Michael", cfg.KontaktURL)
	ein := teilen.Einladung{Token: link[strings.LastIndex(link, "/")+1:], ID: f.ID}
	st.Setzen(id, "michael", "", teilen.Freigabe{Eintragen: true}, ein)
	body := teilenGet(cfg, "/t/"+token).Body.String()
	mustContain(t, body, "<h2>Und du?</h2>", `<form method=post enctype=multipart/form-data action="/`+ein.Token+`"`, `id=t-picker hidden>Aus deinen Kontakten wählen`,
		`Visitenkarte (.vcf) wählen<input type=file id=t-vcf accept=".vcf,text/vcard,text/x-vcard"`, "Damit dich Michael im Adressbuch hat", "Angaben senden")
	mustNotContain(t, body, "<p class=wartet>")
	cfg.Antworten.Beantworten(ein.Token, kontaktantwort.Antwort{Aenderungen: []kontaktantwort.Aenderung{{Art: "vorname", Nachher: "Erika"}}})
	body = teilenGet(cfg, "/t/"+token).Body.String()
	mustContain(t, body, "Danke – Michael hat deine Angaben bekommen.")
	mustNotContain(t, body, "<form")
	st.Setzen(id, "michael", "", teilen.Freigabe{}, teilen.Einladung{}) // taken back: the section goes, the waiting line is back
	mustContain(t, teilenGet(cfg, "/t/"+token).Body.String(), "<p class=wartet>")
}

// Ended, and over: said in words; nothing released is served any more.
func TestTeilenEndedAndExpired(t *testing.T) {
	cfg, st, jetzt, id, token := teilenCfg(t)
	st.Setzen(id, "michael", "", teilen.Freigabe{Karte: &teilen.Karte{Mobil: "0170 1"}, WLAN: &teilen.WLAN{SSID: "Gast", Kennwort: "geheim"}}, teilen.Einladung{})
	st.Beenden(id, "michael")
	body := teilenGet(cfg, "/t/"+token).Body.String()
	mustContain(t, body, "<h1 id=t-titel>Michael hat das Teilen beendet.</h1>", "Was geteilt war, ist hier nicht mehr zu sehen.")
	mustNotContain(t, body, "0170 1", "geheim", "<p class=wartet>")
	if rec := teilenGet(cfg, "/t/"+token+"/karte.vcf"); rec.Code != 404 {
		t.Errorf("the .vcf after the end: %d", rec.Code)
	}
	_, token2, _, _ := st.Neu("michael", "Michael")
	*jetzt = jetzt.Add(teilen.Dauer)
	body = teilenGet(cfg, "/t/"+token2).Body.String()
	mustContain(t, body, "<h1 id=t-titel>Das Teilen ist abgelaufen.</h1>", "Michael hat 30 Minuten lang geteilt, die Zeit ist um. Brauchst du noch etwas, bitte Michael, es noch einmal zu teilen.")
}

// The live stream: the parts at once, again when the sharer releases something, and a last one
// at the end; it outlives the listener's own write and read timeouts (20 s on the box).
func TestTeilenLivePush(t *testing.T) {
	cfg, st, _, id, token := teilenCfg(t)
	cfg.TeaserHost = ""
	alt := teilenPing
	teilenPing = 50 * time.Millisecond
	t.Cleanup(func() { teilenPing = alt })
	srv := httptest.NewUnstartedServer(KontaktServer(cfg))
	srv.Config.WriteTimeout, srv.Config.ReadTimeout = 300*time.Millisecond, 300*time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)
	res, err := http.Get(srv.URL + "/t/" + token + "/live")
	if err != nil || res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%v %v", res, err)
	}
	defer res.Body.Close()
	events := make(chan teilenTeile, 10)
	go func() {
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			if d, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var p teilenTeile
				json.Unmarshal([]byte(d), &p)
				events <- p
			}
		}
		close(events)
	}()
	next := func() teilenTeile {
		t.Helper()
		select {
		case p, ok := <-events:
			if !ok {
				t.Fatal("the stream ended")
			}
			return p
		case <-time.After(3 * time.Second):
			t.Fatal("no event")
		}
		return teilenTeile{}
	}
	if p := next(); !strings.Contains(p.Warten, "wartet") || p.Karte != "" {
		t.Fatalf("first: %+v", p)
	}
	time.Sleep(500 * time.Millisecond) // past both timeouts
	st.Setzen(id, "michael", "", teilen.Freigabe{Karte: &teilen.Karte{Mobil: "0170 1"}}, teilen.Einladung{})
	if p := next(); p.Warten != "" || !strings.Contains(p.Karte, "0170 1") || p.Vorbei {
		t.Fatalf("after the release: %+v", p)
	}
	st.Beenden(id, "michael")
	if p := next(); !p.Vorbei || p.Karte != "" || !strings.Contains(p.Titel, "beendet") {
		t.Fatalf("after the end: %+v", p)
	}
	select {
	case _, ok := <-events:
		if ok {
			t.Error("an event after the end")
		}
	case <-time.After(2 * time.Second):
		t.Error("the stream stayed open after the end")
	}
}
