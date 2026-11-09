package pages

import (
	"cmp"
	"context"
	"html"
	"log"
	"net/http"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/namen"
	"github.com/mdopp/homes/internal/texte"
)

// KontaktePath is the household's address book as a list.
const KontaktePath = "/kontakte"

// kontakteBuchstaben: from this many contacts on, a card per first letter.
const kontakteBuchstaben = 16

// kontaktZeile is one contact in the list: its name, its page, the first
// number or e-mail to tap, what the search finds it by, what waits about
// it and its „Daten prüfen lassen“.
type kontaktZeile struct {
	name, seite, ziel, zielText, suche, knopf string
	zahl                                      int
}

// Kontakte is the household's address book (owner 2026-11-05: „Web zeigt,
// Handy bearbeitet“): the cards /napi/gemeinsam/kontakte serves, by name,
// each leading to its page (Kontakt) with its first number or e-mail to
// tap; a search by name, number and e-mail while typing. Where to
// change them is folded away at the end; Radicale not answering is
// Kontakt's plain line (haus#33). Above the list the area's segments
// (kontakte_nav.go) and the search; on a row what waits about it and,
// for an admin, „Daten prüfen lassen“.
func Kontakte(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !showOnly(w, r, "Die Kontakte") {
			return
		}
		if cfg.Contacts == nil {
			notSetContacts.write(w, r, cfg, "Kontakte")
			return
		}
		var all []caldav.Contact
		var err error
		fetch(r, func(ctx context.Context) { all, _, err = cfg.Contacts.Contacts(ctx) })
		b := kontakteHead(r, cfg, "Kontakte", KontaktePath)
		b.WriteString(kontakteStyle + "<h1>Kontakte</h1>" + netzEinstieg(r, cfg))
		if err != nil {
			log.Printf("homes: kontakte page: %v", err)
			b.WriteString("<p class=k>" + saysNoContacts + "</p>")
			writePage(w, http.StatusOK, b)
			return
		}
		kontakteListe(b, kontaktZeilen(r, cfg, all, kontaktantwort.Haushalt), texte.KontakteLeer)
		b.WriteString(wieDas(texte.KontakteBearbeiten))
		writePage(w, http.StatusOK, b)
	}
}

// kontaktZeilen are the contacts of buch as rows, by name as the family
// reads it, each leading to its page, with how many things wait about it
// and its „Daten prüfen lassen“.
func kontaktZeilen(r *http.Request, cfg StartConfig, all []caldav.Contact, buch string) []kontaktZeile {
	var out []kontaktZeile
	o := offenFuer(r, cfg)
	for _, ct := range all {
		z := kontaktZeile{name: kontaktName(ct)}
		if uid := strings.TrimSpace(ct.UID); uid != "" {
			z.seite = kontaktPfad(buch, uid) // a card without one has no page of its own
			z.zahl, z.knopf = o.zahl(cfg, buch, uid, z.name), pruefenKnopf(r, cfg, buch, uid, z.name, true)
		}
		hay := []string{z.name, ct.Org}
		for _, p := range ct.Phones {
			if p = oneLine(p); p != "" && z.ziel == "" {
				z.ziel, z.zielText = "tel:"+strings.ReplaceAll(p, " ", ""), p
			}
			hay = append(hay, p)
		}
		for _, m := range ct.Emails {
			if m = oneLine(m); m != "" && z.ziel == "" {
				z.ziel, z.zielText = "mailto:"+m, m
			}
			hay = append(hay, m)
		}
		z.suche = namen.Plain(strings.Join(hay, " "))
		out = append(out, z)
	}
	slices.SortStableFunc(out, func(a, b kontaktZeile) int { return cmp.Compare(namen.Plain(a.name), namen.Plain(b.name)) })
	return out
}

// kontakteListe writes the search, filtering while typing, and the rows
// as cards: one, or from kontakteBuchstaben on one per first letter;
// without any, leer.
func kontakteListe(b *strings.Builder, zeilen []kontaktZeile, leer texte.ID) {
	field, buchstaben := wort(texte.KontakteSuchen), len(zeilen) >= kontakteBuchstaben
	if len(zeilen) == 0 {
		b.WriteString("<p class=k>" + wort(leer) + "</p>")
		return
	}
	b.WriteString(`<input type=search id=kontakte-suche placeholder="` + field + `" aria-label="` + field + `" autocomplete=off><p class=k id=kontakte-keiner data-vorlage="` +
		wort(texte.KontakteKeiner) + `" hidden></p>`)
	defer b.WriteString(kontakteScript)
	titel := texte.T(texte.KontakteAdressbuch)
	for i, z := range zeilen {
		if buchstaben {
			titel = buchstabe(z.name)
		}
		if i == 0 || buchstaben && titel != buchstabe(zeilen[i-1].name) {
			id := cards.Card{Title: titel}.ID()
			b.WriteString(map[bool]string{true: "</ul></section>"}[i > 0] + "<section class=\"karte kontakte\" aria-labelledby=" + id + "><h2 id=" + id + ">" + html.EscapeString(titel) + "</h2><ul>")
		}
		b.WriteString(`<li data-suche="` + html.EscapeString(z.suche) + `">`)
		if z.seite != "" {
			b.WriteString(`<a href="` + html.EscapeString(z.seite) + `">` + html.EscapeString(z.name) + zahl(z.zahl) + "</a>")
		} else {
			b.WriteString("<span>" + html.EscapeString(z.name) + "</span>")
		}
		if z.ziel != "" {
			b.WriteString(` <a class=ruf href="` + html.EscapeString(z.ziel) + `">` + html.EscapeString(z.zielText) + "</a>")
		}
		b.WriteString(z.knopf + "</li>")
	}
	b.WriteString("</ul></section>")
}

// buchstabe is a name's letter in the list: its first letter without its
// accent, # for anything else.
func buchstabe(name string) string {
	if c := []rune(namen.Plain(name)); len(c) > 0 && unicode.IsLetter(c[0]) && c[0] < utf8.RuneSelf {
		return strings.ToUpper(string(c[0]))
	}
	return "#"
}

// The row's number or e-mail is a tap of its own above the row's link.
const kontakteStyle = `<style>.kontakte li a.ruf{display:inline-block;position:relative;z-index:1;margin-top:2px;padding:4px 0;font-size:.9rem;color:var(--accent)}.kontakte li a.ruf::after{content:none}</style>`

// kontakteScript filters the rows while typing, by the same plain words
// as the server (namen.Plain); a letter's card with no row left hides.
const kontakteScript = `<script>(function(){if(window.homesKontakte)return;window.homesKontakte=1;
var um={'ä':'a','ö':'o','ü':'u','ß':'ss','é':'e','è':'e','á':'a','à':'a','ó':'o','í':'i','&':'und'};
function plain(s){return s.toLowerCase().replace(/[äöüßéèáàóí&]/g,function(c){return um[c]}).replace(/[^\p{L}\p{N}]/gu,'')}
document.addEventListener('input',function(e){if(e.target.id!=='kontakte-suche')return;var q=plain(e.target.value),n=0;
document.querySelectorAll('.kontakte li').forEach(function(li){li.hidden=li.dataset.suche.indexOf(q)<0;if(!li.hidden)n++});
document.querySelectorAll('section.kontakte').forEach(function(s){s.hidden=!s.querySelector('li:not([hidden])')});
var k=document.getElementById('kontakte-keiner');if(k){k.hidden=n>0;k.textContent=n?'':k.dataset.vorlage.replace('%s',e.target.value)}})})()</script>`
