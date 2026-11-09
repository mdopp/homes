package pages

import (
	"net/http"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/kontaktantwort"
	kontaktnetz "github.com/mdopp/homes/internal/netz"
	"github.com/mdopp/homes/internal/texte"
)

// wartet is what waits for the viewer (owner 2026-11-08: „alles auf dem
// Kontakt, damit weniger Listen“), answered on the contact's page and
// counted on its row: answers by contact key (<buch>/<uid>, the oldest),
// and for the Immich key's owner „Ist das …?“ by key and the pairs by
// either person's Immich page.
type wartet struct {
	antworten map[string]kontaktantwort.Frage
	fragen    map[string][]gesichter.Frage
	paare     map[string][]gesichter.Paar
	netz      map[string][]kontaktnetz.Frage // by node: „Wie steht X zu Y?“ (owner 2026-11-09)
}

func offenFuer(r *http.Request, cfg StartConfig) wartet {
	o := wartet{map[string]kontaktantwort.Frage{}, map[string][]gesichter.Frage{}, map[string][]gesichter.Paar{}, map[string][]kontaktnetz.Frage{}}
	if cfg.pruefenAn() {
		for _, f := range slices.Backward(cfg.Antworten.ZuPruefen(r.Header.Get("Remote-User"))) {
			o.antworten[f.Buch+"/"+f.UID] = f
		}
	}
	if privatEigen(r, cfg) {
		for _, f := range cfg.Gesichter.Fragen() {
			o.fragen[f.Key] = append(o.fragen[f.Key], f)
		}
		for _, p := range cfg.Gesichter.Paare() {
			a, b := cfg.Gesichter.PersonSeite(p.A.ID), cfg.Gesichter.PersonSeite(p.B.ID)
			o.paare[a], o.paare[b] = append(o.paare[a], p), append(o.paare[b], p)
		}
	}
	if netzDa(r, cfg) {
		for _, f := range cfg.Netz.Fragen() {
			o.netz[f.X.ID], o.netz[f.Y.ID] = append(o.netz[f.X.ID], f), append(o.netz[f.Y.ID], f)
		}
	}
	return o
}

// bei is what waits about the contact uid of buch named name.
func (o wartet) bei(cfg StartConfig, buch, uid, name string) (*kontaktantwort.Frage, []gesichter.Frage, []gesichter.Paar) {
	key := buch + "/" + uid
	a, da := o.antworten[key]
	return map[bool]*kontaktantwort.Frage{true: &a}[da], o.fragen[key], o.paare[cfg.Gesichter.Link(key, name)]
}

// zahl is how many things wait about the contact, its row's count.
func (o wartet) zahl(cfg StartConfig, buch, uid, name string) int {
	a, f, p := o.bei(cfg, buch, uid, name)
	return len(f) + len(p) + map[bool]int{true: 1}[a != nil] + len(o.beziehungen(cfg, buch, uid))
}

// beziehungen are the relation questions about the contact uid of buch.
func (o wartet) beziehungen(cfg StartConfig, buch, uid string) []kontaktnetz.Frage {
	if len(o.netz) == 0 {
		return nil
	}
	return o.netz[cfg.Netz.Von(buch+"/"+uid)]
}

// entschieden is what a POST to a contact's page ended with; antwort the
// review just decided, with what became of each change.
type entschieden struct {
	said, problem string
	antwort       *kontaktantwort.Frage
}

// kontaktPost takes an answer given on a contact's page: a review (id,
// aktion, nimm) or a question of Immich's (immichAntwort); a GET passes.
// ok false: answered already.
func kontaktPost(w http.ResponseWriter, r *http.Request, cfg StartConfig, nurZeigen bool) (e entschieden, ok bool) {
	if r.Method != http.MethodPost || nurZeigen {
		return e, showOnly(w, r, "Der Kontakt")
	}
	if !fromHomes(r) {
		http.Error(w, "Nur von Homes' eigener Seite.", http.StatusForbidden)
		return e, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	r.ParseForm()
	if id := r.PostForm.Get("id"); id != "" && cfg.pruefenAn() {
		e.problem = texte.T(texte.AntwortWeg)
		if f, err := cfg.Antworten.Eine(id, r.Header.Get("Remote-User")); err == nil {
			f, e.said, e.problem = antwortEntscheiden(r, cfg, f)
			e.antwort = &f
		}
		return e, true
	}
	e.said, e.problem, ok = immichAntwort(w, r, cfg)
	return e, ok
}

// kontaktOffen writes the outcome of e and what waits about the contact,
// each posted to its page (action).
func kontaktOffen(b *strings.Builder, r *http.Request, cfg StartConfig, buch, uid, name, action string, e entschieden) {
	o := offenFuer(r, cfg)
	antwort, fragen, paare := o.bei(cfg, buch, uid, name)
	if e.antwort != nil {
		antwort = e.antwort
	}
	b.WriteString(gesagt(e.said, e.problem) + pruefenStyle + dieselbeStyle)
	if antwort != nil {
		b.WriteString("<h2>" + wort(texte.KontaktAntwort) + "</h2>")
		antwortFelder(b, *antwort, action)
	}
	b.WriteString(istFragen(cfg, fragen, action))
	for i, p := range paare {
		b.WriteString(map[bool]string{true: "<h2>" + wort(texte.Dieselbe) + "</h2>"}[i == 0] + paarKarte(cfg, p, action))
	}
	b.WriteString(netzFragen(cfg, o.beziehungen(cfg, buch, uid), action))
}
