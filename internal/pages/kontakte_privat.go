package pages

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/texte"
)

// MeineKontaktePath is Meine Kontakte (owner 2026-11-06): the viewer's own
// Handy-Kontakte, read only, never mixed with the household's book, each
// with its page (…/<uid>); for the Immich key's owner (gesichter.Merke)
// with „Fotos“, its person in Immich (gesichter.Link), and its questions.
const MeineKontaktePath = KontaktePath + "/privat"

// privatEigen: the viewer is the Immich key's owner.
func privatEigen(r *http.Request, cfg StartConfig) bool {
	u := r.Header.Get("Remote-User")
	cfg.Gesichter.Merke(u, r.Header.Get("Remote-Email"))
	return cfg.fotosAn() && u != "" && cfg.Gesichter.Wessen() == u
}

// meineDa: the viewer has Meine Kontakte, for their photos or to have
// them checked.
func meineDa(r *http.Request, cfg StartConfig) bool {
	return privatEigen(r, cfg) || cfg.pruefenAn() && r.Header.Get("Remote-User") != ""
}

// meineKontakte is the viewer's own Handy-Kontakte.
func meineKontakte(ctx context.Context, cfg StartConfig, person string) ([]caldav.Contact, error) {
	cards, _, err := cfg.Contacts.BookCards(ctx, "/"+url.PathEscape(person)+"/handy-kontakte/")
	var out []caldav.Contact
	for _, cd := range cards {
		if root, err := caldav.Parse(cd.Data); err == nil {
			out = append(out, caldav.Contacts(root)...)
		}
	}
	return out, err
}

// MeineKontakte is the list, each one's page and its „Fotos“.
func MeineKontakte(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := strings.TrimPrefix(r.URL.Path, MeineKontaktePath+"/")
		e, ok := kontaktPost(w, r, cfg, r.URL.Path == MeineKontaktePath)
		if !ok {
			return
		}
		if !meineDa(r, cfg) || cfg.Contacts == nil {
			http.Redirect(w, r, KontaktePath, http.StatusSeeOther)
			return
		}
		var all []caldav.Contact
		var err error
		fetch(r, func(ctx context.Context) { all, err = meineKontakte(ctx, cfg, r.Header.Get("Remote-User")) })
		for _, ct := range all {
			if r.URL.Path != MeineKontaktePath && ct.UID == uid {
				fotos := ""
				if privatEigen(r, cfg) {
					if l := cfg.Gesichter.Link("privat/"+uid, oneLine(ct.Name)); l != "" && !ct.IsFirm() {
						fotos = "<p class=weiter-in>" + immichLink(l, texte.T(texte.KontaktFotos), wort(texte.KontaktFotosKurz)) + "</p>"
					}
				}
				kontaktSeite(w, r, cfg, ct, kontaktantwort.Privat, MeineKontaktePath, fotos, "", e)
				return
			}
		}
		if r.URL.Path != MeineKontaktePath {
			kontaktWeg(w, r, cfg, e, kontaktantwort.Privat, uid, texte.T(texte.MeineKontakteWeg))
			return
		}
		b := kontakteHead(r, cfg, texte.T(texte.MeineKontakte), MeineKontaktePath, texte.T(texte.KontakteMeine))
		b.WriteString(kontakteStyle + "<h1>" + wort(texte.MeineKontakte) + "</h1>")
		if privatEigen(r, cfg) && cfg.Gesichter.Fehlt() != "" {
			b.WriteString(gesagt("", fmt.Sprintf(texte.T(texte.MeineKontakteRecht), cfg.Gesichter.Fehlt())))
		}
		if err != nil {
			log.Printf("homes: meine kontakte: %v", err)
			b.WriteString("<p class=k>" + saysNoContacts + "</p>")
		} else {
			kontakteListe(b, kontaktZeilen(r, cfg, all, kontaktantwort.Privat), texte.MeineKontakteLeer)
		}
		b.WriteString(wieDas(texte.MeineKontakteHinweis))
		writePage(w, http.StatusOK, b)
	}
}
