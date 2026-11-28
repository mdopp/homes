package kontaktantwort

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/caldav"
)

// Kontaktdaten eintragen lassen (owner 2026-11-28, „Wie wir es schon für
// die Verbesserung von Kontaktdaten haben“): the same link, page, store and
// review as „Kontakt prüfen lassen“, for a person the household has no card
// of yet. The link shows the form empty and works for FristNeu; the answer
// waits for an admin's review like any other, before → after with nothing
// before. The review writes it as a new card into the household's book
// (NeuAnlegen) or, when a household card shares a number or an e-mail
// address with it (Treffer), adds it to that card (Ergaenzen), before →
// after as against that card. Nothing is written without that review.

// FristNeu is how long a new person's link works.
const FristNeu = 7 * 24 * time.Hour

// Einladen makes a new person's link, started by wer (von their first name, as Link): the
// address basis/<token> and its Frage. Several may be open at once.
func (s *Store) Einladen(wer, von, basis string) (string, Frage, error) {
	token, f, err := s.neu(Frage{Wer: wer, Von: s.von(wer, von), Buch: Haushalt, Neu: true}, FristNeu)
	return strings.TrimRight(basis, "/") + "/" + token, f, err
}

// Eingetragen are the fields a new person's answer holds, as its changes say them.
func (f Frage) Eingetragen() Felder {
	var n Felder
	for _, a := range f.Aenderungen {
		switch a.Art {
		case "vorname":
			n.Vorname = a.Nachher
		case "nachname":
			n.Nachname = a.Nachher
		case "telefon":
			n.Telefone = append(n.Telefone, Telefon{a.Nachher, a.NachherArt, a.NachherHaupt})
		case "mail":
			n.Mails = append(n.Mails, Mail{a.Nachher, a.NachherArt, a.NachherHaupt})
		case "adresse":
			if a.NachherAdresse != nil {
				n.Adressen = append(n.Adressen, *a.NachherAdresse)
			}
		case "geburtstag":
			n.Geburtstag = a.Nachher
		case "firma":
			n.Firma = a.Nachher
		case "position":
			n.Position = a.Nachher
		case "foto":
			n.Foto = a.Bild
		case "notiz":
			n.Nachricht = a.Nachher
		}
	}
	return n
}

// Treffer are the household's cards that share a number (caldav.TelSchluessel) or an e-mail
// address with a new person's answer, three at most; none: nil.
func Treffer(all []caldav.Contact, f Frage, land string) []caldav.Contact {
	n := f.Eingetragen()
	var tels, mails []string
	for _, t := range n.Telefone {
		tels = append(tels, t.Nummer)
	}
	for _, m := range n.Mails {
		mails = append(mails, m.Adresse)
	}
	if len(tels)+len(mails) == 0 {
		return nil
	}
	hits := Finden(all, "", tels, mails, land)
	return hits[:min(len(hits), 3)]
}

// Ergaenzung is a new person's answer as changes of the card ct: the names it gives, the
// numbers, e-mail addresses and addresses the card lacks, the birthday, company and position it
// gives, the photo and the message; each with what the checks found about its value.
func Ergaenzung(f Frage, ct caldav.Contact) []Aenderung {
	vorher, n := FelderVon(ct), f.Eingetragen()
	nachher := vorher
	nachher.Vorname, nachher.Nachname = cmp.Or(n.Vorname, vorher.Vorname), cmp.Or(n.Nachname, vorher.Nachname)
	nachher.Telefone = append(slices.Clone(vorher.Telefone), n.Telefone...)
	nachher.Mails = append(slices.Clone(vorher.Mails), n.Mails...)
	nachher.Adressen = append(slices.Clone(vorher.Adressen), n.Adressen...)
	nachher.Geburtstag, nachher.Firma, nachher.Position = cmp.Or(n.Geburtstag, vorher.Geburtstag), cmp.Or(n.Firma, vorher.Firma), cmp.Or(n.Position, vorher.Position)
	nachher.Foto, nachher.Nachricht = n.Foto, n.Nachricht
	as := Vergleich(vorher, nachher)
	for i, a := range as {
		if j := slices.IndexFunc(f.Aenderungen, func(b Aenderung) bool { return b.Art == a.Art && b.Nachher == a.Nachher }); j >= 0 {
			as[i].Hinweise = f.Aenderungen[j].Hinweise
		}
	}
	return as
}

// Ergaenzen writes the ticked (nimm) of Ergaenzung(f, ct) into the card ct: those changes, and
// what became of each (Uebernehmen).
func Ergaenzen(ctx context.Context, dav *caldav.Client, f Frage, ct caldav.Contact, nimm []int, land string) ([]Aenderung, []string, error) {
	f.UID, f.Aenderungen = ct.UID, Ergaenzung(f, ct)
	stand, err := Uebernehmen(ctx, dav, f, Stand(f, false, nimm), land)
	return f.Aenderungen, stand, err
}

// errOhneName is an answer without a name: no card is made of it.
var errOhneName = errors.New("kontaktantwort: a new card needs a name")

// NeuAnlegen writes a new person's answer f as a new card into the household's book: its name
// always, the other changes taken in stand. The card's UID, and what became of each change.
func NeuAnlegen(ctx context.Context, dav *caldav.Client, f Frage, stand []string, land string) (string, []string, error) {
	out, genommen := slices.Clone(stand), f
	genommen.Aenderungen = nil
	for i, a := range f.Aenderungen {
		if a.Art == "vorname" || a.Art == "nachname" {
			out[i] = Angenommen
		}
		if out[i] == Angenommen {
			genommen.Aenderungen = append(genommen.Aenderungen, a)
		}
	}
	want := genommen.Eingetragen()
	if strings.TrimSpace(want.Vorname+want.Nachname) == "" {
		return "", nil, errOhneName
	}
	ct := caldav.Contact{UID: caldav.NewUUID(), Name: want.Name()}
	if _, err := dav.PutContact(ctx, ct); err != nil {
		return "", nil, err
	}
	karte := Frage{Buch: Haushalt, UID: ct.UID, Aenderungen: Vergleich(FelderVon(ct), want)}
	alle := Stand(karte, false, nil)
	for i := range alle {
		alle[i] = Angenommen
	}
	done, err := Uebernehmen(ctx, dav, karte, alle, land)
	for i, a := range f.Aenderungen {
		if out[i] != Angenommen {
			continue
		}
		out[i] = Uebernommen
		for j, b := range karte.Aenderungen {
			if b.Art == a.Art && b.Nachher == a.Nachher && (done == nil || done[j] != Uebernommen) {
				out[i] = Uebersprungen
			}
		}
	}
	return ct.UID, out, err
}
