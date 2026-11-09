package gesichter

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/immich"
	"github.com/mdopp/homes/internal/namen"
)

// kontakt is one contact as Immich gets it; foto its picture, a JPEG (the
// owner's own Handy-Kontakte only); given, family and rev its N and REV.
type kontakt struct {
	key, name, geburtstag string
	foto                  []byte
	given, family, rev    string
}

// kontakte are the Haushalts-Kontakte and the key owner's own
// Handy-Kontakte (once Merke knows the owner's login), firms aside; ok:
// both were read whole, so a contact not among them is gone.
func (g *Gesichter) kontakte(ctx context.Context) (out []kontakt, ok bool, err error) {
	all, partial, err := g.Kontakte.Contacts(ctx)
	if err != nil {
		return nil, false, err
	}
	add := func(prefix string, cts []caldav.Contact) {
		for _, ct := range cts {
			if name := strings.Join(strings.Fields(ct.Name), " "); name != "" && ct.UID != "" && !ct.IsFirm() {
				out = append(out, kontakt{prefix + ct.UID, name, geburtstag(ct.Birthday, g.now().Format("2006-01-02")), nil, ct.Given, ct.Family, ct.Rev})
			}
		}
	}
	add("haushalt/", all)
	if login := g.Wessen(); login != "" {
		cards, _, err := g.Kontakte.BookCards(ctx, "/"+url.PathEscape(login)+"/handy-kontakte/")
		if err != nil {
			return nil, false, err
		}
		for _, cd := range cards {
			if root, err := caldav.Parse(cd.Data); err == nil {
				n, fotos := len(out), bilder(root)
				add("privat/", caldav.Contacts(root))
				for i := n; i < len(out); i++ {
					out[i].foto = fotos[strings.TrimPrefix(out[i].key, "privat/")]
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b kontakt) int { return strings.Compare(a.key, b.key) })
	return out, !partial, nil
}

// geburtstag is a birthday as Immich takes it: a whole date, not after
// heute; "" otherwise.
func geburtstag(b, heute string) string {
	if strings.HasPrefix(b, "--") || !caldav.ValidBirthday(b) || b > heute {
		return ""
	}
	return b
}

// Abgleichen gives every contact an Immich person of its name, at most
// maxAufrufe calls a run (the rest in the next): the one it has (a person
// Homes made follows the contact's name and birthday), else one Immich
// has of that name, else a new one; a Haushalts-Kontakt first takes the
// name its person got in Immich (folgen). Nothing else in Immich changes;
// a contact gone loses only its line in Zuordnung.
func (g *Gesichter) Abgleichen(ctx context.Context) error {
	if g == nil || g.Immich == nil || g.Kontakte == nil {
		return nil
	}
	ks, ganz, err := g.kontakte(ctx)
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.laden()
	alt, seit, personen := g.stand.Zuordnung, g.zug, map[string]immich.Person{}
	for _, p := range g.stand.Personen {
		personen[p.ID] = p
	}
	g.mu.Unlock()
	neu, aufrufe := map[string]zuordnung{}, 0
	for _, k := range ks {
		z := alt[k.key]
		if aufrufe < maxAufrufe && err == nil {
			var n, m int
			if k, z, n, err = g.folgen(ctx, k, z, personen[z.ID]); err == nil {
				z, m, err = g.zuordnen(ctx, k, z, neu)
			}
			aufrufe += n + m
		}
		if z.ID != "" {
			neu[k.key] = z
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for key, z := range alt {
		if _, da := neu[key]; !da && (!ganz || err != nil) { // not read, or the run broke off: kept
			neu[key] = z
		}
	}
	var fehlt immich.FehltRecht
	errors.As(err, &fehlt)
	nachziehen(g, neu, g.stand.Zuordnung, seit)
	g.stand.Zuordnung, g.stand.Fehlt = neu, fehlt.Recht
	if serr := g.save(); err == nil {
		err = serr
	}
	return err
}

// folgen gives a Haushalts-Kontakt k the name its person p got in Immich
// (owner 2026-11-09, „Automatisch zurück“): when p's name is not the one
// Homes last saw (z.Immich) and k's is still the one it last had (z.Name),
// or both changed and Immich's is the later (updatedAt over the card's
// REV). N and FN are written as an owner's correction is (CorrectShared),
// and z notes the name on both sides, so nothing goes back. A card changed
// meanwhile waits for the next run. A Handy-Kontakt is the phone's: Homes
// writes none (laib ADR 0029). n is the calls it took.
func (g *Gesichter) folgen(ctx context.Context, k kontakt, z zuordnung, p immich.Person) (_ kontakt, _ zuordnung, n int, err error) {
	neu := strings.Join(strings.Fields(p.Name), " ")
	if neu == "" || p.ID != z.ID {
		return k, z, 0, nil
	}
	if z.Immich == "" || namen.Plain(neu) == namen.Plain(z.Immich) {
		z.Immich = neu
		return k, z, 0, nil
	}
	uid, haushalt := strings.CutPrefix(k.key, "haushalt/")
	if !haushalt || (k.name != z.Name && !zeit(p.Stand).After(zeit(k.rev))) {
		return k, z, 0, nil
	}
	family, given := k.family, k.given
	if family+given == "" {
		family, given = nameTeile(k.name)
	}
	nf, ng := nameTeile(neu)
	done, err := g.Kontakte.CorrectShared(ctx, uid, []caldav.Korrektur{{Prop: "N-FAMILY", Alt: family, Neu: nf}, {Prop: "N-GIVEN", Alt: given, Neu: ng}})
	if err != nil || slices.Contains(done, false) {
		return k, z, 1, err
	}
	k.name, z.Name, z.Immich = neu, neu, neu
	return k, z, 1, nil
}

// nameTeile are a name's family and given names as a card's N has them
// when only FN was written: the last word the family name.
func nameTeile(name string) (family, given string) {
	ws := strings.Fields(name)
	if len(ws) < 2 {
		return name, ""
	}
	return ws[len(ws)-1], strings.Join(ws[:len(ws)-1], " ")
}

// zeit is a REV or an updatedAt as a time; zero unknown.
func zeit(s string) time.Time {
	for _, f := range []string{time.RFC3339Nano, "20060102T150405Z", "2006-01-02"} {
		if t, err := time.Parse(f, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}

// zuordnen is k's person and how many calls it took: z kept or followed,
// else one of k's name made in this run or in Immich, else a new one.
func (g *Gesichter) zuordnen(ctx context.Context, k kontakt, z zuordnung, neu map[string]zuordnung) (zuordnung, int, error) {
	n := 0
	switch {
	case z.ID != "" && z.Eigen && (z.Name != k.name || z.Geburtstag != k.geburtstag):
		n++
		err := g.Immich.Aendern(ctx, z.ID, k.name, k.geburtstag)
		var a immich.Antwort
		if err == nil {
			return zuordnung{z.ID, k.name, k.geburtstag, true, k.name}, n, nil
		}
		if !errors.As(err, &a) || a.Code != 400 { // 400: merged away or deleted in Immich
			return z, n, err
		}
	case z.ID != "" && (z.Eigen || namen.Plain(z.Name) == namen.Plain(k.name)):
		return z, n, nil
	}
	for _, d := range neu {
		if namen.Plain(d.Name) == namen.Plain(k.name) {
			return zuordnung{ID: d.ID, Name: k.name}, n, nil
		}
	}
	n++
	ps, err := g.Immich.Suchen(ctx, k.name)
	if err != nil {
		return zuordnung{}, n, err
	}
	for _, p := range ps {
		if namen.Plain(p.Name) == namen.Plain(k.name) {
			return zuordnung{ID: p.ID, Name: k.name}, n, nil
		}
	}
	n++
	p, err := g.Immich.Anlegen(ctx, k.name, k.geburtstag)
	return zuordnung{p.ID, k.name, k.geburtstag, true, k.name}, n, err
}
