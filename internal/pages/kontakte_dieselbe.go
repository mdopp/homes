package pages

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/immich"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/texte"
)

// DieselbePath is „Dieselbe Person?“ (owner 2026-11-06): pairs of persons
// whose faces Immich holds close, each with its two faces, names and photo
// counts, and Ja (merge in Immich), Nein (never asked again) and
// Überspringen; from the hourly run (gesichter.Vorschlagen), for the key's
// owner. Asked on the contact's page (2026-11-08): this, the task's
// address, opens the next such contact, then what belongs to none.
const DieselbePath = KontaktePath + "/dieselbe"

const (
	dieselbeJeSeite = 10
	dieselbeWait    = 30 * time.Second // a merge in Immich before the page says so
)

const dieselbeStyle = `<style>.paar{margin:18px 0 0;padding:14px;border-radius:16px;background:var(--card)}.paar .zwei{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin:0}.paar figure{margin:0;text-align:center;min-width:0}.paar img,.paar .ohne{display:block;width:96px;height:96px;margin:0 auto 8px;border-radius:50%;object-fit:cover;background:var(--track)}
.paar figure>a{display:block}.paar figcaption{overflow-wrap:anywhere}.paar figcaption b{display:block}.paar figcaption i,.paar figcaption span,.paar figcaption small{display:block;color:var(--muted);font-size:.85rem}.paar figcaption small{font-size:.78rem;margin-top:2px}.paar form{display:flex;flex-wrap:wrap;gap:8px;margin:14px 0 0}.paar form button{flex:1 1 auto}.paar form button.leise{flex:0 0 auto}</style>`

// Dieselbe is the task's address, and the answers to what is left (POST
// as on a contact's page, immichAntwort).
func Dieselbe(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		said, problem := "", ""
		if r.Method == http.MethodPost {
			if !fromHomes(r) {
				http.Error(w, "Das geht nur von Homes' eigener Seite.", http.StatusForbidden)
				return
			}
			var ok bool
			if said, problem, ok = immichAntwort(w, r, cfg); !ok {
				return
			}
		} else if !showOnly(w, r, texte.T(texte.Dieselbe)) {
			return
		}
		if !privatEigen(r, cfg) {
			http.Redirect(w, r, KontaktePath, http.StatusSeeOther)
			return
		}
		var haus, meine []caldav.Contact
		if cfg.Contacts != nil {
			fetch(r, func(ctx context.Context) {
				haus, _, _ = cfg.Contacts.Contacts(ctx)
				meine, _ = meineKontakte(ctx, cfg, r.Header.Get("Remote-User"))
			})
		}
		var offen []kontaktZeile // the contacts with a question, a list to work through (owner 2026-11-09)
		for _, z := range append(kontaktZeilen(r, cfg, meine, kontaktantwort.Privat), kontaktZeilen(r, cfg, haus, kontaktantwort.Haushalt)...) {
			if z.zahl > 0 {
				z.seite, z.knopf = z.seite+"?weiter", ""
				offen = append(offen, z)
			}
		}
		writePage(w, http.StatusOK, dieselbeRest(r, cfg, offen, said, problem))
	}
}

// dieselbeRest is the page: the contacts with a question (offen), then
// what belongs to none, the outcome, the questions, the first pairs.
func dieselbeRest(r *http.Request, cfg StartConfig, offen []kontaktZeile, said, problem string) *strings.Builder {
	t := func(id texte.ID) string { return html.EscapeString(texte.T(id)) }
	b := kontakteHead(r, cfg, texte.T(texte.Dieselbe), "", texte.T(texte.Dieselbe))
	b.WriteString(dieselbeStyle + "<h1>" + t(texte.Dieselbe) + "</h1>" + gesagt(said, problem))
	if len(offen) > 0 { // the rest (pairs of nobody's) once the list is done
		kontakteListe(b, offen, texte.KontakteLeer)
		return b
	}
	fragen, paare := cfg.Gesichter.Fragen(), cfg.Gesichter.Paare()
	b.WriteString(istFragen(cfg, fragen, DieselbePath))
	if len(paare)+len(fragen) == 0 {
		b.WriteString("<p class=k>" + t(texte.DieselbeLeer) + "</p>")
	}
	for _, p := range paare[:min(len(paare), dieselbeJeSeite)] {
		b.WriteString(paarKarte(cfg, p, DieselbePath))
	}
	b.WriteString(wieDas(texte.IstDasHinweis, texte.DieselbeHinweis))
	return b
}

// immichAntwort passes the key owner's answer on: about a contact's
// picture (kontakt, b, antwort ja|nein|keinfoto) or a pair (a, b, antwort
// ja|nein|spaeter); ok false: refused, and said so.
func immichAntwort(w http.ResponseWriter, r *http.Request, cfg StartConfig) (said, problem string, ok bool) {
	if !privatEigen(r, cfg) {
		http.Redirect(w, r, KontaktePath, http.StatusSeeOther)
		return "", "", false
	}
	r.ParseForm()
	antwort, kontakt := r.PostForm.Get("antwort"), r.PostForm.Get("kontakt")
	if erlaubt := map[bool][]string{false: {"ja", "nein", "spaeter"}, true: {"ja", "nein", "keinfoto"}}[kontakt != ""]; !slices.Contains(erlaubt, antwort) {
		http.Error(w, "Ja, Nein oder Überspringen? Die Antwort fehlt.", http.StatusBadRequest)
		return "", "", false
	}
	a, b := r.PostForm.Get("a"), r.PostForm.Get("b")
	antworten, gesagt := func(ctx context.Context) (string, error) { return cfg.Gesichter.Antworten(ctx, a, b, antwort) }, dieselbeGesagt
	if kontakt != "" {
		antworten, gesagt = func(ctx context.Context) (string, error) { return cfg.Gesichter.Bestaetigen(ctx, kontakt, b, antwort) }, istGesagt
	}
	said, problem = dieselbeAntworten(r.Context(), antwort, antworten, gesagt)
	return said, problem, true
}

// dieselbeGesagt are a pair's answers as the page says them.
var dieselbeGesagt = map[string]texte.ID{"nein": texte.DieselbeGemerkt, "spaeter": texte.DieselbeHinten, "stumm": texte.DieselbeStumm}

// dieselbeAntworten passes the answer on (antworten, to Immich within
// dieselbeWait) and says how it ended: a merge by the name kept, the
// others and Immich not answering ("stumm") as gesagt has them.
func dieselbeAntworten(ctx context.Context, antwort string, antworten func(context.Context) (string, error), gesagt map[string]texte.ID) (said, problem string) {
	ctx, cancel := context.WithTimeout(ctx, dieselbeWait)
	defer cancel()
	name, err := antworten(ctx)
	var fehlt immich.FehltRecht
	switch {
	case errors.Is(err, gesichter.ErrKeinPaar), errors.Is(err, gesichter.ErrKeineFrage):
		return "", texte.T(texte.DieselbeWeg)
	case errors.As(err, &fehlt):
		return "", fmt.Sprintf(texte.T(texte.MeineKontakteRecht), fehlt.Recht)
	case errors.Is(err, gesichter.ErrNichtZusammen):
		return "", texte.T(texte.DieselbeNichtGemacht)
	case err != nil:
		log.Printf("homes: dieselbe person: %v", err)
		return "", texte.T(gesagt["stumm"])
	case antwort == "ja":
		log.Printf("homes: dieselbe person: merged into %q", name)
		return fmt.Sprintf(texte.T(texte.DieselbeZusammen), name), ""
	}
	return texte.T(gesagt[antwort]), ""
}

// paarKarte is one pair, its two faces and its three buttons, posted to action.
func paarKarte(cfg StartConfig, p gesichter.Paar, action string) string {
	t := func(id texte.ID) string { return html.EscapeString(texte.T(id)) }
	return `<section class=paar><div class=zwei>` + dieselbeGesicht(cfg, p.A) + dieselbeGesicht(cfg, p.B) + `</div><form method=post action="` + html.EscapeString(action) +
		`"><input type=hidden name=a value="` + html.EscapeString(p.A.ID) + `"><input type=hidden name=b value="` + html.EscapeString(p.B.ID) +
		`"><button name=antwort value=ja data-laeuft="` + t(texte.DieselbeJaLaeuft) + `">` + t(texte.DieselbeJa) +
		`</button><button name=antwort value=nein data-laeuft="` + t(texte.DieselbeNeinLaeuft) + `">` + t(texte.DieselbeNein) +
		`</button><button name=antwort value=spaeter class=leise data-laeuft="` + t(texte.DieselbeSpaeterLauft) + `">` + t(texte.DieselbeSpaeter) + `</button></form></section>`
}

// dieselbeGesicht is one person of a pair: the face, a link to all its
// photos in Immich (a new tab), the name or „ohne Namen“, the photos when
// counted, and a grey line each for its places and years and who else is
// in its photos, when read.
func dieselbeGesicht(cfg StartConfig, s gesichter.Seite) string {
	bild := `<span class=ohne aria-hidden=true></span>`
	if s.Bild != nil {
		bild = `<img src="data:image/jpeg;base64,` + base64.StdEncoding.EncodeToString(s.Bild) + `" alt="" width=96 height=96>`
	}
	if href := cfg.Gesichter.PersonSeite(s.ID); href != "" {
		label := texte.T(texte.DieselbeInImmichOhne)
		if s.Name != "" {
			label = fmt.Sprintf(texte.T(texte.DieselbeInImmich), s.Name)
		}
		bild = immichLink(href, label, bild)
	}
	name := "<i>" + html.EscapeString(texte.T(texte.DieselbeOhneName)) + "</i>"
	if s.Name != "" {
		name = "<b>" + html.EscapeString(s.Name) + "</b>"
	}
	if s.Fotos >= 0 {
		name += "<span>" + html.EscapeString(texte.Anzahl(s.Fotos, "Foto", "Fotos")) + "</span>"
	}
	jahre := strconv.Itoa(s.Von)
	if s.Bis > s.Von {
		jahre += "–" + strconv.Itoa(s.Bis)
	}
	if orte := strings.Join(s.Orte, ", "); orte != "" || s.Von > 0 {
		if s.Von > 0 {
			orte = strings.TrimPrefix(orte+" · "+jahre, " · ")
		}
		name += "<small>" + html.EscapeString(fmt.Sprintf(texte.T(texte.DieselbeOrte), orte)) + "</small>"
	}
	if len(s.Mit) > 0 {
		name += "<small>" + html.EscapeString(fmt.Sprintf(texte.T(texte.DieselbeMit), strings.Join(s.Mit, ", "))) + "</small>"
	}
	return "<figure>" + bild + "<figcaption>" + name + "</figcaption></figure>"
}

// immichLink is inner as a link to href in Immich, in a new tab.
func immichLink(href, label, inner string) string {
	return `<a href="` + html.EscapeString(href) + `" target=_blank rel=noopener aria-label="` + html.EscapeString(label) + `">` + inner + `</a>`
}
