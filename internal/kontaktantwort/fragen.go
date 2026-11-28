package kontaktantwort

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/haushalt"
	"github.com/mdopp/homes/internal/namen"
	"github.com/mdopp/homes/internal/texte"
)

// Kontakte fragen auffindbar (owner 2026-11-28): what a contact lacks of
// what the link asks for, so the household sees whom to ask. Four fields,
// each one a sentence can name: a mobile number (to send the link to), an
// e-mail address, a postal address, a birthday. A firm's card lacks
// nothing: a firm is not asked.

// The fields Fehlt names, in this order.
const (
	FehltMobil      = "mobil"
	FehltMail       = "mail"
	FehltAdresse    = "adresse"
	FehltGeburtstag = "geburtstag"
)

var fehltWort = map[string]texte.ID{FehltMobil: texte.FehltMobil, FehltMail: texte.FehltMail, FehltAdresse: texte.FehltAdresse, FehltGeburtstag: texte.FehltGeburtstag}

// FehltWorte are Fehlt's fields as the family names them, in sp: „Mobilnummer“, „Adresse“.
func FehltWorte(sp texte.Sprache, fehlt []string) []string {
	out := []string{}
	for _, f := range fehlt {
		out = append(out, sp.T(fehltWort[f]))
	}
	return out
}

// Fehlt are the fields of FehltMobil … FehltGeburtstag that ct lacks; none for a firm's card
// (Homes' Firma marker, or a card whose name is its company's).
func Fehlt(ct caldav.Contact, land string) []string {
	name := strings.TrimSpace(ct.Name)
	if slices.Contains(ct.Categories, caldav.FirmCategory) || name == "" || name == strings.TrimSpace(ct.Org) && ct.Given == "" && ct.Family == "" {
		return nil
	}
	f := FelderVon(ct)
	var out []string
	if Mobil(ct, land) == "" {
		out = append(out, FehltMobil)
	}
	if len(f.Mails) == 0 {
		out = append(out, FehltMail)
	}
	if !slices.ContainsFunc(f.Adressen, func(a Adresse) bool { return !a.Leer() }) && len(ct.Addresses) == 0 {
		out = append(out, FehltAdresse)
	}
	if f.Geburtstag == "" {
		out = append(out, FehltGeburtstag)
	}
	return out
}

// Mobil is ct's mobile number as E.164 (+49151…), "" none: the first marked
// mobil, else the first whose digits are a German mobile's (015…, 016…, 017…).
func Mobil(ct caldav.Contact, land string) string {
	land = cmp.Or(land, haushalt.DefaultCountry)
	ts := FelderVon(ct).Telefone
	for _, t := range ts {
		if t.Art == "mobil" && len(ziffern(t.Nummer)) >= 6 {
			return "+" + haushalt.International(t.Nummer, land)
		}
	}
	for _, t := range ts {
		if n := haushalt.International(t.Nummer, land); len(n) >= 12 && (strings.HasPrefix(n, "4915") || strings.HasPrefix(n, "4916") || strings.HasPrefix(n, "4917")) {
			return "+" + n
		}
	}
	return ""
}

// Finden are the household's contacts that are the phone's one: those with one of its numbers
// (caldav.TelSchluessel) first, else one of its e-mail addresses (any case), else its exact name
// (namen.Plain); several are narrowed by the later of these while some stay. None: nil.
func Finden(all []caldav.Contact, name string, tels, mails []string, land string) []caldav.Contact {
	var keys []string
	for _, t := range tels {
		if k := caldav.TelSchluessel(t, land); len(k) >= 6 {
			keys = append(keys, k)
		}
	}
	nach := []func(caldav.Contact) bool{
		func(ct caldav.Contact) bool {
			return slices.ContainsFunc(ct.Phones, func(p string) bool { return slices.Contains(keys, caldav.TelSchluessel(p, land)) })
		},
		func(ct caldav.Contact) bool {
			return slices.ContainsFunc(ct.Emails, func(m string) bool {
				return slices.ContainsFunc(mails, func(n string) bool { return n != "" && strings.EqualFold(strings.TrimSpace(m), strings.TrimSpace(n)) })
			})
		},
		func(ct caldav.Contact) bool {
			return namen.Plain(name) != "" && namen.Plain(ct.Name) == namen.Plain(name)
		},
	}
	treffer := func(cs []caldav.Contact, ja func(caldav.Contact) bool) []caldav.Contact {
		return slices.DeleteFunc(slices.Clone(cs), func(ct caldav.Contact) bool { return ct.UID == "" || !ja(ct) })
	}
	for i, ja := range nach {
		hits := treffer(all, ja)
		if len(hits) == 0 {
			continue
		}
		for _, weiter := range nach[i+1:] {
			if enger := treffer(hits, weiter); len(enger) > 0 {
				hits = enger
			}
		}
		return hits
	}
	return nil
}

// Anlegen writes the phone's contact into the household's book as a new card: its name, the
// numbers in the household's form (haushalt.Telefon) and the e-mail addresses whose syntax holds,
// GrenzeJeArt of each at most; the card as it was written.
func Anlegen(ctx context.Context, dav *caldav.Client, name string, tels, mails []string, land string) (caldav.Contact, error) {
	ct := caldav.Contact{UID: caldav.NewUUID(), Name: eineZeile(name)}
	for _, t := range tels {
		if t = eineZeile(t); len(ziffern(t)) >= 3 && Nummer(t, land) == "" && len(ct.Phones) < GrenzeJeArt {
			p, _ := haushalt.Telefon(t, cmp.Or(land, haushalt.DefaultCountry))
			ct.Phones = append(ct.Phones, p)
		}
	}
	for _, m := range mails {
		if m = eineZeile(m); m != "" && (Pruefung{}).Mail(ctx, m) == "" && len(ct.Emails) < GrenzeJeArt {
			ct.Emails = append(ct.Emails, m)
		}
	}
	_, err := dav.PutContact(ctx, ct)
	return ct, err
}

// Link makes the link for the contact ct of buch, started by wer: von their first name (""
// the one they last signed a link with, as the app has none), the address basis/<token>.
func (s *Store) Link(wer, von, buch string, ct caldav.Contact, basis string) (string, Frage, error) {
	token, f, err := s.Neu(wer, s.von(wer, von), buch, ct.UID, FelderVon(ct))
	return strings.TrimRight(basis, "/") + "/" + token, f, err
}

// von is von, else the first name wer last signed a link with.
func (s *Store) von(wer, von string) string {
	if von == "" {
		for _, f := range s.read() {
			if f.Wer == wer && f.Von != "" {
				von = f.Von
			}
		}
	}
	return von
}
