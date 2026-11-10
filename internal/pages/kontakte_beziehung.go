package pages

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"

	"github.com/mdopp/homes/internal/gesichter"
	kontaktnetz "github.com/mdopp/homes/internal/netz"
	"github.com/mdopp/homes/internal/texte"
)

// „Beziehungen klären“ (owner 2026-11-09: „Frage, ähnlich wie bei den Fotos, auch den Beziehungsgrad
// ab. Mache gute Vorschläge und ein Dropdown zur Auswahl.“): on „Wer steht wem nahe“ the first
// netzFragenJe of netz.Fragen, on a contact's page those about it; each with both faces, the tie's
// reasons, the kinds with the suggestion chosen and why, Speichern, Keine Beziehung and Überspringen.

const netzFragenJe = 5

const netzFrageStyle = `<style>.beziehung h3{margin:0 0 10px;font-size:1rem}.beziehung>.k{margin:10px 0 0}.beziehung label{flex:1 1 100%;display:flex;flex-direction:column;gap:4px}
.beziehung select{min-height:44px;font:inherit;color:var(--ink);background:var(--bg);border:1px solid var(--edge);border-radius:10px;padding:0 8px}.beziehung form .k{flex:1 1 100%;margin:0}</style>`

// netzFragen is the section of fs, each posted to action; "" without one.
func netzFragen(cfg StartConfig, fs []kontaktnetz.Frage, action string) string {
	if len(fs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(netzFrageStyle + "<h2>" + wort(texte.NetzFragen) + "</h2>")
	for _, f := range fs {
		b.WriteString(netzFrage(cfg, f, action))
	}
	return b.String()
}

// netzDu is Y as the question names it: „dir“ for the key owner.
func netzDu(f kontaktnetz.Frage) string {
	if f.Ich {
		return texte.T(texte.NetzFrageDir)
	}
	return f.Y.Name
}

// netzFrage is one question and its form.
func netzFrage(cfg StartConfig, f kontaktnetz.Frage, action string) string {
	esc := html.EscapeString
	weil := ""
	if f.Weil != "" {
		weil = "<p class=k>" + esc(fmt.Sprintf(texte.T(texte.NetzVorschlag), f.Weil)) + "</p>"
	}
	return `<article class="paar beziehung"><h3>` + esc(fmt.Sprintf(texte.T(texte.NetzFrage), f.X.Name, netzDu(f))) + `</h3><div class=zwei>` + netzGesicht(cfg, f.X) + netzGesicht(cfg, f.Y) +
		"</div><p class=k>" + esc(cfg.Netz.Gruende(f.Y.ID, f.Kante)) + "</p>" + netzWahl(action, f.X, f.Y.ID, f.Vorschlag) + weil +
		netzKnopf("ja", texte.NetzSpeichern, texte.NetzSpeichernLauft, "") + netzKnopf("keine", texte.NetzKeineBez, texte.NetzKeineBezLauft, " class=leise formnovalidate") +
		netzKnopf("spaeter", texte.NetzSpaeter, texte.NetzSpaeterLauft, " class=leise formnovalidate") + "</form></article>"
}

// netzWahl opens the form about x and y, posted to action, with the kinds,
// art chosen: „X ist …“.
func netzWahl(action string, x kontaktnetz.Knoten, y, art string) string {
	esc := html.EscapeString
	opts := `<option value="">` + wort(texte.NetzWaehlen) + "</option>"
	for _, a := range kontaktnetz.Arten {
		opts += `<option value="` + a + `"` + map[bool]string{true: " selected"}[a == art] + ">" + esc(kontaktnetz.Wort(a)) + "</option>"
	}
	opts += `<option value="` + kontaktnetz.Fremd + `"` + map[bool]string{true: " selected"}[art == kontaktnetz.Fremd] + ">" + wort(texte.NetzFremd) + "</option>"
	return `<form method=post action="` + esc(action) + `"><input type=hidden name=x value="` + esc(x.ID) + `"><input type=hidden name=y value="` + esc(y) + `"><label>` +
		esc(fmt.Sprintf(texte.T(texte.NetzIst), x.Name)) + `<select name=art required>` + opts + "</select></label>"
}

func netzKnopf(wert string, id, laeuft texte.ID, klasse string) string {
	return `<button name=antwort value=` + wert + klasse + ` data-laeuft="` + wort(laeuft) + `">` + wort(id) + "</button>"
}

// netzBeziehungen is „Beziehungen“ on a contact's page (owner 2026-11-09:
// „inkl. Änderbarkeit“), for the key's owner: each relation of the
// contact, the owner's answers over the phone books', the kind to change
// with Speichern, and Entfernen (a „Keine Beziehung“: Vergessen).
func netzBeziehungen(r *http.Request, cfg StartConfig, buch, uid, action string) string {
	if !netzDa(r, cfg) {
		return ""
	}
	id := cfg.Netz.Von(buch + "/" + uid)
	bs := cfg.Netz.Beziehungen(id)
	if len(bs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(netzFrageStyle + "<h2>" + wort(texte.NetzBeziehungen) + "</h2>")
	for _, z := range bs {
		herkunft, weg := texte.NetzTelefonbuch, netzKnopf("entfernen", texte.NetzEntfernen, texte.NetzEntfernenLauf, " class=leise formnovalidate")
		if z.Gesagt {
			herkunft = texte.NetzGesagt
		}
		if z.Art == kontaktnetz.Keine || z.Art == kontaktnetz.Fremd {
			weg = netzKnopf("vergessen", texte.NetzVergessen, texte.NetzVergessenLauf, " class=leise formnovalidate")
		}
		b.WriteString(`<article class="paar beziehung"><h3>` + html.EscapeString(z.Mit.Name) + `</h3><p class=k>` + html.EscapeString(fmt.Sprintf(texte.T(herkunft), z.Wie)) + "</p>" +
			netzWahl(action, z.Mit, id, z.Art) + netzKnopf("aendern", texte.NetzSpeichern, texte.NetzSpeichernLauft, "") + weg + "</form></article>")
	}
	return b.String()
}

// netzGesicht is a node as „Dieselbe Person?“ shows a face: its Immich
// person's (none for a contact without one), its contact's link.
func netzGesicht(cfg StartConfig, k kontaktnetz.Knoten) string {
	s := gesichter.Seite{Name: k.Name, Fotos: -1, Bild: cfg.Gesichter.Bild(k.Name), Kontakt: k.Kontakt}
	if !strings.Contains(k.ID, "/") {
		s.ID = k.ID
	}
	return dieselbeGesicht(cfg, s)
}

// netzAntwort takes an answer posted from a question (x, y, antwort
// ja|keine|spaeter, art) or from a contact's relations (antwort
// aendern|entfernen|vergessen) and says how it ended; a kind, and a
// relation taken away, is written into the cards too, within dieselbeWait.
func netzAntwort(r *http.Request, cfg StartConfig) (said, problem string) {
	antwort := r.PostFormValue("antwort")
	art, ok := map[string]string{"ja": r.PostFormValue("art"), "keine": kontaktnetz.Keine, "spaeter": kontaktnetz.Spaeter,
		"aendern": r.PostFormValue("art"), "entfernen": kontaktnetz.Keine, "vergessen": ""}[antwort]
	if !ok || art == "" && antwort != "vergessen" {
		return "", texte.T(texte.NetzOhneWahl)
	}
	var f kontaktnetz.Frage
	var err error
	if gefragt := antwort == "ja" || antwort == "keine" || antwort == "spaeter"; gefragt {
		f, err = cfg.Netz.Beantworten(r.PostFormValue("x"), r.PostFormValue("y"), art)
	} else {
		f.X, f.Y, err = cfg.Netz.Aendern(r.PostFormValue("x"), r.PostFormValue("y"), art)
	}
	switch {
	case errors.Is(err, kontaktnetz.ErrKeineFrage):
		return "", texte.T(texte.DieselbeWeg)
	case err != nil:
		log.Printf("homes: netz: an answer not kept: %v", err)
		return "", texte.T(texte.NetzFehler)
	case antwort == "vergessen":
		return texte.T(texte.NetzGesagtVergess), ""
	case antwort == "entfernen":
		said = fmt.Sprintf(texte.T(texte.NetzGesagtWeg), f.X.Name, f.Y.Name)
	case art == kontaktnetz.Keine || art == kontaktnetz.Spaeter:
		return texte.T(map[bool]texte.ID{true: texte.NetzGesagtKeine, false: texte.NetzGesagtSpaeter}[art == kontaktnetz.Keine]), ""
	case art == kontaktnetz.Fremd && antwort != "aendern": // asked: no relation in a card to take away
		return texte.T(texte.NetzGesagtFremd), ""
	case art == kontaktnetz.Fremd:
		said = texte.T(texte.NetzGesagtFremd)
	default:
		said = fmt.Sprintf(texte.T(texte.NetzGesagtArt), f.X.Name, kontaktnetz.Wort(art), netzDu(f))
	}
	ctx, cancel := context.WithTimeout(r.Context(), dieselbeWait)
	defer cancel()
	if err := cfg.Netz.Eintragen(ctx, f.X, f.Y, art); err != nil {
		log.Printf("homes: netz: a relation not written to the address book: %v", err)
		said += " " + texte.T(texte.NetzNichtImBuch)
	}
	return said, ""
}
