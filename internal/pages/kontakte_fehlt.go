package pages

import (
	"cmp"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/texte"
)

// Kontakte fragen auffindbar (owner 2026-11-28): a contact that lacks what
// the link asks for (kontaktantwort.Fehlt: a mobile number, an e-mail, an
// address, a birthday) says so at the top of its page with „Anna selbst
// fragen“ (Kontakt prüfen lassen), and Kontakte has the chip „Unvollständig
// (n)“: those contacts, each with „Fragen“ and what it lacks. For whoever
// may ask (pruefenDarf), nobody else.

// fehltPfad is Kontakte with the contacts that lack something alone.
const fehltPfad = KontaktePath + "?fehlt=1"

// vorname is the name a sentence calls ct by: its first name, else its whole name.
func vorname(ct caldav.Contact) string {
	return trimRunes(cmp.Or(kontaktantwort.FelderVon(ct).Vorname, kontaktName(ct)), 60)
}

// fehltSatz is „Bei Anna fehlen Mobilnummer und Adresse.“
func fehltSatz(r *http.Request, wer string, fehlt []string) string {
	ws := kontaktantwort.FehltWorte(texte.Von(r), fehlt)
	if len(ws) == 1 {
		return fmt.Sprintf(texte.Von(r).T(texte.FehltEines), wer, ws[0])
	}
	return fmt.Sprintf(texte.Von(r).T(texte.FehltMehrere), wer, strings.Join(ws[:len(ws)-1], ", ")+" und "+ws[len(ws)-1])
}

// fragenKnopf is „Anna selbst fragen“ (kurz: „Fragen“, the row's, named in full for a screen
// reader): the same link and share sheet as „Link teilen“.
func fragenKnopf(r *http.Request, buch string, ct caldav.Contact, kurz bool) string {
	voll := html.EscapeString(fmt.Sprintf(texte.Von(r).T(texte.FehltFragen), vorname(ct)))
	if kurz {
		return pruefenForm(`"teilen fragen"`, buch, ct.UID, ` aria-label="`+voll+`"`, wort(texte.FehltFragenKurz), "")
	}
	return pruefenForm(`"teilen fragen"`, buch, ct.UID, "", voll, "")
}

// fehltKarte is the card at the top of ct's page while it lacks something and the viewer may
// ask (owner 2026-11-28); "" otherwise, and then „Link teilen“ stays where it is.
func fehltKarte(r *http.Request, cfg StartConfig, buch string, ct caldav.Contact) string {
	fehlt := kontaktantwort.Fehlt(ct, cfg.PhoneCountry)
	if len(fehlt) == 0 || !pruefenDarf(r, cfg, buch, ct.UID) {
		return ""
	}
	return `<section class=fehlt role=note><p>` + html.EscapeString(fehltSatz(r, vorname(ct), fehlt)) + `</p>` + fragenKnopf(r, buch, ct, false) + `</section>`
}

// unvollstaendig are the house's contacts that lack something, for an admin who may ask; nil otherwise.
func unvollstaendig(r *http.Request, cfg StartConfig, all []caldav.Contact) []caldav.Contact {
	if !cfg.pruefenAn() || !isAdmin(r) {
		return nil
	}
	var out []caldav.Contact
	for _, ct := range all {
		if strings.TrimSpace(ct.UID) != "" && len(kontaktantwort.Fehlt(ct, cfg.PhoneCountry)) > 0 {
			out = append(out, ct)
		}
	}
	return out
}

// kontakteChips are Kontakte's chips: the groups gs, „Alle“ first, gruppe picked; and while
// offen lack something, or it is picked (fehlt), „Unvollständig (n)“.
func kontakteChips(gs []string, gruppe string, offen int, fehlt bool) string {
	if len(gs) == 0 && offen == 0 && !fehlt {
		return ""
	}
	if fehlt {
		gruppe = "\n" // no group's name: none marked, „Alle“ neither
	}
	b := gruppenStyle + fehltStyle + `<ul class=chips aria-label="` + wort(texte.Gruppen) + `">` + gruppenChips(append([]string{""}, gs...), gruppe, false)
	if offen > 0 || fehlt {
		b += `<li><a href="` + html.EscapeString(fehltPfad) + `"` + map[bool]string{true: ` aria-current=page`}[fehlt] + ">" + html.EscapeString(fmt.Sprintf(texte.T(texte.FehltUnvollstaendig), offen)) + "</a></li>"
	}
	return b + "</ul>"
}

// fehltZeilen are the contacts of buch that lack something as Kontakte's rows: „Fragen“ instead of the
// icon, and what each lacks in one grey line.
func fehltZeilen(r *http.Request, cfg StartConfig, cs []caldav.Contact, buch string) []kontaktZeile {
	zs := kontaktZeilen(r, cfg, cs, buch)
	bySeite := map[string]caldav.Contact{}
	for _, ct := range cs {
		bySeite[kontaktPfad(buch, strings.TrimSpace(ct.UID))] = ct
	}
	for i, z := range zs {
		ct := bySeite[z.seite]
		zs[i].knopf = fragenKnopf(r, buch, ct, true)
		zs[i].fehlt = strings.Join(kontaktantwort.FehltWorte(texte.Von(r), kontaktantwort.Fehlt(ct, cfg.PhoneCountry)), " · ")
	}
	return zs
}

// The card is an invitation, not an alarm; the row's „Fragen“ a small labelled button at its right end.
const fehltStyle = `<style>.fehlt{margin:16px 0 0;padding:14px 16px;border-radius:16px;background:var(--accent-soft)}.fehlt p{margin:0 0 10px}.fehlt form.teilen{margin:0}
.kontakte li:has(form.fragen){padding-right:104px}.kontakte li form.fragen button{min-width:auto;padding:0 14px;min-height:40px;background:var(--accent);color:var(--bg)}.kontakte li .fehlt-zeile{display:block;margin-top:2px;color:var(--muted);font-size:.85rem}</style>`
