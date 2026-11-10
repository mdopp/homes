package haushalt

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/namen"
	"github.com/mdopp/homes/internal/texte"
)

// Gruppen (owner 2026-11-10, ADR 0002: „Stammtisch“, „Kickermannschaft Renegades“, a person in several) are the
// household's own groups of a shared card, its CATEGORIES; Homes' „Firma“ (haus#163) is none: kept, unshown.
func Gruppen(ct caldav.Contact) (out []string) {
	for _, c := range ct.Categories {
		if !strings.EqualFold(c, caldav.FirmCategory) && !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, c) }) {
			out = append(out, c)
		}
	}
	return out
}

// InGruppe says whether ct is in the group g.
func InGruppe(ct caldav.Contact, g string) bool {
	return slices.ContainsFunc(Gruppen(ct), func(o string) bool { return strings.EqualFold(o, g) })
}

// AlleGruppen are the groups used in all, by name, each once.
func AlleGruppen(all []caldav.Contact) (out []string) {
	for _, ct := range all {
		out = append(out, Gruppen(ct)...)
	}
	slices.SortFunc(out, func(a, b string) int { return cmp.Compare(namen.Plain(a), namen.Plain(b)) })
	return slices.CompactFunc(out, strings.EqualFold)
}

const GruppeMax = 60 // the longest name of a group, in characters

// GruppeKorrektur is the change of a card's groups adding dazu or, with weg, taking weg away
// (caldav: one value of CATEGORIES); a name that cannot be one is said by the text's id instead.
func GruppeKorrektur(dazu, weg string) (caldav.Korrektur, texte.ID) {
	name := cmp.Or(weg, dazu)
	switch {
	case name == "":
		return caldav.Korrektur{}, texte.GruppeOhneName
	case utf8.RuneCountInString(name) > GruppeMax:
		return caldav.Korrektur{}, texte.GruppeZuLang
	case weg == "" && strings.EqualFold(dazu, caldav.FirmCategory):
		return caldav.Korrektur{}, texte.GruppeFirma
	case weg != "":
		return caldav.Korrektur{Prop: "CATEGORIES", Alt: weg}, ""
	}
	return caldav.Korrektur{Prop: "CATEGORIES", Neu: dazu}, ""
}
