package haushalt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/mcpserver"
)

// firmenkontakte and kontakt_entfernen (owner 2026-11-09): Dokumente's
// kontakte_aufraeumen.py makes the firm cards kontakt_ergaenzen made twice
// one. It reads them whole, carries their values with kontakt_ergaenzen and
// removes the rest; a card a person edited is listed as such and never removed.

type firmKarte struct {
	Kennung   string              `json:"kennung"`
	Name      string              `json:"name"`
	Org       string              `json:"org,omitempty"`
	Telefon   []string            `json:"telefon"`
	Email     []string            `json:"email"`
	IBAN      []string            `json:"iban"`
	Anschrift []map[string]string `json:"anschrift"`
	Notiz     string              `json:"notiz,omitempty"`
	VonHand   bool                `json:"von_hand"` // no longer as Homes wrote it: a phone or a person changed it
}

// firmenkontakte is every card of the category Firma: a German line, then one line of JSON.
func firmenkontakte(ctx context.Context, c *caldav.Client) (string, error) {
	all, partial, err := c.Cards(ctx)
	if err != nil {
		return "", err
	}
	out := []firmKarte{}
	for _, cd := range all {
		if !cd.IsFirm() {
			continue
		}
		k := firmKarte{Kennung: cd.UID, Name: cd.Name, Org: cd.Org, Telefon: cd.Phones, Email: cd.Emails, IBAN: cardIBANs(cd.Contact),
			Anschrift: []map[string]string{}, Notiz: cd.Note, VonHand: !cd.AsWritten()}
		for _, a := range cd.Postal {
			k.Anschrift = append(k.Anschrift, map[string]string{"strasse": a.Street, "plz": a.Code, "ort": a.Locality, "land": a.Country})
		}
		out = append(out, k)
	}
	b, _ := json.Marshal(map[string]any{"kontakte": out, "teilweise": partial})
	return fmt.Sprintf("%d Firmenkontakte.\n%s", len(out), b), nil
}

// kontaktEntfernen removes one firm card Homes made and nobody changed since.
func kontaktEntfernen(ctx context.Context, c *caldav.Client, a map[string]any) (string, error) {
	uid := oneLine(mcpserver.Str(a, "kennung"))
	if uid == "" {
		return "", errors.New("kennung fehlt")
	}
	all, _, err := c.Cards(ctx)
	if err != nil {
		return "", err
	}
	for _, cd := range all {
		if oneLine(cd.UID) != uid {
			continue
		}
		switch {
		case !cd.IsFirm():
			return "", errors.New(notOurs)
		case !cd.AsWritten():
			return "", errors.New("der Kontakt wurde von Hand bearbeitet; Homes entfernt ihn nicht")
		}
		if err := c.RemoveCard(ctx, cd); errors.Is(err, caldav.ErrChanged) {
			return "", errors.New("der Kontakt wurde gerade woanders geändert; nicht entfernt")
		} else if err != nil {
			return "", err
		}
		b, _ := json.Marshal(map[string]string{"status": "entfernt", "kontakt": cd.UID})
		return "Kontakt „" + cut(orDash(cd.Name), MaxField) + "“ entfernt.\n" + string(b), nil
	}
	return "", errors.New("kein Kontakt mit dieser Kennung")
}
