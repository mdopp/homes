package pages

import (
	"cmp"
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

	"github.com/mdopp/homes/internal/caldav"

	"github.com/mdopp/homes/internal/haushalt"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/texte"
)

// Kontakt prüfen lassen (owner 2026-11-07, kontaktantwort; 2026-11-08:
// „direkt Link teilen“): one button makes the link and hands it to the
// phone's share sheet. The answer comes back to the contact's page, each
// field before → after with what the checks found; nothing reaches an
// address book without that review.
const (
	PruefenPath   = KontaktePath + "/pruefen"
	AntwortenPath = KontaktePath + "/antworten"
)

// pruefenAn: the store, Radicale and the public page's address are there.
func (cfg StartConfig) pruefenAn() bool {
	return cfg.Antworten != nil && cfg.Contacts != nil && cfg.KontaktURL != ""
}

// pruefenKnopf is „Link teilen“ for the contact uid of buch (pruefenScript);
// klein: the row's icon.
func pruefenKnopf(r *http.Request, cfg StartConfig, buch, uid, name string, klein bool) string {
	if !cfg.pruefenAn() || uid == "" || buch == kontaktantwort.Haushalt && !isAdmin(r) {
		return ""
	}
	innen, label := wort(texte.PruefenTeilen), ""
	if klein {
		innen, label = teilenSymbol, ` aria-label="`+html.EscapeString(fmt.Sprintf(texte.T(texte.PruefenTeilenFuer), name))+`"`
	}
	return `<form method=post action=` + PruefenPath + ` class=teilen data-teilen><input type=hidden name=buch value=` + buch + `><input type=hidden name=uid value="` + html.EscapeString(uid) +
		`"><button` + label + ` data-laeuft="` + wort(texte.PruefenErstelltLaeuft) + `" data-kopiert="` + wort(texte.PruefenKopiert) + `" data-nochmal="` + wort(texte.PruefenNochmal) + `">` + innen + `</button></form>`
}

const teilenSymbol = `<svg viewBox="0 0 24 24" width=20 height=20 aria-hidden=true fill=none stroke=currentColor stroke-width=2 stroke-linecap=round><circle cx=18 cy=5 r=3 /><circle cx=6 cy=12 r=3 /><circle cx=18 cy=19 r=3 /><path d="M8.6 13.5l6.8 4M15.4 6.5l-6.8 4"/></svg>`

func wort(id texte.ID) string { return html.EscapeString(texte.T(id)) }

func antwortenAufgabe(n int) string {
	return fmt.Sprintf(texte.T(texte.AntwortenAufgabe), texte.Anzahl(n, "Kontakt-Antwort", "Kontakt-Antworten"))
}

// pruefenKontakte are the contacts of buch the viewer may have checked.
func pruefenKontakte(ctx context.Context, r *http.Request, cfg StartConfig, buch string) ([]caldav.Contact, error) {
	if buch == kontaktantwort.Haushalt {
		all, _, err := cfg.Contacts.Contacts(ctx)
		return all, err
	}
	return meineKontakte(ctx, cfg, r.Header.Get("Remote-User"))
}

// Pruefen makes the link (POST buch=privat|haushalt, uid) and answers the
// sentence to send with it, a refusal as one plain sentence; a GET goes
// to Kontakte.
func Pruefen(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			http.Redirect(w, r, KontaktePath, http.StatusSeeOther)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		buch, uid := r.PostFormValue("buch"), r.PostFormValue("uid")
		if buch != kontaktantwort.Haushalt {
			buch = kontaktantwort.Privat
		}
		switch {
		case !cfg.pruefenAn() || !fromHomes(r): // no button without the one, nor from elsewhere
			http.Error(w, "Nur von Homes' eigener Seite.", http.StatusForbidden)
			return
		case buch == kontaktantwort.Haushalt && !isAdmin(r):
			http.Error(w, texte.T(texte.PruefenNurAdmins), http.StatusForbidden)
			return
		}
		var all []caldav.Contact
		var err error
		fetch(r, func(ctx context.Context) { all, err = pruefenKontakte(ctx, r, cfg, buch) })
		if err != nil {
			log.Printf("homes: kontakt prüfen: %s: %v", buch, err)
			http.Error(w, saysNoContacts, http.StatusServiceUnavailable)
			return
		}
		i := slices.IndexFunc(all, func(ct caldav.Contact) bool { return uid != "" && ct.UID == uid })
		if i < 0 {
			http.Error(w, texte.T(texte.PruefenWeg), http.StatusNotFound)
			return
		}
		f := kontaktantwort.FelderVon(all[i])
		von, _, _ := strings.Cut(strings.TrimSpace(r.Header.Get("Remote-Name")), " ")
		token, frage, err := cfg.Antworten.Neu(r.Header.Get("Remote-User"), von, buch, uid, f)
		if err != nil {
			log.Printf("homes: kontakt prüfen: the link not made: %v", err)
			http.Error(w, texte.T(texte.PruefenNicht), http.StatusServiceUnavailable)
			return
		}
		log.Printf("homes: kontakt prüfen: link %s made (%s)", frage.ID, buch)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, texte.T(texte.PruefenText), cmp.Or(f.Vorname, f.Name()), strings.TrimRight(cfg.KontaktURL, "/")+"/"+token)
	}
}

const pruefenStyle = `<style>.knoepfe{display:flex;gap:10px;flex-wrap:wrap}.aenderung{margin:12px 0;padding:14px;border-radius:16px;background:var(--card)}.aenderung label{display:flex;gap:12px;align-items:flex-start}.aenderung input{width:22px;height:22px;flex:none;margin-top:2px}.aenderung .art{font-size:.85rem;color:var(--muted)}
.aenderung .vorher{color:var(--muted);text-decoration:line-through;overflow-wrap:anywhere}.aenderung .nachher{font-weight:600;overflow-wrap:anywhere}.aenderung .hinweis{margin:8px 0 0 34px;color:var(--warn,#e0a030);font-size:.9rem}.aenderung .weg{font-size:.85rem;font-weight:600;color:var(--warn,#e0a030)}.aenderung img.foto{width:120px;height:auto;border-radius:12px;margin-top:4px}.aenderung blockquote{margin:0 0 10px;padding:10px 12px;border-left:3px solid var(--line);white-space:normal;overflow-wrap:anywhere}.aenderung .stand{margin:6px 0 0 34px;font-size:.85rem;color:var(--muted)}</style>`

// pruefenScript is „Link teilen“: the link made, the share sheet opens,
// else the message is copied („Link kopiert“); a share sheet refused as
// too late is „Jetzt teilen“ for the next tap; a refusal under the button.
const pruefenScript = `<script>(function(){if(window.homesTeilen)return;window.homesTeilen=1;function zeig(b,t){var w=b.innerHTML;b.textContent=t;setTimeout(function(){b.innerHTML=w},3e3)}function sag(f,t){var p=f.querySelector('.k');if(!p){p=document.createElement('p');p.className='k';p.setAttribute('role','alert');f.appendChild(p)}p.textContent=t}function kopier(f,b){function n(){sag(f,f.text)}if(navigator.clipboard)navigator.clipboard.writeText(f.text).then(function(){zeig(b,b.getAttribute('data-kopiert'))},n);else n()}
function teil(f,b){if(!navigator.share)return kopier(f,b);navigator.share({text:f.text}).then(function(){if(f.w)b.innerHTML=f.w},function(x){if(x&&x.name==='NotAllowedError'){f.w=f.w||b.innerHTML;b.textContent=b.getAttribute('data-nochmal')}else if(!x||x.name!=='AbortError')kopier(f,b)})}
document.addEventListener('submit',function(e){var f=e.target;if(!f.hasAttribute('data-teilen')||!window.fetch)return;e.preventDefault();var b=f.querySelector('button');if(f.text)return teil(f,b);if(b.disabled)return;var w=b.innerHTML,d=new URLSearchParams(new FormData(f)),c=window.AbortController?new AbortController():null,u=setTimeout(function(){if(c)c.abort()},3e4);b.disabled=true;b.textContent=b.getAttribute('data-laeuft');
fetch(f.action,{method:'POST',body:d,credentials:'same-origin',signal:c?c.signal:undefined}).then(function(r){return r.text().then(function(t){if(!r.ok)throw t;return t})}).then(function(t){clearTimeout(u);f.text=t;b.disabled=false;b.innerHTML=w;teil(f,b)},function(t){clearTimeout(u);b.disabled=false;b.innerHTML=w;sag(f,typeof t==='string'&&t?t:'Homes ist gerade nicht erreichbar. Versuch es gleich noch einmal.')})},true)})()</script>`

// Antworten is the task's address (/kontakte/antworten, and its old
// …/<id>): the contact of the first answer waiting, which is reviewed there.
func Antworten(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ziel := KontaktePath
		if cfg.pruefenAn() {
			if offen := cfg.Antworten.ZuPruefen(r.Header.Get("Remote-User")); len(offen) > 0 {
				ziel = kontaktPfad(offen[0].Buch, offen[0].UID)
			}
		}
		http.Redirect(w, r, ziel, http.StatusSeeOther)
	}
}

// antwortEntscheiden keeps the review and, for a Haushalts-Kontakt, writes
// what was taken: the answer as it stands then, and what to say.
func antwortEntscheiden(r *http.Request, cfg StartConfig, f kontaktantwort.Frage) (kontaktantwort.Frage, string, string) {
	if !f.ZuPruefen() {
		return f, "", ""
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 8<<10)
	r.ParseForm()
	var nimm []int
	for _, v := range r.PostForm["nimm"] {
		if n, err := strconv.Atoi(v); err == nil {
			nimm = append(nimm, n)
		}
	}
	aktion := r.PostFormValue("aktion")
	stand := kontaktantwort.Stand(f, aktion == "alle", nimm)
	if aktion == "verwerfen" {
		stand = kontaktantwort.Stand(f, false, nil)
	}
	var err error
	fetch(r, func(ctx context.Context) { stand, err = kontaktantwort.Uebernehmen(ctx, cfg.Contacts, f, stand) })
	problem := ""
	switch {
	case errors.Is(err, kontaktantwort.ErrWeg):
		problem = texte.T(texte.AntwortKontaktWeg)
	case err != nil:
		log.Printf("homes: kontakt-antwort %s: not written: %v", f.ID, err)
		return f, "", texte.T(texte.AntwortNicht)
	}
	done, err := cfg.Antworten.Entscheiden(f.ID, f.Wer, stand)
	if err != nil {
		return f, "", texte.T(texte.AntwortNicht)
	}
	n := map[string]int{}
	for _, s := range stand {
		n[s]++
	}
	log.Printf("homes: kontakt-antwort %s decided: %v", f.ID, n)
	var said []string
	if aktion == "verwerfen" || n[kontaktantwort.Abgelehnt] == len(stand) && len(stand) > 0 {
		said = append(said, texte.T(texte.AntwortVerworfen))
	}
	if k := n[kontaktantwort.Uebernommen]; k > 0 {
		said = append(said, fmt.Sprintf(texte.T(texte.AntwortGeschrieben), texte.Anzahl(k, "Änderung", "Änderungen")))
	}
	if k := n[kontaktantwort.Angenommen]; k > 0 {
		said = append(said, fmt.Sprintf(texte.T(texte.AntwortZumHandy), texte.Anzahl(k, "Änderung", "Änderungen")))
	}
	if k := n[kontaktantwort.Uebersprungen]; k > 0 && problem == "" {
		problem = fmt.Sprintf(texte.T(texte.AntwortUebersprungen), texte.Anzahl(k, "Änderung", "Änderungen"))
	}
	if len(stand) == 0 {
		said = append(said, texte.T(texte.AntwortErledigtGesagt))
	}
	return done, strings.Join(said, " "), problem
}

// artText is a field's name as the review says it.
var artText = map[string]texte.ID{"vorname": texte.ArtVorname, "nachname": texte.ArtNachname, "telefon": texte.ArtTelefon, "mail": texte.ArtMail,
	"adresse": texte.ArtAdresse, "geburtstag": texte.ArtGeburtstag, "firma": texte.ArtFirma, "position": texte.ArtPosition, "foto": texte.ArtFoto,
	"notiz": texte.ArtNotiz}

// nummerArt is a number's or an e-mail address's kind as the family says it.
var nummerArt = map[string]texte.ID{"mobil": texte.NummerMobil, "privat": texte.NummerPrivat, "geschaeftlich": texte.NummerGeschaeftlich,
	kontaktantwort.Andere: texte.NummerAndere}

// nummerText is a number or an e-mail address with its kind and, the main
// one, haupt's word: „0171 1234567 (mobil, Haupt-Nummer)“.
func nummerText(n, art string, main bool, haupt texte.ID) string {
	var ps []string
	if id, ok := nummerArt[art]; ok {
		ps = append(ps, texte.T(id))
	}
	if main {
		ps = append(ps, texte.T(haupt))
	}
	if n == "" || len(ps) == 0 {
		return n
	}
	return n + " (" + strings.Join(ps, ", ") + ")"
}

// standText is what became of a change, once decided.
var standText = map[string]texte.ID{kontaktantwort.Uebernommen: texte.StandUebernommen, kontaktantwort.Abgelehnt: texte.StandAbgelehnt,
	kontaktantwort.Uebersprungen: texte.StandUebersprungen, kontaktantwort.Angenommen: texte.StandAngenommen}

func wertText(a kontaktantwort.Aenderung, v, art string, main bool) string {
	switch {
	case a.Art == "geburtstag" && v != "":
		return haushalt.BirthdayText(v)
	case a.Art == "telefon":
		return nummerText(v, art, main, texte.FrageHauptNummer)
	case a.Art == "mail":
		return nummerText(v, art, main, texte.FrageHauptMail)
	}
	return v
}

// antwortFelder is one answer: each field before → after, its findings,
// and while undecided the three ways to end it, posted to action.
func antwortFelder(b *strings.Builder, f kontaktantwort.Frage, action string) {
	buch := texte.T(texte.BuchHandy)
	if f.Buch == kontaktantwort.Haushalt {
		buch = texte.T(texte.BuchHaus)
	}
	am := ""
	if f.Beantwortet != nil {
		am = f.Beantwortet.Local().Format("02.01.2006, 15:04")
	}
	b.WriteString("<p class=k>" + html.EscapeString(fmt.Sprintf(texte.T(texte.AntwortVon), am, buch)) + "</p>")
	for _, h := range f.Hinweise {
		b.WriteString(`<div class=achtung role=note><p>` + html.EscapeString(h) + `</p></div>`)
	}
	offen := f.ZuPruefen()
	if len(f.Aenderungen) == 0 {
		b.WriteString("<p>" + html.EscapeString(fmt.Sprintf(texte.T(texte.AntwortNurBestaetigt), f.Name())) + "</p>")
	}
	if offen {
		b.WriteString(`<form method=post action="` + html.EscapeString(action) + `"><input type=hidden name=id value="` + html.EscapeString(f.ID) + `">`)
	}
	for i, a := range f.Aenderungen {
		vorher, nachher := wertText(a, a.Vorher, a.VorherArt, a.VorherHaupt), wertText(a, a.Nachher, a.NachherArt, a.NachherHaupt)
		if a.Art == "notiz" {
			b.WriteString(`<div class=aenderung><p class=art>` + html.EscapeString(fmt.Sprintf(texte.T(texte.AntwortNachricht), f.Name())) + `</p><blockquote>` +
				strings.ReplaceAll(html.EscapeString(a.Nachher), "\n", "<br>") + `</blockquote>`)
		} else {
			b.WriteString(`<div class=aenderung>`)
		}
		b.WriteString(`<label>`)
		if offen {
			checked := " checked"
			if len(a.Hinweise) > 0 || a.NurAngekreuzt() {
				checked = ""
			}
			b.WriteString(`<input type=checkbox name=nimm value=` + strconv.Itoa(i) + checked + `>`)
		}
		b.WriteString(`<span>`)
		switch {
		case a.Art == "notiz":
			b.WriteString(wort(texte.AntwortNotiz))
		case a.Art == "foto":
			b.WriteString(`<span class=art>` + wort(texte.ArtFoto) + `</span><br><img class=foto src="data:image/jpeg;base64,` + base64.StdEncoding.EncodeToString(a.Bild) +
				`" alt="" width=120> <span class=k>(` + wort(texte.AntwortNeu) + `)</span>`)
		case vorher == "":
			b.WriteString(`<span class=art>` + wort(artText[a.Art]) + `</span><br><span class=nachher>` + html.EscapeString(nachher) + `</span> <span class=k>(` + wort(texte.AntwortNeu) + `)</span>`)
		case nachher == "":
			b.WriteString(`<span class=art>` + wort(artText[a.Art]) + `</span><br><span class=vorher>` + html.EscapeString(vorher) + `</span> <span class=weg>` + wort(texte.AntwortEntfernt) + `</span>`)
		default:
			b.WriteString(`<span class=art>` + wort(artText[a.Art]) + `</span><br><span class=vorher>` + html.EscapeString(vorher) + `</span> → <span class=nachher>` + html.EscapeString(nachher) + `</span>`)
		}
		b.WriteString(`</span></label>`)
		for _, h := range a.Hinweise {
			b.WriteString(`<p class=hinweis>` + html.EscapeString(h) + `</p>`)
		}
		if id, ok := standText[a.Stand]; ok && !offen {
			b.WriteString(`<p class=stand>` + wort(id) + `</p>`)
		}
		b.WriteString(`</div>`)
	}
	if offen {
		if len(f.Aenderungen) == 0 {
			b.WriteString(`<button name=aktion value=alle data-laeuft="` + wort(texte.AntwortLaeuft) + `">` + wort(texte.AntwortErledigt) + `</button></form>`)
		} else {
			b.WriteString(`<p class=knoepfe><button name=aktion value=auswahl data-laeuft="` + wort(texte.AntwortLaeuft) + `">` + wort(texte.AntwortAuswahl) + `</button>` +
				`<button name=aktion value=alle data-laeuft="` + wort(texte.AntwortLaeuft) + `">` + wort(texte.AntwortAlle) + `</button>` +
				`<button name=aktion value=verwerfen class=leise data-frage="` + wort(texte.AntwortVerwerfenFrage) + `" data-laeuft="` + wort(texte.AntwortVerwirft) + `">` + wort(texte.AntwortVerwerfen) + `</button></p></form>`)
		}
	}
}
