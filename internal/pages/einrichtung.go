package pages

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/assistent"
	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/texte"
)

// Deine Aufgaben (homes#362, owner 2026-11-12: „Ich möchte einen zentralen
// Punkt“): everything a person has to do, one list per viewer, in the
// owner's order — decisions waiting (admins), keys and permissions,
// questions, setup. Each row says in a line why and leads with one tap to
// the exact form; it is gone once done. Start's Braucht dich shows the
// first three, Mehr the open ones, /napi/einrichtung the same rows.
const aufgabenPath = "/aufgaben"

// What a row is, in the list's order.
const (
	artEntscheiden = iota + 1 // a decision in Laib's inbox, an access request
	artSchluessel             // a key the box waits for, a permission a key lacks
	artFrage                  // Immich's, a scan's owner, the contacts' answers, the archive's
	artEinrichten             // set up once
)

// aufgabe is one row for one person.
type aufgabe struct {
	titel, warum string
	stand        string // how far, „2 offen“; "" for none
	href         string // the one tap; "" for a Laib task without a way: its warum's links are the way
	extern       string // the tap leaves Homes: its button („In Immich öffnen“)
	art          int
	name         string // its key in the app; "" for a running one
	modul        string // a module's app setup: the app has it as its module already
	icon         string // the app's picture for it
	laib         int    // a Laib task's id: its place on the page
	gespraech    int    // a task done together with the Bote: its id (laib#878)
	erledigt     bool
}

// ziel is where a row's tap goes off the page: its form, else its place on Deine Aufgaben.
func (a aufgabe) ziel() string {
	if a.href == "" && a.laib > 0 {
		return aufgabenPath + "#aufgabe-" + strconv.Itoa(a.laib)
	}
	return a.href
}

// sagt is its line: how far, and why.
func (a aufgabe) sagt() string {
	if a.stand != "" {
		return strings.Trim(a.stand+" · "+a.warum, " ·")
	}
	return a.warum
}

// aufgabenListe is the viewer's whole list, open and done, in the owner's order;
// l is what Laib and the archive said (nil: not read).
func aufgabenListe(r *http.Request, cfg StartConfig, l laibTeil) []aufgabe {
	as := einrichtung(r, cfg)
	if l.aufgaben != nil {
		as = append(as, laibAufgaben(r, cfg, *l.aufgaben)...)
	}
	if isAdmin(r) {
		if n := len(l.wartet); n > 0 && cfg.LaibURL != "" { // deciding is Laib's page's (ADR 0012, haus#157)
			as = append(as, aufgabe{titel: texte.Anzahl(n, texte.T("box.wartet"), texte.T("box.wartet.n")), warum: "Laib fragt, bevor es die Box ändert.",
				href: strings.TrimRight(cfg.LaibURL, "/") + "/inbox", extern: "In Laib entscheiden", art: artEntscheiden, name: "entscheiden", icon: "box"})
		}
		for _, row := range zugangRows(cfg) { // a visitor asking for access (owner 2026-11-08)
			as = append(as, aufgabe{titel: row[0], warum: row[1], href: ZugangPath, art: artEntscheiden, name: "zugang", icon: "box"})
		}
	}
	if z := l.archiv; cfg.ArchivURL != "" { // the viewer's own archive's counts (owner 2026-11-12, archiv#60)
		now, loc := cfg.clock()
		stand := ""
		if !z.Offen && !z.Am.IsZero() { // the last count, kept while the vault is closed
			stand = "Stand: " + dayWord(z.Am, now, loc)
		}
		if z.SchluesselFehlt {
			as = append(as, aufgabe{titel: "Im Archiv fehlt ein Schlüssel", warum: "Ohne ihn kommt nicht alles in dein Archiv.",
				href: cfg.ArchivURL + "/tresor/quellen", extern: "Im Archiv öffnen", art: artSchluessel, name: "archiv-schluessel", icon: "archiv"})
		}
		if z.Fragen > 0 {
			as = append(as, aufgabe{titel: "Im Archiv: " + texte.Anzahl(z.Fragen, "Frage", "Fragen") + " offen", stand: stand, warum: "Wer zu wem gehört: dein Archiv fragt nach.",
				href: cfg.ArchivURL + "/tresor", extern: "Im Archiv öffnen", art: artFrage, name: "archiv-fragen", icon: "archiv"})
		}
		if z.Aufraeumen > 0 {
			as = append(as, aufgabe{titel: texte.Anzahl(z.Aufraeumen, "Ding", "Dinge") + " zum Aufräumen im Archiv", stand: stand, warum: "Dein Archiv hat etwas zum Aufräumen gefunden.",
				href: cfg.ArchivURL + "/tresor/pflege", extern: "Im Archiv öffnen", art: artFrage, name: "archiv-aufraeumen", icon: "archiv"})
		}
	}
	slices.SortStableFunc(as, func(a, b aufgabe) int { return a.art - b.art })
	return as
}

// laibAufgaben are Laib's rows for the viewer (laib#878, laib#1195): the
// keys the box waits for and the admins' tasks for an admin, the
// household's and the person's own for everyone; a task deferred to a
// later day is the Box filter's until then, a done one is gone.
func laibAufgaben(r *http.Request, cfg StartConfig, la laib.Aufgaben) (out []aufgabe) {
	user, admin := r.Header.Get("Remote-User"), isAdmin(r)
	now, loc := cfg.clock()
	today := now.In(loc).Format("2006-01-02")
	zurueck := "&zurueck=" + url.QueryEscape(strings.TrimRight(cfg.PublicURL, "/")+aufgabenPath)
	seen := map[int]bool{}
	for _, k := range la.Schluessel {
		if admin && k.Link != "" {
			out = append(out, aufgabe{titel: cmp.Or(k.Label, k.Name), warum: cmp.Or(k.Warum, "Ohne ihn wartet "+cmp.Or(k.Dienst, "die Box")+"."),
				href: k.Link + zurueck, extern: "Schlüssel eintragen", art: artSchluessel, name: "schluessel-" + k.Name, icon: "box"})
			seen[k.Aufgabe] = true // the task that waits for it: one row, not two
		}
	}
	for _, t := range la.Aufgaben {
		admins := t.Fuer == "" || t.Fuer == "admins"
		if seen[t.ID] || t.Erledigt != nil || t.Erinnern > today || admins && !admin || !admins && t.Fuer != "alle" && t.Fuer != "person:"+user ||
			cfg.Samba != nil && t.Weg != nil && t.Weg.Seite == netzlaufwerkPath { // laib#1168: Homes' own password task, same file, at once
			continue
		}
		a := aufgabe{titel: t.Titel, warum: t.Warum, art: artEinrichten, laib: t.ID, name: "aufgabe-" + strconv.Itoa(t.ID), icon: "aufgabe"}
		switch w := t.Weg; {
		case w == nil:
		case w.Link != "": // an owner's key (laib#1195), its form on Laib's page and back here
			a.href, a.extern, a.art = w.Link+zurueck, "Schlüssel eintragen", artSchluessel
		case strings.HasPrefix(w.Seite, "/") && !strings.HasPrefix(w.Seite, "//"):
			a.href = w.Seite
		case w.Gespraech && cfg.Bote != nil:
			a.gespraech = t.ID
		}
		out = append(out, a)
	}
	return out
}

func logLaibAufgaben(err error) {
	if !errors.Is(err, laib.ErrNoTool) {
		log.Printf("homes: einrichtung: laib: %v", err)
	}
}

// aufgabenLesen reads what Deine Aufgaben needs of Laib and the archive at
// once: the tasks and keys (kept is the list kept a minute, else anew),
// an admin's decisions waiting, the viewer's own archive's counts.
func aufgabenLesen(r *http.Request, cfg StartConfig, kept bool) (l laibTeil, err error) {
	user := r.Header.Get("Remote-User")
	if cfg.Laib == nil || user == "" {
		return l, nil
	}
	reads := []func(context.Context){func(ctx context.Context) {
		var la laib.Aufgaben
		ok := true
		if kept {
			la, ok, err = cfg.Laib.KeptAufgaben(ctx, user)
		} else {
			la, err = cfg.Laib.MeineAufgaben(ctx, user)
		}
		if ok && err == nil {
			l.aufgaben = &la
		}
	}, func(ctx context.Context) { l.archiv = archivZahlen(ctx, cfg, user) }}
	if isAdmin(r) {
		reads = append(reads, func(ctx context.Context) { l.wartet, _ = cfg.Laib.Inbox(ctx) })
	}
	fetch(r, reads...)
	if err != nil {
		logLaibAufgaben(err)
	}
	return l, err
}

// archivVon is what the person's archive says of itself (archiv README
// „Status for laib“), through `laib status archiv-<person>`; ok false
// without one or when it did not answer.
func archivVon(ctx context.Context, cfg StartConfig, person string) (ar archivStand, ok bool) {
	if cfg.Laib == nil || !cfg.Archive[person] {
		return ar, false
	}
	_, own, err := cfg.Laib.ServiceStatus(ctx, "archiv-"+person)
	return ar, err == nil && json.Unmarshal(own, &ar) == nil
}

// archivAufgaben is what the person's own archive counts for them
// (archiv#60, GET /intern/aufgaben): counts only, and never through Laib's
// status, which the admins see (owner 2026-11-12: the archive's owner's only).
type archivAufgaben struct {
	Offen           bool      `json:"offen"` // false: the counts of Am, the vault closed since
	Am              time.Time `json:"am"`
	Fragen          int       `json:"fragen"`
	Aufraeumen      int       `json:"aufraeumen"`
	SchluesselFehlt bool      `json:"schluessel_fehlt"`
}

// archivZahlen asks person's own archive, the way the media come
// (X-Archiv-Homes, ArchivIntern); none without one or when it did not answer.
func archivZahlen(ctx context.Context, cfg StartConfig, person string) (z archivAufgaben) {
	a, ok := cfg.ArchivIntern[person]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL+"/intern/aufgaben", nil)
	if !ok || err != nil {
		return z
	}
	req.Header.Set("X-Archiv-Homes", a.Secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return z
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&z) != nil {
		return archivAufgaben{}
	}
	return z
}

// mitAssistent is a task's „Gemeinsam mit <name>“: the Sprechen field's
// post, the Bote reads the task and leads through it (laib#878).
func mitAssistent(id int) string {
	return `<form method=post action=/sprechen><input type=hidden name=nachricht value="Aufgabe #` + strconv.Itoa(id) + ` mit dir erledigen">` +
		`<button data-laeuft="Fragt ` + html.EscapeString(assistent.Name()) + ` …">Gemeinsam mit ` + html.EscapeString(assistent.Name()) + `</button></form>`
}

// einrichtung are the rows Homes knows itself, without asking a backend.
func einrichtung(r *http.Request, cfg StartConfig) []aufgabe {
	user := r.Header.Get("Remote-User")
	if user == "" {
		return nil
	}
	var out []aufgabe
	if cfg.Samba != nil { // the network drive's own password
		if accounts, err := cfg.Samba.Accounts(); err == nil {
			if seit, member := accounts[user]; member {
				out = append(out, aufgabe{titel: "Netzlaufwerk-Passwort festlegen", warum: "Damit du die Dateien des Hauses am Computer öffnen kannst.",
					href: netzlaufwerkPath, art: artEinrichten, name: "netzlaufwerk", icon: "netzlaufwerk", erledigt: !seit.IsZero()})
			}
		}
	}
	if privatEigen(r, cfg) && cfg.Gesichter.Fehlt() != "" { // the Immich key's missing permission, where its owner sets it
		out = append(out, aufgabe{titel: "Immich-Schlüssel: Berechtigung fehlt", warum: "Homes braucht im Schlüssel „Homes“ noch: " + cfg.Gesichter.Fehlt() + ".",
			href: strings.TrimRight(cfg.Gesichter.Web, "/") + "/user-settings?isOpen=api-keys", extern: "In Immich öffnen", art: artSchluessel, name: "immich-rechte", icon: "fotos"})
	}
	for _, a := range cfg.Einrichtung.Fuer(user) { // each module's own app, and the Homes app (owner 2026-11-04)
		out = append(out, aufgabe{titel: a.Titel, warum: a.Warum, href: einrichtenHref(a.Modul), art: artEinrichten, modul: a.Modul, erledigt: a.Erledigt != nil})
	}
	for _, a := range cfg.Einrichtung.Fragen(user) { // the key owner's questions in Immich, while open
		out = append(out, aufgabe{titel: a.Titel, warum: a.Warum, href: a.Pfad, art: artFrage, name: a.Modul, icon: "fotos"})
	}
	if cfg.Antworten != nil { // Kontakt prüfen lassen: the answers waiting for the review (owner 2026-11-07)
		if n := len(cfg.Antworten.ZuPruefen(user)); n > 0 {
			out = append(out, aufgabe{titel: antwortenAufgabe(n), warum: texte.T(texte.AntwortenAufgabeWarum), href: AntwortenPath, art: artFrage, name: "antworten", icon: "kontakte"})
		}
	}
	if cfg.Zuordnen != nil && isAdmin(r) { // laib DATA.md gap 6: what Dokumente could not give anyone, seen by no one until then
		if n := offeneScans(cfg.Zuordnen); n > 0 {
			out = append(out, aufgabe{titel: "Wem gehört dieser Scan?", stand: strconv.Itoa(n) + " offen", warum: "Bis du es sagst, sieht ihn niemand in der Familie.",
				href: ZuordnenPath, art: artFrage, name: "scans", icon: "dokumente"})
		}
	}
	if cfg.Discogs != nil { // the household's Discogs app is there; each person connects their own
		_, verbunden := cfg.Discogs.Konto(user)
		out = append(out, aufgabe{titel: "Discogs verbinden", warum: "Dann weiß Homes, welche Platten du hast, und deine Wünsche kommen auf deine Wantlist.",
			href: DiscogsPath, art: artEinrichten, name: "discogs", icon: "musik", erledigt: verbunden})
	}
	slices.SortStableFunc(out, func(a, b aufgabe) int { return a.art - b.art })
	return out
}

func offen(as []aufgabe) []aufgabe {
	var out []aufgabe
	for _, a := range as {
		if !a.erledigt {
			out = append(out, a)
		}
	}
	return out
}

// MehrAufgabenPath is where Mehr's Einrichtung card fills itself from
// (owner 2026-11-03: „Warum werden die Aufgaben nicht async geladen?“).
const MehrAufgabenPath = "/mehr/aufgaben"

// What the Einrichtung card says of Laib's tasks while they are read, and
// when they cannot be; the row leads to Deine Aufgaben, script or not.
const (
	saysAufgabenLesen = "Deine Aufgaben von der Box werden gerade gelesen …"
	saysAufgabenNicht = "Die Aufgaben von der Box lassen sich gerade nicht lesen."
	aufgabenRowTitle  = "Alle Aufgaben"
)

// einrichtungKarte is Mehr's „Deine Aufgaben“: the open ones only; none, no
// card. Mehr never waits for Laib: with Laib's list kept (laib.KeptAufgaben,
// a minute) the card is whole; without, it shows Homes' own and a row that
// leads to Deine Aufgaben, and fills itself from MehrAufgabenPath.
func einrichtungKarte(r *http.Request, cfg StartConfig) string {
	user := r.Header.Get("Remote-User")
	if cfg.Laib == nil || user == "" {
		return einrichtungCard(offen(einrichtung(r, cfg)), "")
	}
	if la, ok := cfg.Laib.PeekAufgaben(user); ok {
		return einrichtungCard(offen(aufgabenListe(r, cfg, laibTeil{aufgaben: &la})), "")
	}
	own := offen(einrichtung(r, cfg))
	return nachladen(MehrAufgabenPath, einrichtungCard(own, saysAufgabenLesen), einrichtungCard(own, saysAufgabenNicht))
}

// mehrAufgaben answers MehrAufgabenPath: the card with Laib's tasks, once
// Laib has answered (the list kept a minute behind it), or with the line
// that it cannot be read; "" when nothing is open.
func mehrAufgaben(w http.ResponseWriter, r *http.Request, cfg StartConfig) {
	if !showOnly(w, r, "Mehr") {
		return
	}
	user := r.Header.Get("Remote-User")
	card := einrichtungCard(offen(einrichtung(r, cfg)), "")
	if cfg.Laib != nil && user != "" {
		ctx, cancel := context.WithTimeout(r.Context(), backendWait)
		la, ok, err := cfg.Laib.KeptAufgaben(ctx, user)
		cancel()
		switch {
		case ok:
			card = einrichtungCard(offen(aufgabenListe(r, cfg, laibTeil{aufgaben: &la})), "")
		case errors.Is(err, laib.ErrNoTool): // an older Laib: Homes' own only
		default:
			if err == nil {
				err = context.DeadlineExceeded
			}
			logLaibAufgaben(err)
			card = einrichtungCard(offen(einrichtung(r, cfg)), saysAufgabenNicht)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(card))
}

// einrichtungCard is the card for open; laibSays, when set, is a last row
// for Laib's tasks not in hand, leading to Deine Aufgaben.
func einrichtungCard(open []aufgabe, laibSays string) string {
	c := cards.Card{Title: "Deine Aufgaben"} // one name for them, Mehr's list and the page (the audit)
	for _, a := range open {
		c.AddLink(a.titel, a.sagt(), a.ziel())
	}
	if laibSays != "" {
		c.AddLink(aufgabenRowTitle, laibSays, aufgabenPath)
	}
	return c.HTML()
}

// urlIn is an address in a sentence Laib wrote; a closing mark is not part of it.
var urlIn = regexp.MustCompile(`https://[^\s<>"]*[^\s<>".,;:!?)»“]`)

// mitLinks is escaped text with each https address in it a link.
func mitLinks(s string) string {
	return urlIn.ReplaceAllStringFunc(html.EscapeString(s), func(u string) string { return `<a href="` + u + `">` + u + `</a>` })
}

// aufgabeZeile is one open row on the page: its title the tap, its line
// with the addresses Laib wrote as links, a way out of Homes as a button.
func aufgabeZeile(a aufgabe) string {
	var b strings.Builder
	b.WriteString("<li")
	if a.laib > 0 {
		b.WriteString(" id=aufgabe-" + strconv.Itoa(a.laib))
	}
	t := html.EscapeString(a.titel)
	if a.href != "" {
		t = `<a href="` + html.EscapeString(a.href) + `">` + t + `</a>`
	}
	b.WriteString("><b>" + t + "</b>")
	if s := a.sagt(); s != "" {
		b.WriteString("<span class=text>" + mitLinks(s) + "</span>")
	}
	if a.extern != "" {
		b.WriteString(`<a class=knopf href="` + html.EscapeString(a.href) + `">` + html.EscapeString(a.extern) + `</a>`)
	}
	if a.gespraech > 0 { // done together with the Bote (laib#878)
		b.WriteString(`<span class=text>` + html.EscapeString(assistent.Name()) + ` führt dich Schritt für Schritt hindurch und prüft am Ende, ob es geklappt hat.</span>` + mitAssistent(a.gespraech))
	}
	b.WriteString("</li>")
	return b.String()
}

// aufgabenFilter is an admin's „Alle · Box“ above the list; box marks Box.
func aufgabenFilter(r *http.Request, cfg StartConfig, box bool) string {
	if !isAdmin(r) || cfg.Laib == nil {
		return ""
	}
	cur := map[bool]string{true: " aria-current=page"}
	return `<nav class=segmente aria-label=Aufgaben><a href=` + aufgabenPath + cur[!box] + `>Alle</a><a href="` + aufgabenPath + `?nur=box"` + cur[box] + `>Box</a></nav>`
}

const deineAufgabenStyle = `<style>.segmente{display:flex;gap:0 20px;margin:14px 0 0;border-bottom:1px solid var(--line)}.segmente a{display:inline-flex;align-items:center;min-height:44px;margin-bottom:-1px;border-bottom:2px solid transparent;color:var(--muted);text-decoration:none}
.segmente a[aria-current]{color:var(--ink);font-weight:600;border-color:var(--accent)}.aufgaben .knopf{display:inline-flex;align-items:center;min-height:44px;margin:8px 0 0;padding:0 18px;border-radius:999px;background:var(--accent-soft);color:var(--accent);font-weight:600;text-decoration:none}.aufgaben form{margin:8px 0 0}</style>`

// DeineAufgaben is the page Deine Aufgaben: what is open, in the owner's
// order, and what is done, to do again; ?nur=box an admin's Box filter.
func DeineAufgaben(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != aufgabenPath {
			missing{http.StatusNotFound, "Diese Seite gibt es nicht.", "Mehr", mehrPath, "Mehr"}.write(w, r, cfg)
			return
		}
		if !showOnly(w, r, "Deine Aufgaben") {
			return
		}
		if r.URL.Query().Get("nur") == "box" && isAdmin(r) && cfg.Laib != nil {
			boxAufgabenSeite(w, r, cfg, "")
			return
		}
		fetch(r, func(ctx context.Context) { cfg.Einrichtung.Pruefen(ctx, r.Header.Get("Remote-User")) }) // a module's own check (Jellyfin)
		l, err := aufgabenLesen(r, cfg, false)
		as := aufgabenListe(r, cfg, l)
		open := offen(as)
		b := familyHead(r, cfg, "Deine Aufgaben", "Deine Aufgaben")
		b.WriteString(aufgabenStyle + deineAufgabenStyle + "<h1>Deine Aufgaben" + zahl(len(open)) + "</h1><p class=k>Was du tun kannst, das Wichtigste zuerst. Ist es erledigt, verschwindet es von selbst.</p>" + aufgabenFilter(r, cfg, false))
		if err != nil && !errors.Is(err, laib.ErrNoTool) {
			b.WriteString("<p class=k>" + saysAufgabenNicht + "</p>")
		}
		switch {
		case len(open) > 0:
			b.WriteString(`<section class=karte aria-labelledby=karte-offen><h2 id=karte-offen>Offen</h2><ul class=aufgaben>`)
			for _, a := range open {
				b.WriteString(aufgabeZeile(a))
			}
			b.WriteString("</ul></section>")
		case len(as) > 0:
			b.WriteString("<p class=k>Alles erledigt.</p>")
		default:
			b.WriteString("<p class=k>Für dich gibt es gerade nichts zu tun.</p>")
		}
		done := cards.Card{Title: "Erledigt"}
		for _, a := range as {
			if a.erledigt {
				done.AddLink(a.titel, "Noch einmal: "+a.sagt(), a.ziel())
			}
		}
		b.WriteString(done.HTML())
		writePage(w, http.StatusOK, b)
	}
}

// boxAufgabenSeite is Deine Aufgaben's Box filter: the box's own to-dos with
// Erledigt and Später (aufgabenCard); said is a tick's outcome on top.
func boxAufgabenSeite(w http.ResponseWriter, r *http.Request, cfg StartConfig, said string) {
	now, _ := cfg.clock()
	card := aufgabenCard(r.Context(), cfg.Laib, now, strings.TrimRight(cfg.PublicURL, "/"), said, "")
	b := familyHead(r, cfg, "Deine Aufgaben", "Deine Aufgaben")
	b.WriteString(deineAufgabenStyle + "<h1>Deine Aufgaben</h1>" + aufgabenFilter(r, cfg, true) + card)
	writePage(w, http.StatusOK, b)
}

// AppListe is „Deine Aufgaben“ for the app (/napi/einrichtung, homes#362):
// the open rows that are no module's setup, each with its whole address;
// an admin's rows for whom Homes saw among the admins.
func AppListe(cfg StartConfig) func(ctx context.Context, person string) []einrichten.Zeile {
	return func(ctx context.Context, person string) (out []einrichten.Zeile) {
		h := http.Header{"Remote-User": {person}}
		if cfg.Einrichtung.IstAdmin(person) {
			h.Set("Remote-Groups", "admins")
		}
		r := (&http.Request{Header: h, URL: &url.URL{Path: "/napi/einrichtung"}}).WithContext(ctx)
		l, _ := aufgabenLesen(r, cfg, true)
		for i, a := range offen(aufgabenListe(r, cfg, l)) {
			link := a.ziel()
			if strings.HasPrefix(link, "/") {
				link = strings.TrimRight(cfg.PublicURL, "/") + link
			}
			if a.modul != "" || !strings.HasPrefix(link, "https://") {
				continue
			}
			out = append(out, einrichten.Zeile{Name: cmp.Or(a.name, fmt.Sprint("aufgabe-", i)), Titel: a.titel, Warum: a.sagt(),
				Icon: cmp.Or(a.icon, "aufgabe"), Link: link, Vorn: a.art < artEinrichten})
		}
		return out
	}
}
