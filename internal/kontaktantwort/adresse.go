package kontaktantwort

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// Addresses (owner 2026-11-08): street and house number apart, in the
// order the country writes them; the postal code checked by its country;
// a German PLZ and Ort checked against each other offline, never by
// sending the address to anyone.

var (
	nrHinten = regexp.MustCompile(`^(.*[^\d\s,])[\s,]+(\d+\s?[a-zA-Z]?(?:\s?[-/]\s?\d+\s?[a-zA-Z]?)?)$`)
	nrVorne  = regexp.MustCompile(`^(\d+\s?[a-zA-Z]?(?:[-/]\d+[a-zA-Z]?)?)[\s,]+(\D.*)$`)
)

// Strassenteile splits a street line into street and house number, the
// number at the end („Hauptstraße 5a“) or at the start („5a Hauptstraße“,
// as some phones and other countries write it); a line without one is all
// street.
func Strassenteile(zeile string) (strasse, nummer string) {
	zeile = strings.Join(strings.Fields(zeile), " ")
	if m := nrHinten.FindStringSubmatch(zeile); m != nil {
		return strings.TrimSpace(m[1]), m[2]
	}
	if m := nrVorne.FindStringSubmatch(zeile); m != nil {
		return strings.TrimSpace(m[2]), m[1]
	}
	return zeile, ""
}

// Strassenzeile is street and number as one line: the number after the
// street, as Germany writes it, or before it where the country does so.
func Strassenzeile(strasse, nummer, land string) string {
	if strasse == "" || nummer == "" {
		return strings.TrimSpace(strasse + " " + nummer)
	}
	if c, ok := LandVon(land); ok && c.nrVorne {
		return nummer + " " + strasse
	}
	return strasse + " " + nummer
}

// Land is a country as the checks know it: its code, names, the pattern of
// its postal codes (an HTML pattern too) and whether the number comes first.
type Land struct {
	Code    string
	Namen   []string
	PLZ     string
	nrVorne bool
	plz     *regexp.Regexp
}

// Laender are the countries whose postal codes are checked; another
// country's code may hold letters, digits, spaces and hyphens only.
var Laender = []Land{
	{Code: "DE", Namen: []string{"deutschland", "germany", "bundesrepublik deutschland", "brd", "d", "de"}, PLZ: `[0-9]{5}`},
	{Code: "AT", Namen: []string{"österreich", "austria", "a", "at"}, PLZ: `[0-9]{4}`},
	{Code: "CH", Namen: []string{"schweiz", "switzerland", "suisse", "svizzera", "ch"}, PLZ: `[0-9]{4}`},
	{Code: "LI", Namen: []string{"liechtenstein", "fl", "li"}, PLZ: `94[89][0-9]`},
	{Code: "NL", Namen: []string{"niederlande", "netherlands", "nederland", "holland", "nl"}, PLZ: `[0-9]{4} ?[A-Za-z]{2}`},
	{Code: "BE", Namen: []string{"belgien", "belgium", "belgië", "belgique", "be"}, PLZ: `[0-9]{4}`},
	{Code: "LU", Namen: []string{"luxemburg", "luxembourg", "lu"}, PLZ: `(L-)?[0-9]{4}`},
	{Code: "FR", Namen: []string{"frankreich", "france", "fr"}, PLZ: `[0-9]{5}`, nrVorne: true},
	{Code: "IT", Namen: []string{"italien", "italy", "italia", "it"}, PLZ: `[0-9]{5}`},
	{Code: "ES", Namen: []string{"spanien", "spain", "españa", "espana", "es"}, PLZ: `[0-9]{5}`},
	{Code: "PT", Namen: []string{"portugal", "pt"}, PLZ: `[0-9]{4}-[0-9]{3}`},
	{Code: "DK", Namen: []string{"dänemark", "denmark", "danmark", "dk"}, PLZ: `[0-9]{4}`},
	{Code: "SE", Namen: []string{"schweden", "sweden", "sverige", "se"}, PLZ: `[0-9]{3} ?[0-9]{2}`},
	{Code: "NO", Namen: []string{"norwegen", "norway", "norge", "no"}, PLZ: `[0-9]{4}`},
	{Code: "FI", Namen: []string{"finnland", "finland", "suomi", "fi"}, PLZ: `[0-9]{5}`},
	{Code: "PL", Namen: []string{"polen", "poland", "polska", "pl"}, PLZ: `[0-9]{2}-[0-9]{3}`},
	{Code: "CZ", Namen: []string{"tschechien", "czechia", "czech republic", "česko", "cz"}, PLZ: `[0-9]{3} ?[0-9]{2}`},
	{Code: "HU", Namen: []string{"ungarn", "hungary", "magyarország", "hu"}, PLZ: `[0-9]{4}`},
	{Code: "GR", Namen: []string{"griechenland", "greece", "gr"}, PLZ: `[0-9]{3} ?[0-9]{2}`},
	{Code: "TR", Namen: []string{"türkei", "turkey", "türkiye", "tr"}, PLZ: `[0-9]{5}`},
	{Code: "GB", Namen: []string{"vereinigtes königreich", "großbritannien", "england", "schottland", "wales", "united kingdom", "uk", "gb"},
		PLZ: `[A-Za-z]{1,2}[0-9][A-Za-z0-9]? ?[0-9][A-Za-z]{2}`, nrVorne: true},
	{Code: "IE", Namen: []string{"irland", "ireland", "éire", "ie"}, PLZ: `[A-Za-z][0-9][0-9Ww] ?[A-Za-z0-9]{4}`, nrVorne: true},
	{Code: "US", Namen: []string{"usa", "vereinigte staaten", "united states", "united states of america", "us"}, PLZ: `[0-9]{5}(-[0-9]{4})?`, nrVorne: true},
	{Code: "CA", Namen: []string{"kanada", "canada", "ca"}, PLZ: `[A-Za-z][0-9][A-Za-z] ?[0-9][A-Za-z][0-9]`, nrVorne: true},
	{Code: "AU", Namen: []string{"australien", "australia", "au"}, PLZ: `[0-9]{4}`, nrVorne: true},
}

// PLZSonst is the pattern of a postal code of any other country.
const PLZSonst = `[A-Za-z0-9 \-]{2,10}`

func init() {
	for i := range Laender {
		Laender[i].plz = regexp.MustCompile(`^(?:` + Laender[i].PLZ + `)$`)
	}
}

// LandVon is the country land names; "" is Germany, the house's.
func LandVon(land string) (Land, bool) {
	n := strings.ToLower(strings.Trim(strings.Join(strings.Fields(land), " "), ". "))
	if n == "" {
		return Laender[0], true
	}
	for _, c := range Laender {
		for _, name := range c.Namen {
			if n == name {
				return c, true
			}
		}
	}
	return Land{}, false
}

var plzSonst = regexp.MustCompile(`^(?:` + PLZSonst + `)$`)

// PLZFehler is what is wrong with a postal code for its country, "" when nothing.
func PLZFehler(plz, land string) string {
	if c, ok := LandVon(land); ok && !c.plz.MatchString(plz) {
		if c.Code == "DE" {
			return "Eine deutsche Postleitzahl hat genau fünf Ziffern."
		}
		return "So sieht dort keine Postleitzahl aus."
	} else if !ok && !plzSonst.MatchString(plz) {
		return "Eine Postleitzahl hat nur Buchstaben, Ziffern, Leerzeichen und Bindestriche."
	}
	return ""
}

// plzOrte is the open PLZ↔Ort table shipped with Homes (plz-orte.tsv.gz,
// 136 KB, made by tools/plz/plz.py: every German postal code of a street in
// OpenStreetMap, its places first, then its boroughs and suburbs; from the
// OpenPLZ API data, ODbL, © OpenStreetMap contributors, PLZ-LIZENZ.md).
//
//go:embed plz-orte.tsv.gz
var plzDaten []byte

var plzOrte = sync.OnceValue(func() map[string][]string {
	out := map[string][]string{}
	r, err := gzip.NewReader(bytes.NewReader(plzDaten))
	if err != nil {
		return out
	}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if f := strings.Split(sc.Text(), "\t"); len(f) > 1 {
			out[f[0]] = append(strings.Split(f[1], ";"), f[2:]...)
		}
	}
	return out
})

// OrtFehler is a finding when a German PLZ and Ort do not belong together
// by the table: a code it does not know (a Postfach's or a company's has
// no street), or a place not among those of the code. Another country's
// address is not checked.
func OrtFehler(plz, ort, land string) string {
	if c, ok := LandVon(land); !ok || c.Code != "DE" || plz == "" || ort == "" {
		return ""
	}
	orte, ok := plzOrte()[plz]
	if !ok {
		return "Die PLZ " + plz + " steht in keinem Straßenverzeichnis: vielleicht eine Postfach-PLZ, oder ein Tippfehler."
	}
	o := ortsname(ort)
	for _, x := range orte {
		if x = ortsname(x); o == x || strings.HasPrefix(o, x+" ") || strings.HasPrefix(x, o+" ") || ersteGleich(o, x) {
			return ""
		}
	}
	return "Die PLZ " + plz + " gehört zu " + orte[0] + ", nicht zu " + ort + "."
}

// ersteGleich: the first word is the same and long enough to name the place („frankfurt a m“).
func ersteGleich(a, b string) bool {
	x, _, _ := strings.Cut(a, " ")
	y, _, _ := strings.Cut(b, " ")
	return len(x) >= 5 && x == y
}

// ortsname is a place as two are compared: lower case, umlauts written
// out, „(Main)“ and „a. M.“ as words.
func ortsname(s string) string {
	s = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss").Replace(strings.ToLower(s))
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}
