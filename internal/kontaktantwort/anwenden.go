package kontaktantwort

import (
	"cmp"
	"context"
	"errors"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/haushalt"
)

// Stand is the review's answer per change: taken (alle, or the numbers in
// nimm) or not. A removal and the message for the note are taken only
// when ticked, never by alle (owner 2026-11-08).
func Stand(f Frage, alle bool, nimm []int) []string {
	out := make([]string, len(f.Aenderungen))
	for i, a := range f.Aenderungen {
		out[i] = Abgelehnt
		if alle && !a.NurAngekreuzt() {
			out[i] = Angenommen
		}
	}
	for _, i := range nimm {
		if i >= 0 && i < len(out) {
			out[i] = Angenommen
		}
	}
	return out
}

// Uebernehmen writes the taken changes (stand) of a Haushalts-Kontakt
// into its card in Radicale, over the version it reads: each one written
// is Uebernommen, one whose field had changed meanwhile Uebersprungen.
// A card gone meanwhile is ErrWeg, with every taken change Uebersprungen.
// A Handy-Kontakt's are left Angenommen for the phone. A number is written
// in the household's form (haushalt.Telefon; land is the house's calling code).
func Uebernehmen(ctx context.Context, dav *caldav.Client, f Frage, stand []string, land string) ([]string, error) {
	if f.Buch != Haushalt {
		return stand, nil
	}
	f.Aenderungen = append([]Aenderung(nil), f.Aenderungen...)
	for i := range f.Aenderungen {
		f.Aenderungen[i].Stand = stand[i]
	}
	ks, nr := Korrekturen(f)
	if len(ks) == 0 {
		return stand, nil
	}
	for i, k := range ks {
		if k.Prop == "TEL" {
			ks[i].Neu, _ = haushalt.Telefon(k.Neu, cmp.Or(land, haushalt.DefaultCountry))
		}
	}
	out := append([]string(nil), stand...)
	done, err := dav.CorrectShared(ctx, f.UID, ks)
	if errors.Is(err, caldav.ErrNoCard) {
		for _, i := range nr {
			out[i] = Uebersprungen
		}
		return out, ErrWeg
	}
	if err != nil {
		return nil, err
	}
	for j, i := range nr {
		out[i] = Uebersprungen
		if done[j] {
			out[i] = Uebernommen
		}
	}
	return out, nil
}
