package kontaktantwort

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
)

// Adresse is a postal address as the page asks for it: street and house
// number apart (owner 2026-11-08). Wie is the card's street line as it was
// read, so the line is found again; the page never shows or sends it.
type Adresse struct {
	Strasse    string `json:"strasse,omitempty"`
	Hausnummer string `json:"hausnummer,omitempty"`
	PLZ        string `json:"plz,omitempty"`
	Ort        string `json:"ort,omitempty"`
	Land       string `json:"land,omitempty"`
	Wie        string `json:"wie,omitempty"`
}

// Leer says whether every part is blank.
func (a Adresse) Leer() bool { a.Wie = ""; return a == Adresse{} }

// gleich says whether a and b are the same address, however a was read.
func (a Adresse) gleich(b Adresse) bool {
	a, b = a.geteilt(), b.geteilt()
	a.Wie, b.Wie = "", ""
	return a == b
}

// geteilt is a with its house number apart from the street: a card's
// street line holds both, in either order.
func (a Adresse) geteilt() Adresse {
	if a.Hausnummer == "" && a.Strasse != "" {
		if a.Wie == "" {
			a.Wie = a.Strasse
		}
		a.Strasse, a.Hausnummer = Strassenteile(a.Strasse)
	}
	return a
}

// StrassenZeile is street and number as the address's country writes them.
func (a Adresse) StrassenZeile() string { return Strassenzeile(a.Strasse, a.Hausnummer, a.Land) }

// Zeile is the address on one line, as a letter has it.
func (a Adresse) Zeile() string {
	var ps []string
	for _, p := range []string{a.StrassenZeile(), strings.TrimSpace(a.PLZ + " " + a.Ort), a.Land} {
		if p != "" {
			ps = append(ps, p)
		}
	}
	return strings.Join(ps, ", ")
}

// dav is the address as Radicale writes it; alt: as the card has it.
func (a Adresse) dav(alt bool) *caldav.Address {
	street := a.StrassenZeile()
	if alt && a.Wie != "" {
		street = a.Wie
	}
	return &caldav.Address{Street: street, Code: a.PLZ, Locality: a.Ort, Country: a.Land}
}

// Zeilenweise is a for the phone, whose contact has one street line: the
// street and number in it (alt: the line as it was read); nil stays nil.
func (a *Adresse) Zeilenweise(alt bool) *Adresse {
	if a == nil {
		return nil
	}
	d := a.dav(alt)
	return &Adresse{Strasse: d.Street, PLZ: d.Code, Ort: d.Locality, Land: d.Country}
}

// Telefon is a number, its kind (one of Arten) and whether it is the main one.
type Telefon struct {
	Nummer string `json:"nummer"`
	Art    string `json:"art,omitempty"`
	Haupt  bool   `json:"haupt,omitempty"`
}

// Mail is an e-mail address, its kind (one of MailArten) and whether it is
// the main one. An answer kept before 2026-11-08 holds the address alone.
type Mail struct {
	Adresse string `json:"adresse"`
	Art     string `json:"art,omitempty"`
	Haupt   bool   `json:"haupt,omitempty"`
}

// UnmarshalJSON reads a Mail, or an address alone as answers kept it before.
func (m *Mail) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &m.Adresse)
	}
	type plain Mail
	return json.Unmarshal(b, (*plain)(m))
}

// Arten are the kinds of numbers, MailArten of e-mail addresses (owner
// 2026-11-08); "andere" is a vCard line without one of these TYPEs.
var (
	Arten     = []string{"mobil", "privat", "geschaeftlich", Andere}
	MailArten = []string{"privat", "geschaeftlich", Andere}
)

// Andere is the kind of a number or address that has none of the others.
const Andere = "andere"

// art is kind as one of arten: a card's "" (no such TYPE) and anything else is Andere.
func art(kind string, arten []string) string {
	if slices.Contains(arten, kind) {
		return kind
	}
	return Andere
}

// Felder are the fields the page shows and the contact may check and add
// (owner 2026-11-07): first and last name, numbers with their kind, e-mail
// addresses, postal addresses, the birthday (day, month, the year if they
// like), company and position; in an answer also their own photo and a
// message to the person who sent the link (owner 2026-11-08). Nothing
// else. In an answer each list keeps the places of the fields shown: an
// empty one at a place is that field taken away; what follows is new.
type Felder struct {
	Vorname    string    `json:"vorname,omitempty"`
	Nachname   string    `json:"nachname,omitempty"`
	Telefone   []Telefon `json:"telefone,omitempty"`
	Mails      []Mail    `json:"mails,omitempty"`
	Adressen   []Adresse `json:"adressen,omitempty"`
	Geburtstag string    `json:"geburtstag,omitempty"` // JJJJ-MM-TT, or --MM-TT without a year
	Firma      string    `json:"firma,omitempty"`
	Position   string    `json:"position,omitempty"`
	Foto       []byte    `json:"-"` // an answer's photo, a JPEG Foto made
	Nachricht  string    `json:"-"` // an answer's message
}

// Name is the name as a list shows it: first and last, else the company.
func (f Felder) Name() string {
	if n := strings.TrimSpace(f.Vorname + " " + f.Nachname); n != "" {
		return n
	}
	return f.Firma
}

// Grenze is how many of each kind an answer may hold, how long a value,
// and how long the message.
const (
	GrenzeJeArt     = 12
	GrenzeWert      = 200
	GrenzeNachricht = 1000
)

// FelderVon are a card's fields. A card without N (Homes' own write FN
// only) is split: the last word the last name.
func FelderVon(ct caldav.Contact) Felder {
	f := Felder{Vorname: eineZeile(ct.Given), Nachname: eineZeile(ct.Family), Firma: eineZeile(ct.Company), Position: eineZeile(ct.Title)}
	if f.Vorname == "" && f.Nachname == "" && eineZeile(ct.Name) != eineZeile(ct.Company) {
		if ws := strings.Fields(eineZeile(ct.Name)); len(ws) > 0 {
			f.Vorname, f.Nachname = strings.Join(ws[:len(ws)-1], " "), ws[len(ws)-1]
		}
	}
	for i, p := range ct.Phones {
		if p = eineZeile(p); p != "" {
			f.Telefone = append(f.Telefone, Telefon{p, art(at(ct.PhoneKinds, i), Arten), at(ct.PhonePrefs, i)})
		}
	}
	for i, m := range ct.Emails {
		if m = eineZeile(m); m != "" {
			f.Mails = append(f.Mails, Mail{m, art(at(ct.EmailKinds, i), MailArten), at(ct.EmailPrefs, i)})
		}
	}
	if caldav.ValidBirthday(ct.Birthday) {
		f.Geburtstag = ct.Birthday
	}
	for _, a := range ct.Postal {
		f.Adressen = append(f.Adressen, Adresse{Strasse: a.Street, PLZ: a.Code, Ort: a.Locality, Land: a.Country}.geteilt())
	}
	return f
}

// eineZeile is s on one line, trimmed, without control characters, at
// most GrenzeWert characters.
func eineZeile(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > GrenzeWert {
		s = string(r[:GrenzeWert])
	}
	return s
}

// Sauber is an answer's fields as they are kept: each value on one line
// and cut to its length, each list cut to GrenzeJeArt, a kind one of its
// kinds, one main one of each list at most, the message cut to its length.
func Sauber(f Felder) Felder {
	out := Felder{Vorname: eineZeile(f.Vorname), Nachname: eineZeile(f.Nachname), Geburtstag: strings.TrimSpace(f.Geburtstag),
		Firma: eineZeile(f.Firma), Position: eineZeile(f.Position), Foto: f.Foto, Nachricht: nachricht(f.Nachricht)}
	haupt := false
	for _, t := range f.Telefone[:min(len(f.Telefone), GrenzeJeArt)] {
		n := eineZeile(t.Nummer)
		t.Haupt = t.Haupt && n != "" && !haupt
		haupt = haupt || t.Haupt
		out.Telefone = append(out.Telefone, Telefon{n, art(t.Art, Arten), t.Haupt})
	}
	haupt = false
	for _, m := range f.Mails[:min(len(f.Mails), GrenzeJeArt)] {
		a := eineZeile(m.Adresse)
		m.Haupt = m.Haupt && a != "" && !haupt
		haupt = haupt || m.Haupt
		out.Mails = append(out.Mails, Mail{a, art(m.Art, MailArten), m.Haupt})
	}
	for _, a := range f.Adressen[:min(len(f.Adressen), GrenzeJeArt)] {
		x := Adresse{Strasse: eineZeile(a.Strasse), Hausnummer: eineZeile(a.Hausnummer), PLZ: eineZeile(a.PLZ), Ort: eineZeile(a.Ort), Land: eineZeile(a.Land)}.geteilt()
		x.Wie = ""
		out.Adressen = append(out.Adressen, x)
	}
	return out
}

// nachricht is the message with its lines, without control characters, cut to GrenzeNachricht.
func nachricht(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\n') || r == 0x7f {
			return -1
		}
		return r
	}, strings.ReplaceAll(s, "\r\n", "\n"))
	if r := []rune(strings.TrimSpace(s)); len(r) > GrenzeNachricht {
		return string(r[:GrenzeNachricht])
	}
	return strings.TrimSpace(s)
}

// Aenderung is one field, before → after; Vorher "" is new, Nachher ""
// taken away. A number or an e-mail address carries its kind and whether
// it is the main one before and after; the photo is Bild, the message
// (Art notiz) Nachher.
type Aenderung struct {
	Art            string   `json:"art"` // vorname | nachname | telefon | mail | adresse | geburtstag | firma | position | foto | notiz
	Vorher         string   `json:"vorher,omitempty"`
	Nachher        string   `json:"nachher,omitempty"`
	VorherArt      string   `json:"vorher_art,omitempty"`
	NachherArt     string   `json:"nachher_art,omitempty"`
	VorherHaupt    bool     `json:"vorher_haupt,omitempty"`
	NachherHaupt   bool     `json:"nachher_haupt,omitempty"`
	VorherAdresse  *Adresse `json:"vorher_adresse,omitempty"`
	NachherAdresse *Adresse `json:"nachher_adresse,omitempty"`
	Bild           []byte   `json:"bild,omitempty"`     // a JPEG of at most FotoKante pixels
	Hinweise       []string `json:"hinweise,omitempty"` // what the checks found
	Stand          string   `json:"stand,omitempty"`
}

// Entfernen says whether the change takes a field away (owner 2026-11-08:
// only when the person ticks it).
func (a Aenderung) Entfernen() bool { return a.Vorher != "" && a.Nachher == "" }

// NurAngekreuzt says whether only a tick takes it, never „Alle übernehmen“:
// a removal, and the message added to the note.
func (a Aenderung) NurAngekreuzt() bool { return a.Entfernen() || a.Art == "notiz" }

// Vergleich is every field the answer changes, in the page's order. A
// number written another way (spaces, dashes) with the same kind is no
// change; a name or company cleared is none either (the card keeps it).
// A new number or address the card has already (a browser's autofill) is
// none. The photo and the message come last.
func Vergleich(vorher, nachher Felder) []Aenderung {
	var out []Aenderung
	einzeln := func(art, v, n string, leerErlaubt bool) {
		if v != n && (n != "" || leerErlaubt) {
			out = append(out, Aenderung{Art: art, Vorher: v, Nachher: n})
		}
	}
	einzeln("vorname", vorher.Vorname, nachher.Vorname, vorher.Nachname != "" || nachher.Nachname != "")
	einzeln("nachname", vorher.Nachname, nachher.Nachname, vorher.Vorname != "" || nachher.Vorname != "")
	out = append(out, telefone(vorher.Telefone, nachher.Telefone)...)
	out = append(out, mails(vorher.Mails, nachher.Mails)...)
	out = append(out, adressen(vorher.Adressen, nachher.Adressen)...)
	einzeln("geburtstag", vorher.Geburtstag, nachher.Geburtstag, true)
	einzeln("firma", vorher.Firma, nachher.Firma, true)
	einzeln("position", vorher.Position, nachher.Position, true)
	if len(nachher.Foto) > 0 {
		out = append(out, Aenderung{Art: "foto", Bild: nachher.Foto})
	}
	einzeln("notiz", "", nachher.Nachricht, false)
	return out
}

// at is the place i of vs, the zero value past its end.
func at[T any](vs []T, i int) T {
	var zero T
	if i < len(vs) {
		return vs[i]
	}
	return zero
}

// eintrag is a number or an e-mail address as Vergleich compares them.
type eintrag struct {
	wert, art string
	haupt     bool
}

// eintraege compares two lists place by place: same is one value however
// written; a new one the list has already is none.
func eintraege(was string, vorher, nachher []eintrag, arten []string, same func(string) string) []Aenderung {
	var out []Aenderung
	for i := range max(len(vorher), len(nachher)) {
		v, n := at(vorher, i), at(nachher, i)
		v.art, n.art = art(v.art, arten), art(n.art, arten)
		if same(v.wert) == same(n.wert) && (n.wert == "" || v.art == n.art && v.haupt == n.haupt) {
			continue
		}
		if i >= len(vorher) && slices.ContainsFunc(vorher, func(x eintrag) bool { return same(x.wert) == same(n.wert) }) {
			continue
		}
		a := Aenderung{Art: was, Vorher: v.wert, Nachher: n.wert, VorherHaupt: v.haupt}
		if v.wert != "" {
			a.VorherArt = v.art
		}
		if n.wert != "" {
			a.NachherArt, a.NachherHaupt = n.art, n.haupt
		}
		out = append(out, a)
	}
	return out
}

func telefone(vorher, nachher []Telefon) []Aenderung {
	e := func(ts []Telefon) (out []eintrag) {
		for _, t := range ts {
			out = append(out, eintrag{t.Nummer, t.Art, t.Haupt})
		}
		return out
	}
	return eintraege("telefon", e(vorher), e(nachher), Arten, ziffern)
}

func mails(vorher, nachher []Mail) []Aenderung {
	e := func(ms []Mail) (out []eintrag) {
		for _, m := range ms {
			out = append(out, eintrag{m.Adresse, m.Art, m.Haupt})
		}
		return out
	}
	return eintraege("mail", e(vorher), e(nachher), MailArten, strings.ToLower)
}

func adressen(vorher, nachher []Adresse) []Aenderung {
	var out []Aenderung
	for i := range max(len(vorher), len(nachher)) {
		v, n := at(vorher, i).geteilt(), at(nachher, i)
		if v.gleich(n) || (i >= len(vorher) && (n.Leer() || slices.ContainsFunc(vorher, n.gleich))) {
			continue
		}
		a := Aenderung{Art: "adresse", Vorher: v.Zeile(), Nachher: n.Zeile()}
		if !v.Leer() {
			a.VorherAdresse = &v
		}
		if !n.Leer() {
			a.NachherAdresse = &n
		}
		out = append(out, a)
	}
	return out
}

func ziffern(s string) string {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		if (r >= '0' && r <= '9') || (r == '+' && i == 0) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// davProp is each kind's vCard property.
var davProp = map[string]string{"vorname": "N-GIVEN", "nachname": "N-FAMILY", "telefon": "TEL", "mail": "EMAIL", "adresse": "ADR",
	"geburtstag": "BDAY", "firma": "ORG", "position": "TITLE", "foto": "PHOTO", "notiz": "NOTE"}

// Korrekturen are the accepted changes as Radicale writes them, and the
// number of each among f's changes. A number's or address's kind goes
// along only when it changed, so a line keeps its own TYPEs otherwise.
func Korrekturen(f Frage) ([]caldav.Korrektur, []int) {
	var ks []caldav.Korrektur
	var nr []int
	for i, a := range f.Aenderungen {
		if a.Stand != Angenommen {
			continue
		}
		k := caldav.Korrektur{Prop: davProp[a.Art], Alt: a.Vorher, Neu: a.Nachher, Bild: a.Bild}
		if a.NachherArt != a.VorherArt {
			k.Art = a.NachherArt
		}
		if haupt := a.NachherHaupt; haupt != a.VorherHaupt && a.Nachher != "" {
			k.Haupt = &haupt
		}
		if a.Art == "adresse" {
			k.Alt, k.Neu = "", ""
			if a.VorherAdresse != nil {
				k.AltAdr = a.VorherAdresse.dav(true)
			}
			if a.NachherAdresse != nil {
				k.NeuAdr = a.NachherAdresse.dav(false)
			}
		}
		ks = append(ks, k)
		nr = append(nr, i)
	}
	return ks, nr
}
