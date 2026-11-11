package caldav

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"slices"
	"strings"
)

// Kontakt prüfen lassen (owner 2026-11-07): the owner accepted a person's
// own corrections of their card, field by field. Each is written into the
// stored card as a change of one line: the line that holds the old value
// gets the new one (its parameters kept), is dropped, or a new line is
// added. Every other line of the card stays as it was. A field whose old
// value is no longer on the card (changed meanwhile) is left alone and
// said so; nothing is ever written over a value the owner did not see.

// Korrektur is one accepted change of a card's field: Prop is N-GIVEN,
// N-FAMILY (FN follows them), TEL, EMAIL, ADR, BDAY, ORG (its first part),
// TITLE, PHOTO (Bild, a JPEG, replaces the card's), NOTE (Neu is added
// to it), RELATED (Art its TYPE, korrigiertBeziehung) or CATEGORIES (one group); Alt "" adds, Neu "" removes. For ADR the values are AltAdr and
// NeuAdr; for BDAY JJJJ-MM-TT or --MM-TT; for TEL and EMAIL Art is the new
// kind (mobil, privat, geschaeftlich, andere; "" keeps the line's) and
// Haupt whether it is the main one (nil keeps it; the main one is the
// only one, PREF=1 or in vCard 3.0 TYPE=PREF). Land is the household's
// calling code a TEL is compared in (TelSchluessel; "" is 49).
type Korrektur struct {
	Prop           string
	Alt, Neu, Art  string
	Land           string
	Haupt          *bool
	AltAdr, NeuAdr *Address
	Bild           []byte
}

// ErrNoCard is a card that is not in the shared address books (any more).
var ErrNoCard = errors.New("Adressbuch: diesen Kontakt gibt es im Adressbuch nicht mehr")

// CorrectShared writes ks into the shared card uid over the version it
// read; another write meanwhile is read again, up to three times. The
// answer says, for each of ks, whether it was written.
func (c *Client) CorrectShared(ctx context.Context, uid string, ks []Korrektur) ([]bool, error) {
	for range 3 {
		at, err := c.FamilyCard(ctx, uid)
		if err != nil {
			return nil, err
		}
		if at.Data == "" {
			return nil, ErrNoCard
		}
		text, done := Corrected(at.Data, uid, ks, utc(c.now()))
		if !slices.Contains(done, true) {
			return done, nil
		}
		u, err := c.resolve(at.Collection + url.PathEscape(at.Name))
		if err != nil {
			return nil, err
		}
		if _, err = c.rewrite(ctx, u, "text/vcard", at.ETag, text); !errors.Is(err, ErrChanged) {
			return done, err
		}
	}
	return nil, ErrChanged
}

// Corrected is data with ks written into the VCARD uid, and which of ks
// were: each against the card as the ones before it left it, and a fresh
// REV when any was.
func Corrected(data, uid string, ks []Korrektur, rev string) (string, []bool) {
	done := make([]bool, len(ks))
	var b strings.Builder
	blocks(unfold(data), "VCARD", func(ls []string, block bool) {
		if own, _ := ownUID(ls); !block || own != uid || len(ls) < 2 {
			for _, l := range ls {
				fold(&b, l)
			}
			return
		}
		body := append([]string(nil), ls[1:len(ls)-1]...)
		for i, k := range ks {
			body, done[i] = korrigiert(body, k)
		}
		if slices.Contains(done, true) {
			body = setLine(body, "REV", "REV:"+rev)
		}
		fold(&b, ls[0])
		for _, l := range body {
			fold(&b, l)
		}
		fold(&b, ls[len(ls)-1])
	})
	return b.String(), done
}

// korrigiert is body with k written, and whether it was.
func korrigiert(body []string, k Korrektur) ([]string, bool) {
	switch k.Prop {
	case "N-GIVEN", "N-FAMILY":
		return korrigiertName(body, k)
	case "BDAY":
		i := find(body, "BDAY", func(Prop) bool { return true })
		cur := ""
		if i >= 0 {
			p, _ := parseLine(body[i])
			cur = birthday(UnescapeText(p.Value), p.Params["X-APPLE-OMIT-YEAR"])
		}
		if cur != k.Alt || (k.Neu != "" && bdayValue(k.Neu) == "") {
			return body, false
		}
		if i >= 0 {
			body = append(body[:i:i], body[i+1:]...)
		}
		if k.Neu != "" {
			body = append(body, "BDAY:"+bdayValue(k.Neu))
		}
		return body, true
	case "ORG", "TITLE":
		return korrigiertEinzeln(body, k)
	case "TEL", "EMAIL":
		return korrigiertWert(body, k)
	case "ADR":
		return korrigiertAdresse(body, k.AltAdr, k.NeuAdr)
	case "PHOTO":
		return korrigiertFoto(body, k.Bild)
	case "NOTE":
		return korrigiertNotiz(body, strings.TrimSpace(k.Neu))
	case "RELATED":
		return korrigiertBeziehung(body, k)
	case "CATEGORIES":
		return korrigiertGruppe(body, strings.TrimSpace(k.Alt), strings.TrimSpace(k.Neu))
	}
	return body, false
}

// korrigiertBeziehung writes a relation (owner 2026-11-09, netz): RELATED;TYPE=Art, Neu its other (a
// urn:uuid:, else a name as text). Alt "" adds it, one to the same other already there counts as written;
// Alt the same as Neu sets its TYPE, Neu "" takes Alt's lines away (owner 2026-11-09: änderbar); none is re-pointed.
func korrigiertBeziehung(body []string, k Korrektur) ([]string, bool) {
	alt, neu := strings.TrimSpace(k.Alt), strings.TrimSpace(k.Neu)
	text := map[bool]string{true: ";VALUE=text"}[!strings.HasPrefix(neu, "urn:uuid:")]
	von := func(v string) func(Prop) bool {
		return func(p Prop) bool { return strings.EqualFold(UnescapeText(p.Value), v) }
	}
	switch {
	case alt != "" && neu != "" && alt != neu, alt == "" && neu == "", neu != "" && k.Art == "":
		return body, false
	case alt == "" && find(body, "RELATED", von(neu)) >= 0:
		return body, true
	}
	for i := find(body, "RELATED", von(alt)); alt != "" && i >= 0; i = find(body, "RELATED", von(alt)) {
		body = append(body[:i:i], body[i+1:]...)
	}
	if neu == "" {
		return body, true
	}
	return append(body, "RELATED;TYPE="+k.Art+text+":"+EscapeText(neu)), true
}

// korrigiertGruppe adds the group neu to CATEGORIES or takes alt out of it (owner 2026-11-10: „Gruppen“),
// one value of the list, every other value and line kept; one there already, or gone already, counts as written.
func korrigiertGruppe(body []string, alt, neu string) ([]string, bool) {
	gleich := func(c string) bool { return strings.EqualFold(strings.TrimSpace(c), alt+neu) }
	i := find(body, "CATEGORIES", func(p Prop) bool { return slices.ContainsFunc(listParts(p.Value), gleich) })
	if (alt == "") == (neu == "") || (i >= 0) == (neu != "") {
		return body, (alt == "") != (neu == "")
	}
	if neu != "" {
		if i = find(body, "CATEGORIES", alle); i < 0 {
			return append(body, "CATEGORIES:"+EscapeText(neu)), true
		}
	}
	p, _ := parseLine(body[i])
	var vs []string
	for _, c := range append(slices.DeleteFunc(listParts(p.Value), gleich), neu) {
		if strings.TrimSpace(c) != "" {
			vs = append(vs, EscapeText(c))
		}
	}
	if len(vs) == 0 {
		return append(body[:i:i], body[i+1:]...), true
	}
	return withValue(body, i, strings.Join(vs, ",")), true
}

// korrigiertFoto is body with its own pictures replaced by the JPEG bild.
func korrigiertFoto(body []string, bild []byte) ([]string, bool) {
	if len(bild) == 0 {
		return body, false
	}
	for i := find(body, "PHOTO", alle); i >= 0; i = find(body, "PHOTO", alle) {
		body = append(body[:i:i], body[i+1:]...)
	}
	b64 := base64.StdEncoding.EncodeToString(bild)
	if vier(body) {
		return append(body, "PHOTO:data:image/jpeg;base64,"+b64), true
	}
	return append(body, "PHOTO;ENCODING=b;TYPE=JPEG:"+b64), true
}

// korrigiertNotiz is body with neu added to its NOTE, after an empty line.
func korrigiertNotiz(body []string, neu string) ([]string, bool) {
	i := find(body, "NOTE", alle)
	switch {
	case neu == "":
		return body, false
	case i < 0:
		return append(body, "NOTE:"+EscapeText(neu)), true
	}
	p, _ := parseLine(body[i])
	if cur := UnescapeText(p.Value); !strings.Contains(cur, neu) {
		return withValue(body, i, EscapeText(strings.TrimSpace(cur+"\n\n"+neu))), true
	}
	return body, true // there already
}

func alle(Prop) bool { return true }

// vier says whether the card is a vCard 4.0.
func vier(body []string) bool {
	i := find(body, "VERSION", alle)
	if i < 0 {
		return false
	}
	p, _ := parseLine(body[i])
	return strings.TrimSpace(p.Value) == "4.0"
}

// korrigiertName changes N's given or family name, and FN with them. A
// card without N gets one from its FN first (the page split it so).
func korrigiertName(body []string, k Korrektur) ([]string, bool) {
	i := find(body, "N", func(Prop) bool { return true })
	if i < 0 {
		fn := find(body, "FN", func(Prop) bool { return true })
		if fn < 0 {
			return body, false
		}
		p, _ := parseLine(body[fn])
		body, i = append(body, "N:"+nameParts(UnescapeText(p.Value))), len(body)
	}
	p, _ := parseLine(body[i])
	n := append(parts(p.Value), "", "", "", "", "")[:max(5, len(parts(p.Value)))]
	at := 1
	if k.Prop == "N-FAMILY" {
		at = 0
	}
	if strings.TrimSpace(n[at]) != strings.TrimSpace(k.Alt) {
		return body, false
	}
	n[at] = strings.TrimSpace(k.Neu)
	esc := make([]string, len(n))
	for j, x := range n {
		esc[j] = EscapeText(x)
	}
	body = withValue(body, i, strings.Join(esc, ";"))
	if fn := find(body, "FN", func(Prop) bool { return true }); fn >= 0 {
		if name := strings.TrimSpace(n[1] + " " + n[0]); name != "" {
			body = withValue(body, fn, EscapeText(name))
		}
	}
	return body, true
}

// korrigiertEinzeln changes TITLE, or ORG's first part (its units kept).
func korrigiertEinzeln(body []string, k Korrektur) ([]string, bool) {
	i := find(body, k.Prop, func(Prop) bool { return true })
	var cur []string
	if i >= 0 {
		p, _ := parseLine(body[i])
		cur = []string{UnescapeText(p.Value)}
		if k.Prop == "ORG" {
			cur = parts(p.Value)
		}
	}
	if strings.TrimSpace(append(cur, "")[0]) != strings.TrimSpace(k.Alt) {
		return body, false
	}
	neu := strings.TrimSpace(k.Neu)
	if k.Prop == "ORG" && len(cur) > 1 {
		rest := make([]string, len(cur)-1)
		for j, x := range cur[1:] {
			rest[j] = EscapeText(x)
		}
		return withValue(body, i, EscapeText(neu)+";"+strings.Join(rest, ";")), true
	}
	switch {
	case neu == "" && i >= 0:
		return append(body[:i:i], body[i+1:]...), true
	case neu == "":
		return body, false
	case i >= 0:
		return withValue(body, i, EscapeText(neu)), true
	}
	return append(body, k.Prop+":"+EscapeText(neu)), true
}

// korrigiertWert changes one TEL or EMAIL: alt's line, or a new one; its
// kind and whether it is the main one with it when k says so. A new main
// one takes that from the others of its kind. A number is never on the
// card twice (owner 2026-11-11): the line that says most stays (telRang).
func korrigiertWert(body []string, k Korrektur) ([]string, bool) {
	prop := k.Prop
	same := func(v string) func(Prop) bool {
		want := gleichWert(prop, v, k.Land)
		return func(p Prop) bool { return gleichWert(prop, UnescapeText(p.Value), k.Land) == want }
	}
	neu := strings.TrimSpace(k.Neu)
	i := find(body, prop, same(k.Alt))
	switch {
	case k.Alt == "" && (neu == "" || find(body, prop, same(neu)) >= 0):
		return body, neu != "" // there already: nothing to add, and nothing wrong
	case k.Alt == "":
		body, i = append(body, prop+":"), len(body)
	case i < 0:
		return body, false
	case neu == "":
		return append(body[:i:i], body[i+1:]...), true
	}
	out := append([]string(nil), body...)
	if k.Haupt != nil && *k.Haupt {
		for j, l := range out {
			if p, ok := parseLine(l); ok && j != i && propName(p.Name) == prop && isPref(p) {
				out[j] = wertZeile(l, p, "", false, vier(out)) + p.Value
			}
		}
	}
	p, _ := parseLine(out[i])
	if k.Art == "" && k.Haupt == nil {
		return eineJeNummer(prop, withValue(out, i, EscapeText(neu)), k.Land), true
	}
	pref := isPref(p)
	if k.Haupt != nil {
		pref = *k.Haupt
	}
	out[i] = wertZeile(out[i], p, k.Art, pref, vier(out)) + EscapeText(neu)
	return eineJeNummer(prop, out, k.Land), true
}

// wertZeile is line's group, name and parameters up to the colon, with the
// kind art ("" keeps the line's) and pref as asked; the other parameters
// and TYPE values (VOICE, INTERNET, …) stay.
func wertZeile(line string, p Prop, art string, pref, v4 bool) string {
	var types []string
	for _, t := range strings.Split(strings.ToUpper(p.Params["TYPE"]), ",") {
		t = strings.TrimSpace(t)
		kind := t == "CELL" || t == "HOME" || t == "WORK"
		if t != "" && t != "PREF" && !(kind && art != "") {
			types = append(types, t)
		}
	}
	if t := kindTypes[art]; t != "" {
		types = append(types, t)
	}
	if pref && !v4 {
		types = append(types, "PREF")
	}
	var b strings.Builder
	b.WriteString(line[:strings.IndexAny(line, ";:")])
	keys := make([]string, 0, len(p.Params))
	for key := range p.Params {
		if key != "TYPE" && key != "PREF" {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		v := p.Params[key]
		if strings.ContainsAny(v, ";:,") {
			v = `"` + v + `"`
		}
		b.WriteString(";" + key + "=" + v)
	}
	if len(types) > 0 {
		b.WriteString(";TYPE=" + strings.Join(types, ","))
	}
	if pref && v4 {
		b.WriteString(";PREF=1")
	}
	return b.String() + ":"
}

// gleichWert is a value as two are compared: a number as TelSchluessel
// in the calling code cc, an address by its letters in one case.
func gleichWert(prop, v, cc string) string {
	v = strings.TrimSpace(v)
	if prop == "TEL" {
		return TelSchluessel(v, cc)
	}
	return strings.ToLower(strings.TrimPrefix(v, "mailto:"))
}

// korrigiertAdresse changes one ADR, matched by street, code, place and country.
func korrigiertAdresse(body []string, alt, neu *Address) ([]string, bool) {
	if neu != nil && *neu == (Address{}) {
		neu = nil
	}
	if alt == nil {
		if neu == nil {
			return body, false
		}
		return append(body, "ADR:"+neu.value()), true
	}
	i := find(body, "ADR", func(p Prop) bool { return gleicheAdresse(AddressOf(p.Value), *alt) })
	if i < 0 {
		return body, false
	}
	if neu == nil {
		return append(body[:i:i], body[i+1:]...), true
	}
	return withValue(body, i, neu.value()), true
}

func gleicheAdresse(a, b Address) bool {
	n := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	return n(a.Street) == n(b.Street) && n(a.Code) == n(b.Code) && n(a.Locality) == n(b.Locality) && n(a.Country) == n(b.Country)
}

// AddressOf is an ADR value's street, code, place and country.
func AddressOf(v string) Address {
	p := append(parts(v), make([]string, 7)...)
	t := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	return Address{Street: t(p[2]), Locality: t(p[3]), Code: t(p[5]), Country: t(p[6])}
}

// nameParts is N for a name: the last word the family name, the rest given.
func nameParts(name string) string {
	ws := strings.Fields(name)
	if len(ws) < 2 {
		return EscapeText(name) + ";;;;"
	}
	return EscapeText(ws[len(ws)-1]) + ";" + EscapeText(strings.Join(ws[:len(ws)-1], " ")) + ";;;"
}

// find is the first top-level line of prop (any group) that ok accepts; -1 none.
func find(body []string, prop string, ok func(Prop) bool) int {
	depth := 0
	for i, l := range body {
		p, parsed := parseLine(l)
		switch {
		case !parsed:
		case p.Name == "BEGIN":
			depth++
		case p.Name == "END":
			depth--
		case depth == 0 && propName(p.Name) == prop && ok(p):
			return i
		}
	}
	return -1
}

// withValue is body with line i's value replaced, its name and parameters kept.
func withValue(body []string, i int, value string) []string {
	p, _ := parseLine(body[i])
	out := append([]string(nil), body...)
	out[i] = body[i][:len(body[i])-len(p.Value)] + value
	return out
}

// setLine is body with prop's line(s) replaced by line.
func setLine(body []string, prop, line string) []string {
	var out []string
	for _, l := range body {
		if p, ok := parseLine(l); ok && propName(p.Name) == prop {
			continue
		}
		out = append(out, l)
	}
	return append(out, line)
}
