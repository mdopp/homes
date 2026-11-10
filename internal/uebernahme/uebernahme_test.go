package uebernahme

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/caldav/davfake"
	"github.com/mdopp/homes/internal/kontaktantwort"
)

// Signieren is the archive's side (archiv apps/archiv/uebernahme.go), for the tests.
func Signieren(secret string, p Paket) (d, s string) {
	b, _ := json.Marshal(p)
	d = base64.RawURLEncoding.EncodeToString(b)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("archiv-uebernahme-2." + d))
	return d, base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

var (
	jetzt    = time.Date(2026, 11, 10, 9, 40, 0, 0, time.UTC)
	geheim   = map[string]string{"anna": "anna-geheim", "ben": "ben-geheim"}
	haushalt = Zeile{X: Ende{Name: "Carla Wolf", Haushalt: "c"}, Y: Ende{Name: "Anna Berg", Haushalt: "a"}, Art: "kollege"}
	gemischt = Zeile{X: Ende{Name: "Kim Berg", Handy: "k3"}, Y: Ende{Name: "Anna Berg"}, Art: "kind"} // Anna is in the household book, Kim only on the phone
)

func paket(person string, am time.Time, nonce string, zs ...Zeile) Paket {
	return Paket{Person: person, Am: am, Nonce: nonce, Zeilen: zs}
}

// The packet is taken only as the archive signed it: with the secret of
// the person it names, made within two minutes, with a nonce, with a line
// that can go somewhere and a kind Homes knows.
func TestLesen(t *testing.T) {
	gut := paket("anna", jetzt.Add(-time.Minute), "0123456789abcdef", haushalt, gemischt)
	d, s := Signieren("anna-geheim", gut)
	if p, err := Lesen(d, s, geheim, jetzt); err != nil || len(p.Zeilen) != 2 || p.Zeilen[1] != gemischt || p.Person != "anna" {
		t.Fatalf("%+v %v", p, err)
	}
	for name, c := range map[string]struct {
		d, s   string
		geheim map[string]string
	}{
		"another person's secret": {d, s, map[string]string{"anna": "ben-geheim"}},
		"no secret for anna":      {d, s, map[string]string{"ben": "ben-geheim"}},
		"changed after signed":    {strings.Replace(d, "a", "b", 1), s, geheim},
		"no signature":            {d, "", geheim},
	} {
		if _, err := Lesen(c.d, c.s, c.geheim, jetzt); err != ErrUngueltig {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, p := range map[string]Paket{
		"too old":         paket("anna", jetzt.Add(-3*time.Minute), gut.Nonce, haushalt),
		"from the future": paket("anna", jetzt.Add(time.Minute), gut.Nonce, haushalt),
		"no nonce":        paket("anna", gut.Am, "", haushalt),
		"no kind":         paket("anna", gut.Am, gut.Nonce, Zeile{X: haushalt.X, Y: haushalt.Y, Art: "keine"}),
		"nowhere to go":   paket("anna", gut.Am, gut.Nonce, Zeile{X: Ende{Name: "Kim"}, Y: Ende{Name: "Gerd", Haushalt: "g"}, Art: "freund"}),
		"names ben":       paket("ben", gut.Am, gut.Nonce, haushalt), // signed with anna's secret
	} {
		if d, s := Signieren("anna-geheim", p); func() error { _, err := Lesen(d, s, geheim, jetzt); return err }() != ErrUngueltig {
			t.Errorf("%s: taken", name)
		}
	}
}

type empfangTest struct {
	e      *Empfang
	c      *caldav.Client
	an     *kontaktantwort.Store
	jetzt  time.Time
	nonces int
}

func neuerEmpfang(t *testing.T) *empfangTest {
	dav := davfake.New("homes", "pw")
	t.Cleanup(dav.Close)
	dav.AddAddressBook("/homes/haushalt-kontakte/", "Haushalt")
	karte := func(uid, fn, extra string) string {
		return "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + fn + "\r\n" + extra + "END:VCARD\r\n"
	}
	dav.Put("/homes/haushalt-kontakte/a.vcf", karte("a", "Anna Berg", "RELATED;TYPE=friend;VALUE=text:Carla Wolf\r\n"))
	dav.Put("/homes/haushalt-kontakte/c.vcf", karte("c", "Carla Wolf", ""))
	et := &empfangTest{c: &caldav.Client{URL: dav.URL, User: "homes", Password: "pw", Loc: time.UTC},
		an: &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), kontaktantwort.Datei)}, jetzt: jetzt}
	et.e = &Empfang{Geheim: geheim, Kontakte: et.c, Handy: et.an, Now: func() time.Time { return et.jetzt }, Seit: jetzt.Add(-time.Hour)}
	return et
}

func (et *empfangTest) post(secret string, p Paket, hdr ...string) *httptest.ResponseRecorder {
	d, s := Signieren(secret, p)
	req := httptest.NewRequest("POST", Pfad, strings.NewReader(url.Values{"d": {d}, "s": {s}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	et.e.ServeHTTP(rec, req)
	return rec
}

func (et *empfangTest) nonce() string {
	et.nonces++
	return strings.Repeat("n", 15) + string(rune('a'+et.nonces))
}

func (et *empfangTest) karten(t *testing.T) map[string]string {
	cards, _, _ := et.c.Cards(t.Context())
	text := map[string]string{}
	for _, cd := range cards {
		text[cd.UID] = cd.Text()
	}
	return text
}

// The archive's tap, server to server: a good packet writes at once —
// two household contacts into both cards by UID, the phone contact's only
// to that person's phone, never a name into a household card — and weg
// takes both back. Another person's packet, a forged, an expired or a
// replayed one, and anything that came through the public route: refused,
// nothing written.
func TestEmpfang(t *testing.T) {
	et := neuerEmpfang(t)
	if rec := et.post("anna-geheim", paket("anna", et.jetzt, et.nonce(), haushalt, gemischt)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("good packet: %d %s", rec.Code, rec.Body)
	}
	text := et.karten(t)
	if !strings.Contains(text["a"], "RELATED;TYPE=colleague:urn:uuid:c\r\n") || !strings.Contains(text["c"], "RELATED;TYPE=colleague:urn:uuid:a\r\n") ||
		strings.Count(text["a"], "RELATED") != 1 || strings.Contains(text["a"]+text["c"], "Kim") {
		t.Fatalf("the household cards:\n%s\n%s", text["a"], text["c"])
	}
	if ks := et.an.FuersHandy("anna"); len(ks) != 1 || ks[0].UID != "k3" || ks[0].Aenderungen[0].Nachher != "Anna Berg" || ks[0].Aenderungen[0].NachherArt != "eltern" || len(et.an.FuersHandy("ben")) != 0 {
		t.Fatalf("the phone: %+v", ks)
	}

	carla := Zeile{X: Ende{Name: "Carla Wolf", Haushalt: "c"}, Y: Ende{Name: "Ben", Handy: "k9"}, Art: "freund"}
	for name, c := range map[string]struct {
		secret string
		p      Paket
		hdr    []string
		code   int
	}{
		"wrong person":       {"anna-geheim", paket("ben", et.jetzt, et.nonce(), carla), nil, http.StatusUnauthorized},
		"forged":             {"rate-mal", paket("anna", et.jetzt, et.nonce(), carla), nil, http.StatusUnauthorized},
		"expired":            {"anna-geheim", paket("anna", et.jetzt.Add(-3*time.Minute), et.nonce(), carla), nil, http.StatusUnauthorized},
		"before the start":   {"anna-geheim", paket("anna", et.e.Seit.Add(-time.Second), et.nonce(), carla), nil, http.StatusUnauthorized},
		"public route":       {"anna-geheim", paket("anna", et.jetzt, et.nonce(), carla), []string{"X-Homes-Proxy", "s3cret", "Remote-User", "anna"}, http.StatusForbidden},
		"forwarded":          {"anna-geheim", paket("anna", et.jetzt, et.nonce(), carla), []string{"X-Forwarded-For", "203.0.113.9"}, http.StatusForbidden},
		"a login, no secret": {"anna-geheim", paket("anna", et.jetzt, et.nonce(), carla), []string{"Remote-User", "anna"}, http.StatusForbidden},
	} {
		if rec := et.post(c.secret, c.p, c.hdr...); rec.Code != c.code || strings.Contains(rec.Body.String(), `"ok":true`) {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	// before the start: Seit is after am (Homes restarted since the packet was made)
	et.e.Seit = et.jetzt
	if rec := et.post("anna-geheim", paket("anna", et.jetzt.Add(-time.Second), et.nonce(), carla)); rec.Code != http.StatusConflict {
		t.Errorf("made before Homes started: %d", rec.Code)
	}
	if ks := et.an.FuersHandy("anna"); len(ks) != 1 || len(et.an.FuersHandy("ben")) != 0 {
		t.Fatalf("a refused packet wrote: %+v", ks)
	}

	// replayed: the same packet twice
	weg := paket("anna", et.jetzt, et.nonce(), haushalt, gemischt)
	weg.Weg = true
	if rec := et.post("anna-geheim", weg); rec.Code != http.StatusOK {
		t.Fatalf("weg: %d %s", rec.Code, rec.Body)
	}
	if rec := et.post("anna-geheim", weg); rec.Code != http.StatusConflict {
		t.Fatalf("replayed: %d", rec.Code)
	}
	text = et.karten(t)
	if strings.Contains(text["a"]+text["c"], "RELATED") {
		t.Fatalf("Rückgängig left a relation:\n%s\n%s", text["a"], text["c"])
	}
	if ks := et.an.FuersHandy("anna"); len(ks) != 1 || ks[0].Aenderungen[0].Nachher != "" || ks[0].Aenderungen[0].Vorher != "Anna Berg" {
		t.Fatalf("the phone's take-back: %+v", ks)
	}
	req := httptest.NewRequest("GET", Pfad, nil)
	rec := httptest.NewRecorder()
	et.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}
}

// Without the book or the phone nothing is written, and that is said.
func TestSchreibenOhneZiel(t *testing.T) {
	et := neuerEmpfang(t)
	if Schreiben(t.Context(), nil, et.an, "anna", haushalt, false) != ErrUngueltig || Schreiben(t.Context(), et.c, nil, "anna", gemischt, false) != ErrUngueltig {
		t.Fatal("written without the book or the phone")
	}
}
