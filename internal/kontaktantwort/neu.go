package kontaktantwort

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/namen"
)

// Kontaktdaten eintragen lassen (owner 2026-11-28, „Wie wir es schon für
// die Verbesserung von Kontaktdaten haben“): the same link, page, store and
// review as „Kontakt prüfen lassen“, for a person the household has no card
// of yet. The link shows the form empty and works for FristNeu; the answer
// waits for an admin's review like any other, before → after with nothing
// before. The review first offers the household cards that may be that
// person (Kandidaten: the same number, e-mail address or a similar name),
// and adds the answer to the one picked (Ergaenzen), before → after as
// against that card; else it writes it as a new card into the household's
// book (NeuAnlegen). Nothing is written without that review.

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

// Why a household card may be the person of a new answer (owner 2026-11-28: „mit Vorschlägen,
// mit wem das gemerged werden soll“), strongest first.
const (
	GleicheMobil   = "mobil"   // one of its numbers (caldav.TelSchluessel), a mobile one
	GleicheNummer  = "telefon" // one of its numbers, another kind
	GleicheMail    = "mail"    // one of its e-mail addresses, in any case
	AehnlicherName = "name"    // its name as namen.Plain compares it, in any order of the words
)

// Kandidat is a household card that may be the person of a new answer, and why (Gleiche…).
type Kandidat struct {
	caldav.Contact
	Gruende []string
}

// KandidatenHoechstens is how many the review offers.
const KandidatenHoechstens = 5

// Kandidaten are the household's cards that may be the person of a new answer, the most likely
// first: the same mobile number, another same number, the same e-mail address, a similar name
// (namen.Plain, the name Dieselbe Person compares), each counted, at most KandidatenHoechstens;
// none: nil. A card without a UID is none: it could not be written to.
func Kandidaten(all []caldav.Contact, f Frage, land string) []Kandidat {
	n := f.Eingetragen()
	tels := map[string]bool{} // its numbers' keys, true a mobile one
	for _, t := range n.Telefone {
		if k := caldav.TelSchluessel(t.Nummer, land); len(k) >= 6 {
			tels[k] = tels[k] || t.Art == "mobil" || mobilNummer(k)
		}
	}
	name := namenSchluessel(n.Vorname + " " + n.Nachname)
	gewicht := map[string]int{GleicheMobil: 8, GleicheNummer: 4, GleicheMail: 4, AehnlicherName: 2}
	var out []Kandidat
	punkte := map[string]int{}
	for _, ct := range all {
		if strings.TrimSpace(ct.UID) == "" {
			continue
		}
		var gs []string
		nummer := ""
		for i, p := range ct.Phones {
			k := caldav.TelSchluessel(p, land)
			if mobil, da := tels[k]; da && nummer != GleicheMobil {
				nummer = map[bool]string{true: GleicheMobil, false: GleicheNummer}[mobil || at(ct.PhoneKinds, i) == "mobil"]
			}
		}
		if nummer != "" {
			gs = append(gs, nummer)
		}
		if slices.ContainsFunc(ct.Emails, func(m string) bool {
			return slices.ContainsFunc(n.Mails, func(x Mail) bool { return x.Adresse != "" && strings.EqualFold(strings.TrimSpace(m), x.Adresse) })
		}) {
			gs = append(gs, GleicheMail)
		}
		if name != "" && (namenSchluessel(ct.Name) == name || namenSchluessel(ct.Given+" "+ct.Family) == name) {
			gs = append(gs, AehnlicherName)
		}
		if len(gs) == 0 {
			continue
		}
		for _, g := range gs {
			punkte[ct.UID] += gewicht[g]
		}
		out = append(out, Kandidat{ct, gs})
	}
	slices.SortStableFunc(out, func(a, b Kandidat) int {
		return cmp.Or(cmp.Compare(punkte[b.UID], punkte[a.UID]), cmp.Compare(namen.Plain(a.Name), namen.Plain(b.Name)))
	})
	return out[:min(len(out), KandidatenHoechstens)]
}

// namenSchluessel is a name as Kandidaten compares it: each word as namen.Plain has it, sorted, so
// „Muster Erika“ is „Erika Muster“; "" no letters.
func namenSchluessel(name string) string {
	var ws []string
	for _, w := range strings.Fields(name) {
		if p := namen.Plain(w); p != "" {
			ws = append(ws, p)
		}
	}
	slices.Sort(ws)
	return strings.Join(ws, " ")
}

// mobilNummer says whether a number's digits (TelSchluessel, International) are a German mobile's (+49 15…, 16…, 17…).
func mobilNummer(k string) bool {
	return len(k) >= 12 && (strings.HasPrefix(k, "4915") || strings.HasPrefix(k, "4916") || strings.HasPrefix(k, "4917"))
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
