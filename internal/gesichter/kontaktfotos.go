package gesichter

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/immich"
	"github.com/mdopp/homes/internal/namen"
)

// The household's contact photos help Immich name faces (owner
// 2026-11-11, homes#352; the owner accepted that faces of people outside
// the family then live in Immich).
//
// The hourly run (Kontaktfotos) puts the PHOTO of each Haushalts-Kontakt
// into Immich once, by its hash, archived (out of the timeline, its face
// found all the same) and into one album Homes keeps, fotoAlbum. Immich's
// own face detection runs on it; Homes runs none. A face of an archived
// photo joins a person Immich has, it never starts one (immich.Hochladen).
// When that person has no name, „Wer ist das?“ offers the contact's name
// first, and the owner confirms it with a tap (Vorschlaege); nothing is
// named without that tap. A changed photo replaces its upload, a photo or
// contact gone deletes it. Only the household's own address books are
// read, never a person's phone book or archive; a picture Homes wrote
// onto a card itself (bilder) and a firm's logo are none.

const (
	fotoAlbum = "Homes · Kontaktfotos"
	fotoText  = "Fotos aus den Haushalts-Kontakten, damit Immich Gesichter Namen zuordnen kann. Homes pflegt dieses Album; es erscheint nicht in der Zeitleiste."
	fotoDatei = immich.EigenesFoto + "foto-" // + the hash + .jpg
)

// The stands of a contact photo without an upload of Homes'.
const (
	fotoDoppelt = "doppelt" // Immich had these bytes already: not Homes' photo
	fotoWeg     = "weg"     // deleted in Immich: not put there again
)

// kontaktfoto is a household contact's photo in Immich.
type kontaktfoto struct {
	Asset  string    `json:"asset,omitempty"`  // Homes' upload; "" none
	Stand  string    `json:"stand,omitempty"`  // fotoDoppelt, fotoWeg; "" uploaded
	Seit   time.Time `json:"seit"`             // uploaded
	Person string    `json:"person,omitempty"` // its one face's person; "" none (yet), or more faces
	Namen  []string  `json:"namen,omitempty"`  // the contacts that carry it
	Album  bool      `json:"album,omitempty"`  // in fotoAlbum
}

// haushaltsFotos are the JPEG photos of the Haushalts-Kontakte by hash,
// with the names of the contacts that carry each; ganz: every card was
// read, so a photo not among them is gone.
func (g *Gesichter) haushaltsFotos(ctx context.Context) (fotos map[string][]byte, wer map[string][]string, ganz bool, err error) {
	cards, partial, err := g.Kontakte.Cards(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	fotos, wer = map[string][]byte{}, map[string][]string{}
	for _, cd := range cards {
		name := strings.Join(strings.Fields(cd.Name), " ")
		root, err := caldav.Parse(cd.Text())
		if name == "" || cd.IsFirm() || err != nil {
			continue
		}
		for _, foto := range bilder(root) {
			h := bildHash(foto)
			fotos[h] = foto
			if !slices.Contains(wer[h], name) {
				wer[h] = append(wer[h], name)
			}
		}
	}
	return fotos, wer, !partial, nil
}

// Kontaktfotos brings the Haushalts-Kontakte's photos to Immich and reads
// whose face each is (the file's comment), at most maxHochladen uploads a
// run. Without the key's immich.FotoRechte it does nothing, and Fehlt
// says so.
func (g *Gesichter) Kontaktfotos(ctx context.Context) error {
	if g == nil || g.Immich == nil || g.Kontakte == nil {
		return nil
	}
	g.mu.Lock()
	g.laden()
	darf := !slices.ContainsFunc(immich.FotoRechte, func(r string) bool { return !g.darf(r) })
	alt, album := maps.Clone(g.stand.Kontaktfotos), g.stand.Album
	g.mu.Unlock()
	if !darf {
		return nil
	}
	fotos, wer, ganz, err := g.haushaltsFotos(ctx)
	if err != nil {
		return err
	}
	album, leer, err := g.Immich.Album(ctx, album, fotoAlbum, fotoText)
	if err != nil {
		return err
	}
	neu, hoch := map[string]kontaktfoto{}, 0
	for _, h := range slices.Sorted(maps.Keys(fotos)) {
		f, da := alt[h]
		delete(alt, h)
		switch {
		case err != nil:
		case !da && hoch < maxHochladen:
			hoch++
			f, err = g.fotoHochladen(ctx, h, fotos[h])
			da = err == nil
		case da && f.Asset != "":
			f, err = g.fotoPruefen(ctx, f)
		}
		if da {
			f.Namen, f.Album = wer[h], f.Album && !leer
			neu[h] = f
		}
	}
	for h, f := range alt { // on no card any more: its upload goes
		if ganz && err == nil && f.Asset != "" {
			err = g.Immich.Entfernen(ctx, f.Asset)
		}
		if !ganz || err != nil {
			neu[h] = f
		}
	}
	if err == nil {
		err = g.insAlbum(ctx, album, neu)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stand.Kontaktfotos, g.stand.Album = neu, album
	return errors.Join(err, g.save())
}

// fotoHochladen uploads the photo foto of hash h, archived.
func (g *Gesichter) fotoHochladen(ctx context.Context, h string, foto []byte) (kontaktfoto, error) {
	id, doppelt, err := g.Immich.Hochladen(ctx, fotoDatei+h+".jpg", foto, g.now())
	if doppelt {
		return kontaktfoto{Stand: fotoDoppelt, Seit: g.now()}, err
	}
	return kontaktfoto{Asset: id, Seit: g.now()}, err
}

// fotoPruefen reads whose face Immich found in f's upload: the person of
// its one face, none for none or more.
func (g *Gesichter) fotoPruefen(ctx context.Context, f kontaktfoto) (kontaktfoto, error) {
	funde, err := g.Immich.Funde(ctx, f.Asset)
	var a immich.Antwort
	if errors.As(err, &a) && a.Code == 400 { // deleted in Immich meanwhile: not put there again
		return kontaktfoto{Stand: fotoWeg, Seit: f.Seit}, nil
	}
	if err != nil {
		return f, err
	}
	f.Person = ""
	if len(funde) == 1 && funde[0].Person != nil {
		f.Person = funde[0].Person.ID
	}
	return f, nil
}

// insAlbum puts the uploads not in album yet into it.
func (g *Gesichter) insAlbum(ctx context.Context, album string, fotos map[string]kontaktfoto) error {
	var ids []string
	for _, f := range fotos {
		if f.Asset != "" && !f.Album {
			ids = append(ids, f.Asset)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if err := g.Immich.InsAlbum(ctx, album, ids); err != nil {
		return err
	}
	for h, f := range fotos {
		f.Album = f.Asset != ""
		fotos[h] = f
	}
	return nil
}

// vorschlaege are, by person, the one Haushalts-Kontakt whose photo's face
// Immich put into it; a person the photos of two names point at has none.
// Only an unnamed person's is offered (Wer, Dann).
func (st stand) vorschlaege() map[string]string {
	out, zwei := map[string]string{}, map[string]bool{}
	for _, f := range st.Kontaktfotos {
		for _, n := range f.Namen {
			if v, da := out[f.Person]; f.Person == "" || f.Asset == "" || zwei[f.Person] {
				continue
			} else if da && namen.Plain(v) != namen.Plain(n) {
				zwei[f.Person] = true
				delete(out, f.Person)
			} else {
				out[f.Person] = n
			}
		}
	}
	return out
}

// eigene are the pictures Homes uploaded whose face is in person id: they
// are no photos of the family's, so a count leaves them out.
func (st stand) eigene(id string) int {
	n := 0
	for _, f := range st.Kontaktfotos {
		n += btoi(f.Asset != "" && f.Person == id)
	}
	for _, b := range st.Kontaktbilder {
		n += btoi(b.Asset != "" && b.Person == id)
	}
	return n
}
