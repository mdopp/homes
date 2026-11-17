// Package sprachmodell is the box's language model as a switch (owner
// 2026-11-17): the household model by default, another preset for a
// while when a person asks. Homes takes Laib's lease (package modellease)
// as the person who switched, homes-<person>, renews it every
// renew_after until the end they chose and then gives it back, so the
// box returns to the household by itself. A Homes that restarts or stops
// renews nothing: Laib gives the window back after two missed renewals
// (10 min), and a window of homes-… that this Homes did not take is given
// back at once (ADR 0011's consumer rule). Who may switch is who may
// switch a lamp: any signed-in person, from Homes' own page or a paired
// phone; the surfaces check that, this package switches.
package sprachmodell

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/modellease"
	"github.com/mdopp/homes/internal/texte"
)

// Dauern are the durations a person picks from.
var Dauern = []time.Duration{time.Hour, 4 * time.Hour}

// Prefix starts every holder Homes takes the lease as.
const Prefix = "homes-"

// Schalter is the switch.
type Schalter struct {
	Lease *modellease.Client
	Now   func() time.Time // the clock; nil, time.Now

	mu      sync.Mutex
	ende    map[string]time.Time // each window Homes took: its holder and the end chosen
	presets []string             // the proxy's presets, the household's first, once read
}

// Errors the surfaces say in words.
var (
	ErrNicht = errors.New("sprachmodell: not offered") // a preset or duration the card does not offer
	ErrWeg   = errors.New("sprachmodell: the lease does not answer")
)

// FromEnv is the switch on the lease beside HOMES_LLM_URL with
// HOMES_LLM_KEY; nil without them.
func FromEnv() *Schalter {
	if l := modellease.FromEnv(); l != nil {
		return &Schalter{Lease: l}
	}
	return nil
}

func (s *Schalter) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var unsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// Halter is the lease's holder for person: homes-<person> in the lease's
// alphabet ([a-z0-9-], at most 64).
func Halter(person string) string {
	h := Prefix + strings.Trim(unsafe.ReplaceAllString(strings.ToLower(person), "-"), "-")
	return h[:min(len(h), 64)]
}

// Stand is what the card shows.
type Stand struct {
	Erreichbar bool
	Haushalt   string    // the household's preset
	Preset     string    // the preset that answers now
	Halter     string    // who holds a window; "" in the household
	Bis        time.Time // its end; zero while it loads
	Laedt      bool
	Presets    []string // what a person may switch to: every preset but the household's
}

// Homes says whether the window is one of Homes' persons', and whose.
func (st Stand) Homes() (string, bool) { return strings.CutPrefix(st.Halter, Prefix) }

// Lesen is the lease now, with the presets to offer.
func (s *Schalter) Lesen(ctx context.Context) Stand {
	l, err := s.Lease.Read(ctx)
	if err != nil {
		log.Printf("homes: sprachmodell: %v", err)
		return Stand{}
	}
	all := s.alle(ctx)
	st := Stand{Erreichbar: true, Preset: l.Alias, Halter: l.Holder, Laedt: l.State == "preparing"}
	if len(all) > 0 {
		st.Haushalt, st.Presets = all[0], all[1:]
	}
	if l.Holder == "" {
		st.Haushalt = l.Alias // with no window the lease names the household's
	} else if l.ExpiresAt != nil {
		st.Bis = time.Unix(*l.ExpiresAt, 0)
	}
	if i := slices.Index(st.Presets, st.Haushalt); i >= 0 {
		st.Presets = slices.Delete(slices.Clone(st.Presets), i, i+1)
	}
	return st
}

// alle are the proxy's presets, read once they answer.
func (s *Schalter) alle(ctx context.Context) []string {
	s.mu.Lock()
	have := s.presets
	s.mu.Unlock()
	if have != nil {
		return have
	}
	ps, err := s.Lease.Presets(ctx)
	if err != nil {
		log.Printf("homes: sprachmodell: the presets: %v", err)
		return nil
	}
	s.mu.Lock()
	s.presets = ps
	s.mu.Unlock()
	return ps
}

// Umschalten hands the card to preset for dauer, as person. A window of
// Homes is given back first — another person's, as one lamp switches over
// another's, or the person's own, whose end a take in the same window
// would keep; anyone else's (*modellease.Held) is not.
func (s *Schalter) Umschalten(ctx context.Context, person, preset string, dauer time.Duration) error {
	st := s.Lesen(ctx)
	switch {
	case !st.Erreichbar:
		return ErrWeg
	case person == "" || !slices.Contains(Dauern, dauer) || !slices.Contains(st.Presets, preset):
		return ErrNicht
	}
	halter := Halter(person)
	if _, homes := st.Homes(); st.Halter != "" && !homes {
		return &modellease.Held{Holder: st.Halter, ExpiresAt: max(st.Bis.Unix(), 0)}
	} else if homes {
		if err := s.Lease.Release(ctx, st.Halter); err != nil {
			return err
		}
		s.vergessen(st.Halter)
	}
	ende := s.now().Add(dauer)
	s.mu.Lock() // before the take: Pflegen gives back a homes- window it does not know
	if s.ende == nil {
		s.ende = map[string]time.Time{}
	}
	s.ende[halter] = ende
	s.mu.Unlock()
	if err := s.Lease.Take(ctx, halter, preset, dauer); err != nil {
		s.vergessen(halter)
		return err
	}
	log.Printf("homes: sprachmodell: %s took %s until %s", halter, preset, ende.Format("15:04"))
	return nil
}

// Zurueck gives a window of Homes' back at once; the household is none.
func (s *Schalter) Zurueck(ctx context.Context, person string) error {
	st := s.Lesen(ctx)
	if !st.Erreichbar {
		return ErrWeg
	}
	if _, homes := st.Homes(); !homes {
		if st.Halter == "" {
			return nil
		}
		return &modellease.Held{Holder: st.Halter, ExpiresAt: max(st.Bis.Unix(), 0)}
	}
	if err := s.Lease.Release(ctx, st.Halter); err != nil {
		return err
	}
	s.vergessen(st.Halter)
	log.Printf("homes: sprachmodell: %s gave back %s (%s)", Halter(person), st.Halter, st.Preset)
	return nil
}

func (s *Schalter) vergessen(halter string) {
	s.mu.Lock()
	delete(s.ende, halter)
	s.mu.Unlock()
}

// Pflegen is one look at the lease: a window Homes took is renewed while
// its end lies ahead and renew_after has passed, and given back once the
// end is reached; a window of homes-… this Homes did not take (a Homes
// before a restart) is given back.
func (s *Schalter) Pflegen(ctx context.Context) {
	l, err := s.Lease.Read(ctx)
	if err != nil || !strings.HasPrefix(l.Holder, Prefix) {
		return
	}
	now := s.now()
	s.mu.Lock()
	ende, mine := s.ende[l.Holder]
	s.mu.Unlock()
	switch {
	case !mine || !now.Before(ende):
		if err := s.Lease.Release(ctx, l.Holder); err != nil {
			log.Printf("homes: sprachmodell: giving %s back: %v", l.Holder, err)
			return
		}
		s.vergessen(l.Holder)
		log.Printf("homes: sprachmodell: %s given back, the household's again", l.Holder)
	case l.State == "ready" && l.LastRenewed != nil && l.RenewAfter != nil && !now.Before(time.Unix(*l.LastRenewed+*l.RenewAfter, 0)):
		if err := s.Lease.Take(ctx, l.Holder, l.Alias, ende.Sub(now)); err != nil {
			log.Printf("homes: sprachmodell: renewing %s: %v", l.Holder, err)
		}
	}
}

// Laufen looks at the lease every jede until ctx ends.
func (s *Schalter) Laufen(ctx context.Context, jede time.Duration) {
	for {
		s.Pflegen(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(jede):
		}
	}
}

// Name is a preset as the family reads it: gemma-4-12b → Gemma 4 12B.
func Name(preset string) string {
	parts := strings.Split(preset, "-")
	for i, p := range parts {
		switch {
		case i == 0:
			parts[i] = strings.ToUpper(p[:min(1, len(p))]) + p[min(1, len(p)):]
		case strings.ContainsAny(p, "0123456789"):
			parts[i] = strings.ToUpper(p)
		}
	}
	return strings.Join(parts, " ")
}

// Wahl is one preset a person may switch to, with a button per duration.
type Wahl struct {
	Preset string  `json:"preset"`
	Name   string  `json:"name"`
	Dauern []Dauer `json:"dauern"`
}

// Dauer is one duration's button and the question before it.
type Dauer struct {
	Stunden int    `json:"stunden"`
	Label   string `json:"label"`
	Bis     string `json:"bis"`
	Frage   string `json:"frage"`
	Ja      string `json:"ja"`
	Nein    string `json:"nein"`
}

// Ansicht is the card in words, the same on the page and in the app.
type Ansicht struct {
	Zustand string `json:"zustand"` // haushalt, anderes, laedt, belegt, weg
	Titel   string `json:"titel"`
	Hinweis string `json:"hinweis"`
	Zeile   string `json:"zeile"`   // the model, and for whom when it is not the household's
	Detail  string `json:"detail"`  // a quiet line: the voice control works, or the model loads
	Kosten  string `json:"kosten"`  // what the window costs, until when; "" in the household
	Zurueck string `json:"zurueck"` // the button that gives it back; "" when Homes may not
	Wahl    []Wahl `json:"wahl"`
}

// Sehen is st in lang's words, the times in loc.
func (st Stand) Sehen(lang string, now time.Time, loc *time.Location) Ansicht {
	t := func(id texte.ID) string { return texte.In(lang, id) }
	uhr := func(at time.Time) string {
		if at.IsZero() {
			return "…"
		}
		return at.In(loc).Format("15:04")
	}
	a := Ansicht{Titel: t(texte.SprachmodellTitel), Hinweis: t(texte.SprachmodellHinweis), Wahl: []Wahl{}}
	person, homes := st.Homes()
	switch {
	case !st.Erreichbar:
		a.Zustand, a.Zeile, a.Detail, a.Hinweis = "weg", t(texte.SprachmodellWeg), t(texte.SprachmodellWegWas), ""
		return a
	case st.Halter == "":
		a.Zustand, a.Zeile, a.Detail = "haushalt", fmt.Sprintf(t(texte.SprachmodellHaushalt), Name(st.Haushalt)), t(texte.SprachmodellLaeuft)
	default:
		a.Zustand, a.Zeile = "anderes", fmt.Sprintf(t(texte.SprachmodellFuer), Name(st.Preset), map[bool]string{true: person, false: st.Halter}[homes])
		if st.Laedt || st.Bis.IsZero() {
			a.Zustand, a.Detail = "laedt", t(texte.SprachmodellLaedt)
		} else {
			a.Kosten = fmt.Sprintf(t(texte.SprachmodellKosten), uhr(st.Bis))
		}
		if !homes {
			a.Zustand, a.Hinweis = "belegt", fmt.Sprintf(t(texte.SprachmodellBelegt), st.Halter, uhr(st.Bis))
			return a
		}
		a.Zurueck = t(texte.SprachmodellZurueck)
	}
	for _, p := range st.Presets {
		if p == st.Preset && st.Halter != "" {
			continue
		}
		w := Wahl{Preset: p, Name: Name(p)}
		for _, d := range Dauern {
			bis := uhr(now.Add(d))
			w.Dauern = append(w.Dauern, Dauer{Stunden: int(d.Hours()), Label: fmt.Sprintf(t(texte.SprachmodellStunden), int(d.Hours())), Bis: bis,
				Frage: fmt.Sprintf(t(texte.SprachmodellFrage), w.Name, bis), Ja: t(texte.SprachmodellJa), Nein: t(texte.SprachmodellNein)})
		}
		a.Wahl = append(a.Wahl, w)
	}
	return a
}

// Sagen is what a failed switch says to the person, in lang, a time in loc.
func Sagen(lang string, err error, loc *time.Location) string {
	var held *modellease.Held
	switch {
	case errors.As(err, &held):
		bis := "…"
		if held.ExpiresAt > 0 {
			bis = time.Unix(held.ExpiresAt, 0).In(loc).Format("15:04")
		}
		return fmt.Sprintf(texte.In(lang, texte.SprachmodellBelegt), held.Holder, bis)
	case errors.Is(err, ErrNicht):
		return texte.In(lang, texte.SprachmodellNicht)
	}
	return texte.In(lang, texte.SprachmodellFehler)
}
