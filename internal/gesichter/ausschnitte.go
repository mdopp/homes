package gesichter

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"slices"

	"github.com/mdopp/homes/internal/immich"
)

// Three small faces under each side of a pair on „Dieselbe Person?“
// (owner 2026-11-11): Immich's own face of a person is one crop it chose,
// sometimes only a shadow. Homes cuts the person's face from up to three
// other photos itself, in the daily run of Umfelder (a page never asks
// Immich), from the photo's preview and the box Immich found the face in
// (GET /api/faces, face.read; the preview needs asset.view). A face kept
// is cut again only when its photo is no longer among the person's newest.

const (
	ausschnitte     = 3   // the small faces per person
	ausschnittKante = 160 // their longest side, in pixels
	ausschnittFotos = 8   // the photos looked at per person, the fewest people in them first
)

// Ausschnitt is a person's face cut from one photo.
type Ausschnitt struct {
	Asset string `json:"asset"` // the photo, to open it in Immich
	JPEG  []byte `json:"jpeg"`
}

// ausschneiden are up to ausschnitte faces of person id, each from another
// of its photos: those kept first, while their photo is still among
// fotos, then the photos with the fewest people (their faces are larger).
// A face smaller than a twentieth of its photo's width, or one schlecht
// says little, is none; an Immich that refuses ends it with what it has.
func (g *Gesichter) ausschneiden(ctx context.Context, id string, fotos []immich.Foto, alt []Ausschnitt) []Ausschnitt {
	var out []Ausschnitt
	for _, a := range alt {
		if slices.ContainsFunc(fotos, func(f immich.Foto) bool { return f.ID == a.Asset }) && len(out) < ausschnitte {
			out = append(out, a)
		}
	}
	fotos = slices.Clone(fotos)
	slices.SortStableFunc(fotos, func(a, b immich.Foto) int { return cmp.Compare(len(a.Personen), len(b.Personen)) })
	for _, f := range fotos[:min(len(fotos), ausschnittFotos)] {
		if len(out) >= ausschnitte || slices.ContainsFunc(out, func(a Ausschnitt) bool { return a.Asset == f.ID }) {
			continue
		}
		funde, err := g.Immich.Funde(ctx, f.ID)
		i := slices.IndexFunc(funde, func(f immich.Fund) bool { return f.Person != nil && f.Person.ID == id })
		if err != nil {
			break
		}
		if i < 0 || funde[i].Breite <= 0 || (funde[i].X2-funde[i].X1)*20 < funde[i].Breite {
			continue
		}
		roh, err := g.Immich.Vorschau(ctx, f.ID)
		if err != nil {
			break
		}
		if b, err := zuschneiden(roh, funde[i].Rahmen, ausschnittKante); err == nil && !schlecht(b) {
			out = append(out, Ausschnitt{f.ID, b})
		}
	}
	return out
}

// zuschneiden is the face at r in the photo b (a preview of any size: r is
// in Immich's pixels of the photo), a square half again the face's size
// around its middle, kept inside the photo, made no larger than kante.
func zuschneiden(b []byte, r immich.Rahmen, kante int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(b))
	sub, ok := src.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if err != nil || !ok || r.Breite <= 0 || r.Hoehe <= 0 {
		return nil, errors.Join(err, errors.New("gesichter: no photo or no box to cut"))
	}
	sb := src.Bounds()
	sx, sy := float64(sb.Dx())/float64(r.Breite), float64(sb.Dy())/float64(r.Hoehe)
	seite := int(min(max(float64(r.X2-r.X1)*sx, float64(r.Y2-r.Y1)*sy)*1.5, float64(min(sb.Dx(), sb.Dy()))))
	x := min(max(int(float64(r.X1+r.X2)/2*sx)-seite/2, 0), sb.Dx()-seite)
	y := min(max(int(float64(r.Y1+r.Y2)/2*sy)-seite/2, 0), sb.Dy()-seite)
	if seite < 1 {
		return nil, errors.New("gesichter: an empty box")
	}
	return verkleinern(sub.SubImage(image.Rect(x, y, x+seite, y+seite).Add(sb.Min)), kante)
}

// schlecht: the face b says little, so a cut one is shown big in its place
// (owner 2026-11-11): nearly black (its mean brightness under 40 of 255),
// or nearly one colour (the brightness varies by less than 12), or no
// JPEG at all. It does not see a crop of the wrong thing, a blurred one or
// a face turned away.
func schlecht(b []byte) bool {
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		return true
	}
	var s, q, n float64
	r := img.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			l := float64(color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y)
			s, q, n = s+l, q+l*l, n+1
		}
	}
	m := s / n
	return m < 40 || q/n-m*m < 12*12
}
