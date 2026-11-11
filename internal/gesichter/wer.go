package gesichter

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/immich"
	"github.com/mdopp/homes/internal/namen"
)

// „Wer ist das?“ (owner 2026-11-09): up to werN unnamed persons for the
// key's owner to name, beside the pairs. The hourly run keeps the first
// werKandidaten unnamed ones in Immich's order, the most faces first,
// and those a Haushalts-Kontakt's photo suggests a name for (Auffrischen), with their faces, counts their photos (Vorschlagen) and
// reads their places and company once a day (Umfelder); a page only reads
// what is kept. A name goes to Immich (Benennen); one of a person's name
// already is merged into it by the next run (Zusammenfuehren). „Kenne ich
// nicht“ is kept, and that face is never offered again. The unnamed face
// of a pair the owner said „Nein“ to is named the same way, right there
// (Dann, owner 2026-11-11); the named side's own name for it is a „Ja“
// after all.

const (
	werN, werMinFotos = 5, 5 // persons offered; the photos one is in at least (owner 2026-11-09: no stranger in the back)
	werKandidaten     = 15   // unnamed persons kept to choose from
)

// Wer are the unnamed persons to name, at most werN: those with a name
// suggested (Seite.Vorschlag) first, then in the most photos, then most
// often beside named persons, then at the most recurring places; none
// without a face, nor in fewer than werMinFotos photos but with a name
// suggested, nor one the owner does not know.
func (g *Gesichter) Wer() []Seite {
	if g == nil || g.Immich == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.laden()
	st := g.stand
	vor := st.vorschlaege()
	var out []Seite
	for _, p := range st.Ohne {
		_, nie := st.Unbekannt[p.ID]
		s := st.seite(p.ID, "", st.fotos(p.ID), st.Bilder[p.ID].JPEG)
		if s.Vorschlag = vor[p.ID]; !nie && !g.weg[p.ID] && (s.Fotos >= werMinFotos || s.Vorschlag != "") && s.zeigbar() {
			out = append(out, s)
		}
	}
	slices.SortStableFunc(out, func(a, b Seite) int {
		ua, ub := st.Umfeld[a.ID], st.Umfeld[b.ID]
		return cmp.Or(cmp.Compare(btoi(b.Vorschlag != ""), btoi(a.Vorschlag != "")), cmp.Compare(b.Fotos, a.Fotos), cmp.Compare(ub.MitFotos, ua.MitFotos), cmp.Compare(ub.Wieder, ua.Wieder))
	})
	return out[:min(len(out), werN)]
}

// Dann is the unnamed face id of a pair the owner said „Nein“ to, to name
// right there (owner 2026-11-11: „bei Nein gleich den richtigen Namen“);
// ok false: none such, or it is named, not known or merged away.
func (g *Gesichter) Dann(id string) (Seite, bool) {
	if g == nil || g.Immich == nil {
		return Seite{}, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.laden()
	st := g.stand
	p, da := st.Andere[id]
	_, nie := st.Unbekannt[id]
	if !da || nie || g.weg[id] || strings.TrimSpace(p.Name) != "" || len(st.neinMit(id)) == 0 || slices.ContainsFunc(st.Personen, func(q immich.Person) bool { return q.ID == id }) {
		return Seite{}, false
	}
	s := st.seite(id, "", st.fotos(id), st.Bilder[id].JPEG)
	s.Vorschlag = st.vorschlaege()[id]
	return s, true
}

// fotos are the family's photos kept of person id, Homes' own uploads
// left out (eigene); -1 not counted.
func (st stand) fotos(id string) int {
	if z, ok := st.Fotos[id]; ok {
		return max(z.N-st.eigene(id), 0)
	}
	return -1
}

// neinMit are the persons the owner said „Nein“ to beside id.
func (st stand) neinMit(id string) []string {
	var out []string
	for k := range st.Nein {
		if a, b, _ := strings.Cut(k, "|"); a == id {
			out = append(out, b)
		} else if b == id {
			out = append(out, a)
		}
	}
	return out
}

// Benennen takes the owner's answer about the unnamed person id (of Wer
// or Dann): a name is set in Immich (PUT people, person.update) and
// answered; "" is „Kenne ich nicht“, kept. The name of a person the owner
// said „Nein“ to beside id merges id into it instead (zusammen) and
// answers doch. ErrKeineFrage: id is not offered (any more).
func (g *Gesichter) Benennen(ctx context.Context, id, name string) (genannt string, doch bool, err error) {
	_, dann := g.Dann(id)
	gezeigt := dann || slices.ContainsFunc(g.Wer(), func(s Seite) bool { return s.ID == id })
	name = strings.Join(strings.Fields(name), " ")
	g.mu.Lock()
	p, links := g.stand.Andere[id], immich.Person{}
	if i := slices.IndexFunc(g.stand.Ohne, func(p immich.Person) bool { return p.ID == id }); i >= 0 {
		p = g.stand.Ohne[i]
	}
	for _, q := range g.stand.Personen {
		if name != "" && namen.Plain(q.Name) == namen.Plain(name) && slices.Contains(g.stand.neinMit(id), q.ID) {
			links = q
		}
	}
	g.mu.Unlock()
	if !gezeigt || p.ID == "" {
		return "", false, ErrKeineFrage
	}
	if links.ID != "" {
		return links.Name, true, g.zusammen(ctx, links.ID, id)
	}
	if name != "" {
		if err := g.Immich.Aendern(ctx, id, name, p.Geburtstag); err != nil {
			return "", false, err
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
	return name, false, g.save()
}
