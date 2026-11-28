package teilen

import (
	"strings"

	"github.com/mdopp/homes/internal/caldav"
)

// VCard is the released card as a vCard 3.0 for the guest's phone: only the fields k holds,
// the name (or vorname, when the name is not released) always, as FN needs one.
func (k Karte) VCard(vorname string) string {
	esc := caldav.EscapeText
	name := strings.TrimSpace(k.Name)
	if name == "" {
		name = strings.TrimSpace(vorname)
	}
	vor, nach := name, ""
	if i := strings.LastIndex(name, " "); i > 0 {
		vor, nach = name[:i], name[i+1:]
	}
	var b strings.Builder
	zeile := func(prop, v string) {
		if v = strings.TrimSpace(v); v != "" {
			b.WriteString(prop + ":" + v + "\r\n")
		}
	}
	b.WriteString("BEGIN:VCARD\r\nVERSION:3.0\r\n")
	zeile("FN", esc(name))
	b.WriteString("N:" + esc(nach) + ";" + esc(vor) + ";;;\r\n")
	zeile("TEL;TYPE=CELL", esc(k.Mobil))
	zeile("TEL;TYPE=VOICE", esc(k.Telefon))
	zeile("EMAIL;TYPE=INTERNET", esc(k.Mail))
	if a := strings.TrimSpace(k.Adresse); a != "" {
		b.WriteString("ADR:;;" + esc(strings.ReplaceAll(a, "\n", ", ")) + ";;;;\r\n")
	}
	zeile("BDAY", k.Geburtstag) // pruefen held it to a day
	zeile("ORG", esc(k.Firma))
	b.WriteString("END:VCARD\r\n")
	return b.String()
}
