package pages

import (
	"cmp"
	"context"
	"errors"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/tagaktion"
)

// TagPath is a tag's own page (ADR 0004 §5): the address on a new tag,
// <HOMES_PUBLIC_URL>/tag/<id>. A phone with the Homes app opens the app
// (an Android App Link, AssetLinksPath); anyone else lands here, behind
// the login: the tag's name, what it does, one button that does it as the
// signed-in person. A GET never acts (a link preview touches nothing); a
// scanner tag that asks for whom leads to Dokumente's Scannen. Below, the
// automations that start on it, with Lösen, linking another and a new one
// by a sentence (§8): first the sentence of what it would do, then, once
// confirmed, an admin's change is written and anyone else's waits for an
// admin (tag_freigabe.go).
const TagPath = "/tag/"

var tagPageID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// tagStyle is the page's rows and forms: a row's name beside its button,
// a field over the whole width, its button under it.
const tagStyle = `<style>.tag-zeilen{list-style:none;margin:0 0 8px;padding:0}.tag-zeilen li{display:flex;gap:10px;align-items:center;justify-content:space-between;` +
	`padding:6px 0;border-bottom:1px solid var(--line)}.tag-zeilen li:last-child{border-bottom:0}.tag-zeilen span{min-width:0;overflow-wrap:anywhere}` +
	`.tag-form label{display:block;margin:14px 0 6px}.tag-form select,.tag-form textarea{display:block;width:100%;box-sizing:border-box}.tag-form button{margin-top:8px}</style>`

// tagDoes is what each change's confirmation says, and while it runs.
var tagDoes = map[string][2]string{
	tagaktion.Verbinden:      {"Ja, verbinden", "Verbindet …"},
	tagaktion.Loesen:         {"Ja, lösen", "Löst …"},
	tagaktion.NeueAutomation: {"Ja, weitergeben", "Gibt weiter …"},
}

// Tag is the page and where its buttons post.
func Tag(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, TagPath)
		if !tagPageID.MatchString(id) {
			missing{http.StatusNotFound, "Diesen Tag kennt Homes nicht.", "Geräte", "/raeume", "Zu den Räumen"}.write(w, r, cfg)
			return
		}
		if r.Method != http.MethodPost {
			if cfg.Tags.Von(id) == tagaktion.ScannerFragen { // the scanner asks for whom: Homes' own Scannen
				http.Redirect(w, r, DokumentePath+"#scannen", http.StatusSeeOther)
				return
			}
			tagSeite(w, r, cfg, id, tagGesagt{})
			return
		}
		if !fromHomes(r) {
			http.Error(w, "Das geht nur von Homes' eigener Seite.", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Das Formular ist nicht lesbar.", http.StatusBadRequest)
			return
		}
		tagPost(w, r, cfg, id)
	}
}

// tagGesagt is what a press came to, shown in place.
type tagGesagt struct {
	gut, bote string
	vorschau  *tagaktion.Wunsch // a change to confirm, with its sentence in gut
	freigabe  bool              // the change waits for an admin
	nicht     bool              // an automation did not run: the line is no success
}

func tagPost(w http.ResponseWriter, r *http.Request, cfg StartConfig, id string) {
	tun, user := r.PostForm.Get("tun"), r.Header.Get("Remote-User")
	if tun == "ausloesen" {
		e, err := cfg.Tags.Ausloesen(r.Context(), id, user)
		if err != nil {
			n := tagaktion.Als(err)
			http.Error(w, n.Satz, n.Status)
			return
		}
		satz, nicht := e.Text, slices.ContainsFunc(e.Laeufe, func(l tagaktion.Lauf) bool { return !l.Lief })
		if satz == "" {
			satz = "Ausgelöst: Home Assistant hat es bekommen."
		}
		tagSeite(w, r, cfg, id, tagGesagt{gut: satz, nicht: nicht}) // what its automations made of it (§9)
		return
	}
	if _, ok := tagDoes[tun]; !ok {
		http.Error(w, "Das lässt sich hier nicht tun.", http.StatusBadRequest)
		return
	}
	wunsch := tagaktion.Wunsch{Art: tun, Tag: id, Automation: r.PostForm.Get("automation"), Satz: r.PostForm.Get("satz")}
	wer := tagaktion.Wer{Login: user, Name: r.Header.Get("Remote-Name"), Admin: isAdmin(r)}
	got, err := cfg.Tags.Aendern(r.Context(), wunsch, wer, r.PostForm.Get("bestaetigt") == "1")
	switch {
	case err != nil:
		n := tagaktion.Als(err)
		http.Error(w, n.Satz, n.Status)
	case got.Wartet: // the page follows the wish until an admin answered
		http.Redirect(w, r, TagPath+id+"?freigabe="+url.QueryEscape(got.Freigabe), http.StatusSeeOther)
	case got.Bestaetigen:
		tagSeite(w, r, cfg, id, tagGesagt{gut: got.Satz, vorschau: &wunsch, freigabe: got.FreigabeNoetig})
	default:
		tagSeite(w, r, cfg, id, tagGesagt{gut: got.Satz, bote: got.Bote})
	}
}

// tagKnopf is the one button that does what the tag does.
func tagKnopf(id, akt string) string {
	text, laeuft := "Auslösen", "Löst aus …"
	switch akt {
	case tagaktion.ScannerIch:
		text, laeuft = "Scannen – für mich", "Startet …"
	case tagaktion.ScannerHaushalt:
		text, laeuft = "Scannen – für den Haushalt", "Startet …"
	}
	return `<form method=post action="` + TagPath + id + `"><input type=hidden name=tun value=ausloesen><button data-laeuft="` + laeuft + `">` + text + `</button></form>`
}

func tagSeite(w http.ResponseWriter, r *http.Request, cfg StartConfig, id string, g tagGesagt) {
	akt := cfg.Tags.Von(id)
	name := cfg.Tags.Name(r.Context(), id)
	b := familyHead(r, cfg, "Tag", "Geräte")
	b.WriteString(tagStyle)
	b.WriteString(`<h1>` + html.EscapeString(cmp.Or(name, "Unbekannter Tag")) + `</h1>`)
	if name == "" {
		b.WriteString(`<p class=k>Diesen Tag hat noch niemand benannt. Benennen geht in der Homes-App unter Mehr → Auslöser.</p>`)
	}
	if akt == "" {
		b.WriteString(`<p>Meldet sich bei Home Assistant; dort startet er seine Automationen.</p>`)
	} else {
		b.WriteString(`<p>` + html.EscapeString(tagaktion.TitelVon(akt)) + `.</p>`)
	}
	b.WriteString(tagFreigabeStand(r, cfg))
	switch {
	case g.vorschau != nil:
		b.WriteString(tagBestaetigen(id, g))
	case g.nicht:
		b.WriteString(`<p class=k role=status>` + html.EscapeString(g.gut) + `</p>`)
	case g.gut != "":
		b.WriteString(`<div class=gut role=status><span class=punkt></span><p>` + html.EscapeString(g.gut) + `</p></div>`)
		if g.bote != "" {
			b.WriteString(`<section class=karte><h2>Antwort</h2><p>` + strings.ReplaceAll(html.EscapeString(g.bote), "\n", "<br>") + `</p></section>`)
		}
	}
	b.WriteString(tagKnopf(id, akt))
	if akt == "" {
		b.WriteString(tagAutomationen(r, cfg, id))
	}
	writePage(w, http.StatusOK, b)
}

// tagBestaetigen is the change's sentence with its confirmation.
func tagBestaetigen(id string, g tagGesagt) string {
	does := tagDoes[g.vorschau.Art]
	out := `<section class=karte role=status><h2>So wird es</h2><p>` + html.EscapeString(g.gut) + `</p>`
	if g.freigabe {
		out += `<p class=k>Ein Admin gibt das frei; bis dahin bleibt alles, wie es ist.</p>`
	}
	return out + `<form method=post action="` + TagPath + id + `"><input type=hidden name=tun value="` + html.EscapeString(g.vorschau.Art) + `">` +
		`<input type=hidden name=automation value="` + html.EscapeString(g.vorschau.Automation) + `">` +
		`<input type=hidden name=satz value="` + html.EscapeString(g.vorschau.Satz) + `"><input type=hidden name=bestaetigt value=1>` +
		`<button data-laeuft="` + does[1] + `">` + does[0] + `</button> <a href="` + TagPath + id + `">Abbrechen</a></form></section>`
}

// tagFreigabeStand is the wish the page follows (?freigabe=), for the
// person who asked or an admin: waiting, fetched again every 10 s, then
// what came of it.
func tagFreigabeStand(r *http.Request, cfg StartConfig) string {
	f, ok := cfg.Tags.Freigabe(r.URL.Query().Get("freigabe"))
	if !ok || f.Von != r.Header.Get("Remote-User") && !isAdmin(r) {
		return ""
	}
	if f.Stand == tagaktion.Wartet {
		return `<div hidden data-auffrischen=10></div><p class=k role=status>Wartet auf Freigabe: ` + html.EscapeString(f.Text) + `</p>`
	}
	return `<div class=gut role=status><span class=punkt></span><p>` + html.EscapeString(f.Satz()) + `</p></div>`
}

// tagAutomationen are the automations that start on the tag, each with
// Lösen, and the ways to another: linking one of HA's, a new one by a sentence.
func tagAutomationen(r *http.Request, cfg StartConfig, id string) string {
	var mit map[string][]tagaktion.Verbindung
	var alle []ha.Automation
	var errMit, errAlle error
	fetch(r, func(ctx context.Context) { mit, errMit = cfg.Tags.Verbunden(ctx) }, func(ctx context.Context) { alle, errAlle = cfg.Tags.Automationen(ctx, "") })
	if err := errors.Join(errMit, errAlle); err != nil {
		log.Printf("homes: tag page %s: the automations: %v", id, err)
		return `<section class=karte><h2>Automationen</h2><p class=k>Home Assistant sagt gerade nicht, welche Automationen es gibt. Versuch es gleich noch einmal.</p></section>`
	}
	var b strings.Builder
	act := `<form method=post class=tag-form action="` + TagPath + id + `">`
	b.WriteString(`<section class=karte><h2>Automationen</h2>`)
	if l, ok := cfg.Tags.Zuletzt(r.Context(), map[string][]tagaktion.Verbindung{id: mit[id]})[id]; ok { // its last run, from HA's traces (§9)
		_, loc := cfg.clock()
		wer := ""
		if l.Person != "" {
			wer = " (" + l.Person + ")"
		}
		b.WriteString(`<p class=k>Zuletzt ` + l.Wann.In(loc).Format("02.01. 15:04") + wer + `: ` + html.EscapeString(l.Satz()) + `</p>`)
	}
	if len(mit[id]) == 0 {
		b.WriteString(`<p class=k>Noch startet keine Automation auf diesem Tag.</p>`)
	} else {
		b.WriteString(`<ul class=tag-zeilen>`)
	}
	for _, v := range mit[id] {
		b.WriteString(`<li><span>` + html.EscapeString(v.Name) + `</span><form method=post action="` + TagPath + id + `"><input type=hidden name=tun value=loesen>` +
			`<input type=hidden name=automation value="` + html.EscapeString(v.ID) + `"><button data-laeuft="Prüft …">Lösen</button></form></li>`)
	}
	if len(mit[id]) > 0 {
		b.WriteString(`</ul>`)
	}
	var frei []ha.Automation
	for _, a := range alle {
		if !slices.ContainsFunc(mit[id], func(v tagaktion.Verbindung) bool { return v.ID == a.ID }) {
			frei = append(frei, a)
		}
	}
	if len(frei) > 0 {
		b.WriteString(act + `<input type=hidden name=tun value=verbinden><label for=tag-automation>Mit einer Automation verbinden</label>` +
			`<select id=tag-automation name=automation>`)
		for _, a := range frei {
			b.WriteString(`<option value="` + html.EscapeString(a.ID) + `">` + html.EscapeString(a.Name) + `</option>`)
		}
		b.WriteString(`</select><button data-laeuft="Prüft …">Verbinden</button></form>`)
	}
	if cfg.Tags.Bote != nil {
		b.WriteString(act + `<input type=hidden name=tun value=neu><label for=tag-neu>Neue Automation: Was soll passieren, wenn jemand diesen Tag berührt?</label>` +
			`<textarea id=tag-neu name=satz rows=2 maxlength=500 required placeholder="Licht im Flur an"></textarea><button data-laeuft="Prüft …">Weitergeben</button></form>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}
