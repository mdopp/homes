package tagaktion

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mdopp/homes/internal/assistent"
	"github.com/mdopp/homes/internal/bote"
	"github.com/mdopp/homes/internal/jsonfile"
)

// Who may change a tag's automations (ADR 0004 §8, the owner: „1b, aber
// Bestätigung über Signal-Abfrage an die Admins“): everyone in the
// household. An admin's change is written once they confirmed it in
// place; anyone else's waits for an admin: every admin is told on their
// chat channel (laib tell) with the link to the decision, and it is a
// task on their „Deine Aufgaben“; the first admin's answer counts, and the
// person is told what came of it. Nothing waits longer than Frist.
//
// This is the household approving a change to one of its HA automations,
// not a verdict on the box: Homes' rule that it decides nothing (haus#1)
// is about Laib's inbox, which stays Laib's page alone.

// Frist is how long a wish waits for an admin.
const Frist = 7 * 24 * time.Hour

// behalten is how long a decided wish is kept, so the person's app can
// still read what came of it.
const behalten = 30 * 24 * time.Hour

// A wish's states.
const (
	Wartet      = "wartet"
	Freigegeben = "freigegeben"
	Abgelehnt   = "abgelehnt"
	Verfallen   = "verfallen"
)

// Wer is who asks or decides.
type Wer struct {
	Login, Name string
	Admin       bool
}

// anzeige is the name a sentence says: the display name, else the login
// with a capital.
func (w Wer) anzeige() string {
	if n := strings.TrimSpace(w.Name); n != "" {
		return n
	}
	r, size := utf8.DecodeRuneInString(w.Login)
	return string(unicode.ToUpper(r)) + w.Login[size:]
}

// Freigabe is one wish waiting for an admin, or what came of it.
type Freigabe struct {
	ID string `json:"id"`
	Wunsch
	TagName  string    `json:"tag_name"`
	AutoName string    `json:"auto_name,omitempty"`
	Text     string    `json:"text"`     // the wish as the admins read it
	Vorschau string    `json:"vorschau"` // what it does, in one sentence
	Von      string    `json:"von"`
	Am       time.Time `json:"am"`
	Stand    string    `json:"stand"`
	Durch    string    `json:"durch,omitempty"`    // the admin who answered
	Ergebnis string    `json:"ergebnis,omitempty"` // what came of it, a sentence for the person
}

// Antwort is what a change answers: a preview to confirm, the change
// done, or the wish waiting for an admin.
type Antwort struct {
	Satz           string
	Bestaetigen    bool // a preview: nothing is written until it comes again confirmed
	FreigabeNoetig bool // the preview's change will wait for an admin
	Wartet         bool
	Freigabe       string // the wish's id while it waits
	Bote           string // a new automation: the Bote's reply (its draft, the code it asks for)
}

// Aendern is a change to a tag's automations by wer: unconfirmed, the
// sentence of what it would do; confirmed, written for an admin, a wish
// to the admins for anyone else.
func (d *Dienst) Aendern(ctx context.Context, w Wunsch, wer Wer, bestaetigt bool) (Antwort, error) {
	p, err := d.planen(ctx, w)
	if err != nil {
		return Antwort{}, err
	}
	if !bestaetigt {
		return Antwort{Satz: p.vorschau(), Bestaetigen: true, FreigabeNoetig: !wer.Admin}, nil
	}
	if wer.Admin {
		satz, antwort, err := d.ausfuehren(ctx, p, bote.Person{User: wer.Login, Name: wer.Name})
		return Antwort{Satz: satz, Bote: antwort}, err
	}
	f, err := d.wuenschen(ctx, p, wer)
	if err != nil {
		return Antwort{}, err
	}
	return Antwort{Satz: "Wartet auf Freigabe: " + f.Text + " Ein Admin bekommt Bescheid.", Wartet: true, Freigabe: f.ID}, nil
}

func (d *Dienst) freigabenPfad() string { return filepath.Join(d.Dir, "tag-freigaben.json") }

// laden reads the wishes; fmu is held.
func (d *Dienst) laden() map[string]*Freigabe {
	m := map[string]*Freigabe{}
	if b, err := os.ReadFile(d.freigabenPfad()); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func (d *Dienst) sichern(m map[string]*Freigabe) error {
	if d.Dir == "" {
		return nein(http.StatusServiceUnavailable, "Homes kann hier nichts aufheben, was auf eine Freigabe wartet.")
	}
	return jsonfile.Save(d.freigabenPfad(), m)
}

// stand is f's state now: one waiting past Frist has lapsed.
func (d *Dienst) stand(f *Freigabe) string {
	if f.Stand == Wartet && d.now().Sub(f.Am) > Frist {
		return Verfallen
	}
	return f.Stand
}

// Link is where an admin decides a wish.
func (d *Dienst) Link(id string) string {
	return strings.TrimRight(d.PublicURL, "/") + "/tag/freigabe/" + id
}

func (d *Dienst) wuenschen(ctx context.Context, p plan, wer Wer) (Freigabe, error) {
	var b [8]byte
	rand.Read(b[:])
	f := Freigabe{ID: hex.EncodeToString(b[:]), Wunsch: p.Wunsch, TagName: p.TagName, AutoName: p.AutoName,
		Text: p.wunschSatz(wer.anzeige()), Vorschau: p.vorschau(), Von: wer.Login, Am: d.now(), Stand: Wartet}
	d.fmu.Lock()
	m := d.laden()
	m[f.ID] = &f
	err := d.sichern(m)
	d.fmu.Unlock()
	if err != nil {
		return f, Als(err)
	}
	var admins []string
	if d.Admins != nil {
		admins = d.Admins()
	}
	log.Printf("homes: tag %s: %s asks the admins (%d known): %s", p.Tag, wer.Login, len(admins), p.Art)
	for _, a := range admins {
		if a != wer.Login {
			d.sag(ctx, a, f.Text+" Freigeben: "+d.Link(f.ID))
		}
	}
	return f, nil
}

// sag tells person one line through Laib; a failure is logged, the wish
// stays on the admins' Deine Aufgaben.
func (d *Dienst) sag(ctx context.Context, person, text string) {
	if d.Laib == nil {
		return
	}
	c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := d.Laib.Tell(c, person, text); err != nil {
		log.Printf("homes: tag freigabe: telling %s: %v", person, err)
	}
}

// Freigabe is one wish with its state now; false for none.
func (d *Dienst) Freigabe(id string) (Freigabe, bool) {
	d.fmu.Lock()
	defer d.fmu.Unlock()
	f, ok := d.laden()[id]
	if !ok {
		return Freigabe{}, false
	}
	out := *f
	out.Stand = d.stand(f)
	return out, true
}

// Offen are the wishes waiting, oldest first: the admins' tasks.
func (d *Dienst) Offen() []Freigabe {
	if d == nil {
		return nil
	}
	d.fmu.Lock()
	defer d.fmu.Unlock()
	var out []Freigabe
	for _, f := range d.laden() {
		if d.stand(f) == Wartet {
			out = append(out, *f)
		}
	}
	slices.SortFunc(out, func(a, b Freigabe) int { return a.Am.Compare(b.Am) })
	return out
}

// Entscheiden is an admin's answer to a wish; the first one counts. Yes
// writes the change as that admin (a new automation: the Bote's reply is
// theirs to go on with); the person is told either way.
func (d *Dienst) Entscheiden(ctx context.Context, id string, admin Wer, ja bool) (Freigabe, string, error) {
	if !admin.Admin {
		return Freigabe{}, "", nein(http.StatusForbidden, "Freigeben kann nur ein Admin.")
	}
	f, err := d.nehmen(id)
	if err != nil {
		return f, "", err
	}
	defer d.laeuft.Delete(id)
	vomBote, mitteilung := "", admin.anzeige()+" hat deinen Wunsch nicht freigegeben: "+f.Text
	f.Stand, f.Durch, f.Ergebnis = Abgelehnt, admin.Login, admin.anzeige()+" hat es nicht freigegeben."
	if ja { // written outside the lock: the Bote may take a while, the others' lists do not wait
		satz, antwort, err := d.ausfuehren(ctx, plan{Wunsch: f.Wunsch, TagName: f.TagName, AutoName: f.AutoName}, bote.Person{User: admin.Login, Name: admin.Name})
		if err != nil {
			f.Stand, f.Durch, f.Ergebnis = Wartet, "", ""
			return f, "", err // it stays waiting: another try, or Ablehnen
		}
		vomBote, f.Stand, f.Ergebnis = antwort, Freigegeben, satz
		mitteilung = admin.anzeige() + " hat deinen Wunsch freigegeben. " + satz
		if f.Art == NeueAutomation {
			f.Ergebnis = admin.anzeige() + " richtet die Automation mit " + assistent.Name() + " ein."
			mitteilung = admin.anzeige() + " hat deinen Wunsch freigegeben und richtet die Automation mit " + assistent.Name() + " ein."
		}
	}
	d.fmu.Lock()
	m := d.laden()
	m[id] = &f
	if err := d.sichern(m); err != nil {
		log.Printf("homes: tag-freigaben: %v", err)
	}
	d.fmu.Unlock()
	log.Printf("homes: tag freigabe %s: %s by %s", id, f.Stand, admin.Login)
	d.sag(ctx, f.Von, mitteilung)
	return f, vomBote, nil
}

// nehmen is a waiting wish an admin answers now: refused when it is
// decided, lapsed, or another admin's answer is being written.
func (d *Dienst) nehmen(id string) (Freigabe, error) {
	d.fmu.Lock()
	defer d.fmu.Unlock()
	f, ok := d.laden()[id]
	if !ok {
		return Freigabe{}, nein(http.StatusNotFound, "Diesen Wunsch gibt es nicht (mehr).")
	}
	out := *f
	if out.Stand = d.stand(f); out.Stand != Wartet {
		return out, nein(http.StatusConflict, "Darüber ist schon entschieden: "+stehtSatz(out))
	}
	if _, busy := d.laeuft.LoadOrStore(id, true); busy {
		return out, nein(http.StatusConflict, "Ein anderer Admin antwortet gerade darauf.")
	}
	return out, nil
}

// stehtSatz is a wish's state in one sentence, for the person and the admins.
func stehtSatz(f Freigabe) string {
	switch f.Stand {
	case Wartet:
		return "Wartet auf Freigabe durch einen Admin."
	case Verfallen:
		return "Nach 7 Tagen ohne Antwort verfallen."
	}
	return f.Ergebnis
}

// Satz is f's state in one sentence.
func (f Freigabe) Satz() string { return stehtSatz(f) }

// Pflegen drops what waited past Frist, every tick, telling the person,
// and forgets what was decided long ago.
func (d *Dienst) Pflegen(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		d.pflegen(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (d *Dienst) pflegen(ctx context.Context) {
	d.fmu.Lock()
	m := d.laden()
	type mitteilung struct{ an, text string }
	var sagen []mitteilung
	changed := false
	for id, f := range m {
		switch {
		case f.Stand == Wartet && d.stand(f) == Verfallen:
			f.Stand, f.Ergebnis, changed = Verfallen, "Nach 7 Tagen ohne Antwort verfallen.", true
			sagen = append(sagen, mitteilung{f.Von, "Dein Wunsch ist nach 7 Tagen ohne Antwort eines Admins verfallen: " + f.Text})
		case f.Stand != Wartet && d.now().Sub(f.Am) > behalten:
			delete(m, id)
			changed = true
		}
	}
	if changed {
		if err := d.sichern(m); err != nil {
			log.Printf("homes: tag-freigaben: %v", err)
		}
	}
	d.fmu.Unlock()
	for _, s := range sagen {
		d.sag(ctx, s.an, s.text)
	}
}
