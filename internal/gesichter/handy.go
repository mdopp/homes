package gesichter

import (
	"context"
	"errors"
	"strings"

	"github.com/mdopp/homes/internal/immich"
	"github.com/mdopp/homes/internal/namen"
)

// The key owner's own phone contacts follow Immich (owner 2026-11-09,
// „App erweitern“): their address book in Radicale is the phone's mirror
// (laib ADR 0029), so Homes does not write it but asks the phone, through
// the corrections the Homes app already writes (Handy, kept by
// kontaktantwort.Store.Handy, GET /napi/kontakte/korrekturen): a contact
// renamed when its person is (folgen), and a new contact for a person newly
// named in Immich that no contact has (Neuanlegen). The phone's next
// upload brings each back, and the hourly run links it like any contact.

const maxNeu = 20 // new contacts one run asks for

// HandyAenderung is one change for the owner's phone: UID "" a new
// contact of Vorname and Nachname with the face Bild (a JPEG; nil none);
// else the contact's name from AltVorname, AltNachname to the new one.
// With Mit (owner 2026-11-09, netz) a relation instead: on the contact
// UID (named AltVorname) Mit is its Art, a kind of netz.Arten ("" none).
type HandyAenderung struct {
	UID                     string
	AltVorname, AltNachname string
	Vorname, Nachname       string
	Bild                    []byte
	Mit, Art                string
}

// Neuanlegen asks the owner's phone for a contact of each person named in
// Immich since Homes first looked (Bekannt) that no contact has, by person
// or by name: its name and its face, in the owner's own contacts, never
// the household's; each person once, at most maxNeu a run. Without the
// owner's login or Handy it asks nothing.
func (g *Gesichter) Neuanlegen(ctx context.Context) error {
	wer := g.Wessen()
	if wer == "" || g.Handy == nil {
		return nil
	}
	g.mu.Lock()
	st := &g.stand
	erst, hat := st.Bekannt == nil, map[string]bool{}
	if erst {
		st.Bekannt = map[string]bool{}
	}
	for _, z := range st.Zuordnung {
		hat[z.ID], hat[namen.Plain(z.Name)] = true, true
	}
	var neu []immich.Person
	for _, p := range st.Personen {
		switch {
		case st.Bekannt[p.ID]:
		case erst || hat[p.ID] || hat[namen.Plain(p.Name)] || g.weg[p.ID]:
			st.Bekannt[p.ID] = true
		case len(neu) < maxNeu:
			neu = append(neu, p)
		}
	}
	g.mu.Unlock()
	var err error
	for _, p := range neu {
		bild, ferr := g.Immich.Gesicht(ctx, p.ID) // 250 px; none: without
		if !Gesehen(bild) || ferr != nil {
			bild = nil
		}
		nachname, vorname := nameTeile(strings.Join(strings.Fields(p.Name), " "))
		if vorname == "" { // one word: a first name on a phone
			vorname, nachname = nachname, ""
		}
		if err = g.Handy(wer, HandyAenderung{Vorname: vorname, Nachname: nachname, Bild: bild}); err != nil {
			break
		}
		g.mu.Lock()
		g.stand.Bekannt[p.ID] = true
		g.mu.Unlock()
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return errors.Join(err, g.save())
}
