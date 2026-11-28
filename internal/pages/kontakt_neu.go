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
	"strconv"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/texte"
)

// Kontaktdaten eintragen lassen (owner 2026-11-28): „Kontakt prüfen
// lassen“ for a person the household has no card of. Kontakte's button
// (an admin's) makes the link and hands it to the share sheet as „Daten
// prüfen lassen“ does; the person fills the same public form, empty; the
// answer waits on Deine Aufgaben and is reviewed here, at
// AntwortenPath/<id>, before → after with nothing before: „Als neuen
// Kontakt anlegen“, or for a household card with one of its numbers or
// e-mail addresses „<Name> damit ergänzen“ before → after as against that
// card, or „Verwerfen“. Nothing reaches the address book without it.

// KontaktEinladenPath makes the link (POST), as PruefenPath does for a card.
const KontaktEinladenPath = KontaktePath + "/einladen"

// einladenKnopf is Kontakte's button for an admin (pruefenScript shares the link).
func einladenKnopf(r *http.Request, cfg StartConfig) string {
	if !cfg.pruefenAn() || !isAdmin(r) {
		return ""
	}
	return `<form method=post action=` + KontaktEinladenPath + ` class=einladen data-teilen><button data-laeuft="` + wort(texte.PruefenErstelltLaeuft) + `" data-kopiert="` + wort(texte.PruefenKopiert) +
		`" data-nochmal="` + wort(texte.PruefenNochmal) + `">` + wort(texte.EinladenKnopf) + `</button><small class=wozu>` + wort(texte.EinladenWozu) + `</small></form>`
}

// KontaktEinladen makes a new person's link and answers the message to send with it, a refusal as one
// plain sentence; a GET goes to Kontakte.
func KontaktEinladen(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		switch {
		case r.Method != http.MethodPost:
			http.Redirect(w, r, KontaktePath, http.StatusSeeOther)
			return
		case !cfg.pruefenAn() || !fromHomes(r):
			http.Error(w, "Nur von Homes' eigener Seite.", http.StatusForbidden)
			return
		case !isAdmin(r):
			http.Error(w, texte.Von(r).T(texte.PruefenNurAdmins), http.StatusForbidden)
			return
		}
		von, _, _ := strings.Cut(strings.TrimSpace(r.Header.Get("Remote-Name")), " ")
		link, f, err := cfg.Antworten.Einladen(r.Header.Get("Remote-User"), von, cfg.KontaktURL)
		if err != nil {
			log.Printf("homes: kontakt einladen: the link not made: %v", err)
			http.Error(w, texte.Von(r).T(texte.PruefenNicht), http.StatusServiceUnavailable)
			return
		}
		log.Printf("homes: kontakt einladen: link %s made", f.ID)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, texte.Von(r).T(texte.EinladenText), link)
	}
}

// KontaktNeu is the review of the viewer's new person's answer f (Antworten finds it): GET
// shows it, POST decides it (aktion anlegen | ergaenzen with ziel, a card's UID | verwerfen).
// Open, it asks first „Ist das jemand, den du schon hast?“ (owner 2026-11-28): the household's
// cards that may be that person, each with its reasons and „Damit zusammenführen“, which shows
// the answer before → after against that card (?ziel=<uid>); below them the answer as a new
// card, „Ins Adressbuch übernehmen“.
func KontaktNeu(w http.ResponseWriter, r *http.Request, cfg StartConfig, f kontaktantwort.Frage) {
	w.Header().Set("Cache-Control", "no-store")
	said, problem := "", ""
	if r.Method == http.MethodPost {
		if !fromHomes(r) {
			http.Error(w, "Nur von Homes' eigener Seite.", http.StatusForbidden)
			return
		}
		f, said, problem = neuEntscheiden(r, cfg, f)
	}
	titel, action := fmt.Sprintf(texte.T(texte.NeuAntwortTitel), cmp.Or(f.Name(), "(ohne Namen)")), AntwortenPath+"/"+f.ID
	b := kontakteHead(r, cfg, titel, "", titel)
	b.WriteString(kontakteStyle + pruefenStyle + neuStyle + "<h1>" + html.EscapeString(titel) + "</h1>" + gesagt(said, problem))
	if !f.ZuPruefen() {
		if f.UID != "" {
			b.WriteString(`<p><a href="` + html.EscapeString(cards.ContactPage(f.UID)) + `">` + wort(texte.NeuZumKontakt) + ` ›</a></p>`)
		}
		antwortKopf(b, f)
		antwortListe(b, f, action, "", "")
		writePage(w, http.StatusOK, b)
		return
	}
	b.WriteString("<p class=k>" + wort(texte.NeuAntwortWarum) + "</p>")
	ks := neuKandidaten(r, cfg, f)
	ziel := cmp.Or(r.PostFormValue("ziel"), r.URL.Query().Get("ziel"))
	if i := slices.IndexFunc(ks, func(k kontaktantwort.Kandidat) bool { return k.UID == ziel }); i >= 0 {
		neuZusammen(b, f, ks[i], action)
	} else {
		neuAuswahl(b, f, ks, action)
	}
	writePage(w, http.StatusOK, b)
}

// grundText is why a card may be the person, as the review says it.
var grundText = map[string]texte.ID{kontaktantwort.GleicheMobil: texte.NeuGrundMobil, kontaktantwort.GleicheNummer: texte.NeuGrundNummer,
	kontaktantwort.GleicheMail: texte.NeuGrundMail, kontaktantwort.AehnlicherName: texte.NeuGrundName}

// neuAuswahl is „Ist das jemand, den du schon hast?“ with the candidates ks, then the answer f as a new card.
func neuAuswahl(b *strings.Builder, f kontaktantwort.Frage, ks []kontaktantwort.Kandidat, action string) {
	b.WriteString("<h2>" + wort(texte.NeuWer) + "</h2>")
	if len(ks) == 0 {
		b.WriteString("<p class=k>" + wort(texte.NeuWerKeiner) + "</p>")
	} else {
		b.WriteString("<ul class=kandidaten>")
		for _, k := range ks {
			var gs []string
			for _, g := range k.Gruende {
				gs = append(gs, texte.T(grundText[g]))
			}
			b.WriteString(`<li><span><b>` + html.EscapeString(kontaktName(k.Contact)) + `</b><br><span class=k>` + html.EscapeString(strings.Join(gs, " · ")) + `</span></span>` +
				`<a class=knopf href="` + html.EscapeString(action+"?ziel="+url.QueryEscape(k.UID)) + `">` + wort(texte.NeuZusammen) + `</a></li>`)
		}
		b.WriteString("</ul>")
	}
	b.WriteString("<h2>" + wort(map[bool]texte.ID{true: texte.NeuNeuKeinerTitel, false: texte.NeuNeuTitel}[len(ks) == 0]) + "</h2>")
	antwortKopf(b, f)
	antwortListe(b, f, action, "", `<p class=knoepfe><button name=aktion value=anlegen data-laeuft="`+wort(texte.NeuAnlegenLaeuft)+`">`+wort(texte.NeuAnlegen)+`</button>`+verwerfenKnopf+`</p>`)
}

// neuZusammen is the answer f before → after against the candidate k, „<Name> damit ergänzen“, and the way back.
func neuZusammen(b *strings.Builder, f kontaktantwort.Frage, k kontaktantwort.Kandidat, action string) {
	name := kontaktName(k.Contact)
	b.WriteString("<h2>" + html.EscapeString(fmt.Sprintf(texte.T(texte.NeuZusammenTitel), name)) + "</h2>")
	antwortKopf(b, f)
	g := f
	if g.UID, g.Aenderungen = k.UID, kontaktantwort.Ergaenzung(f, k.Contact); len(g.Aenderungen) == 0 {
		b.WriteString("<p class=k>" + html.EscapeString(fmt.Sprintf(texte.T(texte.NeuNichtsNeues), name)) + "</p>")
	} else {
		antwortListe(b, g, action, `<input type=hidden name=ziel value="`+html.EscapeString(k.UID)+`">`,
			`<p class=knoepfe><button name=aktion value=ergaenzen data-laeuft="`+wort(texte.AntwortLaeuft)+`">`+html.EscapeString(fmt.Sprintf(texte.T(texte.NeuErgaenzen), name))+`</button></p>`)
	}
	b.WriteString(`<p><a href="` + html.EscapeString(action) + `">‹ ` + wort(texte.NeuZurueck) + `</a></p>`)
}

// neuStyle is the candidates' list: each a row, its button at the end.
const neuStyle = `<style>.kandidaten{list-style:none;margin:0 0 18px;padding:0}.kandidaten li{display:flex;gap:12px;align-items:center;justify-content:space-between;margin:0 0 10px;padding:12px 14px;border-radius:16px;background:var(--card)}` +
	`.kandidaten .knopf{flex:none;display:inline-flex;align-items:center;min-height:44px;padding:0 16px;border-radius:999px;background:var(--accent-soft);color:var(--accent);font-weight:600;text-decoration:none}</style>`

// neuKandidaten are the household's cards that may be the person of an open answer f; Radicale not answering: none.
func neuKandidaten(r *http.Request, cfg StartConfig, f kontaktantwort.Frage) []kontaktantwort.Kandidat {
	if !f.ZuPruefen() {
		return nil
	}
	var all []caldav.Contact
	var err error
	fetch(r, func(ctx context.Context) { all, _, err = cfg.Contacts.Contacts(ctx) })
	if err != nil {
		log.Printf("homes: kontakt-antwort %s: no cards to match: %v", f.ID, err)
	}
	return kontaktantwort.Kandidaten(all, f, cfg.PhoneCountry)
}

// nimmVon are the ticked changes' numbers of a review's form.
func nimmVon(r *http.Request) []int {
	var nimm []int
	for _, v := range r.PostForm["nimm"] {
		if n, err := strconv.Atoi(v); err == nil {
			nimm = append(nimm, n)
		}
	}
	return nimm
}

// neuEntscheiden ends the open answer f as its form says: the answer as it stands then, and what to say.
func neuEntscheiden(r *http.Request, cfg StartConfig, f kontaktantwort.Frage) (kontaktantwort.Frage, string, string) {
	if !f.ZuPruefen() {
		return f, "", ""
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 8<<10)
	r.ParseForm()
	sp, nimm, name := texte.Von(r), nimmVon(r), f.Name()
	var uid, said string
	var vorher kontaktantwort.Felder
	var as []kontaktantwort.Aenderung
	var stand []string
	var err error
	switch r.PostFormValue("aktion") {
	case "anlegen":
		fetch(r, func(ctx context.Context) {
			uid, stand, err = kontaktantwort.NeuAnlegen(ctx, cfg.Contacts, f, kontaktantwort.Stand(f, false, nimm), cfg.PhoneCountry)
		})
		as, said = f.Aenderungen, fmt.Sprintf(sp.T(texte.NeuAngelegt), name)
		if uid == "" {
			log.Printf("homes: kontakt-antwort %s: the new card not written: %v", f.ID, err)
			return f, "", sp.T(texte.NeuNichtAngelegt)
		}
	case "ergaenzen":
		ks := neuKandidaten(r, cfg, f)
		i := slices.IndexFunc(ks, func(k kontaktantwort.Kandidat) bool { return k.UID == r.PostFormValue("ziel") })
		if i < 0 {
			return f, "", sp.T(texte.AntwortKontaktWeg)
		}
		ct := ks[i].Contact
		fetch(r, func(ctx context.Context) {
			as, stand, err = kontaktantwort.Ergaenzen(ctx, cfg.Contacts, f, ct, nimm, cfg.PhoneCountry)
		})
		uid, vorher, said = ct.UID, kontaktantwort.FelderVon(ct), fmt.Sprintf(sp.T(texte.NeuErgaenzt), kontaktName(ct))
	default:
		as, stand, said = f.Aenderungen, kontaktantwort.Stand(f, false, nil), sp.T(texte.AntwortVerworfen)
	}
	if err != nil && !errors.Is(err, kontaktantwort.ErrWeg) {
		log.Printf("homes: kontakt-antwort %s: not written: %v", f.ID, err)
	}
	if stand == nil {
		return f, "", sp.T(texte.AntwortNicht)
	}
	done, err := cfg.Antworten.EntscheidenNeu(f.ID, f.Wer, uid, vorher, as, stand)
	if err != nil {
		return f, "", sp.T(texte.AntwortNicht)
	}
	log.Printf("homes: kontakt-antwort %s (neu) decided: %s", f.ID, cmp.Or(r.PostFormValue("aktion"), "verwerfen"))
	n := map[string]int{}
	for _, s := range stand {
		n[s]++
	}
	if k := n[kontaktantwort.Uebersprungen]; k > 0 {
		return done, said, fmt.Sprintf(sp.T(texte.AntwortUebersprungen), texte.Anzahl(k, "Änderung", "Änderungen"))
	}
	return done, said, ""
}
