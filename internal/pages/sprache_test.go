package pages

import "testing"

// TestStartSpeaksTheClientsLanguage (owner 2026-11-18): Start and its frame
// in the browser's language, by Accept-Language and its weights; one Homes
// does not speak is German. A word not translated stays German.
func TestStartSpeaksTheClientsLanguage(t *testing.T) {
	named(t, "Ada")
	type want struct{ has, hasNot []string }
	german := want{[]string{"<html lang=de>", "Dein Zuhause, klar im Blick.", "Mittwoch, 14. Oktober", "<h2 id=karte-deine-geraete>Deine Geräte</h2>",
		"<h2 id=karte-heute>Heute</h2>", "<h2 id=karte-im-haus>Im Haus</h2>", "</svg>Start</a>", "</svg>Abmelden</a>", "Angemeldet als"},
		[]string{"Your home", "Wednesday", "Ta maison"}}
	english := want{[]string{"<html lang=en>", "Your home, at a glance.", "Wednesday, 14 October", "<h2 id=karte-deine-geraete>Your devices</h2>",
		">Today</h2>", "<h2 id=karte-im-haus>At home</h2>", "</svg>Home</a>", "</svg>More<", "</svg>Sign out</a>", "Signed in as",
		"Calendar →"},
		[]string{"lang=de", "Dein Zuhause", "Mittwoch", "Im Haus</h2>", "Abmelden"}}
	french := want{[]string{"<html lang=fr>", "Ta maison, d'un coup d'œil.", "mercredi 14 octobre", ">Aujourd&#39;hui</h2>",
		"<h2 id=karte-im-haus>À la maison</h2>", "</svg>Accueil</a>", "</svg>Se déconnecter</a>"},
		[]string{"lang=de", "Dein Zuhause", "Your home"}}
	spanish := want{[]string{"<html lang=es>", "Tu hogar, de un vistazo.", "miércoles, 14 de octubre", "</svg>Inicio</a>"}, []string{"lang=de", "Your home"}}
	for accept, w := range map[string]want{
		"":                           german,
		"en-US,en;q=0.9":             english,
		"fr":                         french,
		"ja":                         german,
		"ja,fr;q=0.5,en;q=0.8":       english, // the weights, not the order
		"de;q=0.4,es;q=0.6,en;q=0.5": spanish,
		"de-DE,de;q=0.9,en;q=0.8":    german,
	} {
		code, body := startPageIn(t, familyEvening(t), "anna", "family", accept)
		if code != 200 {
			t.Fatalf("%q: status %d", accept, code)
		}
		mustContain(t, body, w.has...)
		mustNotContain(t, body, w.hasNot...)
		if t.Failed() {
			t.Fatalf("Accept-Language %q", accept)
		}
	}
}
