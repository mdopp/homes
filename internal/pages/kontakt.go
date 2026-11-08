package pages

import (
	"cmp"
	"context"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/haushalt"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/texte"
)

// Kontakt is one contact's own page (haus#28), the page a contacts widget
// row opens; its two fixed links are kontakt_sprung.go's. Radicale not
// answering is one plain line (haus#33).
func Kontakt(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, was := kontaktSprungPfad(strings.TrimPrefix(r.URL.Path, cards.ContactPages))
		e, ok := kontaktPost(w, r, cfg, was != "")
		if !ok {
			return
		}
		if cfg.Contacts == nil {
			notSetContacts.write(w, r, cfg, "Kontakte")
			return
		}
		var all []caldav.Contact
		var err error
		if uid != "" {
			fetch(r, func(ctx context.Context) { all, _, err = cfg.Contacts.Contacts(ctx) })
		}
		if err != nil {
			log.Printf("homes: kontakt page: %q: %v", uid, err)
			missing{http.StatusOK, saysNoContacts, "Kontakt", KontaktePath, "Kontakte"}.write(w, r, cfg)
			return
		}
		for _, ct := range all {
			if uid == "" || ct.UID != uid {
				continue
			}
			if was != "" {
				kontaktSprung(w, r, cfg, ct, kontaktName(ct), was)
				return
			}
			kontaktSeite(w, r, cfg, ct, kontaktantwort.Haushalt, KontaktePath, kontaktSpruenge(cfg, ct.UID), e)
			return
		}
		kontaktWeg(w, r, cfg, e, kontaktantwort.Haushalt, uid, "Diesen Kontakt gibt es nicht.")
	}
}

// kontaktWeg is a contact not (or no longer) in its book: one sentence,
// or, while an answer waits about it, its name and the answer.
func kontaktWeg(w http.ResponseWriter, r *http.Request, cfg StartConfig, e entschieden, buch, uid, say string) {
	f, da := offenFuer(r, cfg).antworten[buch+"/"+uid]
	if e.antwort != nil {
		f, da = *e.antwort, true
	}
	if !da || uid == "" {
		missing{http.StatusNotFound, say, "Kontakt", KontaktePath, "Kontakte"}.write(w, r, cfg)
		return
	}
	kontaktSeite(w, r, cfg, caldav.Contact{UID: uid, Name: f.Name()}, buch, "", "", e)
}

// kontaktName is a contact's name as the family reads it.
func kontaktName(ct caldav.Contact) string {
	return cmp.Or(trimRunes(oneLine(ct.Name), 120), trimRunes(oneLine(ct.Org), 120), "(ohne Namen)")
}

// kontaktPfad is the page of the contact uid of buch.
func kontaktPfad(buch, uid string) string {
	if buch == kontaktantwort.Privat {
		return MeineKontaktePath + "/" + url.PathEscape(uid)
	}
	return cards.ContactPage(uid)
}

// kontaktSeite is a contact of buch: its links and „Link teilen“ at the
// top, its fields, then what waits about it (kontaktOffen); seg its list.
func kontaktSeite(w http.ResponseWriter, r *http.Request, cfg StartConfig, ct caldav.Contact, buch, seg, spruenge string, e entschieden) {
	name := kontaktName(ct)
	weg := []string{name}
	if buch == kontaktantwort.Privat {
		weg = []string{texte.T(texte.KontakteMeine), MeineKontaktePath, name}
	}
	b := kontakteHead(r, cfg, name, seg, weg...)
	b.WriteString(kontakteStyle + "<h1>" + html.EscapeString(name) + "</h1><div class=oben>" + spruenge + pruefenKnopf(r, cfg, buch, ct.UID, name, false) + "</div>")
	c := cards.Card{Title: "Kontakt"}
	if org := trimRunes(oneLine(ct.Org), 120); org != "" && org != name {
		c.Add(org, "")
	}
	for _, p := range ct.Phones {
		if p = oneLine(p); p != "" {
			c.AddLink(p, "Telefon", "tel:"+strings.ReplaceAll(p, " ", ""))
		}
	}
	for _, m := range ct.Emails {
		if m = oneLine(m); m != "" {
			c.AddLink(m, "E-Mail", "mailto:"+m)
		}
	}
	if ct.Birthday != "" {
		c.Add(haushalt.BirthdayText(ct.Birthday), "Geburtstag")
	}
	for _, a := range ct.Addresses {
		if a = trimRunes(oneLine(a), 200); a != "" {
			c.Add(a, "Adresse")
		}
	}
	b.WriteString(c.HTML())
	kontaktOffen(b, r, cfg, buch, ct.UID, name, r.URL.EscapedPath(), e)
	writePage(w, http.StatusOK, b)
}
