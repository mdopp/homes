package uebernahme

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	m.Write([]byte("archiv-uebernahme-1." + d))
	return d, base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

var (
	jetzt    = time.Date(2026, 11, 10, 9, 40, 0, 0, time.UTC)
	haushalt = Zeile{X: Ende{Name: "Carla Wolf", Haushalt: "c"}, Y: Ende{Name: "Anna Berg", Haushalt: "a"}, Art: "kollege"}
	gemischt = Zeile{X: Ende{Name: "Kim Berg", Handy: "k3"}, Y: Ende{Name: "Anna Berg"}, Art: "kind"} // Anna is in the household book, Kim only on the phone
)

// The packet is taken only as the archive signed it: for its person, with
// that person's secret, before bis and at most two hours ahead, with a
// line that can go somewhere and a kind Homes knows.
func TestLesen(t *testing.T) {
	gut := Paket{Person: "anna", Bis: jetzt.Add(80 * time.Minute), Zeilen: []Zeile{haushalt, gemischt}}
	d, s := Signieren("geheim", gut)
	if p, err := Lesen(d, s, "geheim", "anna", jetzt); err != nil || len(p.Zeilen) != 2 || p.Zeilen[1] != gemischt {
		t.Fatalf("%+v %v", p, err)
	}
	for name, c := range map[string]struct{ d, s, secret, person string }{
		"another person":       {d, s, "geheim", "ben"},
		"another secret":       {d, s, "ben-geheim", "anna"},
		"no secret":            {d, s, "", "anna"},
		"changed after signed": {strings.Replace(d, "a", "b", 1), s, "geheim", "anna"},
		"no signature":         {d, "", "geheim", "anna"},
	} {
		if _, err := Lesen(c.d, c.s, c.secret, c.person, jetzt); err != ErrUngueltig {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, p := range map[string]Paket{
		"too old":          {Person: "anna", Bis: jetzt, Zeilen: gut.Zeilen},
		"too far ahead":    {Person: "anna", Bis: jetzt.Add(3 * time.Hour), Zeilen: gut.Zeilen},
		"no kind":          {Person: "anna", Bis: gut.Bis, Zeilen: []Zeile{{X: haushalt.X, Y: haushalt.Y, Art: "keine"}}},
		"nowhere to go":    {Person: "anna", Bis: gut.Bis, Zeilen: []Zeile{{X: Ende{Name: "Kim"}, Y: Ende{Name: "Gerd", Haushalt: "g"}, Art: "freund"}}},
		"for someone else": {Person: "ben", Bis: gut.Bis, Zeilen: gut.Zeilen},
	} {
		if d, s := Signieren("geheim", p); func() error { _, err := Lesen(d, s, "geheim", "anna", jetzt); return err }() != ErrUngueltig {
			t.Errorf("%s: taken", name)
		}
	}
}

// Two household contacts: both cards get the other by UID with the kind
// seen from it, a line naming the other goes. Anything with the person's
// phone contact: only the phone, by name; the household card of the other
// end gets nothing, never a name.
func TestSchreiben(t *testing.T) {
	dav := davfake.New("homes", "pw")
	t.Cleanup(dav.Close)
	dav.AddAddressBook("/homes/haushalt-kontakte/", "Haushalt")
	karte := func(uid, fn, extra string) string {
		return "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + fn + "\r\n" + extra + "END:VCARD\r\n"
	}
	dav.Put("/homes/haushalt-kontakte/a.vcf", karte("a", "Anna Berg", "RELATED;TYPE=friend;VALUE=text:Carla Wolf\r\n"))
	dav.Put("/homes/haushalt-kontakte/c.vcf", karte("c", "Carla Wolf", ""))
	c := &caldav.Client{URL: dav.URL, User: "homes", Password: "pw", Loc: time.UTC}
	an := &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), kontaktantwort.Datei)}
	if err := Schreiben(t.Context(), c, an, "anna", haushalt); err != nil {
		t.Fatal(err)
	}
	if err := Schreiben(t.Context(), c, an, "anna", gemischt); err != nil {
		t.Fatal(err)
	}
	cards, _, _ := c.Cards(t.Context())
	text := map[string]string{}
	for _, cd := range cards {
		text[cd.UID] = cd.Text()
	}
	if !strings.Contains(text["a"], "RELATED;TYPE=colleague:urn:uuid:c\r\n") || !strings.Contains(text["c"], "RELATED;TYPE=colleague:urn:uuid:a\r\n") ||
		strings.Contains(text["a"], "Carla Wolf\r\nRELATED") || strings.Count(text["a"], "RELATED") != 1 {
		t.Fatalf("the household cards:\n%s\n%s", text["a"], text["c"])
	}
	if strings.Contains(text["a"]+text["c"], "Kim") {
		t.Fatalf("a phone contact's name in a household card:\n%s", text["a"])
	}
	if ks := an.FuersHandy("anna"); len(ks) != 1 || ks[0].UID != "k3" || ks[0].Aenderungen[0].Nachher != "Anna Berg" || ks[0].Aenderungen[0].NachherArt != "eltern" || len(an.FuersHandy("ben")) != 0 {
		t.Fatalf("the phone: %+v", ks)
	}
	if Schreiben(t.Context(), nil, an, "anna", haushalt) != ErrUngueltig || Schreiben(t.Context(), c, nil, "anna", gemischt) != ErrUngueltig {
		t.Fatal("written without the book or the phone")
	}
}
