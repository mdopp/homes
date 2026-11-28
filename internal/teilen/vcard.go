package teilen

import (
	"strings"

	"github.com/mdopp/homes/internal/caldav"
)

// VCard is the released card as a vCard 3.0 for the guest's phone: only the fields k holds,
// the name (or vorname, when the name is not released) always, as FN needs one. Every value is
// TEXT-escaped (a newline, ; , and \\ stay inside their property, a CR is dropped), the birthday
// was held to a day by pruefen, and each line is folded at 75 octets (RFC 6350 §3.2).
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
			caldav.Fold(&b, prop+":"+v)
		}
	}
	caldav.Fold(&b, "BEGIN:VCARD")
	caldav.Fold(&b, "VERSION:3.0")
	zeile("FN", esc(name))
	caldav.Fold(&b, "N:"+esc(nach)+";"+esc(vor)+";;;")
	zeile("TEL;TYPE=CELL", esc(k.Mobil))
	zeile("TEL;TYPE=VOICE", esc(k.Telefon))
	zeile("EMAIL;TYPE=INTERNET", esc(k.Mail))
	if a := strings.TrimSpace(k.Adresse); a != "" {
		caldav.Fold(&b, "ADR:;;"+esc(strings.ReplaceAll(a, "\n", ", "))+";;;;")
	}
	zeile("BDAY", k.Geburtstag) // pruefen held it to a day
	zeile("ORG", esc(k.Firma))
	caldav.Fold(&b, "END:VCARD")
	return b.String()
}
