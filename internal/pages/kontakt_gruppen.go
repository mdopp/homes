package pages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/haushalt"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/texte"
)

// gruppenChips are the groups (haushalt.Gruppen) as links to Kontakte picked by each, "" all of them,
// the one chosen marked; mit (an admin's) adds to each its ✕, posted as gruppe-weg.
func gruppenChips(gs []string, gewaehlt string, mit bool) string {
	var b strings.Builder
	for _, g := range gs {
		b.WriteString(`<li><a href="` + html.EscapeString(KontaktePath+map[bool]string{true: "?gruppe=" + url.QueryEscape(g)}[g != ""]) + `"` + map[bool]string{true: ` aria-current=page`}[strings.EqualFold(g, gewaehlt)] + ">" +
			html.EscapeString(cmp.Or(g, texte.T(texte.GruppenAlle))) + "</a>")
		if mit {
			b.WriteString(`<button name=gruppe-weg value="` + html.EscapeString(g) + `" aria-label="` + html.EscapeString(fmt.Sprintf(texte.T(texte.GruppeWeg), g)) + `" data-laeuft="` + wort(texte.GruppeWegLauf) + `">✕</button>`)
		}
		b.WriteString("</li>")
	}
	return b.String()
}

// kontaktGruppen is „Gruppen“ on the shared card ct's page (owner 2026-11-10): its groups for everyone; for who may
// correct a shared card there (an admin, as „Daten prüfen lassen“) each with its ✕, and „Zu Gruppe hinzufügen“ (alle).
func kontaktGruppen(r *http.Request, ct caldav.Contact, alle []string) string {
	gs, darf := haushalt.Gruppen(ct), isAdmin(r) && ct.UID != ""
	if len(gs) == 0 && !darf {
		return ""
	}
	ziel := ` action="` + html.EscapeString(kontaktPfad(kontaktantwort.Haushalt, ct.UID)) + `"><input type=hidden name=uid value="` + html.EscapeString(ct.UID) + `">`
	b := gruppenStyle + "<h2>" + wort(texte.Gruppen) + "</h2>" + map[bool]string{true: "<p class=k>" + wort(texte.GruppenKeine) + "</p>"}[len(gs) == 0] +
		`<form method=post` + ziel + `<ul class=chips>` + gruppenChips(gs, "", darf) + `</ul></form>`
	if !darf {
		return b
	}
	b += `<form method=post class=gruppe-dazu` + ziel + `<label>` + wort(texte.GruppeDazu) + `<input name=gruppe-dazu list=gruppen-namen required maxlength=` + fmt.Sprint(haushalt.GruppeMax) +
		` autocomplete=off placeholder="` + wort(texte.GruppeBeispiel) + `"></label><datalist id=gruppen-namen>`
	for _, g := range slices.DeleteFunc(alle, func(g string) bool { return haushalt.InGruppe(ct, g) }) {
		b += `<option value="` + html.EscapeString(g) + `">`
	}
	return b + `</datalist><button data-laeuft="` + wort(texte.GruppeDazuLauf) + `">` + wort(texte.GruppeDazuKnopf) + `</button></form>`
}

const gruppenStyle = `<style>.chips{display:flex;flex-wrap:wrap;gap:8px;margin:10px 0;padding:0;list-style:none}.chips li{display:inline-flex;align-items:center;border-radius:999px;background:var(--accent-soft);color:var(--accent)}
.chips a{display:inline-flex;align-items:center;min-height:40px;padding:0 14px;color:inherit;font-weight:600;font-size:.9rem;text-decoration:none}.chips a[aria-current]{background:var(--accent);color:var(--bg);border-radius:999px}
.chips button{min-height:40px;min-width:40px;margin:0 2px 0 -8px;padding:0;border:0;background:none;color:inherit;font:inherit}.gruppe-dazu{display:flex;flex-wrap:wrap;gap:8px;align-items:flex-end}.gruppe-dazu label{flex:1 1 12em;display:flex;flex-direction:column;gap:4px}</style>`

// gruppeAendern takes a group added (gruppe-dazu) to or taken from (gruppe-weg) the shared card uid; how it ended.
func gruppeAendern(r *http.Request, cfg StartConfig) (said, problem string) {
	weg := oneLine(r.PostForm.Get("gruppe-weg"))
	k, nein := haushalt.GruppeKorrektur(oneLine(r.PostForm.Get("gruppe-dazu")), weg)
	if !isAdmin(r) || cfg.Contacts == nil {
		return "", texte.T(texte.GruppeNurAdmins)
	} else if nein != "" {
		return "", texte.T(nein)
	}
	var done []bool
	var err error
	fetch(r, func(ctx context.Context) {
		done, err = cfg.Contacts.CorrectShared(ctx, oneLine(r.PostForm.Get("uid")), []caldav.Korrektur{k})
	})
	switch {
	case errors.Is(err, caldav.ErrNoCard):
		return "", texte.T(texte.PruefenWeg)
	case err != nil || !slices.Contains(done, true):
		log.Printf("homes: kontakt: a group not written: %v", err)
		return "", texte.T(texte.GruppeFehler)
	case weg != "":
		return fmt.Sprintf(texte.T(texte.GruppeWegGesagt), weg), ""
	}
	return fmt.Sprintf(texte.T(texte.GruppeDazuGesagt), k.Neu), ""
}
