package kontaktantwort

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"slices"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/caldav"
)

// owner 2026-11-08: street and house number apart, in either order on a
// card, written back as the country writes them.
func TestStrassenteile(t *testing.T) {
	for in, want := range map[string][2]string{
		"Hauptstraße 5": {"Hauptstraße", "5"}, "5 Hauptstraße": {"Hauptstraße", "5"}, "12a, Lindenweg": {"Lindenweg", "12a"},
		"Lindenweg 12 a": {"Lindenweg", "12 a"}, "Am Markt 3-5": {"Am Markt", "3-5"}, "Straße des 17. Juni 5": {"Straße des 17. Juni", "5"},
		"Straße des 17. Juni": {"Straße des 17. Juni", ""}, "Postfach": {"Postfach", ""}, "  Weg   1  ": {"Weg", "1"}, "Hauptstr. 5/1": {"Hauptstr.", "5/1"},
	} {
		if s, n := Strassenteile(in); s != want[0] || n != want[1] {
			t.Errorf("Strassenteile(%q) = %q, %q; want %q", in, s, n, want)
		}
	}
	for _, c := range [][4]string{{"Hauptstraße", "5", "", "Hauptstraße 5"}, {"Hauptstraße", "5", "Österreich", "Hauptstraße 5"},
		{"Main Street", "12", "USA", "12 Main Street"}, {"rue de la Paix", "3", "France", "3 rue de la Paix"}, {"Weg", "", "", "Weg"}} {
		if got := Strassenzeile(c[0], c[1], c[2]); got != c[3] {
			t.Errorf("Strassenzeile(%q, %q, %q) = %q", c[0], c[1], c[2], got)
		}
	}
}

// A card written „Hausnummer Straße“ (as the owner found one) is read with
// its parts, unchanged when the contact changes nothing, found by its own
// line when they do, and written back as „Straße Hausnummer“.
func TestTheAddressOfACardBothWays(t *testing.T) {
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:a1\r\nFN:A B\r\nADR;TYPE=HOME:;;5 Hauptstraße;Stadt;;12345;\r\nEND:VCARD\r\n"
	root, _ := caldav.Parse(card)
	f := FelderVon(caldav.Contacts(root)[0])
	if a := f.Adressen[0]; a.Strasse != "Hauptstraße" || a.Hausnummer != "5" || a.Wie != "5 Hauptstraße" {
		t.Fatalf("read %+v", a)
	}
	form := Sauber(Felder{Vorname: "A", Nachname: "B", Adressen: []Adresse{{Strasse: "Hauptstraße", Hausnummer: "5", PLZ: "12345", Ort: "Stadt"}}})
	if as := Vergleich(f, form); len(as) != 0 {
		t.Errorf("nothing changed, yet %+v", as)
	}
	form.Adressen[0].Hausnummer = "7"
	as := Vergleich(f, form)
	if len(as) != 1 || as[0].Vorher != "Hauptstraße 5, 12345 Stadt" || as[0].Nachher != "Hauptstraße 7, 12345 Stadt" {
		t.Fatalf("changes %+v", as)
	}
	as[0].Stand = Angenommen
	ks, _ := Korrekturen(Frage{Aenderungen: as})
	got, done := caldav.Corrected(card, "a1", ks, "x")
	if !done[0] || !strings.Contains(got, "ADR;TYPE=HOME:;;Hauptstraße 7;Stadt;;12345;\r\n") {
		t.Errorf("written %v:\n%s", done, got)
	}
	// the autofill's whole line in the street field alone is split too
	if a := Sauber(Felder{Adressen: []Adresse{{Strasse: "Lindenweg 3", PLZ: "70173"}}}).Adressen[0]; a.Strasse != "Lindenweg" || a.Hausnummer != "3" || a.Wie != "" {
		t.Errorf("one field: %+v", a)
	}
	// the phone gets one street line, the card's own for finding it
	if v, n := as[0].VorherAdresse.Zeilenweise(true), as[0].NachherAdresse.Zeilenweise(false); v.Strasse != "5 Hauptstraße" || n.Strasse != "Hauptstraße 7" || n.Hausnummer != "" {
		t.Errorf("for the phone %+v %+v", v, n)
	}
}

func TestPLZByCountryAndOrtOffline(t *testing.T) {
	for _, c := range [][3]string{{"12345", "", ""}, {"1234", "", "fünf Ziffern"}, {"123456", "Deutschland", "fünf Ziffern"}, {"1010", "Österreich", ""},
		{"1234 AB", "Niederlande", ""}, {"SW1A 1AA", "UK", ""}, {"12345", "Schweiz", "keine Postleitzahl"}, {"AB-12", "Island", ""}, {"<x>", "Island", "nur Buchstaben"}} {
		if h := PLZFehler(c[0], c[1]); !strings.Contains(h, c[2]) || (c[2] == "") != (h == "") {
			t.Errorf("PLZFehler(%q, %q) = %q", c[0], c[1], h)
		}
	}
	for _, c := range [][4]string{{"70173", "Stuttgart", "", ""}, {"70173", "Stuttgart-Mitte", "", ""}, {"70372", "Bad Cannstatt", "", ""},
		{"60311", "Frankfurt", "", ""}, {"60311", "Frankfurt a. M.", "Deutschland", ""}, {"80331", "München", "", ""}, {"10115", "berlin", "", ""},
		{"70173", "München", "", "gehört zu Stuttgart, nicht zu München"}, {"00000", "Irgendwo", "", "in keinem Straßenverzeichnis"},
		{"1010", "Wien", "Österreich", ""}, {"70173", "", "", ""}} {
		if h := OrtFehler(c[0], c[1], c[2]); !strings.Contains(h, c[3]) || (c[3] == "") != (h == "") {
			t.Errorf("OrtFehler(%q, %q, %q) = %q", c[0], c[1], c[2], h)
		}
	}
	if n := len(plzOrte()); n < 8000 {
		t.Errorf("the table holds %d postal codes", n)
	}
}

// What the contact changed must have its form; what the card had stays.
func TestFormfehler(t *testing.T) {
	vorher := Felder{Vorname: "R2D2", Telefone: []Telefon{{Nummer: "123"}}, Mails: []Mail{{Adresse: "kaputt"}}}
	if e := Formfehler(vorher, vorher, "49"); len(e) != 0 {
		t.Errorf("the card's own values refused: %v", e)
	}
	nachher := Felder{Vorname: "R2D2", Nachname: "O'Neill-Ünal", Telefone: []Telefon{{Nummer: "123"}, {Nummer: "0171 12"}, {Nummer: "+49 171 2345678"}},
		Mails:    []Mail{{Adresse: "kaputt"}, {Adresse: "a@b"}, {Adresse: "a@example.org"}},
		Adressen: []Adresse{{Strasse: "Weg", Hausnummer: "a1", PLZ: "1234", Ort: "Stadt1"}, {Strasse: "Weg", Hausnummer: "1", PLZ: "12345", Ort: "Stadt"}}}
	e := Formfehler(vorher, nachher, "49")
	want := []string{"adr-nr.0", "adr-ort.0", "adr-plz.0", "mail.1", "tel.1"}
	var keys []string
	for k := range e {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, want) {
		t.Errorf("errors %v, want %v", e, want)
	}
	if e := Formfehler(Felder{}, Felder{Vorname: "Anna3"}, "49"); !strings.Contains(e["vorname"], "Buchstaben") {
		t.Errorf("a digit in a name: %v", e)
	}
	if e := Formfehler(Felder{}, Felder{Adressen: []Adresse{{Ort: "Stadt"}}}, "49"); e["adr-strasse.0"] == "" || e["adr-plz.0"] == "" {
		t.Errorf("an address without street and code: %v", e)
	}
}

// owner 2026-11-08: the kinds and the main one compared both ways; a
// removal and the message only when ticked; a number the card has already
// (a browser's autofill) is no change; answers kept before read on.
func TestKindsMainRemovalsAndMessage(t *testing.T) {
	vorher := Felder{Telefone: []Telefon{{"0171 1", "mobil", true}, {"0711 2", "", false}}, Mails: []Mail{{"a@x.de", "privat", false}}}
	nachher := Sauber(Felder{Telefone: []Telefon{{"0171 1", "mobil", false}, {"0711 2", Andere, true}, {"0171-1", "mobil", false}},
		Mails: []Mail{{"", "", false}, {"b@x.de", "geschaeftlich", true}}, Nachricht: "Hallo\r\nich bin umgezogen", Foto: []byte{1}})
	as := Vergleich(vorher, nachher)
	var got []string
	for _, a := range as {
		got = append(got, a.Art+":"+a.Vorher+">"+a.Nachher)
	}
	if want := "telefon:0171 1>0171 1 telefon:0711 2>0711 2 mail:a@x.de> mail:>b@x.de foto:> notiz:>Hallo\nich bin umgezogen"; strings.Join(got, " ") != want {
		t.Fatalf("Vergleich\n got %s\nwant %s", strings.Join(got, " "), want)
	}
	if !as[0].VorherHaupt || as[0].NachherHaupt || !as[1].NachherHaupt || as[3].NachherArt != "geschaeftlich" || !as[3].NachherHaupt {
		t.Errorf("kinds %+v", as)
	}
	f := Frage{Aenderungen: as}
	if st := Stand(f, true, nil); !slices.Equal(st, []string{Angenommen, Angenommen, Abgelehnt, Angenommen, Angenommen, Abgelehnt}) {
		t.Errorf("Alle übernehmen took %v", st)
	}
	if st := Stand(f, true, []int{2, 5}); st[2] != Angenommen || st[5] != Angenommen {
		t.Errorf("ticked: %v", st)
	}
	for i := range f.Aenderungen {
		f.Aenderungen[i].Stand = Angenommen
	}
	ks, _ := Korrekturen(f)
	if ks[0].Haupt == nil || *ks[0].Haupt || ks[0].Art != "" || ks[1].Haupt == nil || !*ks[1].Haupt || ks[3].Art != "geschaeftlich" || ks[4].Prop != "PHOTO" || ks[5].Prop != "NOTE" {
		t.Errorf("Korrekturen %+v", ks)
	}
	var old Felder
	if err := json.Unmarshal([]byte(`{"mails":["a@x.de"]}`), &old); err != nil || old.Mails[0].Adresse != "a@x.de" {
		t.Errorf("an answer kept before: %+v %v", old, err)
	}
}

// The public upload: decoded, at most 512 pixels, a new JPEG without the
// file's metadata; anything that does not decode is refused.
func TestFotoIsDecodedAndMadeAgain(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1200, 800))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(0, 0, color.NRGBA{0, 0, 0, 0}) // transparent: white
	var p bytes.Buffer
	png.Encode(&p, img)
	var j bytes.Buffer
	jpeg.Encode(&j, img, nil)
	exif := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0x00, 0x14}, []byte("Exif\x00\x00GPS 48.7758 N...")...)
	withExif := append(exif, j.Bytes()[2:]...)
	for name, in := range map[string][]byte{"png": p.Bytes(), "jpeg with exif": withExif} {
		out, err := Foto(in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
		if err != nil || format != "jpeg" || cfg.Width != 512 || cfg.Height != 341 || bytes.Contains(out, []byte("Exif")) || bytes.Contains(out, []byte("GPS")) {
			t.Errorf("%s: %s %dx%d %v", name, format, cfg.Width, cfg.Height, err)
		}
	}
	for name, in := range map[string][]byte{"text": []byte("<svg onload=x>"), "cut": p.Bytes()[:len(p.Bytes())/2], "gif": []byte("GIF89a\x01\x00\x01\x00"),
		"huge": func() []byte {
			var b bytes.Buffer
			png.Encode(&b, image.NewGray(image.Rect(0, 0, 9000, 9000)))
			return b.Bytes()
		}()} {
		if _, err := Foto(in); err == nil {
			t.Errorf("%s taken", name)
		}
	}
}
