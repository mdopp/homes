package pages

import (
	"html"
	"net/http"
	"strings"

	"github.com/mdopp/homes/internal/tagaktion"
)

// TagFreigabePath is where an admin answers a wish for a tag's automation
// (ADR 0004 §8): the link in the line every admin is told, and the task
// on their Deine Aufgaben. Freigeben writes the change as that admin (a
// new automation: passed to the Bote, whose reply is theirs to go on
// with), Ablehnen does nothing; the first admin's answer counts, the
// person is told either way. The person who asked sees how it stands.
// This is the household approving a change to its HA automation; Laib's
// inbox stays Laib's alone (haus#1).
const TagFreigabePath = TagPath + "freigabe/"

// TagFreigabe is the page and where its two buttons post.
func TagFreigabe(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, user := strings.TrimPrefix(r.URL.Path, TagFreigabePath), r.Header.Get("Remote-User")
		f, ok := cfg.Tags.Freigabe(id)
		if !ok || f.Von != user && !isAdmin(r) {
			missing{http.StatusNotFound, "Diesen Wunsch gibt es nicht (mehr).", "Deine Aufgaben", aufgabenPath, "Zu deinen Aufgaben"}.write(w, r, cfg)
			return
		}
		if r.Method != http.MethodPost {
			tagFreigabeSeite(w, r, cfg, f, "", "")
			return
		}
		if !fromHomes(r) {
			http.Error(w, "Das geht nur von Homes' eigener Seite.", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
		r.ParseForm()
		admin := tagaktion.Wer{Login: user, Name: r.Header.Get("Remote-Name"), Admin: isAdmin(r)}
		f, bote, err := cfg.Tags.Entscheiden(r.Context(), id, admin, r.PostForm.Get("entscheiden") == "ja")
		if err != nil {
			n := tagaktion.Als(err)
			http.Error(w, n.Satz, n.Status)
			return
		}
		tagFreigabeSeite(w, r, cfg, f, f.Satz(), bote)
	}
}

func tagFreigabeSeite(w http.ResponseWriter, r *http.Request, cfg StartConfig, f tagaktion.Freigabe, gut, bote string) {
	b := familyHead(r, cfg, "Freigabe", "Mehr")
	b.WriteString(`<h1>Freigabe</h1><p>` + html.EscapeString(f.Text) + `</p><p class=k>` + html.EscapeString(f.Vorschau) + `</p>`)
	switch {
	case gut != "":
		b.WriteString(`<div class=gut role=status><span class=punkt></span><p>` + html.EscapeString(gut) + `</p></div>`)
		if bote != "" {
			b.WriteString(`<section class=karte><h2>Antwort</h2><p>` + strings.ReplaceAll(html.EscapeString(bote), "\n", "<br>") +
				`</p><p class=k>Antworte im Sprechen-Feld auf der Startseite, zum Beispiel mit dem Code.</p></section>`)
		}
	case f.Stand != tagaktion.Wartet:
		b.WriteString(`<p role=status>` + html.EscapeString(f.Satz()) + `</p>`)
	case isAdmin(r):
		b.WriteString(`<p class=k>Die erste Antwort eines Admins zählt.</p><form method=post action="` + TagFreigabePath + f.ID + `">` +
			`<button name=entscheiden value=ja data-laeuft="Gibt frei …">Freigeben</button> ` +
			`<button name=entscheiden value=nein data-laeuft="Lehnt ab …">Ablehnen</button></form>`)
	default: // the person who asked, until an admin answered
		b.WriteString(`<div hidden data-auffrischen=10></div><p class=k role=status>Wartet auf Freigabe durch einen Admin.</p>`)
	}
	writePage(w, http.StatusOK, b)
}
