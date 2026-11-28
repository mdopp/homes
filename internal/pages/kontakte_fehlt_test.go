package pages

import (
	"net/url"
	"strings"
	"testing"
)

// fehltCfg is pruefenCfg's house with a second card that lacks nothing (h2) and a firm's (f1).
func fehltCfg(t *testing.T) StartConfig {
	t.Helper()
	cfg, dav := pruefenCfg(t)
	dav.Put("/homes/haushalt-kontakte/h2.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:h2\r\nFN:Ben Muster\r\nN:Muster;Ben;;;\r\nTEL;TYPE=HOME:0711 1234567\r\nTEL:0160 9876543\r\n"+
		"EMAIL:ben@example.org\r\nADR:;;Hauptstraße 5;Stadt;;12345;\r\nBDAY:1990-04-01\r\nEND:VCARD\r\n")
	dav.Put("/homes/haushalt-kontakte/f1.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:f1\r\nFN:Stadtwerke\r\nORG:Stadtwerke\r\nCATEGORIES:Firma\r\nEND:VCARD\r\n")
	return cfg
}

// owner 2026-11-28 („Kontakte fragen auffindbar“): a contact that lacks what the link asks for
// says so at the top of its page, with „Anna selbst fragen“ instead of the button under the name
// — for an admin (the owner of the box or not); a family member sees neither; a complete contact
// keeps its button where it was.
func TestKontaktSaysWhatItLacksAtTheTopPerViewer(t *testing.T) {
	cfg := fehltCfg(t)
	karte := `<section class=fehlt role=note><p>Bei Anna fehlen Adresse und Geburtstag.</p><form method=post action=/kontakte/pruefen class="teilen fragen" data-teilen>` +
		`<input type=hidden name=buch value=haushalt><input type=hidden name=uid value="h1"><button data-laeuft="Erstellt den Link …" data-kopiert="Link kopiert" data-nochmal="Jetzt teilen">Anna selbst fragen</button></form></section>`
	for _, admin := range []struct{ user, groups string }{{"mdopp", "family,admins"}, {"ben", "admins"}} {
		body := frag(Kontakt(cfg), "GET", "/kontakte/h1", admin.user, admin.groups, nil).Body.String()
		mustContain(t, body, karte)
		before(t, body, "<h1>Anna Beispiel</h1>", karte)
		before(t, body, karte, "+49 151 23456789")
		mustNotContain(t, body, ">Daten prüfen lassen</button>") // one way to ask, not two

		full := frag(Kontakt(cfg), "GET", "/kontakte/h2", admin.user, admin.groups, nil).Body.String()
		mustContain(t, full, "<div class=oben>", ">Daten prüfen lassen</button>")
		mustNotContain(t, full, "<section class=fehlt", "selbst fragen")
		mustNotContain(t, frag(Kontakt(cfg), "GET", "/kontakte/f1", admin.user, admin.groups, nil).Body.String(), "<section class=fehlt") // a firm is not asked
	}
	other := frag(Kontakt(cfg), "GET", "/kontakte/h1", "anna", "family", nil).Body.String()
	mustContain(t, other, "<h1>Anna Beispiel</h1>")
	mustNotContain(t, other, "<section class=fehlt", "fehlen", "selbst fragen", "action=/kontakte/pruefen")

	cfg.KontaktURL = "" // the feature not on this box: nobody is offered it
	mustNotContain(t, frag(Kontakt(cfg), "GET", "/kontakte/h1", "mdopp", "family,admins", nil).Body.String(), "<section class=fehlt", "action=/kontakte/pruefen")
}

// A Handy-Kontakt is its person's: its card for them alone; another person, an admin among them,
// never reaches it (their own book has no such card).
func TestKontaktSaysWhatItLacksOnTheOwnHandyKontakt(t *testing.T) {
	cfg := fehltCfg(t)
	own := frag(MeineKontakte(cfg), "GET", "/kontakte/privat/3789r1-2B4D", "anna", "family", nil).Body.String()
	mustContain(t, own, `<p>Bei Oma fehlen Mobilnummer, E-Mail, Adresse und Geburtstag.</p>`, `<input type=hidden name=buch value=privat>`, ">Oma selbst fragen</button>")
	for _, v := range []struct{ user, groups string }{{"ben", "family"}, {"mdopp", "family,admins"}} {
		if rec := frag(MeineKontakte(cfg), "GET", "/kontakte/privat/3789r1-2B4D", v.user, v.groups, nil); rec.Code != 404 && !strings.Contains(rec.Header().Get("Location"), KontaktePath) {
			t.Errorf("%s reaches anna's Handy-Kontakt: %d", v.user, rec.Code)
		} else {
			mustNotContain(t, rec.Body.String(), "Oma", "selbst fragen")
		}
	}
}

// Kontakte's chip „Unvollständig (n)“ beside the groups (2: the fake's account sees anna's Oma too), for an admin: the contacts that lack
// something alone, each with a labelled „Fragen“ and what it lacks in one grey line.
func TestKontakteUnvollstaendigPerViewer(t *testing.T) {
	cfg := fehltCfg(t)
	for _, admin := range []struct{ user, groups string }{{"mdopp", "family,admins"}, {"ben", "admins"}} {
		list := frag(Kontakte(cfg), "GET", KontaktePath, admin.user, admin.groups, nil).Body.String()
		mustContain(t, list, `<li><a href="/kontakte" aria-current=page>Alle</a></li><li><a href="/kontakte?fehlt=1">Unvollständig (2)</a></li>`,
			`aria-label="Daten von Anna Beispiel prüfen lassen"`, ">Ben Muster<", ">Stadtwerke<") // the whole book, the quiet icon
		mustNotContain(t, list, ">Fragen</button>", "<small class=fehlt-zeile")

		rec := frag(Kontakte(cfg), "GET", fehltPfad, admin.user, admin.groups, nil)
		only := rec.Body.String()
		if rec.Code != 200 {
			t.Fatalf("Unvollständig: %d", rec.Code)
		}
		mustContain(t, only, `<a href="/kontakte?fehlt=1" aria-current=page>Unvollständig (2)</a>`, `<a href="/kontakte">Alle</a>`,
			`<a href="/kontakte/h1">Anna Beispiel</a>`, `<small class=fehlt-zeile>Adresse · Geburtstag</small>`,
			`class="teilen fragen" data-teilen><input type=hidden name=buch value=haushalt><input type=hidden name=uid value="h1"><button aria-label="Anna selbst fragen" data-laeuft="Erstellt den Link …"`, ">Fragen</button>")
		mustNotContain(t, only, "Ben Muster", "Stadtwerke", `aria-current=page>Alle`)
	}
	for _, q := range []string{KontaktePath, fehltPfad} { // a family member: neither the chip nor the filter
		body := frag(Kontakte(cfg), "GET", q, "anna", "family", nil).Body.String()
		mustContain(t, body, ">Ben Muster<", ">Anna Beispiel<")
		mustNotContain(t, body, "Unvollständig", "Fragen", "fehlt-zeile", "action=/kontakte/pruefen")
	}
}

// Nobody lacks anything: no chip; the filter asked for anyway says so in one sentence.
func TestKontakteUnvollstaendigNobody(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	dav, c := calendar(t)
	dav.AddAddressBook("/homes/haushalt-kontakte/", "Haushalt")
	dav.Put("/homes/haushalt-kontakte/h1.vcf", strings.Replace(pruefenKarte, "END:VCARD", "ADR:;;Hauptstraße 5;Stadt;;12345;\r\nBDAY:1990-04-01\r\nEND:VCARD", 1))
	cfg.Contacts = c
	mustNotContain(t, frag(Kontakte(cfg), "GET", KontaktePath, "mdopp", "family,admins", nil).Body.String(), "Unvollständig", "<ul class=chips")
	body := frag(Kontakte(cfg), "GET", fehltPfad, "mdopp", "family,admins", nil).Body.String()
	mustContain(t, body, `Unvollständig (0)</a>`, "<p class=k>Bei allen Kontakten sind Mobilnummer, E-Mail, Adresse und Geburtstag da.</p>")
	mustNotContain(t, body, "Anna Beispiel</a>")
}

// The card's button is „Link teilen“'s flow: the same link and message, the same refusal in words.
func TestKontaktFragenMakesTheSameLink(t *testing.T) {
	cfg := fehltCfg(t)
	token := startLink(t, cfg, "haushalt", "h1", "family,admins")
	if f, err := cfg.Antworten.Zum(token); err != nil || f.UID != "h1" || f.Vorher.Vorname != "Anna" {
		t.Fatalf("the link: %+v %v", f, err)
	}
	if rec := frag(Pruefen(cfg), "POST", PruefenPath, "anna", "family", url.Values{"buch": {"haushalt"}, "uid": {"h1"}}); rec.Code != 403 || !strings.Contains(rec.Body.String(), "nur ein Admin") {
		t.Errorf("a family member asks: %d %s", rec.Code, rec.Body.String())
	}
}
