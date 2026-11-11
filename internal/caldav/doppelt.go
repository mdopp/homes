package caldav

import (
	"cmp"
	"context"
	"path"
	"strings"
)

// Doppelte Nummern (owner 2026-11-11: „Viele Kontakte haben mehrfach die gleiche Nummer“): the
// phone sends a number once per raw contact it joined (Google, WhatsApp, Signal), and „0171 …“ and
// „+49 171 …“ were two numbers by their digits. A number is one line: Homes writes no second, a
// mirrored card is stored with one, and `homes nummern-bereinigen` takes the others out.

// HandyBuch is the address book in each person's home that mirrors the phone (/<person>/handy-kontakte/).
const HandyBuch = "handy-kontakte"

// TelSchluessel is a number as two are compared: its international digits, a national one (0171 …)
// in the calling code cc ("" is 49), a trunk 0 after the code dropped (+49 (0)171 …); "" no digits.
func TelSchluessel(v, cc string) string {
	cc = cmp.Or(cc, "49")
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "tel:"))
	d := strings.Map(func(r rune) rune { return map[bool]rune{true: r, false: -1}[r >= '0' && r <= '9'] }, v)
	intl := strings.HasPrefix(v, "+")
	if !intl && strings.HasPrefix(d, "00") {
		d, intl = d[2:], true
	}
	if rest, ok := strings.CutPrefix(d, cc); ok && intl {
		return cc + strings.TrimLeft(rest, "0")
	}
	if intl || d == "" {
		return d
	}
	return cc + strings.TrimLeft(d, "0")
}

// telRang is how much a number's line says: the main one first, then the more kinds, then +….
func telRang(p Prop) int {
	r := 2 * len(strings.FieldsFunc(p.Params["TYPE"], func(c rune) bool { return c == ',' }))
	if isPref(p) {
		r += 100
	}
	if strings.HasPrefix(strings.TrimSpace(p.Value), "+") {
		r++
	}
	return r
}

// ohneDoppelteTel is body (a card's lines, unfolded) with each number (TelSchluessel) once: its
// line telRang ranks first stays where it is, the first of equals; removed is how many went.
func ohneDoppelteTel(body []string, cc string) (out []string, removed int) {
	key := func(l string) (string, Prop) {
		if p, ok := parseLine(l); ok && propName(p.Name) == "TEL" {
			return TelSchluessel(UnescapeText(p.Value), cc), p
		}
		return "", Prop{}
	}
	type platz struct{ i, rang int }
	best := map[string]platz{}
	for i, l := range body {
		if k, p := key(l); k != "" {
			if b, seen := best[k]; !seen || telRang(p) > b.rang {
				best[k] = platz{i, telRang(p)}
			}
		}
	}
	for i, l := range body {
		if k, _ := key(l); k == "" || best[k].i == i {
			out = append(out, l)
		}
	}
	return out, len(body) - len(out)
}

// eineJeNummer is a card's lines after a write of prop: a TEL's with each number once.
func eineJeNummer(prop string, body []string, cc string) []string {
	if prop == "TEL" {
		body, _ = ohneDoppelteTel(body, cc)
	}
	return body
}

// OhneDoppelteNummern is data, a text of vCards, with each number once on each card, and how many
// lines went; data as it was when none did.
func OhneDoppelteNummern(data, cc string) (string, int) {
	removed := 0
	var b strings.Builder
	blocks(unfold(data), "VCARD", func(ls []string, block bool) {
		if block && len(ls) >= 2 {
			body, n := ohneDoppelteTel(ls[1:len(ls)-1], cc)
			removed += n
			ls = append(append([]string{ls[0]}, body...), ls[len(ls)-1])
		}
		for _, l := range ls {
			fold(&b, l)
		}
	})
	return map[bool]string{true: data, false: b.String()}[removed == 0], removed
}

// HaushaltsBuecher is the household's address books Homes reads, as URLs; never a person's mirror.
func (c *Client) HaushaltsBuecher(ctx context.Context) (out []string, err error) {
	cols, err := c.readable(ctx, c.books(), "")
	for _, col := range cols {
		if path.Base(strings.TrimRight(col.URL, "/")) != HandyBuch {
			out = append(out, col.URL)
		}
	}
	return out, err
}

// Bereinigt is what NummernBereinigen counted in one book, or did.
type Bereinigt struct{ Karten, Doppelt, Zeilen, Geschrieben int }

// NummernBereinigen takes the extra lines of a number out of each card in the address book at book
// (OhneDoppelteNummern; every other line stays, X-HAUS-INHALT too); without apply it only counts.
// It stops at the first write that fails (a card changed meanwhile); a book not there counts nothing.
func (c *Client) NummernBereinigen(ctx context.Context, book, cc string, apply bool) (r Bereinigt, err error) {
	cards, _, err := c.BookCards(ctx, book)
	for _, bc := range cards {
		text, n := OhneDoppelteNummern(bc.Data, cc)
		r.Karten, r.Doppelt, r.Zeilen = r.Karten+1, r.Doppelt+min(n, 1), r.Zeilen+n
		if n > 0 && apply && err == nil {
			if err = c.PutCard(ctx, bc.URL, bc.ETag, text); err == nil {
				r.Geschrieben++
			}
		}
	}
	return r, err
}
