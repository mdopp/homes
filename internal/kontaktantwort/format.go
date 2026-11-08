package kontaktantwort

import (
	"context"
	"regexp"
	"strconv"
)

// The form of each field (owner 2026-11-08), as the page's HTML pattern
// and here again: the browser says it at once, the server holds to it.
// What the contact changed or added must have its form; what the card had
// stays as it was, even when it has none.
const (
	MusterName       = `[\p{L}\p{M}\s'’\-]+`
	MusterNummer     = `[\+0-9\(][0-9 \(\)\/.\-]{3,}`
	MusterStrasse    = `[\p{L}\p{M}\p{N}\s'’.,\-\/\(\)]+`
	MusterHausnummer = `[0-9][0-9A-Za-z \-\/]{0,9}`
	MusterOrt        = `[\p{L}\p{M}\s'’.\-\/\(\)]+`
	MusterTag        = `[0-9]{1,2}`
	MusterJahr       = `[0-9]{4}`
)

func muster(p string) *regexp.Regexp { return regexp.MustCompile(`^(?:` + p + `)$`) }

var (
	nameForm, strasseForm, nummerForm, ortForm = muster(MusterName), muster(MusterStrasse), muster(MusterHausnummer), muster(MusterOrt)
)

// Formfehler are the fields of nachher whose form is wrong, by the page's
// name of the field and its place ("tel.1", "adr-plz.0"), each one German
// sentence for beside it; land is the house's calling code.
func Formfehler(vorher, nachher Felder, land string) map[string]string {
	out := map[string]string{}
	falsch := func(key, msg string) {
		if _, ok := out[key]; !ok && msg != "" {
			out[key] = msg
		}
	}
	for key, v := range map[string][2]string{"vorname": {vorher.Vorname, nachher.Vorname}, "nachname": {vorher.Nachname, nachher.Nachname}} {
		if v[1] != "" && v[1] != v[0] && !nameForm.MatchString(v[1]) {
			falsch(key, "Ein Name hat nur Buchstaben, Leerzeichen, Bindestriche und Apostrophe.")
		}
	}
	for i, t := range nachher.Telefone {
		if t.Nummer != "" && ziffern(t.Nummer) != ziffern(at(vorher.Telefone, i).Nummer) {
			falsch("tel."+strconv.Itoa(i), Nummer(t.Nummer, land))
		}
	}
	for i, m := range nachher.Mails {
		if m.Adresse != "" && m.Adresse != at(vorher.Mails, i).Adresse {
			falsch("mail."+strconv.Itoa(i), Pruefung{}.Mail(context.Background(), m.Adresse))
		}
	}
	for i, a := range nachher.Adressen {
		if !a.Leer() && !a.gleich(at(vorher.Adressen, i)) {
			adresseFehler(a, strconv.Itoa(i), falsch)
		}
	}
	return out
}

// adresseFehler says what is wrong with address n's parts.
func adresseFehler(a Adresse, n string, falsch func(key, msg string)) {
	switch {
	case a.Strasse == "":
		falsch("adr-strasse."+n, "Die Straße fehlt.")
	case !strasseForm.MatchString(a.Strasse):
		falsch("adr-strasse."+n, "Eine Straße hat nur Buchstaben, Ziffern, Leerzeichen und . , - ' / ( ).")
	}
	if a.Hausnummer != "" && !nummerForm.MatchString(a.Hausnummer) {
		falsch("adr-nr."+n, "Eine Hausnummer beginnt mit einer Ziffer, etwa 5, 12a oder 3-5.")
	}
	if a.PLZ == "" {
		falsch("adr-plz."+n, "Die Postleitzahl fehlt.")
	}
	falsch("adr-plz."+n, PLZFehler(a.PLZ, a.Land))
	switch {
	case a.Ort == "":
		falsch("adr-ort."+n, "Der Ort fehlt.")
	case !ortForm.MatchString(a.Ort):
		falsch("adr-ort."+n, "Ein Ort hat nur Buchstaben, Leerzeichen und . - ' / ( ).")
	}
	if a.Land != "" && !ortForm.MatchString(a.Land) {
		falsch("adr-land."+n, "Ein Land hat nur Buchstaben, Leerzeichen und Bindestriche.")
	}
}
