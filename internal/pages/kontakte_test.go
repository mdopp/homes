package pages

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/caldav/davfake"
)

// addressBook is the household's address book with the cards given as
// "uid|FN|TEL|EMAIL" (empty parts left out).
func addressBook(t *testing.T, cards ...string) (*davfake.Server, *caldav.Client) {
	t.Helper()
	dav, c := calendar(t)
	dav.AddAddressBook("/homes/haushalt-kontakte/", "Haushalt")
	for _, cd := range cards {
		p := strings.Split(cd, "|")
		v := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + p[0] + "\r\nFN:" + p[1] + "\r\n"
		if p[2] != "" {
			v += "TEL:" + p[2] + "\r\n"
		}
		if p[3] != "" {
			v += "EMAIL:" + p[3] + "\r\n"
		}
		dav.Put("/homes/haushalt-kontakte/"+p[0]+".vcf", v+"END:VCARD\r\n")
	}
	return dav, c
}

// owner 2026-11-05 („Web zeigt, Handy bearbeitet“): the household's
// address book, by name, each row leading to its page with its number or
// e-mail to tap, and where to edit folded away at its end.
func TestKontakteListsTheAddressBookByName(t *testing.T) {
	_, c := addressBook(t, "z|Zahnarzt Dr. Berg|0711 12345|", "o|Örtel, Jana||jana@example.org", "a|Anna|+49 151 2222|anna@example.org", "x|Xaver||")
	code, body, _ := familyPage(t, Kontakte, StartConfig{Contacts: c}, "GET", "/kontakte", "family")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body, "<h1>Kontakte</h1>",
		"<details class=wie><summary>Wie das funktioniert</summary><p class=k>Neue Kontakte und Änderungen machst du auf dem Handy in den Haushalts-Kontakten oder über den Boten.",
		`<h2 id=karte-adressbuch>Adressbuch</h2>`,
		`<a href="/kontakte/a">Anna</a> <a class=ruf href="tel:+491512222">+49 151 2222</a>`, // the number before the e-mail
		`<a href="/kontakte/o">Örtel, Jana</a> <a class=ruf href="mailto:jana@example.org">jana@example.org</a>`,
		`<a href="/kontakte/x">Xaver</a></li>`, // nothing to tap
		`<a href="/kontakte/z">Zahnarzt Dr. Berg</a> <a class=ruf href="tel:071112345">`,
		`type=search id=kontakte-suche placeholder=`)
	before(t, body, ">Anna<", ">Örtel, Jana<") // Ö as O
	before(t, body, ">Örtel, Jana<", ">Xaver<")
	before(t, body, ">Xaver<", ">Zahnarzt Dr. Berg<")
	before(t, body, ">Zahnarzt Dr. Berg<", "<details class=wie>")
	mustNotContain(t, body, "class=segmente", "class=pfad") // Haushalt alone: no segments; the list is the top
	mustNotContain(t, body, "<form method=post", "Bearbeiten", "Löschen")
	if _, kal, _ := familyPage(t, Kalender, StartConfig{Calendar: c}, "GET", "/kalender", "family"); !strings.Contains(kal, "machst du auf dem Handy im Haushalts-Kalender oder über den Boten.") {
		t.Errorf("the calendar does not say where to edit, as the contacts do:\n%s", kal)
	}
}

// The search filters while typing (owner 2026-11-08: no button, no second
// page), by each row's name, numbers and e-mails as plain words.
func TestKontakteSearch(t *testing.T) {
	_, c := addressBook(t, "a|Anna|+49 151 2222|anna@example.org", "b|Bäckerei Müller|0711 999|", "c|Clara||clara@schule.example")
	_, body, _ := familyPage(t, Kontakte, StartConfig{Contacts: c}, "GET", "/kontakte", "family")
	mustContain(t, body, `<li data-suche="anna491512222annaexampleorg">`, `<li data-suche="backereimuller0711999">`, `<li data-suche="claraclaraschuleexample">`,
		`<p class=k id=kontakte-keiner data-vorlage="Kein Kontakt passt zu „%s“." hidden></p>`, "window.homesKontakte")
	mustNotContain(t, body, "name=q", ">Suchen</button>")
}

// Many contacts: a card per first letter; few: one card.
func TestKontakteGroupsManyByLetter(t *testing.T) {
	var cs []string
	for i := range kontakteBuchstaben {
		cs = append(cs, fmt.Sprintf("u%d|%c%d Name||", i, 'A'+rune(i%4), i))
	}
	_, c := addressBook(t, append(cs, "e|Ärztehaus||", "d|42 Pizza||")...)
	_, body, _ := familyPage(t, Kontakte, StartConfig{Contacts: c}, "GET", "/kontakte", "family")
	mustContain(t, body, "<h2 id=karte-a>A</h2>", "<h2 id=karte-d>D</h2>", ">#</h2>")
	mustNotContain(t, body, ">Adressbuch</h2>", "<h2 id=karte-e>")
	before(t, body, ">#</h2>", "<h2 id=karte-a>A</h2>")
	if n := strings.Count(body, "<section"); n != 5 {
		t.Errorf("%d cards, want #, A, B, C, D", n)
	}
	if strings.Count(body, "<section") != strings.Count(body, "</section>") {
		t.Error("a card is not closed")
	}
}

// An empty address book says so; Radicale not answering is one plain line.
func TestKontakteEmptyAndDown(t *testing.T) {
	_, c := addressBook(t)
	_, body, _ := familyPage(t, Kontakte, StartConfig{Contacts: c}, "GET", "/kontakte", "family")
	mustContain(t, body, "Im Adressbuch steht noch niemand.")
	c.Password = "falsch"
	code, body, _ := familyPage(t, Kontakte, StartConfig{Contacts: c}, "GET", "/kontakte", "family")
	if code != 200 || strings.Count(body, saysNoContacts) != 1 {
		t.Fatalf("%d:\n%s", code, body)
	}
	mustNotContain(t, body, "kontakte-suche", "steht noch niemand")
	mustNotLeak(t, body, host(c.URL))
	code, body, _ = familyPage(t, Kontakte, StartConfig{}, "GET", "/kontakte", "family")
	if code != 200 || !strings.Contains(body, "Die Kontakte sind hier noch nicht eingerichtet.") {
		t.Errorf("without Radicale: %d", code)
	}
	if code, _, _ := familyPage(t, Kontakte, StartConfig{Contacts: c}, "POST", "/kontakte", "family"); code != 405 {
		t.Errorf("POST → %d, want 405", code)
	}
}
