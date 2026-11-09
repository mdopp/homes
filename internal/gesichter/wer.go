package gesichter

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/immich"
)

// „Wer ist das?“ (owner 2026-11-09): up to werN unnamed persons for the
// key's owner to name, beside the pairs. The hourly run keeps the first
// werKandidaten unnamed ones in Immich's order, the most faces first
// (Auffrischen), with their faces, counts their photos (Vorschlagen) and
// reads their places and company once a day (Umfelder); a page only reads
// what is kept. A name goes to Immich (Benennen); one of a person's name
// already is merged into it by the next run (Zusammenfuehren). „Kenne ich
// nicht“ is kept, and that face is never offered again.

const (
	werN, werMinFotos = 5, 5 // persons offered; the photos one is in at least (owner 2026-11-09: no stranger in the back)
	werKandidaten     = 15   // unnamed persons kept to choose from
)

// Wer are the unnamed persons to name, at most werN: in the most photos
// first, then most often beside named persons, then at the most recurring
// places; none without a face or in fewer than werMinFotos photos, nor one the owner does not know.
func (g *Gesichter) Wer() []Seite {
	if g == nil || g.Immich == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.laden()
	st := g.stand
	var out []Seite
	for _, p := range st.Ohne {
		_, nie := st.Unbekannt[p.ID]
		n := -1
		if z, ok := st.Fotos[p.ID]; ok {
			n = z.N
		}
		if s := st.seite(p.ID, "", n, st.Bilder[p.ID].JPEG); !nie && !g.weg[p.ID] && s.Fotos >= werMinFotos && s.zeigbar() {
			out = append(out, s)
		}
	}
	slices.SortStableFunc(out, func(a, b Seite) int {
		ua, ub := st.Umfeld[a.ID], st.Umfeld[b.ID]
		return cmp.Or(cmp.Compare(b.Fotos, a.Fotos), cmp.Compare(ub.MitFotos, ua.MitFotos), cmp.Compare(ub.Wieder, ua.Wieder))
	})
	return out[:min(len(out), werN)]
}

// Benennen takes the owner's answer about the unnamed person id: a name
// is set in Immich (PUT people, person.update) and answered; "" is „Kenne
// ich nicht“, kept. ErrKeineFrage: id is not offered (any more).
func (g *Gesichter) Benennen(ctx context.Context, id, name string) (string, error) {
	gezeigt := slices.ContainsFunc(g.Wer(), func(s Seite) bool { return s.ID == id })
	name = strings.Join(strings.Fields(name), " ")
	g.mu.Lock()
	var p immich.Person
	if i := slices.IndexFunc(g.stand.Ohne, func(p immich.Person) bool { return p.ID == id }); i >= 0 {
		p = g.stand.Ohne[i]
	}
	g.mu.Unlock()
	if !gezeigt || p.ID == "" {
		return "", ErrKeineFrage
	}
	if name != "" {
		if err := g.Immich.Aendern(ctx, id, name, p.Geburtstag); err != nil {
			return "", err
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stand.Ohne = slices.DeleteFunc(g.stand.Ohne, func(q immich.Person) bool { return q.ID == id })
	if p.Name = name; name == "" {
		g.stand.Unbekannt[id] = g.now()
	} else {
		g.stand.Personen = append(g.stand.Personen, p)
	}
	return name, g.save()
}
