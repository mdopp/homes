// Package plugin is the plugin contract (homes ADR 0003 §3, homes#375): a
// program of its own in layer 2 that Laib registers with Homes appears in
// it with its name, icon and link, and optionally a Start card, tasks for
// „Deine Aufgaben“ and app widgets, each served by the plugin over HTTP.
// Homes knows no plugin by name: Laib's homes template wires each into the
// environment (instances:), and removing a plugin removes its entry. Homes
// keeps nothing of a plugin but its last answer, the tasks with their text
// among it, so they show while the plugin is closed (the archive: while
// the vault is locked). A plugin that does not answer is a quiet line,
// never an error. Standard library and jsonfile only (scripts/loc.sh:
// connectors).
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mdopp/homes/internal/jsonfile"
)

// Version is the contract this Homes speaks; a plugin serves it at Path
// below the address Laib registered, for the person named in ?person=,
// and checks its key in Header.
const (
	Version = 1
	Path    = "/homes/v1"
	Header  = "X-Homes-Plugin-Key"
)

// The environment Laib renders (laib templates/homes, instances:):
// HOMES_PLUGIN_<NAME> is a plugin's address, "<person>=<url> …" for one
// installed per person (each person's own instance, seen by that person
// only), else one url for everyone; HOMES_PLUGINKEY_<NAME>_<PERSON> (or
// HOMES_PLUGINKEY_<NAME>) its key.
const (
	envURL = "HOMES_PLUGIN_"
	envKey = "HOMES_PLUGINKEY_"
)

// Answer is what a plugin serves at Path. Vertrag, Name and Link are
// required, the rest optional.
type Answer struct {
	Vertrag int    `json:"vertrag"`
	Name    string `json:"name"`            // „Archiv“, as the family reads it
	Icon    string `json:"icon,omitempty"`  // one of Homes' icon names; another: a plain one
	Link    string `json:"link"`            // https://, where the program opens
	Zeile   string `json:"zeile,omitempty"` // its line under Mehr
	Start   *Karte `json:"start,omitempty"` // a card on Start
	// Aufgaben for „Deine Aufgaben“; absent (null), the plugin cannot say
	// now (the archive's vault is locked): Homes keeps those handed over last
	Aufgaben *[]Aufgabe `json:"aufgaben,omitempty"`
	Widgets  []Widget   `json:"widgets,omitempty"` // for the app (not shown by Homes' pages)
}

// Karte is a plugin's card on Start: a title and one line, its tap.
type Karte struct {
	Titel string `json:"titel"`
	Zeile string `json:"zeile,omitempty"`
	Link  string `json:"link,omitempty"`
}

// Aufgabe is one row of „Deine Aufgaben“, with its text: Art "frage"
// (the default), "schluessel" or "einrichten" sorts it among Homes' own.
type Aufgabe struct {
	ID    string `json:"id"`
	Titel string `json:"titel"`
	Warum string `json:"warum,omitempty"`
	Link  string `json:"link,omitempty"` // "": the plugin's Link
	Art   string `json:"art,omitempty"`
}

// Widget is a value for the app's widgets.
type Widget struct {
	Name  string `json:"name"`
	Titel string `json:"titel"`
	Wert  string `json:"wert"`
	Link  string `json:"link,omitempty"`
}

// What one answer may hold.
const (
	maxBody     = 64 << 10
	maxAufgaben = 20
	maxWidgets  = 8
	maxTitel    = 160
	maxText     = 400
)

var (
	nameRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)
	wortRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
)

// errVertrag: an answer outside the contract, treated as no answer.
var errVertrag = errors.New("plugin: answer outside contract 1")

func https(s string) bool {
	return strings.HasPrefix(s, "https://") && !strings.ContainsAny(s, " \n\"<>")
}

func kurz(s string, n int) bool { return utf8.RuneCountInString(s) <= n }

// pruefen: a is contract 1, its links https, its texts short.
func (a *Answer) pruefen() error {
	ok := a.Vertrag == Version && a.Name != "" && kurz(a.Name, 40) && https(a.Link) && kurz(a.Zeile, maxText) &&
		(a.Icon == "" || wortRe.MatchString(a.Icon)) && len(a.Widgets) <= maxWidgets
	if s := a.Start; s != nil {
		ok = ok && s.Titel != "" && kurz(s.Titel, maxTitel) && kurz(s.Zeile, maxText) && (s.Link == "" || https(s.Link))
	}
	if a.Aufgaben != nil {
		ok = ok && len(*a.Aufgaben) <= maxAufgaben
		for _, t := range *a.Aufgaben {
			ok = ok && wortRe.MatchString(t.ID) && t.Titel != "" && kurz(t.Titel, maxTitel) && kurz(t.Warum, maxText) &&
				(t.Link == "" || https(t.Link)) && slices.Contains([]string{"", "frage", "schluessel", "einrichten"}, t.Art)
		}
	}
	for _, w := range a.Widgets {
		ok = ok && wortRe.MatchString(w.Name) && kurz(w.Titel, maxTitel) && kurz(w.Wert, maxTitel) && (w.Link == "" || https(w.Link))
	}
	if !ok {
		return errVertrag
	}
	return nil
}

// Instance is one registered plugin: Person's own, or everyone's ("").
type Instance struct {
	Plugin, Person, URL, Key string
}

// FromEnv is every plugin Laib registered in environ (os.Environ), in
// order; a malformed entry is logged and left out.
func FromEnv(environ []string) (out []Instance) {
	env := map[string]string{}
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		name, ok := strings.CutPrefix(k, envURL)
		if !ok || !nameRe.MatchString(name) {
			continue
		}
		plugin := strings.ToLower(name)
		for _, f := range strings.Fields(v) {
			person, u, mine := strings.Cut(f, "=")
			if !mine {
				person, u = "", f
			}
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				log.Printf("homes: %s: %q is no <person>=<url> nor a url", k, f)
				continue
			}
			key := env[envKey+name]
			if person != "" {
				key = env[envKey+name+"_"+strings.ToUpper(strings.ReplaceAll(person, "-", "_"))]
			}
			out = append(out, Instance{plugin, person, strings.TrimRight(u, "/"), key})
		}
	}
	slices.SortFunc(out, func(a, b Instance) int { return strings.Compare(a.Plugin+"/"+a.Person, b.Plugin+"/"+b.Person) })
	return out
}

// Stand is one plugin as one person sees it: its last good answer (the
// tasks those handed over last, at Am), Jetzt when it handed them over in
// this very answer, Still when it did not answer the last time it was asked.
type Stand struct {
	Plugin string `json:"plugin"`
	Answer
	Am    time.Time `json:"am"`
	Jetzt bool      `json:"-"`
	Still bool      `json:"still"`
}

// Plugins are the registered plugins and what Homes keeps of each, per
// person, in Datei (outside every plugin: the tasks' text is the person's,
// laib docs/DATA.md).
type Plugins struct {
	inst   []Instance
	datei  string
	Client *http.Client
	Now    func() time.Time
	mu     sync.Mutex
	kept   map[string]Stand // "<plugin>/<person>"
}

// Open keeps what file holds of the plugins still registered: an entry
// of one Laib no longer registers is gone, with its tasks.
func Open(inst []Instance, file string) *Plugins {
	p := &Plugins{inst: inst, datei: file, Client: &http.Client{Timeout: 4 * time.Second}, Now: time.Now, kept: map[string]Stand{}}
	if b, err := os.ReadFile(file); err == nil {
		if err := json.Unmarshal(b, &p.kept); err != nil {
			log.Printf("homes: %s: %v", file, err)
		}
	}
	n := len(p.kept)
	for k := range p.kept {
		plugin, person, _ := strings.Cut(k, "/")
		if !slices.ContainsFunc(inst, func(i Instance) bool { return i.Plugin == plugin && (i.Person == person || i.Person == "") }) {
			delete(p.kept, k)
		}
	}
	if len(p.kept) != n {
		p.save()
	}
	return p
}

func (p *Plugins) save() {
	if p.datei == "" {
		return
	}
	if err := jsonfile.Save(p.datei, p.kept); err != nil {
		log.Printf("homes: plugins: %v", err)
	}
}

// fuer are the instances person sees.
func (p *Plugins) fuer(person string) (out []Instance) {
	if p == nil || person == "" {
		return nil
	}
	for _, i := range p.inst {
		if i.Person == person || i.Person == "" {
			out = append(out, i)
		}
	}
	return out
}

// Gemerkt is what Homes keeps for person, without asking: the plugins
// that ever answered, in order.
func (p *Plugins) Gemerkt(person string) (out []Stand) {
	for _, i := range p.fuer(person) {
		p.mu.Lock()
		s, ok := p.kept[i.Plugin+"/"+person]
		p.mu.Unlock()
		if ok {
			out = append(out, s)
		}
	}
	return out
}

// Lesen asks each plugin person sees, side by side, and keeps what they
// say; one that does not answer in ctx is its kept Stand, Still. A plugin
// that never answered is not there at all.
func (p *Plugins) Lesen(ctx context.Context, person string) []Stand {
	is := p.fuer(person)
	got := make([]*Stand, len(is))
	var wg sync.WaitGroup
	for n, i := range is {
		wg.Add(1)
		go func() { defer wg.Done(); got[n] = p.fragen(ctx, i, person) }()
	}
	wg.Wait()
	var out []Stand
	for _, s := range got {
		if s != nil {
			out = append(out, *s)
		}
	}
	return out
}

// fragen asks i for person and merges the answer into what is kept.
func (p *Plugins) fragen(ctx context.Context, i Instance, person string) *Stand {
	a, err := p.abrufen(ctx, i, person)
	key := i.Plugin + "/" + person
	p.mu.Lock()
	defer p.mu.Unlock()
	s, had := p.kept[key]
	if err != nil {
		if !had { // never answered, or no contract (yet, errKeins): no trace
			return nil
		}
		if !s.Still { // once per silence in the log, not every poll
			log.Printf("homes: plugin %s for %s: %v", i.Plugin, person, err)
			s.Still = true
			p.kept[key] = s
			p.save()
		}
		return &s
	}
	neu := s
	neu.Plugin, neu.Still, neu.Answer = i.Plugin, false, a
	switch now := p.Now(); {
	case a.Aufgaben == nil: // the plugin cannot say now: the tasks handed over last stay
		neu.Aufgaben = s.Aufgaben
	case !had || mustJSON(*a.Aufgaben) != mustJSON(s.Aufgaben) || now.Sub(s.Am) >= frisch:
		neu.Am = now
	}
	if !had || mustJSON(neu) != mustJSON(s) {
		p.kept[key] = neu
		p.save()
	}
	neu.Jetzt = a.Aufgaben != nil
	return &neu
}

// frisch: tasks unchanged get a new Am after this long, so Am says when
// they were last handed over, not when they last changed.
const frisch = 15 * time.Minute

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// errKeins: the address answers, but serves no contract (404): an older
// plugin, or none.
var errKeins = errors.New("plugin: no contract at this address")

func (p *Plugins) abrufen(ctx context.Context, i Instance, person string) (Answer, error) {
	var a Answer
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i.URL+Path+"?person="+url.QueryEscape(person), nil)
	if err != nil {
		return a, err
	}
	if i.Key != "" {
		req.Header.Set(Header, i.Key)
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return a, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return a, errKeins
	case resp.StatusCode != http.StatusOK:
		return a, fmt.Errorf("plugin: %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&a); err != nil {
		return a, fmt.Errorf("plugin: %w", err)
	}
	return a, a.pruefen()
}

// Pflegen asks every person's own plugins every so often, so a plugin's
// tasks reach Homes while it is open even when nobody looks (the vault
// open in the archive, Homes' pages closed).
func (p *Plugins) Pflegen(ctx context.Context, every time.Duration) {
	if p == nil {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		seen := map[string]bool{}
		for _, i := range p.inst {
			if i.Person != "" && !seen[i.Person] {
				seen[i.Person] = true
				c, cancel := context.WithTimeout(ctx, 10*time.Second)
				p.Lesen(c, i.Person)
				cancel()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
